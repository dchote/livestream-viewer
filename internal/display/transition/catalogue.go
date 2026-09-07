package transition

// Spec describes a configured transition (type / subtype split from SMIL / SMPTE 258M).
type Spec struct {
	Type       string  `json:"type"`
	Subtype    string  `json:"subtype,omitempty"`
	DurationMS int     `json:"duration_ms"`
	Easing     string  `json:"easing,omitempty"`
	Color      *string `json:"color"`
}

// TypeInfo is one entry in the transition catalogue.
type TypeInfo struct {
	Type       string   `json:"type"`
	Subtypes   []string `json:"subtypes"`
	Family     string   `json:"family"`
	Available  bool     `json:"available"`
	DefaultSub string   `json:"default_subtype,omitempty"`
}

// EasingPreset is a named CSS cubic-bezier.
type EasingPreset struct {
	Name   string     `json:"name"`
	Points [4]float64 `json:"points"`
}

const (
	FamilyImmediate = "immediate"
	FamilyAlpha     = "alpha"
	FamilyGeometric = "geometric"
	FamilyMasked    = "masked"
)

// Catalogue returns every supported type with valid subtypes.
// Masked types are listed; Available is true for this control-plane scaffold
// (the engine will report platform capability later).
func Catalogue() []TypeInfo {
	return []TypeInfo{
		{Type: "cut", Family: FamilyImmediate, Available: true},
		{Type: "fade", Subtypes: []string{"crossfade", "fadeToColor", "fadeFromColor"}, Family: FamilyAlpha, Available: true, DefaultSub: "crossfade"},
		{Type: "barWipe", Subtypes: []string{"leftToRight", "topToBottom"}, Family: FamilyGeometric, Available: true, DefaultSub: "leftToRight"},
		{Type: "boxWipe", Subtypes: []string{"topLeft", "topRight", "bottomRight", "bottomLeft"}, Family: FamilyGeometric, Available: true, DefaultSub: "topLeft"},
		{Type: "barnDoorWipe", Subtypes: []string{"vertical", "horizontal"}, Family: FamilyGeometric, Available: true, DefaultSub: "vertical"},
		{Type: "pushWipe", Subtypes: []string{"fromLeft", "fromRight", "fromTop", "fromBottom"}, Family: FamilyGeometric, Available: true, DefaultSub: "fromLeft"},
		{Type: "slideWipe", Subtypes: []string{"fromLeft", "fromRight", "fromTop", "fromBottom"}, Family: FamilyGeometric, Available: true, DefaultSub: "fromLeft"},
		{Type: "irisWipe", Subtypes: []string{"rectangle"}, Family: FamilyMasked, Available: true, DefaultSub: "rectangle"},
		{Type: "ellipseWipe", Subtypes: []string{"circle", "horizontal", "vertical"}, Family: FamilyMasked, Available: true, DefaultSub: "circle"},
		{Type: "clockWipe", Subtypes: []string{"clockwiseTwelve", "clockwiseThree", "clockwiseSix", "clockwiseNine"}, Family: FamilyMasked, Available: true, DefaultSub: "clockwiseTwelve"},
	}
}

// EasingPresets returns CSS named easings as cubic-bezier control points.
func EasingPresets() []EasingPreset {
	return []EasingPreset{
		{Name: "linear", Points: [4]float64{0, 0, 1, 1}},
		{Name: "ease", Points: [4]float64{0.25, 0.1, 0.25, 1}},
		{Name: "ease-in", Points: [4]float64{0.42, 0, 1, 1}},
		{Name: "ease-out", Points: [4]float64{0, 0, 0.58, 1}},
		{Name: "ease-in-out", Points: [4]float64{0.42, 0, 0.58, 1}},
	}
}

// ValidSubtype reports whether subtype is allowed for type.
func ValidSubtype(typ, subtype string) bool {
	for _, t := range Catalogue() {
		if t.Type != typ {
			continue
		}
		if len(t.Subtypes) == 0 {
			return subtype == ""
		}
		for _, s := range t.Subtypes {
			if s == subtype {
				return true
			}
		}
		return false
	}
	return false
}
