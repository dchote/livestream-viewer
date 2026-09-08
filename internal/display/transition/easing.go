package transition

import "math"

// EvalBezier evaluates a CSS cubic-bezier at t in [0,1] using Newton–Raphson.
func EvalBezier(pts [4]float64, t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	x1, y1, x2, y2 := pts[0], pts[1], pts[2], pts[3]
	u := t
	for i := 0; i < 8; i++ {
		x := sampleX(x1, x2, u)
		dx := sampleDX(x1, x2, u)
		if math.Abs(dx) < 1e-6 {
			break
		}
		u -= (x - t) / dx
		if u < 0 {
			u = 0
		}
		if u > 1 {
			u = 1
		}
	}
	return sampleY(y1, y2, u)
}

func sampleX(x1, x2, t float64) float64 {
	return 3*(1-t)*(1-t)*t*x1 + 3*(1-t)*t*t*x2 + t*t*t
}

func sampleDX(x1, x2, t float64) float64 {
	return 3*(1-t)*(1-t)*x1 + 6*(1-t)*t*(x2-x1) + 3*t*t*(1-x2)
}

func sampleY(y1, y2, t float64) float64 {
	return 3*(1-t)*(1-t)*t*y1 + 3*(1-t)*t*t*y2 + t*t*t
}

// Ease maps raw progress through spec.Easing.
func Ease(spec Spec, raw float64) float64 {
	// NaN fails both comparisons below and would propagate through the bezier
	// into layer alphas, where it renders as a blank tile rather than an
	// error. Treat it as the start of the transition.
	if math.IsNaN(raw) || raw < 0 {
		raw = 0
	}
	if raw > 1 {
		raw = 1
	}
	pts, err := ParseEasing(spec.Easing)
	if err != nil {
		pts = [4]float64{0, 0, 1, 1}
	}
	return EvalBezier(pts, raw)
}
