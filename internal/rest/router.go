package rest

import (
	"bytes"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/dchote/livestream-viewer/api"
	"github.com/dchote/livestream-viewer/internal/config"
	"github.com/dchote/livestream-viewer/internal/display/strategy"
	"github.com/dchote/livestream-viewer/internal/events"
	"github.com/dchote/livestream-viewer/internal/handler"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/schedule"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
	"github.com/dchote/livestream-viewer/internal/source/youtube"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"
)

func New(db *gorm.DB, cfg *config.Config, feFS fs.FS, rt *schedule.Runtime, hub *events.Hub, tools resolver.Tools) http.Handler {
	return NewWith(db, cfg, feFS, rt, hub, tools, nil)
}

func NewWith(db *gorm.DB, cfg *config.Config, feFS fs.FS, rt *schedule.Runtime, hub *events.Hub, tools resolver.Tools, setup func(*handler.Handlers)) http.Handler {
	if rt == nil {
		rt = schedule.New(nil, cfg.DisplayEnabled)
	}
	if hub == nil {
		hub = events.NewHub()
	}
	h := handler.New(db, cfg, rt, hub)
	if tools.CookiesFile == "" {
		tools.CookiesFile = youtube.CookiesPath(cfg.DataDir)
	}
	if tools.POTokenFile == "" {
		tools.POTokenFile = youtube.POTokenPath(cfg.DataDir)
	}
	h.Tools = tools
	if setup != nil {
		setup(h)
	}
	rt.SetOnChange(func(st *schedule.State) {
		hub.PublishDisplayState(st)
	})
	if snap, err := strategy.Build(db); err != nil {
		slog.Error("initial strategy build failed; scheduler starts with no snapshot", "error", err)
	} else {
		rt.ApplyStrategy(snap)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(cors)
	r.Use(handler.LimitRequestBody)

	r.Get("/health", h.Health)
	r.Get("/api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write(api.OpenAPI)
	})
	r.Get("/docs", swaggerUIHandler)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/auth/status", h.AuthStatus)
		r.Post("/auth/login", h.Login)

		r.Group(func(r chi.Router) {
			r.Use(handler.Auth(db, cfg.JWTSecret))
			r.Use(handler.EnforcePasswordChange)

			r.Get("/auth/me", h.Me)
			r.Post("/auth/change-password", h.ChangePassword)

			r.Get("/system/info", h.SystemInfo)
			r.Get("/system/youtube", h.GetYouTube)
			r.Get("/layouts", h.Layouts)
			r.Get("/transitions", h.Transitions)
			r.Get("/config", h.GetConfig)

			r.Get("/sources", h.ListSources)
			r.Get("/sources/{id}", h.GetSource)
			r.Get("/sources/{id}/thumbnail", h.SourceThumbnail)

			r.Get("/uploads", h.ListUploads)

			r.Get("/screens", h.ListScreens)
			r.Get("/screens/{id}", h.GetScreen)

			r.Get("/tour", h.GetTour)

			r.Get("/display/state", h.DisplayState)
			r.Get("/preview/stream", h.PreviewStream)
			r.Get("/preview/frame", h.PreviewFrame)
			r.Get("/events", h.Events)

			r.Group(func(r chi.Router) {
				r.Use(handler.RequireRole(model.RoleAdmin))

				r.Get("/users", h.ListUsers)
				r.Post("/users", h.CreateUser)
				r.Patch("/users/{id}", h.UpdateUser)
				r.Delete("/users/{id}", h.DeleteUser)

				r.Patch("/config", h.PatchConfig)

				r.Put("/system/youtube/cookies", h.PutYouTubeCookies)
				r.Delete("/system/youtube/cookies", h.DeleteYouTubeCookies)
				r.Put("/system/youtube/po-token", h.PutYouTubePOToken)

				r.Post("/sources", h.CreateSource)
				r.Patch("/sources/{id}", h.PatchSource)
				r.Delete("/sources/{id}", h.DeleteSource)
				r.Post("/sources/{id}/probe", h.ProbeSource)

				r.Post("/uploads", h.CreateUpload)
				r.Delete("/uploads/{id}", h.DeleteUpload)

				r.Post("/screens", h.CreateScreen)
				r.Patch("/screens/{id}", h.PatchScreen)
				r.Delete("/screens/{id}", h.DeleteScreen)

				r.Put("/tour", h.PutTour)

				r.Post("/display/next", h.DisplayNext)
				r.Post("/display/previous", h.DisplayPrevious)
				r.Post("/display/goto/{screenId}", h.DisplayGoto)
				r.Post("/display/pause", h.DisplayPause)
				r.Post("/display/resume", h.DisplayResume)
			})
		})
	})

	if feFS != nil {
		spa := spaHandler(feFS)
		r.Get("/", spa)
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			if strings.HasPrefix(req.URL.Path, "/api/") {
				http.NotFound(w, req)
				return
			}
			spa(w, req)
		})
	} else {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("livestream-viewer. Start with -frontend-embed=false and use the API at /api/v1, /health, /docs\n"))
		})
	}

	return r
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:3000" || origin == "http://127.0.0.1:3000" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>livestream-viewer API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    const base = document.querySelector('base')?.getAttribute('href') || '/';
    const url = (base.endsWith('/') ? base : base + '/') + 'api/v1/openapi.yaml';
    SwaggerUIBundle({ url: url, dom_id: '#swagger-ui' });
  </script>
</body>
</html>
`

func swaggerUIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func isAssetPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".js", ".mjs", ".css", ".woff2", ".woff", ".ttf", ".ico", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".json", ".map":
		return true
	}
	return strings.HasPrefix(path, "assets/")
}

func appBaseHref(requestPath string) string {
	path := strings.TrimPrefix(requestPath, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "hassio_ingress" && parts[2] != "" {
		return "/" + strings.Join(parts[0:3], "/") + "/"
	}
	return "/"
}

func spaHandler(feFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if feFS == nil {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		_, statErr := fs.Stat(feFS, path)
		if statErr != nil {
			if isAssetPath(path) {
				http.NotFound(w, r)
				return
			}
			path = "index.html"
		}
		f, err := feFS.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, r)
			return
		}
		if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		if path == "index.html" {
			body, err := io.ReadAll(f)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			requestPath := strings.TrimSuffix(r.URL.Path, "/")
			if requestPath == "" {
				requestPath = "/"
			}
			if requestPath != "/" {
				base := appBaseHref(r.URL.Path)
				baseTag := []byte("<base href=\"" + base + "\">")
				head := []byte("<head>")
				idx := bytes.Index(body, head)
				if idx >= 0 {
					body = bytes.Join([][]byte{body[:idx+len(head)], baseTag, body[idx+len(head):]}, nil)
				}
			}
			_, _ = w.Write(body)
			return
		}
		if rs, ok := f.(io.ReadSeeker); ok {
			http.ServeContent(w, r, path, stat.ModTime(), rs)
		} else {
			_, _ = io.Copy(w, f)
		}
	}
}
