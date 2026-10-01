package store

import (
	"context"
	"path/filepath"
	"testing"
)

// seedCatalog inserts a tiny, hand-made dataset so the query methods can be
// asserted against known-correct results:
//
//	vos     (it: Volpe)    traits: sluw, snel, solitair
//	wolf    (it: Lupo)     traits: sluw, snel
//	kikker  (it: Rana)     traits: nat
func seedCatalog(t *testing.T, ctx context.Context, s *Store) {
	t.Helper()
	type a struct {
		slug, nameIT string
		traits       []string
	}
	data := []a{
		{"vos", "Volpe", []string{"sluw", "snel", "solitair"}},
		{"wolf", "Lupo", []string{"sluw", "snel"}},
		{"kikker", "Rana", []string{"nat"}},
	}
	// Italian labels for the Dutch trait keys.
	labelIT := map[string]string{"sluw": "astuto", "snel": "veloce", "solitair": "solitario", "nat": "bagnato"}
	traitID := map[string]int64{}

	for _, x := range data {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO animal(slug, name_nl) VALUES (?, ?)`, x.slug, x.slug)
		if err != nil {
			t.Fatalf("insert animal %s: %v", x.slug, err)
		}
		aid, _ := res.LastInsertId()
		if _, err := s.DB.ExecContext(ctx,
			`INSERT INTO translation(animal_id, lang, name, description) VALUES (?, 'it', ?, ?)`,
			aid, x.nameIT, "desc "+x.nameIT); err != nil {
			t.Fatalf("insert translation: %v", err)
		}
		for _, key := range x.traits {
			if _, ok := traitID[key]; !ok {
				r, err := s.DB.ExecContext(ctx, `INSERT INTO trait(key_nl) VALUES (?)`, key)
				if err != nil {
					t.Fatalf("insert trait %s: %v", key, err)
				}
				tid, _ := r.LastInsertId()
				traitID[key] = tid
				if _, err := s.DB.ExecContext(ctx,
					`INSERT INTO trait_translation(trait_id, lang, value) VALUES (?, 'it', ?)`,
					tid, labelIT[key]); err != nil {
					t.Fatalf("insert trait_translation: %v", err)
				}
			}
			if _, err := s.DB.ExecContext(ctx,
				`INSERT INTO animal_trait(animal_id, trait_id) VALUES (?, ?)`, aid, traitID[key]); err != nil {
				t.Fatalf("link animal_trait: %v", err)
			}
		}
	}
}

func newSeededStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "totem.db"), testKey)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	seedCatalog(t, ctx, s)
	return s, ctx
}

func slugs(as []Animal) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Slug
	}
	return out
}

func TestGetAnimal(t *testing.T) {
	s, ctx := newSeededStore(t)

	d, err := s.GetAnimal(ctx, "vos", "it")
	if err != nil {
		t.Fatalf("GetAnimal: %v", err)
	}
	if d.Name != "Volpe" || d.NameNL != "vos" {
		t.Fatalf("names: %+v", d)
	}
	if len(d.Traits) != 3 || d.Traits[0] != "astuto" { // sorted: astuto, solitario, veloce
		t.Fatalf("traits: %v", d.Traits)
	}

	if _, err := s.GetAnimal(ctx, "nope", "it"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestListAnimals_Filter(t *testing.T) {
	s, ctx := newSeededStore(t)

	// include sluw → vos + wolf
	got, err := s.ListAnimals(ctx, Filter{Lang: "it", Include: []string{"sluw"}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if want := []string{"Lupo", "Volpe"}; !equalSet(slugsNames(got), want) { // ordered by IT name
		t.Fatalf("include sluw: got %v", slugsNames(got))
	}

	// include sluw, exclude solitair → wolf only
	got, _ = s.ListAnimals(ctx, Filter{Lang: "it", Include: []string{"sluw"}, Exclude: []string{"solitair"}})
	if len(got) != 1 || got[0].Slug != "wolf" {
		t.Fatalf("include/exclude: %v", slugs(got))
	}

	// free-text q on the Italian name
	got, _ = s.ListAnimals(ctx, Filter{Lang: "it", Query: "rana"})
	if len(got) != 1 || got[0].Slug != "kikker" {
		t.Fatalf("query: %v", slugs(got))
	}
}

func TestSimilar(t *testing.T) {
	s, ctx := newSeededStore(t)

	// vos shares {sluw,snel} with wolf (jaccard 2/3); shares nothing with kikker,
	// so kikker must not appear at all.
	got, err := s.Similar(ctx, "vos", "it", 10)
	if err != nil {
		t.Fatalf("similar: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "wolf" {
		t.Fatalf("similar to vos: %v", slugs(got))
	}
}

func TestListTraits(t *testing.T) {
	s, ctx := newSeededStore(t)

	traits, err := s.ListTraits(ctx, "it")
	if err != nil {
		t.Fatalf("list traits: %v", err)
	}
	if len(traits) != 4 {
		t.Fatalf("want 4 traits, got %d", len(traits))
	}
	// most-used first: sluw & snel are used twice.
	if traits[0].Count != 2 {
		t.Fatalf("top trait count: %+v", traits[0])
	}
}

func slugsNames(as []Animal) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name
	}
	return out
}

func equalSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, g := range got {
		seen[g]++
	}
	for _, w := range want {
		if seen[w] == 0 {
			return false
		}
		seen[w]--
	}
	return true
}
