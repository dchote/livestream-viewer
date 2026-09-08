package resolver

import (
	"context"
	"errors"
	"os"
	"os/exec"
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
	var args []string
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "yt-dlp" {
				return "/bin/yt-dlp", nil
			}
			return "", errors.New("missing")
		},
		Command: func(ctx context.Context, name string, got ...string) ([]byte, error) {
			args = append([]string(nil), got...)
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
	// Live streams only expose video-only HLS; `best` (muxed) is not available.
	if !containsArg(args, "-f", youtubeFormat) {
		t.Fatalf("args = %v, want -f %q", args, youtubeFormat)
	}
	if containsFlag(args, "--cookies") {
		t.Fatalf("args = %v, unconfigured session must not pass --cookies", args)
	}
}

func TestResolveYouTubePassesCookiesAndPOToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cookies := dir + "/youtube.cookies"
	token := dir + "/youtube.po_token"
	if err := os.WriteFile(cookies, []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tA\tB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(token, []byte("web.gvs+abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var args []string
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "yt-dlp" {
				return "/bin/yt-dlp", nil
			}
			return "", errors.New("missing")
		},
		Command: func(ctx context.Context, name string, got ...string) ([]byte, error) {
			args = append([]string(nil), got...)
			return []byte("https://manifest.example/index.m3u8\n"), nil
		},
		CookiesFile: cookies,
		POTokenFile: token,
	}
	if _, err := tools.ResolveYouTube(context.Background(), "https://youtube.com/watch?v=x"); err != nil {
		t.Fatal(err)
	}
	if !containsArg(args, "--cookies", cookies) {
		t.Fatalf("args = %v, want --cookies %q", args, cookies)
	}
	if !containsArg(args, "--extractor-args", "youtube:po_token=web.gvs+abc") {
		t.Fatalf("args = %v, want po_token extractor-args", args)
	}
	if containsArg(args, "--cookies", "https://youtube.com/watch?v=x") {
		t.Fatal("page URL must not be passed as the cookies path")
	}
}

func TestResolveYouTubePOTBaseURL(t *testing.T) {
	t.Parallel()
	var args []string
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "yt-dlp" {
				return "/bin/yt-dlp", nil
			}
			return "", errors.New("missing")
		},
		Command: func(ctx context.Context, name string, got ...string) ([]byte, error) {
			args = append([]string(nil), got...)
			return []byte("https://manifest.example/index.m3u8\n"), nil
		},
		POTBaseURL: "http://127.0.0.1:4416",
		PluginDir:  "/tmp/yt-dlp-plugins",
	}
	if _, err := tools.ResolveYouTube(context.Background(), "https://youtube.com/watch?v=x"); err != nil {
		t.Fatal(err)
	}
	if containsArg(args, "--extractor-args", "youtubepot-bgutilhttp:base_url=http://127.0.0.1:4416") {
		t.Fatalf("default POT URL must not be passed, args %v", args)
	}
	if !containsArg(args, "--plugin-dirs", "/tmp/yt-dlp-plugins") {
		t.Fatalf("args = %v, want --plugin-dirs", args)
	}
	if !containsArg(args, "--plugin-dirs", "default") {
		t.Fatalf("args = %v, want --plugin-dirs default", args)
	}

	args = nil
	tools.POTBaseURL = "http://127.0.0.1:8080"
	if _, err := tools.ResolveYouTube(context.Background(), "https://youtube.com/watch?v=x"); err != nil {
		t.Fatal(err)
	}
	if !containsArg(args, "--extractor-args", "youtubepot-bgutilhttp:base_url=http://127.0.0.1:8080") {
		t.Fatalf("args = %v, want custom base_url", args)
	}
}

