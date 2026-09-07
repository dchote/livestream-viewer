package resolver

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAvailable(t *testing.T) {
	t.Parallel()
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "yt-dlp" {
				return "/bin/yt-dlp", nil
			}
			return "", errors.New("missing")
		},
	}
	yt, ffprobe, ffmpeg := tools.Available()
	if !yt || ffprobe || ffmpeg {
		t.Fatalf("yt=%v ffprobe=%v ffmpeg=%v", yt, ffprobe, ffmpeg)
	}
}

func TestResolveYouTube(t *testing.T) {
	t.Parallel()
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "yt-dlp" {
				return "/bin/yt-dlp", nil
			}
			return "", errors.New("missing")
		},
		Command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("https://manifest.example/index.m3u8\n"), nil
		},
	}
	u, err := tools.ResolveYouTube(context.Background(), "https://youtube.com/watch?v=x")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://manifest.example/index.m3u8" {
		t.Fatalf("url = %q", u)
	}
}

func TestResolveYouTubeMissing(t *testing.T) {
	t.Parallel()
	tools := Tools{LookPath: func(string) (string, error) { return "", errors.New("missing") }}
	if _, err := tools.ResolveYouTube(context.Background(), "https://youtube.com/x"); err == nil {
		t.Fatal("expected missing binary")
	}
}

func TestResolveYouTubeTimeout(t *testing.T) {
	t.Parallel()
	tools := Tools{
		LookPath: func(string) (string, error) { return "/bin/yt-dlp", nil },
		Command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := tools.ResolveYouTube(ctx, "https://youtube.com/x"); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestProbeOpenURL(t *testing.T) {
	t.Parallel()
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "ffprobe" {
				return "/bin/ffprobe", nil
			}
			return "", errors.New("missing")
		},
		Command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte(`{"streams":[{"codec_name":"h264","width":1920,"height":1080,"r_frame_rate":"30/1"}]}`), nil
		},
	}
	codec, w, h, fps, err := tools.ProbeOpenURL(context.Background(), "rtsp://x")
	if err != nil {
		t.Fatal(err)
	}
	if codec != "h264" || w != 1920 || h != 1080 || fps != 30 {
		t.Fatalf("got %s %dx%d %f", codec, w, h, fps)
	}
}
