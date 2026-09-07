package strategy

import (
	"fmt"
	"sort"

	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/display/transition"
	"github.com/dchote/livestream-viewer/internal/model"
	"gorm.io/gorm"
)

// ValidateScreen checks a screen against the layout catalogue and known sources.
func ValidateScreen(s *model.Screen, sources map[uint]model.Source) error {
	if s == nil {
		return fmt.Errorf("screen is required")
	}
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if !model.ValidScreenKind(s.Kind) {
		return fmt.Errorf("invalid screen kind %q", s.Kind)
	}
	switch s.Kind {
	case model.ScreenKindGrid:
		return validateGrid(s, sources)
	case model.ScreenKindTransition:
		return validateTransitionScreen(s, sources)
	}
	return nil
}

func validateGrid(s *model.Screen, sources map[uint]model.Source) error {
	l, ok := layout.ByID(s.Layout)
	if !ok {
		return fmt.Errorf("unknown layout %q", s.Layout)
	}
	seen := map[int]bool{}
	for i := range s.Tiles {
		t := &s.Tiles[i]
		if t.CellIndex < 0 || t.CellIndex >= l.Cells {
			return fmt.Errorf("tile index %d is outside layout %s (%d cells)", t.CellIndex, l.ID, l.Cells)
		}
		if seen[t.CellIndex] {
			return fmt.Errorf("duplicate tile index %d", t.CellIndex)
		}
		seen[t.CellIndex] = true
		if t.Fit == "" {
			t.Fit = model.FitContain
		}
		if !model.ValidFit(t.Fit) {
			return fmt.Errorf("invalid fit %q", t.Fit)
		}
		if len(t.Sequence) > 0 {
			t.SourceID = nil
			sort.Slice(t.Sequence, func(i, j int) bool { return t.Sequence[i].Position < t.Sequence[j].Position })
			for j := range t.Sequence {
				seq := &t.Sequence[j]
				seq.Position = j
				if err := requireSource(seq.SourceID, sources); err != nil {
					return err
				}
				if seq.DwellMS < model.MinDwellMS {
					return fmt.Errorf("sequence dwell_ms must be at least %d", model.MinDwellMS)
				}
			}
			continue
		}
		if t.SourceID != nil {
			if err := requireSource(*t.SourceID, sources); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTransitionScreen(s *model.Screen, sources map[uint]model.Source) error {
	if len(s.Items) < 1 {
		return fmt.Errorf("transition screen needs at least one playlist item")
	}
	if err := transition.Validate(s.Transition); err != nil {
		return err
	}
	sort.Slice(s.Items, func(i, j int) bool { return s.Items[i].Position < s.Items[j].Position })
	for i := range s.Items {
		it := &s.Items[i]
		it.Position = i
		if err := requireSource(it.SourceID, sources); err != nil {
			return err
		}
		if it.DwellMS < model.MinDwellMS {
			return fmt.Errorf("playlist dwell_ms must be at least %d", model.MinDwellMS)
		}
		if it.Fit == "" {
			it.Fit = model.FitContain
		}
		if !model.ValidFit(it.Fit) {
			return fmt.Errorf("invalid fit %q", it.Fit)
		}
	}
	return nil
}

func requireSource(id uint, sources map[uint]model.Source) error {
	if _, ok := sources[id]; !ok {
		return fmt.Errorf("source %d does not exist", id)
	}
	return nil
}

// ValidateTour checks tour entries and screens.
func ValidateTour(t *model.Tour, screens map[uint]model.Screen) error {
	if t == nil {
		return fmt.Errorf("tour is required")
	}
	if t.Enabled && len(t.Entries) < 1 {
		return fmt.Errorf("an enabled tour needs at least one entry")
	}
	sort.Slice(t.Entries, func(i, j int) bool { return t.Entries[i].Position < t.Entries[j].Position })
	for i := range t.Entries {
		e := &t.Entries[i]
		e.Position = i
		if _, ok := screens[e.ScreenID]; !ok {
			return fmt.Errorf("screen %d does not exist", e.ScreenID)
		}
		if e.DwellMS < model.MinDwellMS {
			return fmt.Errorf("tour dwell_ms must be at least %d", model.MinDwellMS)
		}
		if e.Transition.Type == "" {
			e.Transition = transition.DefaultCut()
		}
		if err := transition.Validate(e.Transition); err != nil {
			return err
		}
	}
	return nil
}

// RemapTiles keeps overlapping cell assignments when the layout changes.
// Cell 0 (hotspot) is always preserved when present in both layouts.
func RemapTiles(old []model.ScreenTile, newLayout layout.Layout) []model.ScreenTile {
	byIndex := map[int]model.ScreenTile{}
	for _, t := range old {
		byIndex[t.CellIndex] = t
	}
	out := make([]model.ScreenTile, 0, newLayout.Cells)
	for i := 0; i < newLayout.Cells; i++ {
		if t, ok := byIndex[i]; ok {
			t.ID = 0
			t.ScreenID = 0
			t.CellIndex = i
			if t.Fit == "" {
				t.Fit = model.FitContain
			}
			seq := make([]model.TileSequence, len(t.Sequence))
			copy(seq, t.Sequence)
			for j := range seq {
				seq[j].ID = 0
				seq[j].TileID = 0
			}
			t.Sequence = seq
			out = append(out, t)
			continue
		}
		out = append(out, model.ScreenTile{
			CellIndex: i,
			Fit:       model.FitContain,
		})
	}
	return out
}

// PadGridTiles ensures a tile exists for every cell.
func PadGridTiles(tiles []model.ScreenTile, cells int) []model.ScreenTile {
	return RemapTiles(tiles, layout.Layout{Cells: cells})
}

// LoadSourcesMap loads all sources keyed by ID.
func LoadSourcesMap(db *gorm.DB) (map[uint]model.Source, error) {
	var items []model.Source
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]model.Source, len(items))
	for _, s := range items {
		out[s.ID] = s
	}
	return out, nil
}

// LoadScreensMap loads screens keyed by ID (no associations).
func LoadScreensMap(db *gorm.DB) (map[uint]model.Screen, error) {
	var items []model.Screen
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]model.Screen, len(items))
	for _, s := range items {
		out[s.ID] = s
	}
	return out, nil
}

// TourEntriesForScreen returns tour entries that reference screenID.
func TourEntriesForScreen(db *gorm.DB, screenID uint) ([]model.TourEntry, error) {
	var entries []model.TourEntry
	if err := db.Where("screen_id = ?", screenID).Order("position asc").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}
