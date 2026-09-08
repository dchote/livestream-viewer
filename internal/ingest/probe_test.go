package ingest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
)

func TestInspectFileFixture(t *testing.T) {
	_, this, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(this), "testdata", "tiny.mp4")
	dir := t.TempDir()
	s := &model.Source{ID: 7, Name: "fixture", Kind: model.KindFile, URL: path, Enabled: true}
	got := Inspect(context.Background(), ProbeInput{Source: s, Caps: capability.Info{}, ThumbDir: dir})
	if got.Status != model.ProbeOK {
		t.Fatalf("probe %+v", got)
	}
	if got.Codec != "h264" || got.Width != 64 || got.Height != 64 {
		t.Fatalf("geometry %+v", got)
	}
	if got.HWDecode == nil || *got.HWDecode {
		t.Fatalf("fixture should report software on empty caps: %+v", got.HWDecode)
	}
	if _, err := os.Stat(filepath.Join(dir, "7.jpg")); err != nil {
		t.Fatalf("thumbnail (%s): %v", got.Message, err)
	}
}

func TestInspectYouTubeMissingYtDlp(t *testing.T) {
	t.Parallel()
	s := &model.Source{Name: "yt", Kind: model.KindYouTube, URL: "https://youtube.com/watch?v=x"}
	got := Inspect(context.Background(), ProbeInput{
		Source: s,
		Tools:  resolver.Tools{LookPath: func(string) (string, error) { return "", errors.New("missing") }},
	})
	if got.Status != model.ProbeUnavailable {
		t.Fatalf("status %+v", got)
	}
	if got.Code != resolver.CodeToolMissing {
		t.Fatalf("code %q", got.Code)
	}
}

func TestInspectYouTubeResolveError(t *testing.T) {
	t.Parallel()
	s := &model.Source{Name: "yt", Kind: model.KindYouTube, URL: "https://youtube.com/watch?v=x"}
	got := Inspect(context.Background(), ProbeInput{
		Source: s,
		Tools: resolver.Tools{
			LookPath: func(file string) (string, error) {
				if file == "yt-dlp" {
					return "/bin/yt-dlp", nil
				}
				return "", errors.New("missing")
			},
			Command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				return nil, errors.New("Sign in to confirm you’re not a bot. Use --cookies-from-browser or --cookies for the authentication.")
			},
		},
	})
	if got.Status != model.ProbeError {
		t.Fatalf("status %+v", got)
	}
	if got.Code != resolver.CodeYouTubeBotCheck {
		t.Fatalf("code %q", got.Code)
	}
	if !strings.Contains(got.Message, "treating this host as a bot") {
		t.Fatalf("message %q", got.Message)
	}
	if strings.Contains(got.Message, "ERROR:") {
		t.Fatalf("probe stored the extractor dump: %q", got.Message)
	}
}

func TestInspectYouTubeGenericResolveError(t *testing.T) {
	t.Parallel()
	s := &model.Source{Name: "yt", Kind: model.KindYouTube, URL: "https://youtube.com/watch?v=x"}
	got := Inspect(context.Background(), ProbeInput{
		Source: s,
		Tools: resolver.Tools{
			LookPath: func(file string) (string, error) {
				if file == "yt-dlp" {
					return "/bin/yt-dlp", nil
				}
				return "", errors.New("missing")
			},
			Command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				return nil, errors.New("Requested format is not available")
			},
		},
	})
	if got.Status != model.ProbeError {
		t.Fatalf("status %+v", got)
	}
	if got.Code != "" {
		t.Fatalf("generic errors have no code, got %q", got.Code)
	}
	if !strings.Contains(got.Message, "Requested format is not available") {
		t.Fatalf("message %q", got.Message)
	}
}

// RTSP credentials are injected into the open URL, and libav diagnostics
// sometimes echo that URL. The probe result is persisted on the source row and
// returned over the API, so it must not carry the password.
func TestProbeMessageRedactsCredentials(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "rtsp userinfo",
			in:   "open: rtsp://admin:s3cret@cam.local:554/stream: Connection refused",
			want: "open: rtsp://***@cam.local:554/stream: Connection refused",
		},
		{
			name: "password only",
			in:   "rtsps://:hunter2@10.0.0.9/live failed",
			want: "rtsps://***@10.0.0.9/live failed",
		},
		{
			name: "no credentials is untouched",
			in:   "open: rtsp://cam.local:554/stream: Connection refused",
			want: "open: rtsp://cam.local:554/stream: Connection refused",
		},
		{
			name: "plain message is untouched",
			in:   "no video stream",
			want: "no video stream",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := probeMessage(tc.in); got != tc.want {
				t.Fatalf("probeMessage(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestProbeMessageIsBounded(t *testing.T) {
	t.Parallel()
	got := probeMessage(strings.Repeat("x", 4096))
	if len(got) > maxProbeMessage+3 {
		t.Fatalf("message length %d exceeds the cap", len(got))
	}
}
