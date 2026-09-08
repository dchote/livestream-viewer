package ingest

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/dchote/livestream-viewer/internal/frame"
	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
)

func TestFileFixturePublishes(t *testing.T) {
	_, this, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(this), "testdata", "tiny.mp4")
	slot := frame.NewSlot()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunWorker(ctx, WorkerConfig{
			Source:  model.Source{Name: "fixture", Kind: model.KindFile, URL: path, Enabled: true},
			Slot:    slot,
			Caps:    capability.Info{},
			AllowSW: true,
			UseHW:   false,
		})
	}()
	deadline := time.Now().Add(6 * time.Second)
	var got *frame.Frame
	for time.Now().Before(deadline) {
		got = slot.Take()
		if got != nil && got.Width == 64 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if got == nil || got.Width != 64 || got.Height != 64 {
		t.Fatalf("expected 64x64 frame, got %+v", got)
	}
	switch got.Format {
	case frame.FormatI420:
		if len(got.Planes) != 3 {
			t.Fatalf("i420 %+v", got)
		}
	case frame.FormatNV12:
		if len(got.Planes) != 2 {
			t.Fatalf("nv12 %+v", got)
		}
	default:
		t.Fatalf("unexpected format %v %+v", got.Format, got)
	}
}

func TestFileFixtureSoftwareHealth(t *testing.T) {
	_, this, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(this), "testdata", "tiny.mp4")
	slot := frame.NewSlot()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var decoded string
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunWorker(ctx, WorkerConfig{
			Source:  model.Source{Name: "fixture", Kind: model.KindFile, URL: path, Enabled: true},
			Slot:    slot,
			Caps:    capability.Info{},
			AllowSW: true,
			UseHW:   true,
			OnHealth: func(status, _, _ string) {
				if status == "software" || status == "hardware" {
					decoded = status
				}
			},
		})
	}()
	deadline := time.Now().Add(6 * time.Second)
	var got *frame.Frame
	for time.Now().Before(deadline) {
		got = slot.Take()
		if got != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if got == nil {
		t.Fatal("expected a decoded frame")
	}
	if decoded != "software" {
		t.Fatalf("health %q, want software", decoded)
	}
}

func TestStartDelayCancel(t *testing.T) {
	slot := frame.NewSlot()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(done)
		RunWorker(ctx, WorkerConfig{
			Source:     model.Source{Name: "delay", Kind: model.KindFile, URL: "/nope.mp4", Enabled: true},
			Slot:       slot,
			AllowSW:    true,
			StartDelay: 5 * time.Second,
		})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
	if time.Since(start) > time.Second {
		t.Fatalf("cancel during start delay took %v", time.Since(start))
	}
}

func TestDiscardUntilKeyframe(t *testing.T) {
	t.Parallel()
	pkt := astiav.AllocPacket()
	defer pkt.Free()

	got := false
	if !discardUntilKeyframe(true, &got, pkt) {
		t.Fatal("hardware decode must wait for a keyframe")
	}
	if got {
		t.Fatal("non-key must not arm the decoder")
	}

	pkt.SetFlags(astiav.NewPacketFlags(astiav.PacketFlagKey))
	if discardUntilKeyframe(true, &got, pkt) {
		t.Fatal("keyframe must be sent")
	}
	if !got {
		t.Fatal("keyframe should arm the decoder")
	}

	pkt.SetFlags(astiav.NewPacketFlags())
	if discardUntilKeyframe(true, &got, pkt) {
		t.Fatal("after a keyframe, P-frames must be sent")
	}

	got = false
	if discardUntilKeyframe(false, &got, pkt) {
		t.Fatal("software decode can start mid-GOP")
	}

	pkt.SetFlags(astiav.NewPacketFlags(astiav.PacketFlagCorrupt))
	got = true
	if !discardUntilKeyframe(true, &got, pkt) {
		t.Fatal("corrupt packets must be dropped")
	}
}
