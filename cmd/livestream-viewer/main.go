package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/dchote/livestream-viewer/internal/config"
	"github.com/dchote/livestream-viewer/internal/database"
	"github.com/dchote/livestream-viewer/internal/display"
	"github.com/dchote/livestream-viewer/internal/display/strategy"
	"github.com/dchote/livestream-viewer/internal/events"
	"github.com/dchote/livestream-viewer/internal/handler"
	"github.com/dchote/livestream-viewer/internal/ingest"
	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/preview"
	"github.com/dchote/livestream-viewer/internal/rest"
	"github.com/dchote/livestream-viewer/internal/schedule"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
	"github.com/dchote/livestream-viewer/internal/source/youtube"
	"github.com/dchote/livestream-viewer/internal/source/youtube/pot"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// readTimeout bounds how long a client may take to deliver a request.
// Streaming responses are unaffected (the deadline covers the request, not the
// response); the upload handler extends it for its own body.
const readTimeout = 30 * time.Second

func main() {
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
	}

	if cfg.DisplayEnabled {
		runtime.LockOSThread()
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rt := schedule.New(nil, cfg.DisplayEnabled)
	hub := events.NewHub()
	caps := capability.Probe()
	prev := preview.New()
	potProv := pot.New(pot.Config{
		Mode:      cfg.YouTubePOTMode,
		URL:       cfg.YouTubePOTURL,
		Port:      cfg.YouTubePOTPort,
		ServerDir: cfg.YouTubePOTServerDir,
	})
	pluginDir, err := pot.InstallPlugin(cfg.DataDir)
	if err != nil {
		slog.Warn("youtube yt-dlp plugin not installed", "err", err)
	}
	tools := resolver.Tools{
		CookiesFile:        youtube.CookiesPath(cfg.DataDir),
		CookiesFromBrowser: cfg.YouTubeCookiesFromBrowser,
		POTokenFile:        youtube.POTokenPath(cfg.DataDir),
		POTBaseURL:         potProv.BaseURL(),
		PluginDir:          pluginDir,
	}

	winW, winH := 1920, 1080
	if snap, err := strategy.Build(db); err == nil && snap != nil {
		rt.ApplyStrategy(snap)
		if snap.OutputWidth > 0 {
			winW = snap.OutputWidth
		}
		if snap.OutputHeight > 0 {
			winH = snap.OutputHeight
		}
	}

	var eng *display.Engine
	var mgr *ingest.Manager
	if cfg.DisplayEnabled {
		eng = display.NewEngine(display.Config{
			Driver: cfg.DisplayDriver,
			Device: cfg.DisplayDevice,
			Width:  winW,
			Height: winH,
			Cancel: stop,
		}, rt, nil, prev)
		mgr = ingest.NewManager(db, rt, caps, tools, eng)
		eng.SetManager(mgr)
	}

	mux := rest.NewWith(db, cfg, feFS, rt, hub, tools, func(h *handler.Handlers) {
		h.Caps = caps
		h.Preview = prev
		h.Manager = mgr
		h.SetPOT(potProv)
		if eng != nil {
			h.Displays = func() []map[string]any {
				out := []map[string]any{}
				for _, d := range eng.Displays() {
					out = append(out, map[string]any{
						"id": d.ID, "name": d.Name, "width": d.Width, "height": d.Height,
					})
				}
				return out
			}
		}
	})

	go potProv.Start(rootCtx)

	// No WriteTimeout: the SSE and MJPEG endpoints are deliberately long-lived
	// and a global write deadline would cut them off. The read side and idle
	// connections are bounded instead, which is what protects against a peer
	// that opens sockets and never finishes a request.
	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       readTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,
		BaseContext: func(_ net.Listener) context.Context {
			return rootCtx
		},
	}

	go func() {
		slog.Info("listening", "addr", cfg.Addr(), "frontend_embed", cfg.FrontendEmbed && feFS != nil, "display", cfg.DisplayEnabled)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "err", err)
			stop()
		}
	}()

	// Escape hatch: a second signal during graceful shutdown exits now. A
	// service manager sends one signal and then SIGKILL, so this only fires
	// when an operator asks twice.
	go func() {
		<-rootCtx.Done()
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		slog.Warn("forced exit")
		os.Exit(1)
	}()

	if cfg.DisplayEnabled && eng != nil && mgr != nil {
		go mgr.Run(rootCtx)
		if err := eng.Run(rootCtx); err != nil {
			slog.Error("display engine failed; scheduler continues headless", "err", err)
			go rt.Run(rootCtx)
			<-rootCtx.Done()
		}
	} else {
		go rt.Run(rootCtx)
		<-rootCtx.Done()
	}

	slog.Info("shutting down")
	hub.Close()
	prev.Close()
	if mgr != nil {
		mgr.StopAll()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown timed out, closing", "err", err)
		_ = srv.Close()
	}
	slog.Info("stopped")
}
