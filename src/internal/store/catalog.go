package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrNotFound is returned when a slug does not exist.
var ErrNotFound = errors.New("animal not found")

// traitSep is the in-band delimiter used to pack a row's trait labels into one
// group_concat column. Unit Separator (0x1f) never occurs in the data.
const traitSep = "\x1f"

// Animal is the list/summary view, projected to one language.
type Animal struct {
	Slug   string
	Name   string
	Traits []string
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

// traitsSubquery is the correlated subquery that packs an animal's trait labels
// (for ?=lang) into one delimited column, avoiding an N+1 per-animal fetch.
const traitsSubquery = `COALESCE((SELECT group_concat(tt.value, char(31))
	FROM animal_trait at
	JOIN trait_translation tt ON tt.trait_id = at.trait_id AND tt.lang = ?
	WHERE at.animal_id = a.id), '')`

// GetAnimal returns one animal projected to lang, or ErrNotFound.
func (s *Store) GetAnimal(ctx context.Context, slug, lang string) (*AnimalDetail, error) {
	q := `
SELECT a.slug, a.name_nl, a.alt_names, a.source_url, t.name, t.description, ` + traitsSubquery + `
FROM animal a
JOIN translation t ON t.animal_id = a.id AND t.lang = ?
WHERE a.slug = ?`

	var d AnimalDetail
	var traits string
	// arg order follows the SQL text: traits-subquery lang, join lang, slug.
	err := s.DB.QueryRowContext(ctx, q, lang, lang, slug).Scan(
		&d.Slug, &d.NameNL, &d.AltNames, &d.SourceURL, &d.Name, &d.Description, &traits)
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
SELECT a.slug, t.name, ` + traitsSubquery + `
FROM animal a
JOIN translation t ON t.animal_id = a.id AND t.lang = ?
WHERE 1 = 1`)

	// arg order must track the SQL text exactly.
	args := []any{f.Lang, f.Lang}

	if q := strings.TrimSpace(f.Query); q != "" {
		b.WriteString(" AND lower(t.name) LIKE '%' || lower(?) || '%'")
		args = append(args, q)
	}
	if len(f.Include) > 0 {
		b.WriteString(` AND (SELECT COUNT(*) FROM animal_trait ai
			JOIN trait ti ON ti.id = ai.trait_id
			WHERE ai.animal_id = a.id AND ti.key_nl IN (` + placeholders(len(f.Include)) + `)) = ?`)
		for _, k := range f.Include {
			args = append(args, k)
		}
		args = append(args, len(f.Include))
	}
	if len(f.Exclude) > 0 {
		b.WriteString(` AND NOT EXISTS (SELECT 1 FROM animal_trait ae
			JOIN trait te ON te.id = ae.trait_id
			WHERE ae.animal_id = a.id AND te.key_nl IN (` + placeholders(len(f.Exclude)) + `))`)
		for _, k := range f.Exclude {
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
SELECT a.slug, t.name, ` + traitsSubquery + `
FROM animal_trait oat
JOIN animal a ON a.id = oat.animal_id
JOIN translation t ON t.animal_id = a.id AND t.lang = ?
JOIN (SELECT animal_id, COUNT(*) n FROM animal_trait GROUP BY animal_id) ocnt
	ON ocnt.animal_id = a.id
WHERE oat.trait_id IN (SELECT trait_id FROM target) AND a.slug <> ?
GROUP BY a.id
ORDER BY (1.0 * COUNT(*) / ((SELECT n FROM tsize) + ocnt.n - COUNT(*))) DESC,
	COUNT(*) DESC, t.name
LIMIT ?`
	// arg order: target slug, traits-subquery lang, join lang, exclude-self slug, limit.
	return s.queryAnimals(ctx, q, slug, lang, lang, slug, limit)
}

// ListTraits returns the trait cloud projected to lang, most-used first.
func (s *Store) ListTraits(ctx context.Context, lang string) ([]Trait, error) {
	q := `
SELECT t.key_nl, tt.value, COUNT(at.animal_id) AS cnt
FROM trait t
JOIN trait_translation tt ON tt.trait_id = t.id AND tt.lang = ?
LEFT JOIN animal_trait at ON at.trait_id = t.id
GROUP BY t.id
ORDER BY cnt DESC, tt.value`

	rows, err := s.DB.QueryContext(ctx, q, lang)
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
		if err := rows.Scan(&a.Slug, &a.Name, &traits); err != nil {
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

// splitTraits unpacks a group_concat column into a sorted slice (nil when empty).
func splitTraits(packed string) []string {
	if packed == "" {
		return nil
	}
	parts := strings.Split(packed, traitSep)
	sort.Strings(parts)
	return parts
}
