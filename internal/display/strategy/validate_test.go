package strategy

import (
	"testing"

	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/display/transition"
	"github.com/dchote/livestream-viewer/internal/model"
)

func TestValidateGrid(t *testing.T) {
	t.Parallel()
	srcID := uint(1)
	sources := map[uint]model.Source{1: {ID: 1, Name: "cam"}}
	s := &model.Screen{
		Name:   "Wall",
		Kind:   model.ScreenKindGrid,
		Layout: "2x2",
		Tiles: []model.ScreenTile{
			{CellIndex: 0, SourceID: &srcID, Fit: model.FitContain},
			{CellIndex: 1},
		},
	}
	if err := ValidateScreen(s, sources); err != nil {
		t.Fatal(err)
	}

	s.Tiles[0].CellIndex = 9
	if err := ValidateScreen(s, sources); err == nil {
		t.Fatal("expected out of range")
	}
}

func TestValidateTransitionScreen(t *testing.T) {
	t.Parallel()
	sources := map[uint]model.Source{1: {ID: 1}}
	s := &model.Screen{
		Name:       "Feat",
		Kind:       model.ScreenKindTransition,
		Transition: transition.DefaultCut(),
		Items:      []model.ScreenItem{{SourceID: 1, DwellMS: 5000, Fit: model.FitCover}},
	}
	if err := ValidateScreen(s, sources); err != nil {
		t.Fatal(err)
	}
	s.Items = nil
	if err := ValidateScreen(s, sources); err == nil {
		t.Fatal("need items")
	}
}

func TestValidateTour(t *testing.T) {
	t.Parallel()
	screens := map[uint]model.Screen{3: {ID: 3, Name: "A"}}
	tour := &model.Tour{Enabled: true, Entries: []model.TourEntry{
		{ScreenID: 3, DwellMS: 5000, Transition: transition.DefaultCut()},
	}}
	if err := ValidateTour(tour, screens); err != nil {
		t.Fatal(err)
	}
	tour.Entries = nil
	if err := ValidateTour(tour, screens); err == nil {
		t.Fatal("enabled needs entries")
	}
}

func TestRemapTilesPreservesHotspot(t *testing.T) {
	t.Parallel()
	src0 := uint(10)
	src1 := uint(11)
	old := []model.ScreenTile{
		{CellIndex: 0, SourceID: &src0, Fit: model.FitCover},
		{CellIndex: 1, SourceID: &src1},
	}
	l, ok := layout.ByID("1+7")
	if !ok {
		t.Fatal("missing layout")
	}
	out := RemapTiles(old, l)
	if len(out) != l.Cells {
		t.Fatalf("len %d want %d", len(out), l.Cells)
	}
	if out[0].SourceID == nil || *out[0].SourceID != 10 {
		t.Fatal("hotspot not preserved")
	}
	if out[1].SourceID == nil || *out[1].SourceID != 11 {
		t.Fatal("index 1 not preserved")
	}
	if out[0].Fit != model.FitCover {
		t.Fatal("fit not preserved")
	}
}

func TestSnapshotGeometryMatchesCatalogue(t *testing.T) {
	t.Parallel()
	l, _ := layout.ByID("1+5")
	snap := &Snapshot{Screens: map[uint]*ScreenSnap{
		1: {ID: 1, Layout: l.ID, Rects: append([]layout.Rect(nil), l.Rects...)},
	}}
	if len(snap.Screens[1].Rects) != l.Cells {
		t.Fatalf("rects %d cells %d", len(snap.Screens[1].Rects), l.Cells)
	}
	for i, r := range snap.Screens[1].Rects {
		if r != l.Rects[i] {
			t.Fatalf("rect %d mismatch", i)
		}
	}
}
