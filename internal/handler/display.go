package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func (h *Handlers) DisplayState(w http.ResponseWriter, r *http.Request) {
	if h.Runtime == nil {
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
		return
	}
	WriteJSON(w, http.StatusOK, h.Runtime.State())
}

func (h *Handlers) DisplayNext(w http.ResponseWriter, r *http.Request) {
	if h.Runtime != nil {
		h.Runtime.Next()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) DisplayPrevious(w http.ResponseWriter, r *http.Request) {
	if h.Runtime != nil {
		h.Runtime.Previous()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) DisplayGoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "screenId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid screen id", nil)
		return
	}
	if h.Runtime == nil || !h.Runtime.Goto(uint(id)) {
		WriteError(w, http.StatusNotFound, "not_found", "screen not found", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) DisplayPause(w http.ResponseWriter, r *http.Request) {
	if h.Runtime != nil {
		h.Runtime.Pause()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) DisplayResume(w http.ResponseWriter, r *http.Request) {
	if h.Runtime != nil {
		h.Runtime.Resume()
	}
	w.WriteHeader(http.StatusNoContent)
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

	// Subscribe before writing the opening snapshot. The other order drops any
	// state change published in between, leaving the client stale until the
	// next event.
	var chunks <-chan []byte
	unsub := func() {}
	if h.Hub != nil {
		chunks, unsub = h.Hub.Subscribe()
	}
	defer unsub()

	if h.Runtime != nil {
		payload, _ := json.Marshal(h.Runtime.State())
		_, _ = w.Write([]byte("event: display.state\ndata: " + string(payload) + "\n\n"))
		flusher.Flush()
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case chunk, ok := <-chunks:
			if !ok {
				return
			}
			if _, err := w.Write(chunk); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
