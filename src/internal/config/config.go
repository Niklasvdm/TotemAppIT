// Package config loads all runtime configuration — environment variables and
// the files they point at — in ONE place. The rest of the app receives plain
// values (and an fs.FS for images), so no other package needs to import "os".
package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
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

	// GameOrigins are extra host patterns allowed to open a game WebSocket,
	// from TOTEM_ALLOWED_ORIGINS. Same-origin is always permitted; this is
	// only needed where the browser's Origin and the Host the backend sees
	// differ, as with the Vite dev proxy. Empty in production, where the WAF
	// forwards its own Host.
	GameOrigins []string

	// TrustedProxies are the networks (CIDRs or bare IPs) whose X-Forwarded-For
	// / X-Real-IP headers the server will believe, from TOTEM_TRUSTED_PROXIES.
	// Only the reverse-proxy/WAF host belongs here. When empty, forwarding
	// headers are ignored and the real TCP peer is used, so a direct client
	// cannot spoof its IP to defeat the rate limiter.
	TrustedProxies []*net.IPNet
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
		Addr:        env("TOTEM_ADDR", "127.0.0.1:8683"), // 8683 = "TOTE"; below the ephemeral range
		DBPath:      env("TOTEM_DB_PATH", "totem.db"),
		DBKey:       key,
		GameOrigins: splitList(env("TOTEM_ALLOWED_ORIGINS", "")),
	}

	for _, item := range splitList(env("TOTEM_TRUSTED_PROXIES", "")) {
		if n, ok := parseCIDR(item); ok {
			c.TrustedProxies = append(c.TrustedProxies, n)
		} else {
			c.Warnings = append(c.Warnings,
				fmt.Sprintf("ignoring invalid TOTEM_TRUSTED_PROXIES entry %q (want an IP or CIDR)", item))
		}
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

// parseCIDR accepts either a CIDR ("192.168.10.0/24") or a bare IP
// ("192.168.10.1", treated as a /32 or /128), returning the network.
func parseCIDR(s string) (*net.IPNet, bool) {
	if _, n, err := net.ParseCIDR(s); err == nil {
		return n, true
	}
	if ip := net.ParseIP(s); ip != nil {
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, true
	}
	return nil, false
}

// splitList parses a comma-separated env value into trimmed, non-empty items.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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
