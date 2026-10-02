// Command totemd serves the Totem Finder API (and, later, the embedded SPA).
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Niklasvdm/TotemAppIT/internal/api"
	"github.com/Niklasvdm/TotemAppIT/internal/config"
	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	for _, w := range cfg.Warnings {
		log.Printf("warning: %s", w)
	}

	st, err := store.Open(cfg.DBPath, cfg.DBKey)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := st.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	apiSrv := api.New(st, cfg.Images, cfg.Emoji)
	defer apiSrv.Close()

	// The read/write timeouts suit the JSON API. The WebSocket game route
	// clears them per-connection (see api.gameWS), so game sockets are not
	// capped at 10s.
	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      apiSrv.Router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("totemd listening on %s", cfg.Addr)
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
