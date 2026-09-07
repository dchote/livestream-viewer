package layout

import "math"

// Rect is a normalised rectangle in the unit square.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Layout is a named arrangement of tiles. Geometry is data, not code.
type Layout struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Family string `json:"family"`
	Cells  int    `json:"cells"`
	Rects  []Rect `json:"rects"`
}

// PixelRect is a layout cell resolved onto an output in whole pixels.
type PixelRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

const (
	FamilyEqual     = "equal"
	FamilyHotspot   = "hotspot"
	FamilyVertical  = "vertical"
	FamilyPanoramic = "panoramic"
)

// All returns the layout catalogue. Cell index 0 is the hotspot where one exists.
func All() []Layout {
	return []Layout{
		grid("1x1", "1×1", FamilyEqual, 1, 1),
		grid("2x1", "2×1", FamilyEqual, 2, 1),
		grid("1x2", "1×2", FamilyEqual, 1, 2),
		grid("2x2", "2×2", FamilyEqual, 2, 2),
		grid("3x3", "3×3", FamilyEqual, 3, 3),
		grid("4x4", "4×4", FamilyEqual, 4, 4),
		hotspot1Plus3(),
		hotspot1Plus5(),
		hotspot1Plus7(),
		hotspot1Plus12(),
		vertical3(),
		vertical1Plus6(),
		panoramic2(),
		panoramic1Plus6(),
	}
}

// ByID returns a layout from the catalogue, or false if unknown.
func ByID(id string) (Layout, bool) {
	for _, l := range All() {
		if l.ID == id {
			return l, true
		}
	}
	return Layout{}, false
}

func grid(id, name, family string, cols, rows int) Layout {
	rects := make([]Rect, 0, cols*rows)
	cw := 1.0 / float64(cols)
	ch := 1.0 / float64(rows)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			rects = append(rects, Rect{X: float64(c) * cw, Y: float64(r) * ch, W: cw, H: ch})
		}
	}
	return Layout{ID: id, Name: name, Family: family, Cells: len(rects), Rects: rects}
}

func hotspot1Plus3() Layout {
	return Layout{
		ID: "1+3", Name: "1 + 3 hotspot", Family: FamilyHotspot, Cells: 4,
		Rects: []Rect{
			{X: 0, Y: 0, W: 0.75, H: 1},
			{X: 0.75, Y: 0, W: 0.25, H: 1.0 / 3},
			{X: 0.75, Y: 1.0 / 3, W: 0.25, H: 1.0 / 3},
			{X: 0.75, Y: 2.0 / 3, W: 0.25, H: 1.0 / 3},
		},
	}
}

func hotspot1Plus5() Layout {
	return Layout{
		ID: "1+5", Name: "1 + 5 hotspot", Family: FamilyHotspot, Cells: 6,
		Rects: []Rect{
			{X: 0.0, Y: 0.0, W: 0.6667, H: 0.6667},
			{X: 0.6667, Y: 0.0, W: 0.3333, H: 0.3333},
			{X: 0.6667, Y: 0.3333, W: 0.3333, H: 0.3333},
			{X: 0.0, Y: 0.6667, W: 0.3333, H: 0.3333},
			{X: 0.3333, Y: 0.6667, W: 0.3333, H: 0.3333},
			{X: 0.6667, Y: 0.6667, W: 0.3333, H: 0.3333},
		},
	}
}

func hotspot1Plus7() Layout {
	return Layout{
		ID: "1+7", Name: "1 + 7 hotspot", Family: FamilyHotspot, Cells: 8,
		Rects: []Rect{
			{X: 0, Y: 0, W: 0.75, H: 0.75},
			{X: 0.75, Y: 0, W: 0.25, H: 0.25},
			{X: 0.75, Y: 0.25, W: 0.25, H: 0.25},
			{X: 0.75, Y: 0.50, W: 0.25, H: 0.25},
			{X: 0, Y: 0.75, W: 0.25, H: 0.25},
			{X: 0.25, Y: 0.75, W: 0.25, H: 0.25},
			{X: 0.50, Y: 0.75, W: 0.25, H: 0.25},
			{X: 0.75, Y: 0.75, W: 0.25, H: 0.25},
		},
	}
}

func hotspot1Plus12() Layout {
	rects := []Rect{{X: 0, Y: 0, W: 0.5, H: 0.5}}
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			if r < 2 && c < 2 {
				continue
			}
			rects = append(rects, Rect{X: float64(c) * 0.25, Y: float64(r) * 0.25, W: 0.25, H: 0.25})
		}
	}
	return Layout{ID: "1+12", Name: "1 + 12 hotspot", Family: FamilyHotspot, Cells: len(rects), Rects: rects}
}

func vertical3() Layout {
	return Layout{
		ID: "3v", Name: "3 vertical", Family: FamilyVertical, Cells: 3,
		Rects: []Rect{
			{X: 0, Y: 0, W: 1, H: 1.0 / 3},
			{X: 0, Y: 1.0 / 3, W: 1, H: 1.0 / 3},
			{X: 0, Y: 2.0 / 3, W: 1, H: 1.0 / 3},
		},
	}
}

func vertical1Plus6() Layout {
	rects := []Rect{{X: 0, Y: 0, W: 0.5, H: 1}}
	for r := 0; r < 3; r++ {
		for c := 0; c < 2; c++ {
			rects = append(rects, Rect{
				X: 0.5 + float64(c)*0.25,
				Y: float64(r) / 3,
				W: 0.25,
				H: 1.0 / 3,
			})
		}
	}
	return Layout{ID: "1v+6", Name: "1 vertical + 6", Family: FamilyVertical, Cells: len(rects), Rects: rects}
}

func panoramic2() Layout {
	return Layout{
		ID: "2p", Name: "2 panoramic", Family: FamilyPanoramic, Cells: 2,
		Rects: []Rect{
			{X: 0, Y: 0, W: 1, H: 0.5},
			{X: 0, Y: 0.5, W: 1, H: 0.5},
		},
	}
}

func panoramic1Plus6() Layout {
	rects := []Rect{{X: 0, Y: 0, W: 1, H: 1.0 / 3}}
	for r := 0; r < 2; r++ {
		for c := 0; c < 3; c++ {
			rects = append(rects, Rect{
				X: float64(c) / 3,
				Y: 1.0/3 + float64(r)*(1.0/3),
				W: 1.0 / 3,
				H: 1.0 / 3,
			})
		}
	}
	return Layout{ID: "1p+6", Name: "1 panoramic + 6", Family: FamilyPanoramic, Cells: len(rects), Rects: rects}
}

// Solve maps normalised rects onto an output resolution, insets a uniform gutter, and rounds to pixels.
func Solve(l Layout, width, height, gutter int) []PixelRect {
	out := make([]PixelRect, len(l.Rects))
	inset := float64(gutter) / 2
	for i, r := range l.Rects {
		x0 := r.X*float64(width) + inset
		y0 := r.Y*float64(height) + inset
		x1 := (r.X+r.W)*float64(width) - inset
		y1 := (r.Y+r.H)*float64(height) - inset
		px := int(math.Round(x0))
		py := int(math.Round(y0))
		pw := int(math.Round(x1)) - px
		ph := int(math.Round(y1)) - py
		if pw < 0 {
			pw = 0
		}
		if ph < 0 {
			ph = 0
		}
		out[i] = PixelRect{X: px, Y: py, W: pw, H: ph}
	}
	return out
}
