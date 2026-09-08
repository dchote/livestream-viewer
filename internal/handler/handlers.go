package handler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/dchote/livestream-viewer/internal/config"
	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/display/strategy"
	"github.com/dchote/livestream-viewer/internal/display/transition"
	"github.com/dchote/livestream-viewer/internal/events"
	"github.com/dchote/livestream-viewer/internal/ingest"
	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/preview"
	"github.com/dchote/livestream-viewer/internal/schedule"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
	"github.com/dchote/livestream-viewer/internal/source/youtube/pot"
)

type Handlers struct {
	DB       *gorm.DB
	Cfg      *config.Config
	Runtime  *schedule.Runtime
	Hub      *events.Hub
	Tools    resolver.Tools
	Caps     capability.Info
	Preview  *preview.Service
	Manager  *ingest.Manager
	Displays func() []map[string]any
	POT      *pot.Provider

	loginLimit     *rateLimiter
	probeSlots     chan struct{}
	sseClients     atomic.Int32
	previewClients atomic.Int32
}

func New(db *gorm.DB, cfg *config.Config, rt *schedule.Runtime, hub *events.Hub) *Handlers {
	return &Handlers{
		DB:         db,
		Cfg:        cfg,
		Runtime:    rt,
		Hub:        hub,
		loginLimit: newRateLimiter(loginBurst, loginRefill),
		probeSlots: make(chan struct{}, maxConcurrentProbes),
	}
}

// Health reports liveness and readiness.
//
// A display failure yields 200 with status "degraded" rather than 503: the
// engine is allowed to fail while the API stays up so the operator can read
// the error and fix the host, and a 503 here would make a service manager
// restart the process in a loop instead. Only an unreachable database, where
// nothing works and a restart is the right response, returns 503.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	dbOK := h.databaseReachable(r.Context())

	displayEnabled := h.Cfg.DisplayEnabled
	displayRunning := false
	displayError := ""
	if h.Runtime != nil {
		st := h.Runtime.State()
		displayRunning = st.DisplayRunning
		displayError = st.DisplayError
	}

	ready := dbOK && (!displayEnabled || displayRunning)
	status := "ok"
	code := http.StatusOK
	switch {
	case !dbOK:
		status = "unhealthy"
		code = http.StatusServiceUnavailable
	case !ready:
		status = "degraded"
	}

	body := map[string]any{
		"status":          status,
		"ready":           ready,
		"database":        dbOK,
		"display":         displayEnabled,
		"display_running": displayRunning,
	}
	if displayError != "" {
		body["display_error"] = displayError
	}
	WriteJSON(w, code, body)
}

// databaseReachable pings the pool with a short deadline so a locked or
// missing SQLite file surfaces instead of hanging the health check.
func (h *Handlers) databaseReachable(ctx context.Context) bool {
	if h.DB == nil {
		return false
	}
	sqlDB, err := h.DB.DB()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return sqlDB.PingContext(ctx) == nil
}

func (h *Handlers) SystemInfo(w http.ResponseWriter, r *http.Request) {
	yt, ffprobe, _ := h.Tools.Available()
	ytVersion := ""
	if yt {
		ytVersion = h.Tools.YtDlpVersion(r.Context())
	}
	running := false
	if h.Runtime != nil {
		running = h.Runtime.State().DisplayRunning
	}
	caps := h.Caps
	displays := []any{}
	if h.Displays != nil {
		if d := h.Displays(); d != nil {
			for _, item := range d {
				displays = append(displays, item)
			}
		}
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"version":         appVersion,
		"commit":          appCommit,
		"build_time":      appBuildTime,
		"platform":        runtimePlatform(),
		"display_running": running,
		"display_enabled": h.Cfg.DisplayEnabled,
		"yt_dlp":          yt,
		"yt_dlp_version":  ytVersion,
		"ffprobe":         ffprobe,
		"capabilities": map[string]any{
			"h264_hw":      caps.H264HW,
			"hevc_hw":      caps.HEVCHW,
			"videotoolbox": caps.VideoToolbox,
			"vaapi":        caps.VAAPI,
			"v4l2":         caps.V4L2,
			"drm":          caps.DRM,
		},
		"displays": displays,
		"youtube":  h.youtubeStatus(),
	})
}

func (h *Handlers) Layouts(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"layouts": layout.All()})
}

func (h *Handlers) Transitions(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"transitions":    transition.Catalogue(),
		"easing_presets": transition.EasingPresets(),
	})
}

func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	var rc model.RuntimeConfig
	if err := h.DB.First(&rc).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "config_error", "failed to load config", nil)
		return
	}
	WriteJSON(w, http.StatusOK, rc)
}

