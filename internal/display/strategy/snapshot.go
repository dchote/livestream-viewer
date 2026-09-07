package strategy

import (
	"fmt"
	"sync/atomic"

	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/display/transition"
	"github.com/dchote/livestream-viewer/internal/model"
	"gorm.io/gorm"
)

var revCounter atomic.Int64

// Snapshot is an immutable, fully-resolved display strategy.
type Snapshot struct {
	Rev          int64
	OutputWidth  int
	OutputHeight int
	GutterPx     int
	Tour         TourSnap
	Screens      map[uint]*ScreenSnap
	Sources      map[uint]*SourceRef
}

// TourSnap is the resolved tour.
type TourSnap struct {
	Enabled bool
	Loop    bool
	Entries []TourEntrySnap
}

// TourEntrySnap is one tour step.
type TourEntrySnap struct {
	ScreenID   uint
	DwellMS    int
	Transition transition.Spec
}

// ScreenSnap is a resolved screen with layout geometry.
type ScreenSnap struct {
	ID         uint
	Name       string
	Kind       string
	Layout     string
	Rects      []layout.Rect
	Loop       bool
	Transition transition.Spec
	Tiles      []TileSnap
	Items      []PlaylistItemSnap
}

// TileSnap is one grid cell.
type TileSnap struct {
	Index    int
	SourceID *uint
	Fit      string
	Sequence []SequenceItemSnap
}

// SequenceItemSnap is one source in a tile sequence.
type SequenceItemSnap struct {
	SourceID uint
	DwellMS  int
}

// PlaylistItemSnap is one transition-screen playlist item.
type PlaylistItemSnap struct {
	SourceID uint
	DwellMS  int
	Fit      string
}

// SourceRef is the scheduler's view of a source.
type SourceRef struct {
	ID      uint
	Name    string
	Kind    string
	Enabled bool
}

// Build loads the current configuration into an immutable snapshot.
func Build(db *gorm.DB) (*Snapshot, error) {
	var cfg model.RuntimeConfig
	if err := db.First(&cfg).Error; err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	var tour model.Tour
	if err := model.PreloadTourEntries(db).First(&tour).Error; err != nil {
		return nil, fmt.Errorf("load tour: %w", err)
	}
	var screens []model.Screen
	if err := model.PreloadScreenAssociations(db).Find(&screens).Error; err != nil {
		return nil, fmt.Errorf("load screens: %w", err)
	}
	var sources []model.Source
	if err := db.Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("load sources: %w", err)
	}

	snap := &Snapshot{
		Rev:          revCounter.Add(1),
		OutputWidth:  cfg.OutputWidth,
		OutputHeight: cfg.OutputHeight,
		GutterPx:     cfg.GutterPx,
		Tour: TourSnap{
			Enabled: tour.Enabled,
			Loop:    tour.Loop,
			Entries: make([]TourEntrySnap, 0, len(tour.Entries)),
		},
		Screens: make(map[uint]*ScreenSnap, len(screens)),
		Sources: make(map[uint]*SourceRef, len(sources)),
	}
	for _, e := range tour.Entries {
		tr := e.Transition
		if tr.Type == "" {
			tr = transition.DefaultCut()
		}
		snap.Tour.Entries = append(snap.Tour.Entries, TourEntrySnap{
			ScreenID:   e.ScreenID,
			DwellMS:    e.DwellMS,
			Transition: tr,
		})
	}
	for i := range sources {
		s := sources[i]
		snap.Sources[s.ID] = &SourceRef{ID: s.ID, Name: s.Name, Kind: s.Kind, Enabled: s.Enabled}
	}
	for i := range screens {
		sc := screens[i]
		ss := &ScreenSnap{
			ID:         sc.ID,
			Name:       sc.Name,
			Kind:       sc.Kind,
			Layout:     sc.Layout,
			Loop:       sc.Loop,
			Transition: sc.Transition,
			Tiles:      make([]TileSnap, 0, len(sc.Tiles)),
			Items:      make([]PlaylistItemSnap, 0, len(sc.Items)),
		}
		if l, ok := layout.ByID(sc.Layout); ok {
			ss.Rects = append([]layout.Rect(nil), l.Rects...)
		} else if sc.Kind == model.ScreenKindTransition {
			if l, ok := layout.ByID(layout.FullBleedID); ok {
				ss.Layout = l.ID
				ss.Rects = append([]layout.Rect(nil), l.Rects...)
			}
		}
		for _, t := range sc.Tiles {
			ts := TileSnap{Index: t.CellIndex, SourceID: t.SourceID, Fit: t.Fit}
			for _, seq := range t.Sequence {
				ts.Sequence = append(ts.Sequence, SequenceItemSnap{SourceID: seq.SourceID, DwellMS: seq.DwellMS})
			}
			ss.Tiles = append(ss.Tiles, ts)
		}
		for _, it := range sc.Items {
			ss.Items = append(ss.Items, PlaylistItemSnap{SourceID: it.SourceID, DwellMS: it.DwellMS, Fit: it.Fit})
		}
		snap.Screens[sc.ID] = ss
	}
	return snap, nil
}
