//go:build sdl

package output

import (
	"testing"
)

func TestInitWindowed(t *testing.T) {
	out, err := Init(Config{Width: 320, Height: 180, Title: "lsv-sdl-test"})
	if err != nil {
		t.Skip(err)
	}
	defer out.Close()
	if out.Width < 1 || out.Height < 1 {
		t.Fatalf("size %dx%d", out.Width, out.Height)
	}
	_ = out.Pump()
}
