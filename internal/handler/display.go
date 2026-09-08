package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dchote/livestream-viewer/internal/schedule"
)

func (h *Handlers) DisplayState(w http.ResponseWriter, r *http.Request) {
	if h.Runtime == nil {
		// The router always supplies a runtime, so this only guards a
		// directly constructed Handlers. Returning the typed zero value keeps
		// the response shape from drifting away from the real one.
		WriteJSON(w, http.StatusOK, &schedule.State{Paused: true, Tiles: []schedule.TileState{}})
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

func (h *Handlers) engineRunning() bool {
	return h.Preview != nil && h.Runtime != nil && h.Runtime.State().DisplayRunning
}

func (h *Handlers) PreviewStream(w http.ResponseWriter, r *http.Request) {
	if !h.engineRunning() {
		WriteError(w, http.StatusServiceUnavailable, "engine_not_running", "display engine is not running", nil)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "sse_unsupported", "streaming unsupported", nil)
		return
	}
	// Each viewer costs a connection, a goroutine, and a share of the
	// encoder's output for as long as it stays connected.
	releaseSlot, ok := tryAcquireStream(&h.previewClients, maxPreviewClients)
	if !ok {
		w.Header().Set("Retry-After", "5")
		WriteError(w, http.StatusTooManyRequests, "too_many_clients", "preview viewer limit reached", nil)
		return
	}
	defer releaseSlot()

	unsub := h.Preview.Subscribe()
	defer unsub()

	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	frame := h.Preview.WaitNextCtx(r.Context(), 3*time.Second)
	for r.Context().Err() == nil && !h.Preview.Closed() {
		if frame != nil {
			_, _ = fmt.Fprintf(w, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame))
			if _, err := w.Write(frame); err != nil {
				return
			}
			_, _ = w.Write([]byte("\r\n"))
			flusher.Flush()
		}
		frame = h.Preview.WaitNextCtx(r.Context(), 2*time.Second)
	}
}

func (h *Handlers) PreviewFrame(w http.ResponseWriter, r *http.Request) {
	if !h.engineRunning() {
		WriteError(w, http.StatusServiceUnavailable, "engine_not_running", "display engine is not running", nil)
		return
	}
	unsub := h.Preview.Subscribe()
	defer unsub()
	frame := h.Preview.WaitNextCtx(r.Context(), 3*time.Second)
	if frame == nil {
		WriteError(w, http.StatusServiceUnavailable, "preview_not_ready", "preview frame not ready", nil)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(frame)
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
	releaseSlot, ok := tryAcquireStream(&h.sseClients, maxSSEClients)
	if !ok {
		w.Header().Set("Retry-After", "5")
		WriteError(w, http.StatusTooManyRequests, "too_many_clients", "event stream client limit reached", nil)
		return
	}
	defer releaseSlot()

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
