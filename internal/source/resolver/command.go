package resolver

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

const versionTimeout = 2 * time.Second

// cancelWait is how long to wait after signalling the process group before
// Go's CommandContext kills the parent PID as a fallback.
const cancelWait = time.Second

// ExecError is a failed yt-dlp or ffprobe run. Error() prefers stderr so
// Classify still matches extractor dumps after wrapping.
type ExecError struct {
	Err      error
	ExitCode int
	Stdout   string
	Stderr   string
}

func (e *ExecError) Error() string {
	if e == nil {
		return ""
	}
	if s := strings.TrimSpace(e.Stderr); s != "" {
		return s
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "command failed"
}

func (e *ExecError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (t Tools) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	if t.Command != nil {
		return t.Command(ctx, name, args...)
	}
	return runCommand(ctx, name, args...)
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	prepareCommand(cmd)
	if err := cmd.Run(); err != nil {
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		return stdout.Bytes(), &ExecError{
			Err:      err,
			ExitCode: exitCode,
			Stdout:   strings.TrimSpace(stdout.String()),
			Stderr:   strings.TrimSpace(stderr.String()),
		}
	}
	return stdout.Bytes(), nil
}

// YtDlpVersion returns `yt-dlp --version`, or "" if the binary is missing.
func (t Tools) YtDlpVersion(ctx context.Context) string {
	bin, err := t.lookPath("yt-dlp")
	if err != nil {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	out, err := t.command(ctx, bin, "--version")
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	return line
}
