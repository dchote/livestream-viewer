package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/source"
	"gorm.io/gorm"
)

type sourceRequest struct {
	Name     string               `json:"name"`
	Kind     string               `json:"kind"`
	URL      string               `json:"url"`
	Username string               `json:"username"`
	Password *string              `json:"password"`
	Options  *model.SourceOptions `json:"options"`
	Enabled  *bool                `json:"enabled"`
}

func (h *Handlers) ListSources(w http.ResponseWriter, r *http.Request) {
	var items []model.Source
	if err := h.DB.Order("id asc").Find(&items).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to list sources", nil)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"sources": emptyIfNil(items)})
}

func (h *Handlers) CreateSource(w http.ResponseWriter, r *http.Request) {
	var req sourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	s := model.Source{
		Name:     req.Name,
		Kind:     req.Kind,
		URL:      req.URL,
		Username: req.Username,
		Enabled:  true,
	}
	if req.Options != nil {
		s.Options = *req.Options
	}
	if req.Enabled != nil {
		s.Enabled = *req.Enabled
	}
	if req.Password != nil {
		s.Password = *req.Password
	}
	if s.Kind == model.KindFile {
		if err := h.applyUploadToSource(&s); err != nil {
			WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
			return
		}
	}
	if err := source.ValidateCreate(&s); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	if err := h.DB.Create(&s).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to create source", nil)
		return
	}
	h.reloadStrategy()
	WriteJSON(w, http.StatusCreated, s)
}

func (h *Handlers) GetSource(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid source id", nil)
		return
	}
	var s model.Source
	if err := h.DB.First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "source not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load source", nil)
		return
	}
	WriteJSON(w, http.StatusOK, s)
}

func (h *Handlers) PatchSource(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid source id", nil)
		return
	}
	var s model.Source
	if err := h.DB.First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "source not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load source", nil)
		return
	}
	var req sourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	if req.Name != "" {
		s.Name = req.Name
	}
	if req.Kind != "" {
		s.Kind = req.Kind
	}
	if req.URL != "" {
		s.URL = req.URL
	}
	if req.Username != "" || req.Kind == model.KindRTSP {
		s.Username = req.Username
	}
	if req.Password != nil {
		s.Password = *req.Password
	}
	if req.Options != nil {
		s.Options = *req.Options
	}
	if req.Enabled != nil {
		s.Enabled = *req.Enabled
	}
	if s.Kind == model.KindFile {
		if err := h.applyUploadToSource(&s); err != nil {
			WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
			return
		}
	}
	if err := source.ValidateUpdate(&s); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	if err := h.DB.Save(&s).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to update source", nil)
		return
	}
	h.reloadStrategy()
	WriteJSON(w, http.StatusOK, s)
}

func (h *Handlers) DeleteSource(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid source id", nil)
		return
	}
	var s model.Source
	if err := h.DB.First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "source not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load source", nil)
		return
	}
	refs, err := source.ReferencingScreens(h.DB, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to check references", nil)
		return
	}
	if len(refs) > 0 {
		WriteError(w, http.StatusConflict, "in_use", "source is referenced by one or more screens", map[string]any{"screens": refs})
		return
	}
	if err := h.DB.Delete(&s).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to delete source", nil)
		return
	}
	_ = os.Remove(h.thumbnailPath(id))
	h.reloadStrategy()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ProbeSource(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid source id", nil)
		return
	}
	var s model.Source
	if err := h.DB.First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "source not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load source", nil)
		return
	}
	prober := source.Prober{Tools: h.Tools}
	s.Probe = prober.Probe(r.Context(), &s)
	if err := h.DB.Save(&s).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to save probe", nil)
		return
	}
	WriteJSON(w, http.StatusOK, s)
}

func (h *Handlers) SourceThumbnail(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid source id", nil)
		return
	}
	path := h.thumbnailPath(id)
	if _, err := os.Stat(path); err != nil {
		WriteError(w, http.StatusNotFound, "not_found", "thumbnail not found", nil)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, path)
}

func (h *Handlers) applyUploadToSource(s *model.Source) error {
	if s.Options.UploadID == nil {
		return nil
	}
	var up model.Upload
	if err := h.DB.First(&up, *s.Options.UploadID).Error; err != nil {
		return errors.New("upload not found")
	}
	s.URL = up.Path
	return nil
}

func (h *Handlers) thumbnailPath(id uint) string {
	return filepath.Join(h.Cfg.DataDir, "thumbnails", fmt.Sprintf("%d.jpg", id))
}