// isHexColor accepts the #rgb and #rrggbb forms the renderer can parse.
func isHexColor(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return false
	}
	s = s[1:]
	if len(s) != 3 && len(s) != 6 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func (h *Handlers) PatchConfig(w http.ResponseWriter, r *http.Request) {
	var rc model.RuntimeConfig
	if err := h.DB.First(&rc).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "config_error", "failed to load config", nil)
		return
	}
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	// Every numeric knob feeds the render loop or the decoders, where an
	// absurd value is not a cosmetic problem: preview_width sizes a GPU render
	// target, preview_fps gates a full read-back, gutter_px can consume the
	// whole cell. Reject out-of-range values rather than clamping silently, so
	// the caller learns their setting did not apply.
	bad := func(field, msg string) {
		WriteError(w, http.StatusBadRequest, "invalid_config", field+": "+msg, map[string]any{"field": field})
	}
	intField := func(field string, min, max int, dst *int) bool {
		raw, ok := patch[field]
		if !ok {
			return true
		}
		v, ok := raw.(float64)
		if !ok || v != math.Trunc(v) {
			bad(field, "must be a whole number")
			return false
		}
		if v < float64(min) || v > float64(max) {
			bad(field, fmt.Sprintf("must be between %d and %d", min, max))
			return false
		}
		*dst = int(v)
		return true
	}

	ok := intField("output_width", 160, 7680, &rc.OutputWidth) &&
		intField("output_height", 120, 4320, &rc.OutputHeight) &&
		intField("default_dwell_ms", 100, 24*60*60*1000, &rc.DefaultDwellMS) &&
		// 0 means "no limit".
		intField("max_hw_decoders", 0, 64, &rc.MaxHWDecoders) &&
		intField("reconnect_backoff_ms", 100, 60_000, &rc.ReconnectBackoffMS) &&
		intField("preview_fps", 1, 30, &rc.PreviewFPS) &&
		intField("preview_width", 160, 1920, &rc.PreviewWidth) &&
		intField("gutter_px", 0, 256, &rc.GutterPx)
	if !ok {
		return
	}
	if v, ok := patch["allow_software_fallback"].(bool); ok {
		rc.AllowSoftwareFallback = v
	}
	if v, ok := patch["placeholder_color"].(string); ok {
		if !isHexColor(v) {
			bad("placeholder_color", "must be a hex colour such as #1a1a1a")
			return
		}
		rc.PlaceholderColor = v
	}
	if v, ok := patch["default_transition"]; ok && v != nil {
		b, err := json.Marshal(v)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "bad_request", "invalid default_transition", nil)
			return
		}
		var spec transition.Spec
		if err := json.Unmarshal(b, &spec); err != nil {
			WriteError(w, http.StatusBadRequest, "bad_request", "invalid default_transition", nil)
			return
		}
		if err := transition.Validate(spec); err != nil {
			WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
			return
		}
		rc.DefaultTransition = spec
	}
	if err := h.DB.Save(&rc).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "config_error", "failed to save config", nil)
		return
	}
	h.reloadStrategy()
	WriteJSON(w, http.StatusOK, rc)
}

// reloadStrategy rebuilds the immutable snapshot and hands it to the scheduler.
// The database write that triggered it has already committed, so a build
// failure is not reported to the client — retrying would duplicate the write.
// It is logged at error level instead: the scheduler is now serving a stale
// snapshot and that needs operator attention.
func (h *Handlers) reloadStrategy() {
	if h.Runtime == nil {
		return
	}
	snap, err := strategy.Build(h.DB)
	if err != nil {
		slog.Error("strategy reload failed; scheduler is serving a stale snapshot", "error", err)
		return
	}
	h.Runtime.ApplyStrategy(snap)
	if h.Manager != nil {
		// Resync, not RequestSync: an edit can leave the needed set identical
		// while changing the rows behind it (URL, credentials, enabled).
		h.Manager.Resync(h.Runtime.ComposeView().Needed)
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	// Login is the only unauthenticated write endpoint, and bcrypt makes each
	// attempt expensive, so throttle per client address.
	if h.loginLimit != nil && !h.loginLimit.allow(clientKey(r), time.Now()) {
		w.Header().Set("Retry-After", strconv.Itoa(int(loginRefill.Seconds())))
		WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts; try again shortly", nil)
		return
	}
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	var user model.User
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password", nil)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password", nil)
		return
	}
	token, err := h.issueToken(&user)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "token_error", "failed to issue token", nil)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  publicUser(&user),
	})
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
		return
	}
	WriteJSON(w, http.StatusOK, publicUser(u))
}

func (h *Handlers) AuthStatus(w http.ResponseWriter, r *http.Request) {
	var n int64
	_ = h.DB.Model(&model.User{}).Count(&n)
	WriteJSON(w, http.StatusOK, map[string]any{"setup_required": n == 0})
}

func publicUser(u *model.User) map[string]any {
	return map[string]any{
		"id":                   u.ID,
		"username":             u.Username,
		"role":                 u.Role,
		"must_change_password": u.MustChangePassword,
	}
}

func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.CurrentPassword)) != nil {
		WriteError(w, http.StatusBadRequest, "invalid_credentials", "current password is incorrect", nil)
		return
	}
	if len(req.NewPassword) < model.MinPasswordLength {
		WriteError(w, http.StatusBadRequest, "bad_request", shortPasswordMessage, nil)
		return
	}
	if req.NewPassword == req.CurrentPassword {
		WriteError(w, http.StatusBadRequest, "bad_request", "new password must be different from the current password", nil)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "failed to hash password", nil)
		return
	}
	u.PasswordHash = string(hash)
	u.MustChangePassword = false
	if err := h.DB.Save(u).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "failed to update password", nil)
		return
	}
	WriteJSON(w, http.StatusOK, publicUser(u))
}

func (h *Handlers) issueToken(u *model.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":  strconv.FormatUint(uint64(u.ID), 10),
		"name": u.Username,
		"role": u.Role,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(h.Cfg.JWTSecret))
}
