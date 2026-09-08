package ingest

import (
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/dchote/livestream-viewer/internal/frame"
)

func TestNativeYUVKeeps420(t *testing.T) {
	t.Parallel()
	if f, ok := nativeYUV(astiav.PixelFormatYuv420P); !ok || f != frame.FormatI420 {
		t.Fatalf("yuv420p: %v %v", f, ok)
	}
	if f, ok := nativeYUV(astiav.PixelFormatYuvj420P); !ok || f != frame.FormatI420 {
		t.Fatalf("yuvj420p: %v %v", f, ok)
	}
	if f, ok := nativeYUV(astiav.PixelFormatNv12); !ok || f != frame.FormatNV12 {
		t.Fatalf("nv12: %v %v", f, ok)
	}
	if _, ok := nativeYUV(astiav.PixelFormatYuv422P); ok {
		t.Fatal("yuv422p must convert, not upload natively")
	}
}

func TestPackedYUVSizeOddHeight(t *testing.T) {
	t.Parallel()
	if got := packedYUVSize(8, 9, frame.FormatNV12); got != 8*9+8*5 {
		t.Fatalf("nv12 odd height: %d", got)
	}
	if got := packedYUVSize(8, 9, frame.FormatI420); got != 8*9+2*4*5 {
		t.Fatalf("i420 odd height: %d", got)
	}
}

func TestPtsDurationZeroIsValid(t *testing.T) {
	t.Parallel()
	tb := astiav.NewRational(1, 90000)
	if got := ptsDuration(0, tb); got != 0 {
		t.Fatalf("zero PTS: %v", got)
	}
	if got := ptsDuration(-1, tb); got >= 0 {
		t.Fatalf("NOPTS should stay negative, got %v", got)
	}
	if got := ptsDuration(90000, tb); got != time.Second {
		t.Fatalf("1s PTS: %v", got)
	}
}
