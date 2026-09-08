package transition

import (
	"fmt"
	"math"
	"testing"
)

func TestEvalCut(t *testing.T) {
	d0 := Eval(DefaultCut(), 0, 100, 100)
	if d0.Incoming.Alpha != 0 && d0.Incoming.Dst.W != 0 {
		if d0.Outgoing.Alpha != 1 {
			t.Fatalf("t=0 %+v", d0)
		}
	}
	d1 := Eval(DefaultCut(), 1, 100, 100)
	if d1.Incoming.Alpha != 1 {
		t.Fatalf("t=1 %+v", d1)
	}
}

func TestEvalCrossfade(t *testing.T) {
	spec := Spec{Type: "fade", Subtype: "crossfade", DurationMS: 500}
	mid := Eval(spec, 0.5, 1920, 1080)
	if mid.Family != FamilyAlpha {
		t.Fatalf("family %s", mid.Family)
	}
	if mid.Incoming.Alpha < 0.4 || mid.Incoming.Alpha > 0.6 {
		t.Fatalf("alpha %v", mid.Incoming.Alpha)
	}
}

func TestEvalPushBothMove(t *testing.T) {
	spec := Spec{Type: "pushWipe", Subtype: "fromLeft", DurationMS: 400}
	mid := Eval(spec, 0.5, 200, 100)
	if mid.Outgoing.Dst.X == 0 {
		t.Fatal("push should move outgoing")
	}
	if mid.Incoming.Dst.X == 0 {
		t.Fatal("push should move incoming")
	}
}

func TestEvalSlideOutgoingStationary(t *testing.T) {
	spec := Spec{Type: "slideWipe", Subtype: "fromLeft", DurationMS: 400}
	mid := Eval(spec, 0.5, 200, 100)
	if mid.Outgoing.Dst.X != 0 || mid.Outgoing.Dst.Y != 0 {
		t.Fatalf("slide outgoing should be stationary, got %+v", mid.Outgoing.Dst)
	}
	if mid.Incoming.Dst.X == 0 {
		t.Fatal("slide incoming should move")
	}
}

func TestEvalBarClip(t *testing.T) {
	spec := Spec{Type: "barWipe", Subtype: "leftToRight", DurationMS: 400}
	mid := Eval(spec, 0.5, 200, 100)
	if mid.Incoming.Clip == nil || mid.Incoming.Clip.W != 100 {
		t.Fatalf("clip %+v", mid.Incoming.Clip)
	}
}

func TestEvalBezierLinear(t *testing.T) {
	y := EvalBezier([4]float64{0, 0, 1, 1}, 0.5)
	if y < 0.49 || y > 0.51 {
		t.Fatalf("linear 0.5 = %v", y)
	}
}

// Configurations saved before the masked wipes were withdrawn must keep
// rendering, as a crossfade, and must say so.
func TestRetiredTypesDegradeToFade(t *testing.T) {
	for typ := range RetiredTypes {
		d := Eval(Spec{Type: typ, DurationMS: 400}, 0.5, 100, 100)
		if !d.Degraded {
			t.Errorf("%s: expected Degraded", typ)
		}
		if d.Family != FamilyAlpha {
			t.Errorf("%s: family %s, want %s", typ, d.Family, FamilyAlpha)
		}
		if d.Incoming.Alpha <= 0 || d.Incoming.Alpha >= 1 {
			t.Errorf("%s: mid-transition alpha %v", typ, d.Incoming.Alpha)
		}
	}
}

// An unknown type is not the same as a retired one: it falls back to a
// crossfade but is not reported as a degraded masked wipe.
func TestUnknownTypeIsNotReportedDegraded(t *testing.T) {
	d := Eval(Spec{Type: "somethingElse", DurationMS: 400}, 0.5, 100, 100)
	if d.Degraded {
		t.Fatal("unknown type should not report a degradation")
	}
	if d.Family != FamilyAlpha {
		t.Fatalf("family %s", d.Family)
	}
}

func TestRetireRewritesSpec(t *testing.T) {
	spec, changed := Retire(Spec{Type: "clockWipe", Subtype: "clockwiseSix", DurationMS: 700})
	if !changed {
		t.Fatal("clockWipe should be retired")
	}
	if spec.Type != "fade" || spec.Subtype != "crossfade" {
		t.Fatalf("got %+v", spec)
	}
	if spec.DurationMS != 700 {
		t.Fatal("duration should be preserved")
	}
	if _, changed := Retire(Spec{Type: "pushWipe", Subtype: "fromLeft"}); changed {
		t.Fatal("pushWipe is not retired")
	}
}

