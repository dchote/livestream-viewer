package resolver

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/dchote/livestream-viewer/internal/source/youtube"
)

const defaultTimeout = 25 * time.Second

// youtubeFormat prefers a single HLS video URL. YouTube live streams no longer
// publish muxed (audio+video) formats, so `best` fails with "Requested format
// is not available". The wall is video-only, so video-only HLS is the right
// pick; muxed HLS and non-HLS remain as fallbacks for VOD.
const youtubeFormat = "bv*[protocol*=m3u8]/b[protocol*=m3u8]/bv*/b"

// LookPathFunc locates an executable on PATH.
type LookPathFunc func(file string) (string, error)

// CommandFunc starts a subprocess.
type CommandFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// Tools discovers optional binaries and the optional YouTube session files.
type Tools struct {
	LookPath           LookPathFunc
	Command            CommandFunc
	CookiesFile        string // Netscape jar; omitted from argv when missing
	CookiesFromBrowser string // yt-dlp --cookies-from-browser value; ignored when CookiesFile exists
	POTokenFile        string
	POTBaseURL         string // BgUtils provider; omitted from extractor-args when empty or the default
	PluginDir          string // bgutil yt-dlp plugin; omitted when empty or POT is off
}

func (t Tools) lookPath(file string) (string, error) {
	if t.LookPath != nil {
		return t.LookPath(file)
	}
	return exec.LookPath(file)
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

// withTimeout always bounds a subprocess. Decode workers pass their own
// cancellation context, which has no deadline: without this a hung yt-dlp or
// ffprobe would wedge the calling worker for the life of the process.
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

// ResolveYouTube returns a playable video HLS URL. The result must not be persisted.
func (t Tools) ResolveYouTube(ctx context.Context, pageURL string) (string, error) {
	bin, err := t.lookPath("yt-dlp")
	if err != nil {
		return "", fmt.Errorf("yt-dlp is not installed")
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	out, err := t.command(ctx, bin, t.youtubeArgs(pageURL)...)
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

func (t Tools) youtubeArgs(pageURL string) []string {
	args := []string{"-g", "--no-warnings"}
	if t.PluginDir != "" && strings.TrimSpace(t.POTBaseURL) != "" {
		args = append(args, "--plugin-dirs", t.PluginDir, "--plugin-dirs", "default")
	}
	if youtube.CookiesConfigured(t.CookiesFile) {
		args = append(args, "--cookies", t.CookiesFile)
	} else if b := strings.TrimSpace(t.CookiesFromBrowser); b != "" {
		args = append(args, "--cookies-from-browser", b)
	}
	if tok := youtube.ReadPOTokenFile(t.POTokenFile); tok != "" {
		args = append(args, "--extractor-args", "youtube:po_token="+tok)
	}
	if u := strings.TrimRight(strings.TrimSpace(t.POTBaseURL), "/"); u != "" && u != "http://127.0.0.1:4416" {
		args = append(args, "--extractor-args", "youtubepot-bgutilhttp:base_url="+u)
	}
	return append(args, "-f", youtubeFormat, pageURL)
}
