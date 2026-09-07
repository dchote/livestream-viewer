package transition

import "testing"

func TestValidate(t *testing.T) {
	t.Parallel()
	if err := Validate(DefaultCut()); err != nil {
		t.Fatal(err)
	}
	if err := Validate(Spec{Type: "cut", DurationMS: 10}); err == nil {
		t.Fatal("cut duration")
	}
	if err := Validate(Spec{Type: "fade", Subtype: "crossfade", DurationMS: 400}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(Spec{Type: "fade", Subtype: "crossfade", DurationMS: 0}); err == nil {
		t.Fatal("fade duration")
	}
	if err := Validate(Spec{Type: "fade", Subtype: "nope", DurationMS: 400}); err == nil {
		t.Fatal("bad subtype")
	}
	color := "#000000"
	if err := Validate(Spec{Type: "fade", Subtype: "fadeToColor", DurationMS: 400}); err == nil {
		t.Fatal("color required")
	}
	if err := Validate(Spec{Type: "fade", Subtype: "fadeToColor", DurationMS: 400, Color: &color}); err != nil {
		t.Fatal(err)
	}
}

func TestParseEasing(t *testing.T) {
	t.Parallel()
	pts, err := ParseEasing("ease-in-out")
	if err != nil {
		t.Fatal(err)
	}
	if pts != [4]float64{0.42, 0, 0.58, 1} {
		t.Fatalf("pts %+v", pts)
	}
	pts, err = ParseEasing("cubic-bezier(0.4, 0.0, 0.2, 1.0)")
	if err != nil {
		t.Fatal(err)
	}
	if pts[0] != 0.4 || pts[2] != 0.2 {
		t.Fatalf("bezier %+v", pts)
	}
	if _, err := ParseEasing("not-a-curve"); err == nil {
		t.Fatal("expected error")
	}
}
