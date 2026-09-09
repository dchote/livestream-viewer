//go:build sdl

package texture

import (
	"testing"

	"github.com/dchote/livestream-viewer/internal/display/output"
	"github.com/dchote/livestream-viewer/internal/frame"
)

func nv12(w, h int, rng string) *frame.Frame {
	y := make([]byte, w*h)
	uv := make([]byte, w*(h/2))
	return &frame.Frame{
		Width:   w,
		Height:  h,
		Format:  frame.FormatNV12,
		Planes:  [][]byte{y, uv},
		Strides: []int{w, w},
		Color:   frame.Color{Space: "bt709", Range: rng},
	}
}

// An adaptive stream changes resolution mid-session. The cache must replace the
// source's texture, not accumulate one per geometry, or it leaks for the life
// of the process and Get can return a stale-sized texture.
func TestCacheReplacesTextureOnGeometryChange(t *testing.T) {
	out, err := output.Init(output.Config{Device: -1, Width: 320, Height: 180, Title: "lsv-texcache-test"})
	if err != nil {
		t.Skip(err)
	}
	defer out.Close()

	c := NewCache(out.Renderer)
	defer c.Close()

	if got := c.Upload(1, nv12(64, 64, "limited")); got == nil {
		t.Fatal("first upload failed")
	}
	if c.Len() != 1 {
		t.Fatalf("len after first upload = %d", c.Len())
	}
	first := c.Get(1)

	for _, f := range []*frame.Frame{
		nv12(128, 128, "limited"),
		nv12(256, 144, "limited"),
		nv12(256, 144, "full"),
		nv12(64, 64, "limited"),
	} {
		if got := c.Upload(1, f); got == nil {
			t.Fatalf("upload %dx%d %s failed", f.Width, f.Height, f.Color.Range)
		}
		if c.Len() != 1 {
			t.Fatalf("cache grew to %d entries after geometry change", c.Len())
		}
		if c.Get(1) != c.Upload(1, f) {
			t.Fatal("Get did not return the current texture")
		}
	}

	if c.Get(1) == first {
		t.Fatal("expected a new texture after the geometry churn")
	}
	c.Drop(1)
	if c.Len() != 0 || c.Get(1) != nil {
		t.Fatalf("drop left %d entries", c.Len())
	}
}

func i420(w, h int) *frame.Frame {
	cw, ch := (w+1)/2, (h+1)/2
	y := make([]byte, w*h)
	u := make([]byte, cw*ch)
	v := make([]byte, cw*ch)
	return &frame.Frame{
		Width:   w,
		Height:  h,
		Format:  frame.FormatI420,
		Planes:  [][]byte{y, u, v},
		Strides: []int{w, cw, cw},
		Color:   frame.Color{Space: "bt709", Range: "limited"},
	}
}

func TestCacheUploadsI420(t *testing.T) {
	out, err := output.Init(output.Config{Device: -1, Width: 320, Height: 180, Title: "lsv-texcache-i420"})
	if err != nil {
		t.Skip(err)
	}
	defer out.Close()

	c := NewCache(out.Renderer)
	defer c.Close()

	nv := nv12(64, 64, "limited")
	if got := c.Upload(1, nv); got == nil {
		t.Fatal("nv12 upload failed")
	}
	first := c.Get(1)
	if got := c.Upload(1, i420(64, 64)); got == nil {
		t.Fatal("i420 upload failed")
	}
	if c.Len() != 1 {
		t.Fatalf("format change grew the cache to %d", c.Len())
	}
	if c.Get(1) == first {
		t.Fatal("I420 must recreate the texture; NV12 and IYUV are different formats")
	}
}

func TestCacheIsolatesSources(t *testing.T) {
	out, err := output.Init(output.Config{Device: -1, Width: 320, Height: 180, Title: "lsv-texcache-test2"})
	if err != nil {
		t.Skip(err)
	}
	defer out.Close()

	c := NewCache(out.Renderer)
	defer c.Close()

	c.Upload(1, nv12(64, 64, "limited"))
	c.Upload(2, nv12(64, 64, "limited"))
	if c.Len() != 2 {
		t.Fatalf("len = %d, want 2", c.Len())
	}
	if c.Get(1) == c.Get(2) {
		t.Fatal("sources must not share a texture")
	}
	c.Drop(1)
	if c.Get(1) != nil || c.Get(2) == nil {
		t.Fatal("drop hit the wrong source")
	}
}
