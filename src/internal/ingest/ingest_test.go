package ingest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

func TestLoad(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "totem.db"), "test-key")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	animals := []SourceAnimal{
		{Slug: "vos", NL: "Vos", IT: "Volpe", EN: "Fox", DescIT: "d",
			TraitsNL: []string{"sluw", "snel"}, TraitsIT: []string{"astuto", "veloce"}, TraitsEN: []string{"sly", "fast"}},
		{Slug: "wolf", NL: "Wolf", IT: "Lupo", EN: "Wolf", DescIT: "d",
			TraitsNL: []string{"sluw"}, TraitsIT: []string{"astuto"}, TraitsEN: []string{"sly"}},
	}

	st, err := Load(ctx, s.DB, animals, false)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if st.Animals != 2 || st.Traits != 2 { // "sluw" deduplicated across both
		t.Fatalf("stats: %+v", st)
	}

	d, err := s.GetAnimal(ctx, "vos", "it")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if d.Name != "Volpe" || len(d.Traits) != 2 {
		t.Fatalf("animal: %+v", d)
	}

	// Re-seeding without force must refuse.
	if _, err := Load(ctx, s.DB, animals, false); err == nil {
		t.Fatal("expected error re-seeding without force")
	}

	// With force it replaces the catalogue.
	if _, err := Load(ctx, s.DB, animals[:1], true); err != nil {
		t.Fatalf("force reload: %v", err)
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM animal`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("after force reload want 1 animal, got %d", n)
	}
}
