package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLevelDefaultsToError(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelError, // unset
		"bogus":   slog.LevelError, // unrecognised
	} {
		if got := parseLevel(in); got != want {
			t.Fatalf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDailyWriterPrunesByAge(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := now.AddDate(0, 0, -20).Format("2006-01-02")   // beyond a 14-day window
	recent := now.AddDate(0, 0, -3).Format("2006-01-02") // inside it
	for _, d := range []string{old, recent} {
		if err := os.WriteFile(filepath.Join(dir, "totemd-"+d+".log"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A file that does not match the log pattern must be left untouched.
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Opening the writer rotates to today's file and prunes on the way.
	if _, err := newDailyWriter(dir, 14); err != nil {
		t.Fatalf("newDailyWriter: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "totemd-"+old+".log")); !os.IsNotExist(err) {
		t.Fatalf("a log older than the retention window should be pruned (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "totemd-"+recent+".log")); err != nil {
		t.Fatalf("a log inside the retention window should survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("a non-log file must be left alone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "totemd-"+now.Format("2006-01-02")+".log")); err != nil {
		t.Fatalf("today's log file should have been created: %v", err)
	}
}
