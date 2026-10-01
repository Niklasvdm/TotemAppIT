// Package ingest loads the canonical data/animals.json into the (encrypted)
// store. It is the Go equivalent of scripts/seed_db.py; the Python one produces
// an unencrypted DB for Phase-1 testing, this one writes through the store so the
// file is Adiantum-encrypted and totemd can open it.
package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

// langs is the fixed set of languages carried per animal/trait.
var langs = []string{"nl", "it", "en"}

// SourceAnimal mirrors one record in data/animals.json.
type SourceAnimal struct {
	Slug     string   `json:"slug"`
	NL       string   `json:"nl"`
	IT       string   `json:"it"`
	EN       string   `json:"en"`
	Alt      string   `json:"alt"`
	DescNL   string   `json:"desc_nl"`
	DescIT   string   `json:"desc_it"`
	DescEN   string   `json:"desc_en"`
	TraitsNL []string `json:"traits_nl"`
	TraitsIT []string `json:"traits_it"`
	TraitsEN []string `json:"traits_en"`
}

// Stats summarises what was loaded.
type Stats struct {
	Animals, Traits, Links int
}

// LoadFile reads animals.json at path and loads it into s. When force is false
// it refuses to run against a non-empty catalogue.
func LoadFile(ctx context.Context, s *store.Store, path string, force bool) (Stats, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Stats{}, fmt.Errorf("read %s: %w", path, err)
	}
	var animals []SourceAnimal
	if err := json.Unmarshal(raw, &animals); err != nil {
		return Stats{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return Load(ctx, s.DB, animals, force)
}

// Load inserts animals into db in a single transaction. With force it first
// clears the catalogue tables; without it, it errors if any animal exists.
func Load(ctx context.Context, db *sql.DB, animals []SourceAnimal, force bool) (Stats, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM animal`).Scan(&count); err != nil {
		return Stats{}, fmt.Errorf("count animals: %w", err)
	}
	if count > 0 && !force {
		return Stats{}, fmt.Errorf("catalogue already has %d animals (pass force to replace)", count)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Stats{}, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	if force {
		// ON DELETE CASCADE clears the dependent tables from animal/trait.
		for _, t := range []string{"animal", "trait"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+t); err != nil {
				return Stats{}, fmt.Errorf("clear %s: %w", t, err)
			}
		}
	}

	traitID := make(map[string]int64) // key_nl -> id, deduplicated
	st := Stats{}

	for _, a := range animals {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO animal(slug, name_nl, alt_names, source_url) VALUES (?, ?, ?, ?)`,
			a.Slug, a.NL, a.Alt, "")
		if err != nil {
			return Stats{}, fmt.Errorf("insert animal %s: %w", a.Slug, err)
		}
		aid, _ := res.LastInsertId()
		st.Animals++

		names := map[string]string{"nl": a.NL, "it": a.IT, "en": a.EN}
		descs := map[string]string{"nl": a.DescNL, "it": a.DescIT, "en": a.DescEN}
		for _, lg := range langs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO translation(animal_id, lang, name, description) VALUES (?, ?, ?, ?)`,
				aid, lg, names[lg], descs[lg]); err != nil {
				return Stats{}, fmt.Errorf("insert translation %s/%s: %w", a.Slug, lg, err)
			}
		}

		for i, key := range a.TraitsNL {
			tid, ok := traitID[key]
			if !ok {
				r, err := tx.ExecContext(ctx, `INSERT INTO trait(key_nl) VALUES (?)`, key)
				if err != nil {
					return Stats{}, fmt.Errorf("insert trait %q: %w", key, err)
				}
				tid, _ = r.LastInsertId()
				traitID[key] = tid
				st.Traits++

				vals := map[string]string{"nl": key, "it": at(a.TraitsIT, i), "en": at(a.TraitsEN, i)}
				for _, lg := range langs {
					if _, err := tx.ExecContext(ctx,
						`INSERT INTO trait_translation(trait_id, lang, value) VALUES (?, ?, ?)`,
						tid, lg, vals[lg]); err != nil {
						return Stats{}, fmt.Errorf("insert trait_translation %q/%s: %w", key, lg, err)
					}
				}
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO animal_trait(animal_id, trait_id) VALUES (?, ?)`, aid, tid); err != nil {
				return Stats{}, fmt.Errorf("link %s/%q: %w", a.Slug, key, err)
			}
			st.Links++
		}
	}

	if err := tx.Commit(); err != nil {
		return Stats{}, fmt.Errorf("commit: %w", err)
	}
	return st, nil
}

// LoadImages reads data/images/attributions.json ({slug: {author, license,
// source_url}}) and records image metadata on the matching animals. It sets
// image_path to "<slug>.webp" (the file the image endpoint serves). Returns the
// number of animals updated. A missing file is not an error (returns 0).
func LoadImages(ctx context.Context, db *sql.DB, attributionsPath string) (int, error) {
	raw, err := os.ReadFile(attributionsPath)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", attributionsPath, err)
	}
	var attrs map[string]struct {
		Author    string `json:"author"`
		License   string `json:"license"`
		SourceURL string `json:"source_url"`
	}
	if err := json.Unmarshal(raw, &attrs); err != nil {
		return 0, fmt.Errorf("parse %s: %w", attributionsPath, err)
	}

	updated := 0
	for slug, a := range attrs {
		res, err := db.ExecContext(ctx,
			`UPDATE animal SET image_path = ?, image_author = ?, image_license = ?, image_source = ? WHERE slug = ?`,
			slug+".webp", a.Author, a.License, a.SourceURL, slug)
		if err != nil {
			return updated, fmt.Errorf("set image for %s: %w", slug, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			updated++
		}
	}
	return updated, nil
}

// at returns s[i] or "" when out of range (defends against misaligned arrays).
func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}
