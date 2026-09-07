package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/display/strategy"
	"github.com/dchote/livestream-viewer/internal/display/transition"
	"github.com/dchote/livestream-viewer/internal/model"
	"gorm.io/gorm"
)

type screenRequest struct {
	Name       string             `json:"name"`
	Kind       string             `json:"kind"`
	Layout     string             `json:"layout"`
	Loop       *bool              `json:"loop"`
	Transition *transition.Spec   `json:"transition"`
	Tiles      []model.ScreenTile `json:"tiles"`
	Items      []model.ScreenItem `json:"items"`
}

func screenQuery(db *gorm.DB) *gorm.DB {
	return model.PreloadScreenAssociations(db)
}

func (h *Handlers) ListScreens(w http.ResponseWriter, r *http.Request) {
	var items []model.Screen
	if err := screenQuery(h.DB).Order("id asc").Find(&items).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to list screens", nil)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"screens": emptyIfNil(items)})
}

func (h *Handlers) CreateScreen(w http.ResponseWriter, r *http.Request) {
	var req screenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	s := model.Screen{
		Name:   req.Name,
		Kind:   req.Kind,
		Layout: req.Layout,
		Tiles:  req.Tiles,
		Items:  req.Items,
	}
	if req.Loop != nil {
		s.Loop = *req.Loop
	}
	if req.Transition != nil {
		s.Transition = *req.Transition
	} else if s.Kind == model.ScreenKindTransition {
		s.Transition = transition.DefaultCut()
	}
	if s.Kind == model.ScreenKindGrid {
		if s.Layout == "" {
			s.Layout = layout.FullBleedID
		}
		l, ok := layout.ByID(s.Layout)
		if !ok {
			WriteError(w, http.StatusBadRequest, "bad_request", "unknown layout", nil)
			return
		}
		s.Tiles = strategy.PadGridTiles(s.Tiles, l.Cells)
	}
	sources, err := strategy.LoadSourcesMap(h.DB)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load sources", nil)
		return
	}
	if err := strategy.ValidateScreen(&s, sources); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	if err := h.DB.Session(&gorm.Session{FullSaveAssociations: true}).Create(&s).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to create screen", nil)
		return
	}
	if err := h.seedTourIfEmpty(s.ID); err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to seed tour", nil)
		return
	}
	if err := screenQuery(h.DB).First(&s, s.ID).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to reload screen", nil)
		return
	}
	h.reloadStrategy()
	WriteJSON(w, http.StatusCreated, s)
}

func (h *Handlers) GetScreen(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid screen id", nil)
		return
	}
	var s model.Screen
	if err := screenQuery(h.DB).First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "screen not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load screen", nil)
		return
	}
	WriteJSON(w, http.StatusOK, s)
}

func (h *Handlers) PatchScreen(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid screen id", nil)
		return
	}
	var existing model.Screen
	if err := screenQuery(h.DB).First(&existing, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "screen not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load screen", nil)
		return
	}
	var req screenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Kind != "" {
		existing.Kind = req.Kind
	}
	if req.Loop != nil {
		existing.Loop = *req.Loop
	}
	if req.Transition != nil {
		existing.Transition = *req.Transition
	}
	layoutChanged := req.Layout != "" && req.Layout != existing.Layout
	if req.Layout != "" {
		existing.Layout = req.Layout
	}
	if req.Tiles != nil {
		existing.Tiles = req.Tiles
	} else if layoutChanged && existing.Kind == model.ScreenKindGrid {
		l, ok := layout.ByID(existing.Layout)
		if !ok {
			WriteError(w, http.StatusBadRequest, "bad_request", "unknown layout", nil)
			return
		}
		existing.Tiles = strategy.RemapTiles(existing.Tiles, l)
	}
	if req.Items != nil {
		existing.Items = req.Items
	}
	if existing.Kind == model.ScreenKindGrid {
		l, ok := layout.ByID(existing.Layout)
		if !ok {
			WriteError(w, http.StatusBadRequest, "bad_request", "unknown layout", nil)
			return
		}
		if req.Tiles == nil && !layoutChanged {
			existing.Tiles = strategy.PadGridTiles(existing.Tiles, l.Cells)
		} else if req.Tiles != nil {
			existing.Tiles = strategy.PadGridTiles(existing.Tiles, l.Cells)
		}
	}
	sources, err := strategy.LoadSourcesMap(h.DB)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load sources", nil)
		return
	}
	if err := strategy.ValidateScreen(&existing, sources); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	if err := h.replaceScreen(existing); err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to update screen", nil)
		return
	}
	var out model.Screen
	if err := screenQuery(h.DB).First(&out, id).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to reload screen", nil)
		return
	}
	h.reloadStrategy()
	WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) DeleteScreen(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid screen id", nil)
		return
	}
	var s model.Screen
	if err := h.DB.First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "screen not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load screen", nil)
		return
	}
	entries, err := strategy.TourEntriesForScreen(h.DB, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to check tour", nil)
		return
	}
	if len(entries) > 0 {
		WriteError(w, http.StatusConflict, "in_use", "screen is referenced by the tour", map[string]any{"tour_entries": entries})
		return
	}
	if err := h.replaceScreen(model.Screen{ID: id}); err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to delete screen associations", nil)
		return
	}
	if err := h.DB.Delete(&model.Screen{}, id).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to delete screen", nil)
		return
	}
	h.reloadStrategy()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) replaceScreen(s model.Screen) error {
	return h.DB.Transaction(func(tx *gorm.DB) error {
		var tiles []model.ScreenTile
		if err := tx.Where("screen_id = ?", s.ID).Find(&tiles).Error; err != nil {
			return err
		}
		ids := make([]uint, 0, len(tiles))
		for _, t := range tiles {
			ids = append(ids, t.ID)
		}
		if len(ids) > 0 {
			if err := tx.Where("tile_id IN ?", ids).Delete(&model.TileSequence{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("screen_id = ?", s.ID).Delete(&model.ScreenTile{}).Error; err != nil {
			return err
		}
		if err := tx.Where("screen_id = ?", s.ID).Delete(&model.ScreenItem{}).Error; err != nil {
			return err
		}
		if s.Name == "" {
			return nil
		}
		for i := range s.Tiles {
			s.Tiles[i].ID = 0
			s.Tiles[i].ScreenID = s.ID
			for j := range s.Tiles[i].Sequence {
				s.Tiles[i].Sequence[j].ID = 0
				s.Tiles[i].Sequence[j].TileID = 0
			}
		}
		for i := range s.Items {
			s.Items[i].ID = 0
			s.Items[i].ScreenID = s.ID
		}
		return tx.Session(&gorm.Session{FullSaveAssociations: true}).Save(&s).Error
	})
}

func (h *Handlers) seedTourIfEmpty(screenID uint) error {
	var tour model.Tour
	if err := h.DB.Preload("Entries").First(&tour).Error; err != nil {
		return err
	}
	if len(tour.Entries) > 0 {
		return nil
	}
	var cfg model.RuntimeConfig
	if err := h.DB.First(&cfg).Error; err != nil {
		return err
	}
	dwell := cfg.DefaultDwellMS
	if dwell < model.MinDwellMS {
		dwell = 30_000
	}
	entry := model.TourEntry{
		TourID:     tour.ID,
		Position:   0,
		ScreenID:   screenID,
		DwellMS:    dwell,
		Transition: cfg.DefaultTransition,
	}
	if entry.Transition.Type == "" {
		entry.Transition = transition.DefaultCut()
	}
	return h.DB.Create(&entry).Error
}
