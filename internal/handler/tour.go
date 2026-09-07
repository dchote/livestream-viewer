package handler

import (
	"encoding/json"
	"net/http"

	"github.com/dchote/livestream-viewer/internal/display/strategy"
	"github.com/dchote/livestream-viewer/internal/model"
	"gorm.io/gorm"
)

type tourRequest struct {
	Enabled *bool              `json:"enabled"`
	Loop    *bool              `json:"loop"`
	Entries *[]model.TourEntry `json:"entries"`
}

func (h *Handlers) GetTour(w http.ResponseWriter, r *http.Request) {
	var tour model.Tour
	if err := model.PreloadTourEntries(h.DB).First(&tour).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load tour", nil)
		return
	}
	tour.Entries = emptyIfNil(tour.Entries)
	WriteJSON(w, http.StatusOK, tour)
}

func (h *Handlers) PutTour(w http.ResponseWriter, r *http.Request) {
	var req tourRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	// PUT is a full replace of the tour document; entries must be present
	// (empty array is allowed when the tour is disabled).
	if req.Entries == nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "entries is required", nil)
		return
	}
	var tour model.Tour
	if err := h.DB.First(&tour).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load tour", nil)
		return
	}
	if req.Enabled != nil {
		tour.Enabled = *req.Enabled
	}
	if req.Loop != nil {
		tour.Loop = *req.Loop
	}
	tour.Entries = *req.Entries
	screens, err := strategy.LoadScreensMap(h.DB)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load screens", nil)
		return
	}
	if err := strategy.ValidateTour(&tour, screens); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tour_id = ?", tour.ID).Delete(&model.TourEntry{}).Error; err != nil {
			return err
		}
		for i := range tour.Entries {
			tour.Entries[i].ID = 0
			tour.Entries[i].TourID = tour.ID
			tour.Entries[i].Position = i
		}
		return tx.Session(&gorm.Session{FullSaveAssociations: true}).Save(&tour).Error
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to save tour", nil)
		return
	}
	if err := model.PreloadTourEntries(h.DB).First(&tour).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to reload tour", nil)
		return
	}
	h.reloadStrategy()
	WriteJSON(w, http.StatusOK, tour)
}
