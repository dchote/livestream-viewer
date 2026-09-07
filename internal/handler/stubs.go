package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dchote/livestream-viewer/internal/model"
)

func (h *Handlers) ListSources(w http.ResponseWriter, r *http.Request) {
	var items []model.Source
	if err := h.DB.Find(&items).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to list sources", nil)
		return
	}
	if items == nil {
		items = []model.Source{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"sources": items})
}

func (h *Handlers) CreateSource(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "source create is not implemented in this scaffold", nil)
}

func (h *Handlers) GetSource(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotFound, "not_found", "source not found", nil)
}

func (h *Handlers) PatchSource(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "source update is not implemented in this scaffold", nil)
}

func (h *Handlers) DeleteSource(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "source delete is not implemented in this scaffold", nil)
}

func (h *Handlers) ProbeSource(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "source probe is not implemented in this scaffold", nil)
}

func (h *Handlers) SourceThumbnail(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func (h *Handlers) ListUploads(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"uploads": []any{}})
}

func (h *Handlers) CreateUpload(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "uploads are not implemented in this scaffold", nil)
}

func (h *Handlers) DeleteUpload(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "uploads are not implemented in this scaffold", nil)
}

func (h *Handlers) ListScreens(w http.ResponseWriter, r *http.Request) {
	var items []model.Screen
	if err := h.DB.Preload("Tiles").Preload("Items").Find(&items).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to list screens", nil)
		return
	}
	if items == nil {
		items = []model.Screen{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"screens": items})
}

func (h *Handlers) CreateScreen(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "screen create is not implemented in this scaffold", nil)
}

func (h *Handlers) GetScreen(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotFound, "not_found", "screen not found", nil)
}

func (h *Handlers) PatchScreen(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "screen update is not implemented in this scaffold", nil)
}

func (h *Handlers) DeleteScreen(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "screen delete is not implemented in this scaffold", nil)
}

func (h *Handlers) GetTour(w http.ResponseWriter, r *http.Request) {
	var tour model.Tour
	if err := h.DB.Preload("Entries").First(&tour).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load tour", nil)
		return
	}
	if tour.Entries == nil {
		tour.Entries = []model.TourEntry{}
	}
	WriteJSON(w, http.StatusOK, tour)
}

func (h *Handlers) PutTour(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "tour update is not implemented in this scaffold", nil)
}

func (h *Handlers) DisplayState(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"display_running":    false,
		"paused":             true,
		"pinned":             false,
		"active_screen":      nil,
		"next_screen":        nil,
		"dwell_remaining_ms": 0,
		"fps":                0,
		"tiles":              []any{},
	})
}

func (h *Handlers) DisplayNext(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "display engine is not running", nil)
}

func (h *Handlers) DisplayPrevious(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "display engine is not running", nil)
}

func (h *Handlers) DisplayGoto(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "screenId")
	WriteError(w, http.StatusNotImplemented, "not_implemented", "display engine is not running", nil)
}

func (h *Handlers) DisplayPause(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "display engine is not running", nil)
}

func (h *Handlers) DisplayResume(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented", "display engine is not running", nil)
}

func (h *Handlers) PreviewStream(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusServiceUnavailable, "engine_not_running", "display engine is not running", nil)
}

func (h *Handlers) PreviewFrame(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusServiceUnavailable, "engine_not_running", "display engine is not running", nil)
}

func (h *Handlers) Events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "sse_unsupported", "streaming unsupported", nil)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"type":            "display.state",
		"display_running": false,
		"paused":          true,
		"tiles":           []any{},
	})
	_, _ = w.Write([]byte("event: display.state\ndata: " + string(payload) + "\n\n"))
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		}
	}
}
