// Command totem-seed loads data/animals.json into the encrypted store.
//
//	TOTEM_DB_KEY=... go run ./cmd/totem-seed --db totem.db --data data/animals.json --force
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/Niklasvdm/TotemAppIT/internal/ingest"
	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

func main() {
	dbPath := flag.String("db", "totem.db", "path to the (encrypted) database file")
	// Default assumes you run from src/ (the module root); data/ lives at the repo root.
	dataPath := flag.String("data", "../data/animals.json", "path to animals.json")
	force := flag.Bool("force", false, "replace an already-seeded catalogue")
	flag.Parse()

	key := os.Getenv("TOTEM_DB_KEY")
	if key == "" {
		log.Fatal("TOTEM_DB_KEY is required (the Adiantum encryption key)")
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

	stats, err := ingest.LoadFile(ctx, s, *dataPath, *force)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Printf("seeded %s: %d animals, %d traits, %d links",
		*dbPath, stats.Animals, stats.Traits, stats.Links)
}
