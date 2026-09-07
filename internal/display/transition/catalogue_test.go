package transition

import "testing"

func TestCatalogueTypes(t *testing.T) {
	want := []string{
		"cut", "fade", "barWipe", "boxWipe", "barnDoorWipe",
		"pushWipe", "slideWipe", "irisWipe", "ellipseWipe", "clockWipe",
	}
	got := Catalogue()
	if len(got) != len(want) {
		t.Fatalf("len = %d want %d", len(got), len(want))
	}
	seen := map[string]bool{}
	for _, item := range got {
		seen[item.Type] = true
	}
	for _, id := range want {
		if !seen[id] {
			t.Errorf("missing %s", id)
		}
	}
}

func TestValidSubtype(t *testing.T) {
	if !ValidSubtype("cut", "") {
		t.Error("cut should allow empty subtype")
	}
	if ValidSubtype("cut", "crossfade") {
		t.Error("cut should reject subtypes")
	}
	if !ValidSubtype("fade", "crossfade") {
		t.Error("fade/crossfade")
	}
	if !ValidSubtype("pushWipe", "fromLeft") {
		t.Error("pushWipe/fromLeft")
	}
	if ValidSubtype("slideWipe", "leftToRight") {
		t.Error("slideWipe should not use barWipe subtypes")
	}
}

func TestEasingPresets(t *testing.T) {
	if len(EasingPresets()) != 5 {
		t.Fatalf("want 5 presets, got %d", len(EasingPresets()))
	}
}