// fadeToColor dips through the colour and shows both screens on the way;
// fadeFromColor starts on the colour and only ever shows the incoming.
func TestFadeColorSubtypesDiffer(t *testing.T) {
	red := "#ff0000"
	to := Spec{Type: "fade", Subtype: "fadeToColor", DurationMS: 500, Color: &red}
	from := Spec{Type: "fade", Subtype: "fadeFromColor", DurationMS: 500, Color: &red}

	toEarly := Eval(to, 0.25, 100, 100)
	if toEarly.Outgoing.Alpha <= 0 {
		t.Fatal("fadeToColor should still show the outgoing screen early on")
	}
	fromEarly := Eval(from, 0.25, 100, 100)
	if fromEarly.Outgoing.Alpha != 0 {
		t.Fatal("fadeFromColor should never show the outgoing screen")
	}
	if fromEarly.Incoming.Alpha <= 0 || fromEarly.Incoming.Alpha >= 1 {
		t.Fatalf("fadeFromColor incoming alpha %v", fromEarly.Incoming.Alpha)
	}
	for _, d := range []Draw{toEarly, fromEarly} {
		if d.Fill == nil || *d.Fill != red {
			t.Fatalf("expected the fill colour, got %+v", d.Fill)
		}
	}
	if end := Eval(from, 1, 100, 100); end.Incoming.Alpha != 1 {
		t.Fatalf("fadeFromColor should end fully on the incoming, got %v", end.Incoming.Alpha)
	}
}

// Every advertised type and subtype must reach a real implementation.
func TestEveryAdvertisedSubtypeIsImplemented(t *testing.T) {
	for _, info := range Catalogue() {
		subs := info.Subtypes
		if len(subs) == 0 {
			subs = []string{""}
		}
		for _, sub := range subs {
			spec := Spec{Type: info.Type, Subtype: sub, DurationMS: 400}
			d := Eval(spec, 0.5, 200, 100)
			if d.Degraded {
				t.Errorf("%s/%s is advertised but degrades", info.Type, sub)
			}
			if d.Family != info.Family {
				t.Errorf("%s/%s drew family %s, want %s", info.Type, sub, d.Family, info.Family)
			}
		}
	}
}

// Distinct subtypes must not render identically, or the catalogue is lying.
func TestWipeSubtypesAreDistinct(t *testing.T) {
	for _, info := range Catalogue() {
		if len(info.Subtypes) < 2 {
			continue
		}
		seen := map[string]string{}
		for _, sub := range info.Subtypes {
			d := Eval(Spec{Type: info.Type, Subtype: sub, DurationMS: 400}, 0.5, 200, 100)
			key := describe(d)
			if prev, dup := seen[key]; dup {
				t.Errorf("%s: %q and %q render identically", info.Type, prev, sub)
			}
			seen[key] = sub
		}
	}
}

// NaN fails both bounds comparisons, so without an explicit check it would
// flow through the bezier into layer alphas and blank the tile instead of
// raising anything.
func TestEvalRejectsNonFiniteProgress(t *testing.T) {
	for _, raw := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, info := range Catalogue() {
			spec := Spec{Type: info.Type, DurationMS: 400}
			if len(info.Subtypes) > 0 {
				spec.Subtype = info.Subtypes[0]
			}
			d := Eval(spec, raw, 200, 100)
			if math.IsNaN(float64(d.Incoming.Alpha)) || math.IsNaN(float64(d.Outgoing.Alpha)) {
				t.Fatalf("%s at raw=%v produced NaN alpha: %+v", info.Type, raw, d)
			}
		}
	}
}

func TestEaseClampsNonFiniteProgress(t *testing.T) {
	spec := Spec{Type: "fade", Subtype: "crossfade", Easing: "ease-in-out"}
	if got := Ease(spec, math.NaN()); got != 0 {
		t.Fatalf("Ease(NaN) = %v, want 0", got)
	}
	if got := Ease(spec, math.Inf(-1)); got != 0 {
		t.Fatalf("Ease(-Inf) = %v, want 0", got)
	}
	if got := Ease(spec, math.Inf(1)); got != 1 {
		t.Fatalf("Ease(+Inf) = %v, want 1", got)
	}
}

func describe(d Draw) string {
	s := fmt.Sprintf("out%+v/%v in%+v/%v", d.Outgoing.Dst, d.Outgoing.Alpha, d.Incoming.Dst, d.Incoming.Alpha)
	if d.Outgoing.Clip != nil {
		s += fmt.Sprintf(" oclip%+v", *d.Outgoing.Clip)
	}
	if d.Incoming.Clip != nil {
		s += fmt.Sprintf(" iclip%+v", *d.Incoming.Clip)
	}
	return s
}
