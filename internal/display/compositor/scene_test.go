package compositor

import (
	"testing"

	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/model"
)

func TestBuildPlaceholderWhenNoSize(t *testing.T) {
	sid := uint(1)
	sc := Build(Layer{
		Kind:   model.ScreenKindGrid,
		Layout: "2x2",
		Tiles:  []LayerTile{{Index: 0, SourceID: &sid, Fit: model.FitContain}},
	}, 1920, 1080, 4, nil, "#111")
	if len(sc.Tiles) != 4 {
		t.Fatalf("tiles %d", len(sc.Tiles))
	}
	if !sc.Tiles[0].Placeholder {
		t.Fatal("expected placeholder without video size")
	}
}

func TestBuildFitGeometry(t *testing.T) {
	sid := uint(7)
	l, _ := layout.ByID("full")
	sc := Build(Layer{
		Layout: "full",
		Rects:  l.Rects,
		Tiles:  []LayerTile{{Index: 0, SourceID: &sid, Fit: model.FitContain}},
	}, 1920, 1080, 0, map[uint]VideoSize{7: {W: 1920, H: 1080}}, "#000")
	if sc.Tiles[0].Placeholder {
		t.Fatal("should have video")
	}
	if sc.Tiles[0].Dst.W != 1920 || sc.Tiles[0].Dst.H != 1080 {
		t.Fatalf("dst %+v", sc.Tiles[0].Dst)
	}
}
