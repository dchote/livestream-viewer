package texture

import (
	"testing"

	"github.com/dchote/livestream-viewer/internal/frame"
)

func TestKeyOfDistinguishesGeometryAndColor(t *testing.T) {
	base := &frame.Frame{Width: 64, Height: 64, Format: frame.FormatNV12, Color: frame.Color{Space: "bt709", Range: "limited"}}
	same := &frame.Frame{Width: 64, Height: 64, Format: frame.FormatNV12, Color: frame.Color{Space: "bt709", Range: "limited"}}
	if keyOf(base) != keyOf(same) {
		t.Fatal("identical frames must share a key")
	}
	for name, f := range map[string]*frame.Frame{
		"width":  {Width: 128, Height: 64, Format: frame.FormatNV12, Color: base.Color},
		"height": {Width: 64, Height: 128, Format: frame.FormatNV12, Color: base.Color},
		"range":  {Width: 64, Height: 64, Format: frame.FormatNV12, Color: frame.Color{Space: "bt709", Range: "full"}},
		"space":  {Width: 64, Height: 64, Format: frame.FormatNV12, Color: frame.Color{Space: "bt601", Range: "limited"}},
		"format": {Width: 64, Height: 64, Format: frame.FormatI420, Color: base.Color},
	} {
		if keyOf(base) == keyOf(f) {
			t.Fatalf("%s change must produce a different key", name)
		}
	}
}

// Guards against a nil renderer (engine init failure) and an abandoned window.
func TestCacheNilAndAbandonedAreSafe(t *testing.T) {
	var nilCache *Cache
	if nilCache.Get(1) != nil || nilCache.Len() != 0 {
		t.Fatal("nil cache must be inert")
	}
	nilCache.Drop(1)
	nilCache.Abandon()
	nilCache.Close()

	c := NewCache(nil)
	if got := c.Upload(1, &frame.Frame{Width: 64, Height: 64}); got != nil {
		t.Fatal("upload without a renderer must fail closed")
	}
	c.Abandon()
	if c.Len() != 0 {
		t.Fatalf("abandon left %d entries", c.Len())
	}
	c.Close()
}

func TestUploadRejectsMalformedFrames(t *testing.T) {
	c := NewCache(nil)
	for name, f := range map[string]*frame.Frame{
		"nil":        nil,
		"no planes":  {Width: 64, Height: 64},
		"one plane":  {Width: 64, Height: 64, Planes: [][]byte{{0}}},
		"no strides": {Width: 64, Height: 64, Planes: [][]byte{{0}, {0}}},
		"one stride": {Width: 64, Height: 64, Planes: [][]byte{{0}, {0}}, Strides: []int{64}},
		"i420 two":   {Width: 64, Height: 64, Format: frame.FormatI420, Planes: [][]byte{{0}, {0}}, Strides: []int{64, 32}},
	} {
		if got := c.Upload(1, f); got != nil {
			t.Fatalf("%s should not upload", name)
		}
	}
}