func TestResolveYouTubeCookiesFromBrowser(t *testing.T) {
	t.Parallel()
	var args []string
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "yt-dlp" {
				return "/bin/yt-dlp", nil
			}
			return "", errors.New("missing")
		},
		Command: func(ctx context.Context, name string, got ...string) ([]byte, error) {
			args = append([]string(nil), got...)
			return []byte("https://manifest.example/index.m3u8\n"), nil
		},
		CookiesFromBrowser: "chrome",
	}
	if _, err := tools.ResolveYouTube(context.Background(), "https://youtube.com/watch?v=x"); err != nil {
		t.Fatal(err)
	}
	if !containsArg(args, "--cookies-from-browser", "chrome") {
		t.Fatalf("args = %v, want --cookies-from-browser chrome", args)
	}

	dir := t.TempDir()
	cookies := dir + "/youtube.cookies"
	if err := os.WriteFile(cookies, []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tA\tB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args = nil
	tools.CookiesFile = cookies
	if _, err := tools.ResolveYouTube(context.Background(), "https://youtube.com/watch?v=x"); err != nil {
		t.Fatal(err)
	}
	if containsFlag(args, "--cookies-from-browser") {
		t.Fatal("cookie file must win over --cookies-from-browser")
	}
	if !containsArg(args, "--cookies", cookies) {
		t.Fatalf("args = %v, want --cookies", args)
	}
}

func TestResolveYouTubeOmitsPluginWhenPOTOff(t *testing.T) {
	t.Parallel()
	var args []string
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "yt-dlp" {
				return "/bin/yt-dlp", nil
			}
			return "", errors.New("missing")
		},
		Command: func(ctx context.Context, name string, got ...string) ([]byte, error) {
			args = append([]string(nil), got...)
			return []byte("https://manifest.example/index.m3u8\n"), nil
		},
		PluginDir: "/tmp/yt-dlp-plugins",
	}
	if _, err := tools.ResolveYouTube(context.Background(), "https://youtube.com/watch?v=x"); err != nil {
		t.Fatal(err)
	}
	if containsFlag(args, "--plugin-dirs") {
		t.Fatalf("plugin-dirs without POT URL would stall on /ping, args %v", args)
	}
}

func containsFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func containsArg(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
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

// Available drives the ffprobe field of GET /system/info, so a missing binary
// must be reported rather than assumed present.
func TestAvailableReportsMissingTools(t *testing.T) {
	t.Parallel()
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "ffmpeg" {
				return "/bin/ffmpeg", nil
			}
			return "", errors.New("missing")
		},
	}
	yt, ffprobe, ffmpeg := tools.Available()
	if yt || ffprobe {
		t.Fatalf("expected yt-dlp and ffprobe absent, got %v %v", yt, ffprobe)
	}
	if !ffmpeg {
		t.Fatal("expected ffmpeg present")
	}
}

func TestExecErrorPrefersStderr(t *testing.T) {
	t.Parallel()
	err := &ExecError{Err: context.DeadlineExceeded, ExitCode: -1, Stderr: "Sign in to confirm you're not a bot"}
	if err.Error() != "Sign in to confirm you're not a bot" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Unwrap")
	}
	code, _ := Classify("yt-dlp: " + err.Error())
	if code != CodeYouTubeBotCheck {
		t.Fatalf("code = %q", code)
	}
}

func TestYtDlpVersion(t *testing.T) {
	t.Parallel()
	missing := Tools{LookPath: func(string) (string, error) { return "", errors.New("missing") }}
	if got := missing.YtDlpVersion(context.Background()); got != "" {
		t.Fatalf("missing version = %q", got)
	}
	tools := Tools{
		LookPath: func(file string) (string, error) {
			if file == "yt-dlp" {
				return "/bin/yt-dlp", nil
			}
			return "", errors.New("missing")
		},
		Command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if len(args) != 1 || args[0] != "--version" {
				t.Fatalf("args = %v", args)
			}
			return []byte("2026.08.19\n"), nil
		},
	}
	if got := tools.YtDlpVersion(context.Background()); got != "2026.08.19" {
		t.Fatalf("version = %q", got)
	}
}

func TestRunCommandCapturesStderr(t *testing.T) {
	t.Parallel()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	_, err = runCommand(context.Background(), sh, "-c", "echo out; echo err >&2; exit 7")
	var ex *ExecError
	if !errors.As(err, &ex) {
		t.Fatalf("got %v", err)
	}
	if ex.ExitCode != 7 {
		t.Fatalf("exit = %d", ex.ExitCode)
	}
	if ex.Stderr != "err" {
		t.Fatalf("stderr = %q", ex.Stderr)
	}
	if ex.Stdout != "out" {
		t.Fatalf("stdout = %q", ex.Stdout)
	}
}
