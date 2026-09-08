package compositor

import (
	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/model"
)

// Tile is one resolved cell to draw.
type Tile struct {
	Index       int
	SourceID    *uint
	Fit         string
	Cell        layout.PixelRect
	Src         layout.PixelRect
	Dst         layout.PixelRect
	Placeholder bool
}

// Scene is an ordered list of draw ops, built before any SDL call.
type Scene struct {
	Background string
	Width      int
	Height     int
	Tiles      []Tile
}

// VideoSize is the decoded size of a source, if known.
type VideoSize struct {
	W, H int
}

// Layer is the compositor's view of one screen (outgoing or incoming).
type Layer struct {
	Kind   string
	Layout string
	Rects  []layout.Rect
	Tiles  []LayerTile
}

// LayerTile is a cell with its current source assignment.
type LayerTile struct {
	Index    int
	SourceID *uint
	Fit      string
}

// Build maps a layer onto output pixels.
func Build(layer Layer, outW, outH, gutter int, sizes map[uint]VideoSize, placeholder string) Scene {
	s := Scene{Background: placeholder, Width: outW, Height: outH}
	if outW <= 0 || outH <= 0 {
		return s
	}
	l := layout.Layout{Rects: layer.Rects}
	if len(l.Rects) == 0 {
		if ly, ok := layout.ByID(layer.Layout); ok {
			l = ly
		} else if ly, ok := layout.ByID(layout.FullBleedID); ok {
			l = ly
		}
	}
	cells := layout.Solve(l, outW, outH, gutter)
	byIndex := map[int]LayerTile{}
	for _, t := range layer.Tiles {
		byIndex[t.Index] = t
	}
	for i, cell := range cells {
		tile := Tile{Index: i, Cell: cell, Placeholder: true, Fit: model.FitContain}
		if lt, ok := byIndex[i]; ok {
			tile.Index = lt.Index
			tile.SourceID = lt.SourceID
			tile.Fit = lt.Fit
			if lt.SourceID != nil {
				if sz, ok := sizes[*lt.SourceID]; ok && sz.W > 0 && sz.H > 0 {
					src, dst := layout.Fit(cell, sz.W, sz.H, lt.Fit)
					tile.Src, tile.Dst = src, dst
					tile.Placeholder = false
				}
			}
		}
		s.Tiles = append(s.Tiles, tile)
	}
	return s
}
