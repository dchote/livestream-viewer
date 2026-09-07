package schedule

import (
	"testing"
	"time"

	"github.com/dchote/livestream-viewer/internal/display/strategy"
	"github.com/dchote/livestream-viewer/internal/display/transition"
	"github.com/dchote/livestream-viewer/internal/model"
)

func TestTourDwellExcludesTransition(t *testing.T) {
	clock := NewManualClock(time.Unix(0, 0).UTC())
	rt := New(clock, false)
	s1 := uint(1)
	s2 := uint(2)
	// Incoming transition: entry 2's fade plays after entry 1's dwell, before landing on 2.
	rt.ApplyStrategy(&strategy.Snapshot{
		Tour: strategy.TourSnap{
			Enabled: true,
			Loop:    true,
			Entries: []strategy.TourEntrySnap{
				{ScreenID: s1, DwellMS: 1000, Transition: transition.DefaultCut()},
				{ScreenID: s2, DwellMS: 1000, Transition: transition.Spec{Type: "fade", Subtype: "crossfade", DurationMS: 500}},
			},
		},
		Screens: map[uint]*strategy.ScreenSnap{
			1: {ID: 1, Name: "A", Kind: model.ScreenKindGrid, Tiles: []strategy.TileSnap{{Index: 0}}},
			2: {ID: 2, Name: "B", Kind: model.ScreenKindGrid, Tiles: []strategy.TileSnap{{Index: 0}}},
		},
	})
	if id := rt.State().ActiveScreen; id == nil || *id != 1 {
		t.Fatalf("start %+v", rt.State().ActiveScreen)
	}
	clock.Add(999 * time.Millisecond)
	rt.Tick()
	if id := rt.State().ActiveScreen; id == nil || *id != 1 {
		t.Fatal("still dwelling")
	}
	clock.Add(2 * time.Millisecond)
	rt.Tick()
	if rt.State().Transition == nil {
		t.Fatal("expected incoming transition after dwell")
	}
	if rt.State().Transition.Type != "fade" {
		t.Fatalf("want incoming fade, got %+v", rt.State().Transition)
	}
	if id := rt.State().ActiveScreen; id == nil || *id != 1 {
		t.Fatal("outgoing screen still active during incoming transition")
	}
	clock.Add(500 * time.Millisecond)
	rt.Tick()
	if id := rt.State().ActiveScreen; id == nil || *id != 2 {
		t.Fatalf("want screen 2 got %+v", rt.State().ActiveScreen)
	}
}

func TestSequence(t *testing.T) {
	clock := NewManualClock(time.Unix(0, 0).UTC())
	rt := New(clock, false)
	srcA, srcB := uint(10), uint(11)
	rt.ApplyStrategy(&strategy.Snapshot{
		Tour: strategy.TourSnap{
			Enabled: false,
			Entries: []strategy.TourEntrySnap{{ScreenID: 1, DwellMS: 60_000, Transition: transition.DefaultCut()}},
		},
		Screens: map[uint]*strategy.ScreenSnap{
			1: {
				ID: 1, Name: "Grid", Kind: model.ScreenKindGrid,
				Tiles: []strategy.TileSnap{{
					Index: 0,
					Sequence: []strategy.SequenceItemSnap{
						{SourceID: srcA, DwellMS: 1000},
						{SourceID: srcB, DwellMS: 1000},
					},
				}},
			},
		},
		Sources: map[uint]*strategy.SourceRef{
			10: {ID: 10, Name: "A", Enabled: true},
			11: {ID: 11, Name: "B", Enabled: true},
		},
	})
	clock.Add(time.Millisecond)
	rt.Tick()
	if got := *rt.State().Tiles[0].SourceID; got != srcA {
		t.Fatalf("seq start %d", got)
	}
	clock.Add(1000 * time.Millisecond)
	rt.Tick()
	if got := *rt.State().Tiles[0].SourceID; got != srcB {
		t.Fatalf("seq step %d", got)
	}
}

func TestPausePinResume(t *testing.T) {
	clock := NewManualClock(time.Unix(0, 0).UTC())
	rt := New(clock, false)
	rt.ApplyStrategy(&strategy.Snapshot{
		Tour: strategy.TourSnap{
			Enabled: true,
			Loop:    true,
			Entries: []strategy.TourEntrySnap{
				{ScreenID: 1, DwellMS: 1000, Transition: transition.DefaultCut()},
				{ScreenID: 2, DwellMS: 1000, Transition: transition.DefaultCut()},
			},
		},
		Screens: map[uint]*strategy.ScreenSnap{
			1: {ID: 1, Name: "A", Kind: model.ScreenKindGrid},
			2: {ID: 2, Name: "B", Kind: model.ScreenKindGrid},
		},
	})
	rt.Pause()
	clock.Add(5 * time.Second)
	rt.Tick()
	if id := rt.State().ActiveScreen; id == nil || *id != 1 {
		t.Fatal("paused should not advance")
	}
	if !rt.Goto(2) {
		t.Fatal("goto")
	}
	if !rt.State().Pinned || !rt.State().Paused {
		t.Fatal("goto pins")
	}
	rt.Next()
	if rt.State().Pinned {
		t.Fatal("next should clear pin")
	}
	rt.Goto(2)
	rt.Resume()
	if rt.State().Pinned || rt.State().Paused {
		t.Fatal("resume")
	}
}

func TestPlaylistScreen(t *testing.T) {
	clock := NewManualClock(time.Unix(0, 0).UTC())
	rt := New(clock, false)
	a, b := uint(1), uint(2)
	rt.ApplyStrategy(&strategy.Snapshot{
		Tour: strategy.TourSnap{Entries: []strategy.TourEntrySnap{
			{ScreenID: 9, DwellMS: 60_000, Transition: transition.DefaultCut()},
		}},
		Screens: map[uint]*strategy.ScreenSnap{
			9: {
				ID: 9, Kind: model.ScreenKindTransition, Loop: true,
				Transition: transition.DefaultCut(),
				Items: []strategy.PlaylistItemSnap{
					{SourceID: a, DwellMS: 1000, Fit: model.FitContain},
					{SourceID: b, DwellMS: 1000, Fit: model.FitContain},
				},
			},
		},
		Sources: map[uint]*strategy.SourceRef{1: {ID: 1, Name: "a", Enabled: true}, 2: {ID: 2, Name: "b", Enabled: true}},
	})
	clock.Add(time.Millisecond)
	rt.Tick()
	if *rt.State().Tiles[0].SourceID != a {
		t.Fatal("playlist start")
	}
	clock.Add(1000 * time.Millisecond)
	rt.Tick()
	if *rt.State().Tiles[0].SourceID != b {
		t.Fatal("playlist step")
	}
}
