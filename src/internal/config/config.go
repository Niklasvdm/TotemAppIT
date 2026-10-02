// Package config loads all runtime configuration — environment variables and
// the files they point at — in ONE place. The rest of the app receives plain
// values (and an fs.FS for images), so no other package needs to import "os".
package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Config is everything totemd needs to run, resolved from the environment.
type Config struct {
	Addr     string            // TOTEM_ADDR
	DBPath   string            // TOTEM_DB_PATH
	DBKey    string            // TOTEM_DB_KEY or TOTEM_DB_KEY_FILE
	Images   fs.FS             // TOTEM_IMAGE_DIR (nil when unset/missing)
	Emoji    map[string]string // TOTEM_EMOJI_FILE (nil when missing)
	Warnings []string          // non-fatal config problems for the caller to log
}

// Load resolves the full configuration for the server. Missing optional assets
// (emoji file, image dir) are non-fatal but recorded in Warnings so the caller
// can surface them — the relative defaults only resolve when the process runs
// from src/, which is a common production foot-gun (see deploy/totemd.service).
func Load() (Config, error) {
	key, err := DBKey()
	if err != nil {
		return Config{}, err
	}
	c := Config{
		Addr:   env("TOTEM_ADDR", "127.0.0.1:8683"), // 8683 = "TOTE"; below the ephemeral range
		DBPath: env("TOTEM_DB_PATH", "totem.db"),
		DBKey:  key,
	}

	emojiPath := env("TOTEM_EMOJI_FILE", "../data/emoji.json")
	if c.Emoji = loadEmoji(emojiPath); c.Emoji == nil {
		c.Warnings = append(c.Warnings,
			fmt.Sprintf("emoji map not loaded from %q — cards fall back to 🐾 (set TOTEM_EMOJI_FILE)", emojiPath))
	}

	if dir := env("TOTEM_IMAGE_DIR", "../data/images"); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			c.Images = os.DirFS(dir)
		} else {
			c.Warnings = append(c.Warnings,
				fmt.Sprintf("image dir %q not accessible — animal images will 404 (set TOTEM_IMAGE_DIR)", dir))
		}
	}
	return c, nil
}

// DBKey resolves the Adiantum key from TOTEM_DB_KEY, or the file named by
// TOTEM_DB_KEY_FILE (e.g. a systemd credential). Used by the seed command too.
func DBKey() (string, error) {
	if k := os.Getenv("TOTEM_DB_KEY"); k != "" {
		return k, nil
	}
	if f := os.Getenv("TOTEM_DB_KEY_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("read TOTEM_DB_KEY_FILE: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return "", fmt.Errorf("set TOTEM_DB_KEY or TOTEM_DB_KEY_FILE (the Adiantum encryption key)")
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadEmoji reads a slug->emoji JSON map (nil on any error → frontend falls back).
func loadEmoji(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]string
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}
