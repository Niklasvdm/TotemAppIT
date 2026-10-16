// Package config loads all runtime configuration — environment variables and
// the files they point at — in ONE place. The rest of the app receives plain
// values (and an fs.FS for images), so no other package needs to import "os".
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
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

	// LogLevel is the floor for the general logger (TOTEM_LOG_LEVEL:
	// debug|info|warn|error, default error, so the server is quiet by default).
	// LogFormat is text (default, dev-friendly) or json (for the journal).
	LogLevel  string
	LogFormat string

	// LogDir, when set (TOTEM_LOG_DIR), switches logging from stderr to a per-day
	// file in that directory (totemd-YYYY-MM-DD.log), pruned to LogRetentionDays
	// (TOTEM_LOG_RETENTION_DAYS, default 14). Left empty, logs go to stderr / the
	// systemd journal as before, which does its own retention.
	LogDir           string
	LogRetentionDays int

	// RoomIdleTimeout is how long an empty game room is kept before the reaper closes
	// it. From TOTEM_ROOM_IDLE_TIMEOUT (a Go duration like "5m"), default 5 minutes.
	RoomIdleTimeout time.Duration
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
		Addr:             env("TOTEM_ADDR", "127.0.0.1:8683"), // 8683 = "TOTE"; below the ephemeral range
		DBPath:           env("TOTEM_DB_PATH", "totem.db"),
		DBKey:            key,
		GameOrigins:      splitList(env("TOTEM_ALLOWED_ORIGINS", "")),
		LogLevel:         strings.ToLower(env("TOTEM_LOG_LEVEL", "error")),
		LogFormat:        strings.ToLower(env("TOTEM_LOG_FORMAT", "text")),
		LogDir:           env("TOTEM_LOG_DIR", ""),
		LogRetentionDays: envInt("TOTEM_LOG_RETENTION_DAYS", 14),
		RoomIdleTimeout:  envDuration("TOTEM_ROOM_IDLE_TIMEOUT", 5*time.Minute),
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

// SetupLogging installs the process-wide structured logger and returns a second,
// always-on "verbose" logger. Two loggers share one stderr stream (captured by the
// journal), differing only in their level floor:
//
//   - the DEFAULT logger (installed via slog.SetDefault) is gated at cfg.LogLevel
//     (default error), so general and gameplay logs are quiet unless the operator
//     lowers the level. This is what package code reaches through slog.Info/Debug/….
//   - the VERBOSE logger always emits (floor debug), for the things we want to see
//     regardless of the quiet default: connection drops and reconnects (every room),
//     and a room's gameplay trace once it is started in debug (gated at the call site
//     so only a debug-activated room is noisy). The caller wires it in with
//     game.SetVerboseLogger.
//
// The DB key is never logged; it stays env-only (see DBKey and the README security
// section). Keep that invariant: never pass cfg.DBKey to any logger.
func SetupLogging(cfg Config) *slog.Logger {
	// Both loggers share one writer: stderr (the journal) by default, or a pruned
	// per-day file when TOTEM_LOG_DIR is set. A failed file open falls back to stderr
	// and is reported once the default logger is installed.
	var w io.Writer = os.Stderr
	var openErr error
	if cfg.LogDir != "" {
		if dw, err := newDailyWriter(cfg.LogDir, cfg.LogRetentionDays); err == nil {
			w = dw
		} else {
			openErr = err
		}
	}
	opts := func(level slog.Level) *slog.HandlerOptions { return &slog.HandlerOptions{Level: level} }
	newHandler := func(level slog.Level) slog.Handler {
		if cfg.LogFormat == "json" {
			return slog.NewJSONHandler(w, opts(level))
		}
		return slog.NewTextHandler(w, opts(level))
	}
	slog.SetDefault(slog.New(newHandler(parseLevel(cfg.LogLevel))))
	if openErr != nil {
		slog.Error("log dir unavailable, falling back to stderr", "dir", cfg.LogDir, "err", openErr)
	}
	return slog.New(newHandler(slog.LevelDebug))
}

// dailyWriter writes log output to a per-day file (totemd-YYYY-MM-DD.log) under dir,
// rotating when the date rolls over and deleting files older than retention days. It is
// safe for concurrent use: both loggers write through it, so the mutex serialises them
// (and the rotation) onto one stream. This is the app-side log cleanup; the systemd
// journal path (stderr, when TOTEM_LOG_DIR is unset) relies on journald's own retention.
type dailyWriter struct {
	dir       string
	retention int
	mu        sync.Mutex
	day       string
	f         *os.File
}

func newDailyWriter(dir string, retentionDays int) (*dailyWriter, error) {
	if retentionDays < 1 {
		retentionDays = 14
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &dailyWriter{dir: dir, retention: retentionDays}
	if err := w.rotate(time.Now()); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if now := time.Now(); now.Format("2006-01-02") != w.day {
		if err := w.rotate(now); err != nil {
			return 0, err
		}
	}
	return w.f.Write(p)
}

// rotate opens today's file (closing yesterday's) and prunes expired files. The caller
// holds the mutex, or is the constructor.
func (w *dailyWriter) rotate(now time.Time) error {
	if w.f != nil {
		_ = w.f.Close()
	}
	w.day = now.Format("2006-01-02")
	f, err := os.OpenFile(filepath.Join(w.dir, "totemd-"+w.day+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	w.f = f
	w.prune(now)
	return nil
}

// prune deletes totemd-*.log files whose date is older than the retention window.
func (w *dailyWriter) prune(now time.Time) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -w.retention)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "totemd-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		d, err := time.Parse("2006-01-02", strings.TrimSuffix(strings.TrimPrefix(name, "totemd-"), ".log"))
		if err != nil {
			continue
		}
		if d.Before(cutoff) {
			_ = os.Remove(filepath.Join(w.dir, name))
		}
	}
}

// parseLevel maps a level name to its slog level, defaulting to error (the quiet
// default) for an empty or unrecognised value.
func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
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

// envInt reads an integer env var, falling back to def when unset or unparseable.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envDuration reads a Go duration env var (e.g. "5m"), falling back to def when unset or
// unparseable.
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
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
