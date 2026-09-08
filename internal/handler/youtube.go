package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/source/youtube"
	"github.com/dchote/livestream-viewer/internal/source/youtube/pot"
)

const cookieMultipartMemory = 32 << 10

func (h *Handlers) youtubeStatus() youtube.Status {
	st := youtube.Status{POT: pot.Status{Mode: pot.ModeOff}}
	if h.Cfg != nil {
		st = youtube.LoadStatus(h.Cfg.DataDir)
	}
	if h.POT != nil {
		st.POT = h.POT.Status()
	} else if st.POT.Mode == "" {
		st.POT.Mode = pot.ModeOff
	}
	if h.Runtime != nil {
		st.LastErrorCode, st.LastError = h.Runtime.YouTubeAuthIssue()
	}
	return st
}

func (h *Handlers) GetYouTube(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, h.youtubeStatus())
}

func (h *Handlers) PutYouTubeCookies(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, youtube.MaxCookiesBytes+1024)
	if err := r.ParseMultipartForm(cookieMultipartMemory); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			WriteError(w, http.StatusRequestEntityTooLarge, "too_large", "cookie file exceeds 256 KiB", nil)
			return
		}
		WriteError(w, http.StatusBadRequest, "bad_request", "upload is not multipart", nil)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "file field is required", nil)
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, youtube.MaxCookiesBytes+1))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "failed to read cookie file", nil)
		return
	}
	if _, err := youtube.WriteCookies(h.Cfg.DataDir, raw); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	h.reloadYouTubeWorkers()
	WriteJSON(w, http.StatusOK, h.youtubeStatus())
}

func (h *Handlers) DeleteYouTubeCookies(w http.ResponseWriter, r *http.Request) {
	if err := youtube.DeleteCookies(h.Cfg.DataDir); err != nil {
		WriteError(w, http.StatusInternalServerError, "io_error", "failed to remove cookies", nil)
		return
	}
	h.reloadYouTubeWorkers()
	WriteJSON(w, http.StatusOK, h.youtubeStatus())
}

func (h *Handlers) PutYouTubePOToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		POToken string `json:"po_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	if err := youtube.WritePOToken(h.Cfg.DataDir, req.POToken); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	h.reloadYouTubeWorkers()
	WriteJSON(w, http.StatusOK, h.youtubeStatus())
}

func (h *Handlers) reloadYouTubeWorkers() {
	if h.Manager == nil {
		return
	}
	h.Manager.ReloadYouTube()
	if h.Runtime != nil {
		h.Manager.Resync(h.Runtime.ComposeView().Needed)
	}
}

// SetPOT attaches the PO token provider and restarts YouTube workers when it becomes ready.
func (h *Handlers) SetPOT(p *pot.Provider) {
	h.POT = p
	if p != nil {
		p.OnReady(h.reloadYouTubeWorkers)
	}
}

func (h *Handlers) attachIngest(s *model.Source) {
	if h.Runtime == nil || s == nil {
		return
	}
	s.Decoder, s.ErrorCode, s.Error = h.Runtime.SourceHealth(s.ID)
}

func (h *Handlers) attachIngestAll(items []model.Source) {
	for i := range items {
		h.attachIngest(&items[i])
	}
}
