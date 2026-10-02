// Command totem-seed loads data/animals.json into the encrypted store.
//
//	TOTEM_DB_KEY=... go run ./cmd/totem-seed --db totem.db --data data/animals.json --force
package main

import (
	"context"
	"flag"
	"log"

	"github.com/Niklasvdm/TotemAppIT/internal/config"
	"github.com/Niklasvdm/TotemAppIT/internal/ingest"
	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

func main() {
	dbPath := flag.String("db", "totem.db", "path to the (encrypted) database file")
	// Default assumes you run from src/ (the module root); data/ lives at the repo root.
	dataPath := flag.String("data", "../data/animals.json", "path to animals.json")
	attrPath := flag.String("attributions", "../data/images/attributions.json", "path to image attributions (optional)")
	force := flag.Bool("force", false, "replace an already-seeded catalogue (DESTRUCTIVE: wipes animals + cascade-wipes reports)")
	merge := flag.Bool("merge", false, "idempotently add/update animals by slug without clearing (safe on a live DB; preserves reports)")
	flag.Parse()

	key, err := config.DBKey()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	s, err := store.Open(*dbPath, key)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer s.Close()

	if err := s.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	var stats ingest.Stats
	if *merge {
		stats, err = ingest.MergeFile(ctx, s, *dataPath)
	} else {
		stats, err = ingest.LoadFile(ctx, s, *dataPath, *force)
	}
	if err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Printf("seeded %s (merge=%v): %d animals, %d traits, %d links",
		*dbPath, *merge, stats.Animals, stats.Traits, stats.Links)

	imgs, err := ingest.LoadImages(ctx, s.DB, *attrPath)
	if err != nil {
		log.Fatalf("load images: %v", err)
	}
	log.Printf("image attributions loaded for %d animals (from %s)", imgs, *attrPath)
}
