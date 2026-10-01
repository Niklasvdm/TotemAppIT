// Package store is the data layer: it opens the Adiantum-encrypted SQLite
// database (pure Go, no CGO) and applies the embedded schema migrations.
//
// Phase 2 foundation. The query methods and the Store interface the domain
// depends on come next; this file proves the encrypted store + migrations work.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strings"

	// Pure-Go SQLite driver (registers the "sqlite3" database/sql driver;
	// the embedded WASM binary comes with it — importing .../embed is unnecessary).
	_ "github.com/ncruces/go-sqlite3/driver"
	// The Adiantum encrypting VFS (auto-registers the "adiantum" VFS).
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the database handle. Keep *sql.DB exported for now so the Phase-2
// query layer (and tests) can use it directly; it will move behind query methods.
type Store struct {
	DB *sql.DB
}

// dsn builds the connection string for an Adiantum-encrypted database.
//
// The key is passed as the `textkey` URI parameter, which the Adiantum VFS reads
// at open time (Argon2id-derived) — so every pooled connection is keyed, not just
// the first. `vfs=adiantum` selects the encrypting VFS. busy_timeout and
// foreign_keys are applied per-connection via the driver's `_pragma` mechanism.
//
// NOTE: if the `textkey` URI param ever stops being honoured by the database/sql
// driver, the documented fallback is to set it as the FIRST pragma instead:
// `_pragma=textkey('<key>')` (keys must be the first pragma, per the driver docs).
func dsn(path, key string) string {
	q := url.Values{}
	q.Set("vfs", "adiantum")
	q.Set("textkey", key)
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(on)")
	return "file:" + path + "?" + q.Encode()
}

// Open opens (creating if absent) the encrypted database at path, keyed by key.
// An empty key is rejected — an unkeyed Adiantum database is a plaintext one.
func Open(path, key string) (*Store, error) {
	if key == "" {
		return nil, fmt.Errorf("store: empty encryption key")
	}
	db, err := sql.Open("sqlite3", dsn(path, key))
	if err != nil {
		return nil, err
	}
	// SQLite is single-writer; a connection pool only adds lock contention here.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: open encrypted db (wrong key?): %w", err)
	}
	return &Store{DB: db}, nil
}

// Close closes the underlying handle.
func (s *Store) Close() error { return s.DB.Close() }

// Migrate applies every embedded migration not yet recorded, in filename order,
// each in its own transaction. Idempotent: already-applied files are skipped.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}

	names, err := migrationNames()
	if err != nil {
		return err
	}

	for _, name := range names {
		applied, err := s.isApplied(ctx, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		raw, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if err := s.applyMigration(ctx, name, string(raw)); err != nil {
			return fmt.Errorf("store: migration %s: %w", name, err)
		}
	}
	return nil
}

func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) isApplied(ctx context.Context, name string) (bool, error) {
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE name = ?`, name).Scan(&one)
	switch err {
	case nil:
		return true, nil
	case sql.ErrNoRows:
		return false, nil
	default:
		return false, err
	}
}

func (s *Store) applyMigration(ctx context.Context, name, script string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit

	// Execute statement-by-statement: the database/sql Exec contract is one
	// statement per call, so a multi-statement migration file must be split.
	for _, stmt := range splitStatements(script) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(name) VALUES(?)`, name); err != nil {
		return err
	}
	return tx.Commit()
}

// splitStatements strips line comments and splits a SQL script on ';' into
// individual statements. Safe for this project's controlled migration files,
// which contain no ';' inside string literals or triggers.
func splitStatements(script string) []string {
	var cleaned strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		cleaned.WriteString(line)
		cleaned.WriteByte('\n')
	}
	var out []string
	for _, stmt := range strings.Split(cleaned.String(), ";") {
		if strings.TrimSpace(stmt) != "" {
			out = append(out, stmt)
		}
	}
	return out
}
