package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/dchote/livestream-viewer/internal/config"
	"github.com/dchote/livestream-viewer/internal/database"
	"github.com/dchote/livestream-viewer/internal/handler"
	"github.com/dchote/livestream-viewer/internal/rest"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	runtime.LockOSThread()

	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("livestream-viewer %s (commit: %s, built: %s)\n", version, commit, buildTime)
		os.Exit(0)
	}

	cfg, err := config.Load("")
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}
	config.ParseFlags(cfg)

	if len(flag.Args()) > 0 && flag.Arg(0) == "--version" {
		fmt.Printf("livestream-viewer %s (commit: %s, built: %s)\n", version, commit, buildTime)
		os.Exit(0)
	}

	level := slog.LevelInfo
	switch cfg.LoggingLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if err := cfg.EnsureDirs(); err != nil {
		slog.Error("create data dirs", "err", err)
		os.Exit(1)
	}
	if _, err := cfg.ResolveJWTSecret(); err != nil {
		slog.Error("jwt secret", "err", err)
		os.Exit(1)
	}

	handler.SetVersion(version, commit, buildTime)

	db, err := database.Open(cfg)
	if err != nil {
		slog.Error("database open failed", "err", err)
		os.Exit(1)
	}

	var feFS fs.FS
	if cfg.FrontendEmbed {
		feFS, err = getFrontendFS()
		if err != nil {
			slog.Warn("frontend embed unavailable, SPA routes will 404", "err", err)
			feFS = nil
		}
	} else {
		feFS = nil
	}

	if cfg.DisplayEnabled {
		slog.Warn("display engine requested but not implemented in this scaffold; continuing API-only")
	}

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           rest.New(db, cfg, feFS),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", cfg.Addr(), "frontend_embed", cfg.FrontendEmbed && feFS != nil)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}
