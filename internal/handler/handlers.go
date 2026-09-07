package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/dchote/livestream-viewer/internal/config"
	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/display/strategy"
	"github.com/dchote/livestream-viewer/internal/display/transition"
	"github.com/dchote/livestream-viewer/internal/events"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/schedule"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
)

type Handlers struct {
	DB      *gorm.DB
	Cfg     *config.Config
	Runtime *schedule.Runtime
	Hub     *events.Hub
	Tools   resolver.Tools
}

func New(db *gorm.DB, cfg *config.Config, rt *schedule.Runtime, hub *events.Hub) *Handlers {
	return &Handlers{DB: db, Cfg: cfg, Runtime: rt, Hub: hub}
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"ready":   true,
		"display": h.Cfg.DisplayEnabled,
	})
}

func (h *Handlers) SystemInfo(w http.ResponseWriter, r *http.Request) {
	yt, ffprobe, _ := h.Tools.Available()
	WriteJSON(w, http.StatusOK, map[string]any{
		"version":         appVersion,
		"commit":          appCommit,
		"build_time":      appBuildTime,
		"platform":        runtimePlatform(),
		"display_running": false,
		"display_enabled": h.Cfg.DisplayEnabled,
		"yt_dlp":          yt,
		"ffprobe":         ffprobe,
		"capabilities": map[string]any{
			"h264_hw": false,
			"hevc_hw": false,
			"v4l2":    []string{},
			"drm":     []string{},
		},
		"displays": []any{},
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
	if v, ok := patch["output_width"].(float64); ok {
		rc.OutputWidth = int(v)
	}
	if v, ok := patch["output_height"].(float64); ok {
		rc.OutputHeight = int(v)
	}
	if v, ok := patch["output_rotation"].(float64); ok {
		rc.OutputRotation = int(v)
	}
	if v, ok := patch["default_dwell_ms"].(float64); ok {
		rc.DefaultDwellMS = int(v)
	}
	if v, ok := patch["allow_software_fallback"].(bool); ok {
		rc.AllowSoftwareFallback = v
	}
	if v, ok := patch["max_hw_decoders"].(float64); ok {
		rc.MaxHWDecoders = int(v)
	}
	if v, ok := patch["reconnect_backoff_ms"].(float64); ok {
		rc.ReconnectBackoffMS = int(v)
	}
	if v, ok := patch["offline_grace_ms"].(float64); ok {
		rc.OfflineGraceMS = int(v)
	}
	if v, ok := patch["preview_fps"].(float64); ok {
		rc.PreviewFPS = int(v)
	}
	if v, ok := patch["preview_width"].(float64); ok {
		rc.PreviewWidth = int(v)
	}
	if v, ok := patch["placeholder_color"].(string); ok {
		rc.PlaceholderColor = v
	}
	if v, ok := patch["gutter_px"].(float64); ok {
		rc.GutterPx = int(v)
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
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
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
