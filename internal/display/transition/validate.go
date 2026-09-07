package transition

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var cubicBezierRe = regexp.MustCompile(`(?i)^cubic-bezier\(\s*(-?[0-9]*\.?[0-9]+)\s*,\s*(-?[0-9]*\.?[0-9]+)\s*,\s*(-?[0-9]*\.?[0-9]+)\s*,\s*(-?[0-9]*\.?[0-9]+)\s*\)$`)

// Validate reports whether spec is a legal catalogue entry.
func Validate(s Spec) error {
	if strings.TrimSpace(s.Type) == "" {
		return errors.New("transition type is required")
	}
	if !ValidSubtype(s.Type, s.Subtype) {
		if s.Subtype == "" {
			return fmt.Errorf("subtype is required for type %q", s.Type)
		}
		return fmt.Errorf("invalid subtype %q for type %q", s.Subtype, s.Type)
	}
	if s.Type == "cut" {
		if s.DurationMS != 0 {
			return errors.New("cut duration_ms must be 0")
		}
	} else if s.DurationMS <= 0 {
		return errors.New("duration_ms must be greater than 0")
	}
	if s.Easing != "" {
		if _, err := ParseEasing(s.Easing); err != nil {
			return err
		}
	}
	if s.Subtype == "fadeToColor" || s.Subtype == "fadeFromColor" {
		if s.Color == nil || strings.TrimSpace(*s.Color) == "" {
			return errors.New("color is required for fade to/from color")
		}
	}
	return nil
}

// ParseEasing expands a named preset or cubic-bezier(...) to control points.
func ParseEasing(s string) ([4]float64, error) {
	name := strings.TrimSpace(s)
	if name == "" {
		name = "linear"
	}
	for _, p := range EasingPresets() {
		if p.Name == name {
			return p.Points, nil
		}
	}
	m := cubicBezierRe.FindStringSubmatch(name)
	if m == nil {
		return [4]float64{}, fmt.Errorf("invalid easing %q", s)
	}
	var pts [4]float64
	for i := 0; i < 4; i++ {
		v, err := strconv.ParseFloat(m[i+1], 64)
		if err != nil {
			return [4]float64{}, fmt.Errorf("invalid easing %q", s)
		}
		pts[i] = v
	}
	if pts[0] < 0 || pts[0] > 1 || pts[2] < 0 || pts[2] > 1 {
		return [4]float64{}, fmt.Errorf("easing x coordinates must be in [0, 1]")
	}
	return pts, nil
}

// DefaultCut is the zero-duration default transition.
func DefaultCut() Spec {
	return Spec{Type: "cut", DurationMS: 0}
}
