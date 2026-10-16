// Command totemd serves the Totem Finder API and the embedded SPA.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Niklasvdm/TotemAppIT/internal/api"
	"github.com/Niklasvdm/TotemAppIT/internal/config"
	"github.com/Niklasvdm/TotemAppIT/internal/game"
	"github.com/Niklasvdm/TotemAppIT/internal/store"
	"github.com/Niklasvdm/TotemAppIT/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	// Install the structured logger (default level from TOTEM_LOG_LEVEL, quiet by
	// default) and hand the games engine the always-on logger for connection lifecycle
	// and per-room debug traces.
	game.SetVerboseLogger(config.SetupLogging(cfg))
	game.SetRoomIdleTimeout(cfg.RoomIdleTimeout) // TOTEM_ROOM_IDLE_TIMEOUT, default 5m
	for _, w := range cfg.Warnings {
		slog.Warn("config", "warning", w)
	}

	st, err := store.Open(cfg.DBPath, cfg.DBKey)
	if err != nil {
		slog.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := st.Migrate(ctx); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}

	spa, ok := web.FS()
	if ok {
		slog.Info("serving embedded SPA (built with -tags embed)")
	} else {
		slog.Info("no embedded SPA (API-only build); the UI is served by the Vite dev server in dev")
	}

	apiSrv := api.New(st, cfg.Images, cfg.Emoji,
		api.WithSPA(spa),
		api.WithGameOrigins(cfg.GameOrigins),
		api.WithTrustedProxies(cfg.TrustedProxies))
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
		slog.Info("totemd listening", "addr", cfg.Addr, "level", cfg.LogLevel)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("serve", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done() // SIGINT/SIGTERM
	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}
