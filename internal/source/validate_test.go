package source

import (
	"testing"

	"github.com/dchote/livestream-viewer/internal/model"
)

func TestValidateCreate(t *testing.T) {
	t.Parallel()
	s := &model.Source{Name: "cam", Kind: model.KindRTSP, URL: "rtsp://cam.local/stream"}
	if err := ValidateCreate(s); err != nil {
		t.Fatal(err)
	}
	if s.Options.Transport != "tcp" {
		t.Fatalf("default transport = %q", s.Options.Transport)
	}

	if err := ValidateCreate(&model.Source{Kind: model.KindRTSP, URL: "rtsp://x"}); err == nil {
		t.Fatal("expected name required")
	}
	if err := ValidateCreate(&model.Source{Name: "cam", Kind: "nope", URL: "http://x"}); err == nil {
		t.Fatal("expected invalid kind")
	}
	if err := ValidateCreate(&model.Source{Name: "yt", Kind: model.KindYouTube}); err == nil {
		t.Fatal("expected url required")
	}
	if err := ValidateCreate(&model.Source{Name: "file", Kind: model.KindFile}); err == nil {
		t.Fatal("expected upload or path")
	}

	id := uint(3)
	s = &model.Source{Name: "file", Kind: model.KindFile, Options: model.SourceOptions{UploadID: &id}}
	if err := ValidateCreate(s); err != nil {
		t.Fatal(err)
	}

	s = &model.Source{Name: "cam", Kind: model.KindHLS, URL: "http://example/live.m3u8", Options: model.SourceOptions{Transport: "tcp"}}
	if err := ValidateCreate(s); err == nil {
		t.Fatal("transport only for rtsp")
	}

	s = &model.Source{Name: "cam", Kind: model.KindRTSP, URL: "rtsp://x", Options: model.SourceOptions{Transport: "warp"}}
	if err := ValidateCreate(s); err == nil {
		t.Fatal("invalid transport")
	}
}

func TestValidateUpdate(t *testing.T) {
	t.Parallel()
	s := &model.Source{Name: "cam", Kind: model.KindRTSP, URL: "rtsp://x", Options: model.SourceOptions{Transport: "udp"}}
	if err := ValidateUpdate(s); err != nil {
		t.Fatal(err)
	}
}
