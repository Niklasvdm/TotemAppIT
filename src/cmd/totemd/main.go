// Command totemd serves the Totem Finder API (and, later, the embedded SPA).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Niklasvdm/TotemAppIT/internal/api"
	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

func main() {
	// 8683 = "TOTE" on a phone keypad; unusual, and below the ephemeral range.
	addr := env("TOTEM_ADDR", "127.0.0.1:8683")
	imageDir := env("TOTEM_IMAGE_DIR", "../data/images")
	dbPath := env("TOTEM_DB_PATH", "totem.db")

	key := dbKey()
	if key == "" {
		log.Fatal("set TOTEM_DB_KEY or TOTEM_DB_KEY_FILE (the Adiantum encryption key)")
	}

	st, err := store.Open(dbPath, key)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := st.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	app := api.New(st, imageDir)
	app.Emoji = api.LoadEmoji(env("TOTEM_EMOJI_FILE", "../data/emoji.json"))

	srv := &http.Server{
		Addr:         addr,
		Handler:      app.Router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("totemd listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done() // SIGINT/SIGTERM
	log.Println("shutting down…")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// env returns the environment variable, or def when unset.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// dbKey reads the Adiantum key from TOTEM_DB_KEY, or from the file named by
// TOTEM_DB_KEY_FILE (e.g. a systemd credential), trimming a trailing newline.
func dbKey() string {
	if k := os.Getenv("TOTEM_DB_KEY"); k != "" {
		return k
	}
	if f := os.Getenv("TOTEM_DB_KEY_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("read TOTEM_DB_KEY_FILE: %v", err)
		}
		return strings.TrimSpace(string(b))
	}
	return ""
}
