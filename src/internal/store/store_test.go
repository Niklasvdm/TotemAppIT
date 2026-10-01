package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testKey = "correct-horse-battery-staple"

// TestEncryptedRoundTrip proves the whole Phase-2 foundation end-to-end:
// the encrypted DB opens, migrations apply, CRUD works, the on-disk file is
// actually ciphertext, a wrong key fails, and the right key reads it back.
func TestEncryptedRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "totem.db")

	st, err := Open(path, testKey)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert one animal + its Italian translation (exercises the FK + join).
	res, err := st.DB.ExecContext(ctx,
		`INSERT INTO animal(slug, name_nl, alt_names) VALUES ('adder', 'Adder', '')`)
	if err != nil {
		t.Fatalf("insert animal: %v", err)
	}
	id, _ := res.LastInsertId()
	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO translation(animal_id, lang, name, description) VALUES (?, 'it', 'Vipera', 'La vipera...')`,
		id); err != nil {
		t.Fatalf("insert translation: %v", err)
	}

	var name string
	if err := st.DB.QueryRowContext(ctx,
		`SELECT t.name FROM animal a
		 JOIN translation t ON t.animal_id = a.id AND t.lang = 'it'
		 WHERE a.slug = 'adder'`).Scan(&name); err != nil {
		t.Fatalf("select join: %v", err)
	}
	if name != "Vipera" {
		t.Fatalf("got name %q, want Vipera", name)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// (1) At-rest encryption: the plaintext must NOT appear in the raw file.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if strings.Contains(string(raw), "Vipera") {
		t.Fatal("plaintext 'Vipera' found in db file — the database is NOT encrypted")
	}

	// (2) Wrong key must not be able to read the data.
	if bad, err := Open(path, "the-wrong-key"); err == nil {
		var n int
		if qerr := bad.DB.QueryRowContext(ctx, `SELECT count(*) FROM animal`).Scan(&n); qerr == nil {
			t.Fatal("opened and read the encrypted db with the WRONG key")
		}
		bad.Close()
	}

	// (3) Correct key reopens and reads the row back.
	st2, err := Open(path, testKey)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	var n int
	if err := st2.DB.QueryRowContext(ctx, `SELECT count(*) FROM animal`).Scan(&n); err != nil {
		t.Fatalf("count after reopen: %v", err)
	}
	if n != 1 {
		t.Fatalf("got %d animals, want 1", n)
	}
}

// TestMigrateIsIdempotent verifies re-running Migrate is a no-op.
func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "totem.db")

	st, err := Open(path, testKey)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate #1: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate #2 (should be no-op): %v", err)
	}

	var applied int
	if err := st.DB.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if applied < 1 {
		t.Fatalf("expected >=1 recorded migration, got %d", applied)
	}
}
