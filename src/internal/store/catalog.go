package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ErrNotFound is returned when a slug does not exist.
var ErrNotFound = errors.New("animal not found")

// traitSep is the in-band delimiter used to pack a row's trait labels into one
// group_concat column. Unit Separator (0x1f) never occurs in the data.
const traitSep = "\x1f"

// Animal is the list/summary view, projected to one language.
type Animal struct {
	Slug        string
	Name        string
	Description string  // for the card preview (clamped client-side)
	Traits      []string
	Score       float64 // Jaccard similarity 0..1 (only set by similarity queries)
}

// AnimalDetail is the full single-animal view.
type AnimalDetail struct {
	Slug        string
	Name        string
	NameNL      string
	AltNames    string
	Description string
	Traits      []string
	SourceURL   string

	// Image metadata (the file itself is served separately from TOTEM_IMAGE_DIR).
	ImagePath    string // e.g. "adder.webp"; empty when there is no image
	ImageAuthor  string
	ImageLicense string
	ImageSource  string
}

// Trait is one entry in the trait cloud.
type Trait struct {
	Key   string // canonical Dutch key (stable id used for filtering)
	Label string // projected to the requested language
	Count int    // how many animals have it
}

// Filter describes a catalogue query. Include/Exclude hold trait Keys (key_nl).
type Filter struct {
	Lang    string
	Query   string
	Include []string
	Exclude []string
}

// descProjection returns the description in the requested language, falling back
// to Italian then Dutch when that translation is empty (31 animals lack en/nl).
const descProjection = `COALESCE(NULLIF(t.description, ''),
	(SELECT description FROM translation WHERE animal_id = a.id AND lang = 'it' AND description <> ''),
	(SELECT description FROM translation WHERE animal_id = a.id AND lang = 'nl' AND description <> ''), '')`

// traitsSubquery is the correlated subquery that packs an animal's trait labels
// (for ?=lang) into one delimited column, avoiding an N+1 per-animal fetch.
const traitsSubquery = `COALESCE((SELECT group_concat(tt.value, char(31))
	FROM animal_trait at
	JOIN trait_translation tt ON tt.trait_id = at.trait_id AND tt.lang = ?
	WHERE at.animal_id = a.id), '')`

// GetAnimal returns one animal projected to lang, or ErrNotFound.
func (s *Store) GetAnimal(ctx context.Context, slug, lang string) (*AnimalDetail, error) {
	q := `
SELECT a.slug, a.name_nl, a.alt_names, a.source_url,
       a.image_path, a.image_author, a.image_license, a.image_source,
       t.name, ` + descProjection + ` AS description, ` + traitsSubquery + `
FROM animal a
JOIN translation t ON t.animal_id = a.id AND t.lang = ?
WHERE a.slug = ?`

	var d AnimalDetail
	var traits string
	// arg order follows the SQL text: traits-subquery lang, join lang, slug.
	err := s.DB.QueryRowContext(ctx, q, lang, lang, slug).Scan(
		&d.Slug, &d.NameNL, &d.AltNames, &d.SourceURL,
		&d.ImagePath, &d.ImageAuthor, &d.ImageLicense, &d.ImageSource,
		&d.Name, &d.Description, &traits)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get animal %s: %w", slug, err)
	}
	d.Traits = splitTraits(traits)
	return &d, nil
}

// ListAnimals returns animals matching f, projected to f.Lang, ordered by name.
func (s *Store) ListAnimals(ctx context.Context, f Filter) ([]Animal, error) {
	var b strings.Builder
	b.WriteString(`
SELECT a.slug, t.name, ` + descProjection + ` AS description, ` + traitsSubquery + `, 0.0 AS score
FROM animal a
JOIN translation t ON t.animal_id = a.id AND t.lang = ?
WHERE 1 = 1`)

	// arg order must track the SQL text exactly.
	args := []any{f.Lang, f.Lang}

	if q := strings.TrimSpace(f.Query); q != "" {
		b.WriteString(" AND lower(t.name) LIKE '%' || lower(?) || '%'")
		args = append(args, q)
	}
	// include: each entry is a synonym group ("rustig|stil"); the animal must have
	// at least one key from EVERY included entry (OR within an entry, AND across them).
	for _, entry := range f.Include {
		keys := strings.Split(entry, "|")
		b.WriteString(` AND EXISTS (SELECT 1 FROM animal_trait ai JOIN trait ti ON ti.id = ai.trait_id
			WHERE ai.animal_id = a.id AND ti.key_nl IN (` + placeholders(len(keys)) + `))`)
		for _, k := range keys {
			args = append(args, k)
		}
	}
	// exclude: the animal must have NONE of the keys in any excluded entry.
	if ex := splitGroups(f.Exclude); len(ex) > 0 {
		b.WriteString(` AND NOT EXISTS (SELECT 1 FROM animal_trait ae JOIN trait te ON te.id = ae.trait_id
			WHERE ae.animal_id = a.id AND te.key_nl IN (` + placeholders(len(ex)) + `))`)
		for _, k := range ex {
			args = append(args, k)
		}
	}
	b.WriteString(" ORDER BY t.name")

	return s.queryAnimals(ctx, b.String(), args...)
}

