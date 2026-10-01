// Command totemd serves the Totem Finder API (and, later, the embedded SPA).
package main

import (
	"context"
	"errors"
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
	// 8683 = "TOTE" on a phone keypad; deliberately unusual, and below the Linux
	// ephemeral range (32768+) so the listener can't clash with an outbound port.
	addr := env("TOTEM_ADDR", "127.0.0.1:8683")
	dbPath := env("TOTEM_DB_PATH", "totem.db")
	// The Adiantum key comes from the environment: TOTEM_DB_KEY directly, or
	// TOTEM_DB_KEY_FILE pointing at a file (e.g. a systemd-provided credential,
	// so the key is never baked into the unit or visible in the process list).
	key := os.Getenv("TOTEM_DB_KEY")
	if key == "" {
		if f := os.Getenv("TOTEM_DB_KEY_FILE"); f != "" {
			b, err := os.ReadFile(f)
			if err != nil {
				log.Fatalf("read TOTEM_DB_KEY_FILE: %v", err)
			}
			key = strings.TrimSpace(string(b))
		}
	}
	if key == "" {
		log.Fatal("set TOTEM_DB_KEY or TOTEM_DB_KEY_FILE (the Adiantum encryption key)")
	}

	st, err := store.Open(dbPath, key)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      api.New(st).Router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("totemd listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
