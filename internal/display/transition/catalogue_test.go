package transition

import "testing"

func TestCatalogueTypes(t *testing.T) {
	want := []string{
		"cut", "fade", "barWipe", "boxWipe", "barnDoorWipe",
		"pushWipe", "slideWipe",
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

// The catalogue is the contract the API and UI are built from: it must never
// advertise a type that has been withdrawn, and must mark what it does offer
// as available.
func TestCatalogueExcludesRetiredTypes(t *testing.T) {
	for _, info := range Catalogue() {
		if _, retired := RetiredTypes[info.Type]; retired {
			t.Errorf("%s is retired but still advertised", info.Type)
		}
		if !info.Available {
			t.Errorf("%s is advertised but not available", info.Type)
		}
		if len(info.Subtypes) > 0 && info.DefaultSub == "" {
			t.Errorf("%s has subtypes but no default", info.Type)
		}
		if info.DefaultSub != "" && !ValidSubtype(info.Type, info.DefaultSub) {
			t.Errorf("%s default subtype %q is not valid", info.Type, info.DefaultSub)
		}
	}
	for typ := range RetiredTypes {
		if ValidSubtype(typ, "") {
			t.Errorf("%s should no longer validate", typ)
		}
	}
}
