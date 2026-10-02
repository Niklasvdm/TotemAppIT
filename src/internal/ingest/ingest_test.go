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

func TestMergePreservesIDsAndReports(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "totem.db"), "test-key")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	base := []SourceAnimal{
		{Slug: "vos", NL: "Vos", IT: "Volpe", EN: "Fox", TraitsNL: []string{"sluw"}, TraitsIT: []string{"astuto"}, TraitsEN: []string{"sly"}},
	}
	if _, err := Merge(ctx, s.DB, base); err != nil {
		t.Fatalf("merge base: %v", err)
	}
	var vosID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM animal WHERE slug='vos'`).Scan(&vosID); err != nil {
		t.Fatal(err)
	}
	// A community report on vos.
	if err := s.AddReport(ctx, "vos", "unknown", "test"); err != nil {
		t.Fatalf("add report: %v", err)
	}

	// Merge again: edit vos + ADD a new animal. Must not wipe the report nor move vos's id.
	next := []SourceAnimal{
		{Slug: "vos", NL: "Vos", IT: "Volpe rossa", EN: "Red fox", TraitsNL: []string{"sluw", "snel"}, TraitsIT: []string{"astuto", "veloce"}, TraitsEN: []string{"sly", "fast"}},
		{Slug: "das", NL: "Das", IT: "Tasso", EN: "Badger", TraitsNL: []string{"sterk"}, TraitsIT: []string{"forte"}, TraitsEN: []string{"strong"}},
	}
	if _, err := Merge(ctx, s.DB, next); err != nil {
		t.Fatalf("merge next: %v", err)
	}

	var newVosID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM animal WHERE slug='vos'`).Scan(&newVosID); err != nil {
		t.Fatal(err)
	}
	if newVosID != vosID {
		t.Fatalf("vos id changed on merge: %d -> %d", vosID, newVosID)
	}
	reports, err := s.ListReports(ctx, "pending")
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].AnimalSlug != "vos" {
		t.Fatalf("report not preserved across merge: %+v", reports)
	}
	// New animal present, edit applied.
	d, err := s.GetAnimal(ctx, "vos", "it")
	if err != nil || d.Name != "Volpe rossa" || len(d.Traits) != 2 {
		t.Fatalf("vos edit not applied: %+v (err %v)", d, err)
	}
	if _, err := s.GetAnimal(ctx, "das", "it"); err != nil {
		t.Fatalf("new animal das missing: %v", err)
	}
}
