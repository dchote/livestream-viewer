package resolver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeout = 25 * time.Second

// LookPathFunc locates an executable on PATH.
type LookPathFunc func(file string) (string, error)

// CommandFunc starts a subprocess.
type CommandFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// Tools discovers optional binaries.
type Tools struct {
	LookPath LookPathFunc
	Command  CommandFunc
}

func (t Tools) lookPath(file string) (string, error) {
	if t.LookPath != nil {
		return t.LookPath(file)
	}
	return exec.LookPath(file)
}

func (t Tools) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	if t.Command != nil {
		return t.Command(ctx, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("%s", msg)
	}
	return stdout.Bytes(), nil
}

// Available reports which optional tools are on PATH.
func (t Tools) Available() (ytDlp, ffprobe, ffmpeg bool) {
	_, err := t.lookPath("yt-dlp")
	ytDlp = err == nil
	_, err = t.lookPath("ffprobe")
	ffprobe = err == nil
	_, err = t.lookPath("ffmpeg")
	ffmpeg = err == nil
	return
}

// ResolveYouTube returns a playable HLS manifest URL. The result must not be persisted.
func (t Tools) ResolveYouTube(ctx context.Context, pageURL string) (string, error) {
	bin, err := t.lookPath("yt-dlp")
	if err != nil {
		return "", fmt.Errorf("yt-dlp is not installed")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
	}
	out, err := t.command(ctx, bin, "-g", "--no-warnings", "-f", "best[protocol*=m3u8]", pageURL)
	if err != nil {
		return "", fmt.Errorf("yt-dlp: %w", err)
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if line == "" {
		return "", fmt.Errorf("yt-dlp returned no URL")
	}
	return line, nil
}

type ffprobeStreams struct {
	Streams []struct {
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		FrameRate string `json:"r_frame_rate"`
		AvgRate   string `json:"avg_frame_rate"`
	} `json:"streams"`
}

// ProbeOpenURL runs ffprobe against a libav-openable URL.
func (t Tools) ProbeOpenURL(ctx context.Context, openURL string) (codec string, width, height int, fps float64, err error) {
	bin, err := t.lookPath("ffprobe")
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("ffprobe is not installed")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
	}
	out, err := t.command(ctx, bin, "-v", "quiet", "-print_format", "json", "-show_streams", "-select_streams", "v:0", openURL)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("ffprobe: %w", err)
	}
	var parsed ffprobeStreams
	if err := json.Unmarshal(out, &parsed); err != nil {
		return "", 0, 0, 0, fmt.Errorf("ffprobe json: %w", err)
	}
	if len(parsed.Streams) == 0 {
		return "", 0, 0, 0, fmt.Errorf("no video stream")
	}
	st := parsed.Streams[0]
	fps = parseFrameRate(st.FrameRate)
	if fps == 0 {
		fps = parseFrameRate(st.AvgRate)
	}
	return st.CodecName, st.Width, st.Height, fps, nil
}

func parseFrameRate(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0/0" {
		return 0
	}
	var a, b float64
	if _, err := fmt.Sscanf(s, "%f/%f", &a, &b); err == nil && b != 0 {
		return a / b
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err == nil {
		return v
	}
	return 0
}
