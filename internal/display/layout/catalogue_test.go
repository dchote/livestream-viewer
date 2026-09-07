package layout

import "testing"

func TestCatalogueIDs(t *testing.T) {
	want := []string{
		"full",
		"2x1", "1x2", "2x2", "3x3", "4x4",
		"1+3", "1+5", "1+7", "1+12",
		"3v", "1v+6",
		"2p", "1p+6",
	}
	all := All()
	if len(all) != len(want) {
		t.Fatalf("catalogue len = %d, want %d", len(all), len(want))
	}
	seen := map[string]bool{}
	for _, l := range all {
		if l.Cells != len(l.Rects) {
			t.Errorf("%s cells=%d rects=%d", l.ID, l.Cells, len(l.Rects))
		}
		if seen[l.ID] {
			t.Errorf("duplicate id %s", l.ID)
		}
		seen[l.ID] = true
		for _, r := range l.Rects {
			if r.W <= 0 || r.H <= 0 {
				t.Errorf("%s has non-positive rect %+v", l.ID, r)
			}
			if r.X < 0 || r.Y < 0 || r.X+r.W > 1.001 || r.Y+r.H > 1.001 {
				t.Errorf("%s rect out of unit square %+v", l.ID, r)
			}
		}
	}
	for _, id := range want {
		if !seen[id] {
			t.Errorf("missing layout %s", id)
		}
	}
}

func TestHotspotIsCellZero(t *testing.T) {
	l, ok := ByID("1+5")
	if !ok {
		t.Fatal("1+5 missing")
	}
	hot := l.Rects[0]
	for i, r := range l.Rects[1:] {
		if r.W*r.H >= hot.W*hot.H {
			t.Errorf("cell %d area >= hotspot", i+1)
		}
	}
}

func TestSolveRoundsToPixels(t *testing.T) {
	l, _ := ByID("2x2")
	cells := Solve(l, 1920, 1080, 4)
	if len(cells) != 4 {
		t.Fatalf("got %d cells", len(cells))
	}
	for i, c := range cells {
		if c.W <= 0 || c.H <= 0 {
			t.Errorf("cell %d empty: %+v", i, c)
		}
	}
}

func TestByIDUnknown(t *testing.T) {
	if _, ok := ByID("nope"); ok {
		t.Fatal("expected miss")
	}
}

// "1x1" was retired in favour of the full-bleed layout, which has identical
// geometry. Databases are migrated by database.migrateRetiredLayouts.
func TestRetired1x1IsGone(t *testing.T) {
	if _, ok := ByID("1x1"); ok {
		t.Fatal("1x1 should be retired in favour of the full-bleed layout")
	}
	full, ok := ByID(FullBleedID)
	if !ok {
		t.Fatal("full-bleed layout missing")
	}
	if full.Cells != 1 {
		t.Fatalf("full-bleed cells = %d, want 1", full.Cells)
	}
	for _, l := range All() {
		if l.ID != FullBleedID && l.Cells == 1 {
			t.Errorf("%s is a second single-cell layout; full is the only one", l.ID)
		}
	}
}