// Similar ranks animals by Jaccard overlap of trait sets with slug's set:
// shared / (|target| + |other| - shared). One query, no full-table load.
func (s *Store) Similar(ctx context.Context, slug, lang string, limit int) ([]Animal, error) {
	if limit <= 0 {
		limit = 10
	}
	q := `
WITH target AS (
	SELECT at.trait_id FROM animal_trait at
	JOIN animal a ON a.id = at.animal_id WHERE a.slug = ?
),
tsize AS (SELECT COUNT(*) AS n FROM target)
SELECT a.slug, t.name, ` + descProjection + ` AS description, ` + traitsSubquery + `,
       (1.0 * COUNT(*) / ((SELECT n FROM tsize) + ocnt.n - COUNT(*))) AS score
FROM animal_trait oat
JOIN animal a ON a.id = oat.animal_id
JOIN translation t ON t.animal_id = a.id AND t.lang = ?
JOIN (SELECT animal_id, COUNT(*) n FROM animal_trait GROUP BY animal_id) ocnt
	ON ocnt.animal_id = a.id
WHERE oat.trait_id IN (SELECT trait_id FROM target) AND a.slug <> ?
GROUP BY a.id
ORDER BY score DESC, COUNT(*) DESC, t.name
LIMIT ?`
	// arg order: target slug, traits-subquery lang, join lang, exclude-self slug, limit.
	return s.queryAnimals(ctx, q, slug, lang, lang, slug, limit)
}

// SimilarByTraits ranks animals by Jaccard overlap between their trait set and
// the selected trait profile (the flattened include groups), optionally excluding
// animals that have any excluded key. This powers "Similarity" mode in the finder.
func (s *Store) SimilarByTraits(ctx context.Context, include, exclude []string, lang string, limit int) ([]Animal, error) {
	target := splitGroups(include)
	if len(target) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 60
	}

	// len(target) is an int from len() — safe to inline as a literal (no injection).
	n := strconv.Itoa(len(target))

	var b strings.Builder
	b.WriteString(`
SELECT a.slug, t.name, ` + descProjection + ` AS description, ` + traitsSubquery + `,
       (1.0 * sh.shared / (` + n + ` + ocnt.n - sh.shared)) AS score
FROM animal a
JOIN translation t ON t.animal_id = a.id AND t.lang = ?
JOIN (SELECT animal_id, COUNT(*) n FROM animal_trait GROUP BY animal_id) ocnt ON ocnt.animal_id = a.id
JOIN (SELECT at.animal_id, COUNT(*) shared FROM animal_trait at
      WHERE at.trait_id IN (SELECT id FROM trait WHERE key_nl IN (` + placeholders(len(target)) + `))
      GROUP BY at.animal_id) sh ON sh.animal_id = a.id`)

	args := []any{lang, lang} // traits-subquery lang, join lang
	for _, k := range target {
		args = append(args, k)
	}
	if ex := splitGroups(exclude); len(ex) > 0 {
		b.WriteString(` WHERE NOT EXISTS (SELECT 1 FROM animal_trait ae JOIN trait te ON te.id = ae.trait_id
			WHERE ae.animal_id = a.id AND te.key_nl IN (` + placeholders(len(ex)) + `))`)
		for _, k := range ex {
			args = append(args, k)
		}
	}
	b.WriteString(` ORDER BY score DESC, sh.shared DESC, t.name LIMIT ?`)
	args = append(args, limit)

	return s.queryAnimals(ctx, b.String(), args...)
}

// ListTraits returns the trait cloud projected to lang, most-used first.
// Distinct Dutch traits can translate to the SAME label (e.g. rustig+stil ->
// "quiet"), so entries are grouped BY label: Key is the synonym group joined with
// "|", Count is the number of animals having any of those keys.
func (s *Store) ListTraits(ctx context.Context, lang string) ([]Trait, error) {
	q := `
SELECT (SELECT group_concat(t2.key_nl, '|') FROM trait t2
        JOIN trait_translation x ON x.trait_id = t2.id AND x.lang = ? WHERE x.value = tt.value) AS keys,
       tt.value AS label,
       COUNT(DISTINCT at.animal_id) AS cnt
FROM trait t
JOIN trait_translation tt ON tt.trait_id = t.id AND tt.lang = ?
LEFT JOIN animal_trait at ON at.trait_id = t.id
GROUP BY tt.value
ORDER BY cnt DESC, tt.value`

	rows, err := s.DB.QueryContext(ctx, q, lang, lang)
	if err != nil {
		return nil, fmt.Errorf("list traits: %w", err)
	}
	defer rows.Close()

	var out []Trait
	for rows.Next() {
		var t Trait
		if err := rows.Scan(&t.Key, &t.Label, &t.Count); err != nil {
			return nil, fmt.Errorf("scan trait: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// queryAnimals runs a query whose columns are (slug, name, packed-traits).
func (s *Store) queryAnimals(ctx context.Context, query string, args ...any) ([]Animal, error) {
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query animals: %w", err)
	}
	defer rows.Close()

	var out []Animal
	for rows.Next() {
		var a Animal
		var traits string
		if err := rows.Scan(&a.Slug, &a.Name, &a.Description, &traits, &a.Score); err != nil {
			return nil, fmt.Errorf("scan animal: %w", err)
		}
		a.Traits = splitTraits(traits)
		out = append(out, a)
	}
	return out, rows.Err()
}

// placeholders returns "?,?,?" for n > 0.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// splitGroups flattens synonym-group entries ("a|b", "c") into a flat key list.
func splitGroups(entries []string) []string {
	var out []string
	for _, e := range entries {
		out = append(out, strings.Split(e, "|")...)
	}
	return out
}

// splitTraits unpacks a group_concat column into a sorted slice (nil when empty).
func splitTraits(packed string) []string {
	if packed == "" {
		return nil
	}
	parts := strings.Split(packed, traitSep)
	sort.Strings(parts)
	return parts
}
