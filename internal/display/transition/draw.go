package transition

// Rect is a pixel rectangle used by Draw.
type Rect struct {
	X, Y, W, H int
}

// Layer is how one of the two screens should be drawn during a transition.
type Layer struct {
	Dst   Rect
	Clip  *Rect
	Alpha float32
}

// Draw is a pure function of eased progress: how to composite outgoing and incoming layers.
type Draw struct {
	Family   string
	Outgoing Layer
	Incoming Layer
	Fill     *string // solid colour for fadeToColor / fadeFromColor
	Degraded bool    // true when a masked wipe fell back to fade
}

func full(w, h int) Rect {
	return Rect{X: 0, Y: 0, W: w, H: h}
}

func clampT(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

func round(v float64) int {
	if v < 0 {
		return int(v - 0.5)
	}
	return int(v + 0.5)
}

// Eval returns draw instructions for spec at raw progress (easing applied here).
func Eval(spec Spec, raw float64, w, h int) Draw {
	t := Ease(spec, clampT(raw))
	fw, fh := full(w, h), full(w, h)
	opaqueOut := Layer{Dst: fw, Alpha: 1}
	opaqueIn := Layer{Dst: fh, Alpha: 1}

	switch spec.Type {
	case "cut":
		if t >= 1 {
			return Draw{Family: FamilyImmediate, Incoming: opaqueIn}
		}
		return Draw{Family: FamilyImmediate, Outgoing: opaqueOut}
	case "fade":
		return evalFade(spec, t, w, h)
	case "barWipe":
		return evalBar(spec.Subtype, t, w, h)
	case "boxWipe":
		return evalBox(spec.Subtype, t, w, h)
	case "barnDoorWipe":
		return evalBarn(spec.Subtype, t, w, h)
	case "pushWipe":
		return evalPush(spec.Subtype, t, w, h)
	case "slideWipe":
		return evalSlide(spec.Subtype, t, w, h)
	default:
		if _, retired := RetiredTypes[spec.Type]; retired {
			// A stored config from before these were withdrawn. Render the
			// replacement and report it so the UI can explain the difference.
			d := evalFade(Spec{Type: "fade", Subtype: "crossfade"}, t, w, h)
			d.Degraded = true
			return d
		}
		return evalFade(Spec{Type: "fade", Subtype: "crossfade"}, t, w, h)
	}
}

func evalFade(spec Spec, t float64, w, h int) Draw {
	fw := full(w, h)
	switch spec.Subtype {
	case "fadeToColor":
		if t < 0.5 {
			a := float32(t * 2)
			return Draw{Family: FamilyAlpha, Outgoing: Layer{Dst: fw, Alpha: 1 - a}, Fill: spec.Color}
		}
		a := float32((t - 0.5) * 2)
		return Draw{Family: FamilyAlpha, Incoming: Layer{Dst: fw, Alpha: a}, Fill: spec.Color}
	case "fadeFromColor":
		// The outgoing screen is already gone: the frame starts on the solid
		// colour and the incoming rises out of it. Distinct from fadeToColor,
		// which dips through the colour and shows both screens on the way.
		return Draw{Family: FamilyAlpha, Fill: spec.Color, Incoming: Layer{Dst: fw, Alpha: float32(t)}}
	default: // crossfade
		return Draw{
			Family:   FamilyAlpha,
			Outgoing: Layer{Dst: fw, Alpha: 1},
			Incoming: Layer{Dst: fw, Alpha: float32(t)},
		}
	}
}

func evalBar(sub string, t float64, w, h int) Draw {
	fw := full(w, h)
	clip := fw
	switch sub {
	case "topToBottom":
		clip.H = round(float64(h) * t)
	default: // leftToRight
		clip.W = round(float64(w) * t)
	}
	return Draw{Family: FamilyGeometric, Outgoing: Layer{Dst: fw, Alpha: 1}, Incoming: Layer{Dst: fw, Clip: &clip, Alpha: 1}}
}

func evalBox(sub string, t float64, w, h int) Draw {
	fw := full(w, h)
	cw, ch := round(float64(w)*t), round(float64(h)*t)
	clip := Rect{W: cw, H: ch}
	switch sub {
	case "topRight":
		clip.X = w - cw
	case "bottomLeft":
		clip.Y = h - ch
	case "bottomRight":
		clip.X = w - cw
		clip.Y = h - ch
	}
	return Draw{Family: FamilyGeometric, Outgoing: Layer{Dst: fw, Alpha: 1}, Incoming: Layer{Dst: fw, Clip: &clip, Alpha: 1}}
}

func evalBarn(sub string, t float64, w, h int) Draw {
	fw := full(w, h)
	if sub == "horizontal" {
		open := round(float64(h) * t / 2)
		clip := Rect{X: 0, Y: h/2 - open, W: w, H: open * 2}
		return Draw{Family: FamilyGeometric, Outgoing: Layer{Dst: fw, Alpha: 1}, Incoming: Layer{Dst: fw, Clip: &clip, Alpha: 1}}
	}
	open := round(float64(w) * t / 2)
	clip := Rect{X: w/2 - open, Y: 0, W: open * 2, H: h}
	return Draw{Family: FamilyGeometric, Outgoing: Layer{Dst: fw, Alpha: 1}, Incoming: Layer{Dst: fw, Clip: &clip, Alpha: 1}}
}

func evalPush(sub string, t float64, w, h int) Draw {
	fw := full(w, h)
	out, in := Layer{Dst: fw, Alpha: 1}, Layer{Dst: fw, Alpha: 1}
	switch sub {
	case "fromRight":
		in.Dst.X = w - round(float64(w)*t)
		out.Dst.X = -round(float64(w) * t)
	case "fromTop":
		in.Dst.Y = round(float64(h)*t) - h
		out.Dst.Y = round(float64(h) * t)
	case "fromBottom":
		in.Dst.Y = h - round(float64(h)*t)
		out.Dst.Y = -round(float64(h) * t)
	default: // fromLeft
		in.Dst.X = round(float64(w)*t) - w
		out.Dst.X = round(float64(w) * t)
	}
	return Draw{Family: FamilyGeometric, Outgoing: out, Incoming: in}
}

func evalSlide(sub string, t float64, w, h int) Draw {
	fw := full(w, h)
	out := Layer{Dst: fw, Alpha: 1}
	in := Layer{Dst: fw, Alpha: 1}
	switch sub {
	case "fromRight":
		in.Dst.X = w - round(float64(w)*t)
	case "fromTop":
		in.Dst.Y = round(float64(h)*t) - h
	case "fromBottom":
		in.Dst.Y = h - round(float64(h)*t)
	default: // fromLeft
		in.Dst.X = round(float64(w)*t) - w
	}
	return Draw{Family: FamilyGeometric, Outgoing: out, Incoming: in}
}
