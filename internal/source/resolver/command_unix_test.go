//go:build unix

package resolver

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunCommandKillsProcessGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	_, err := runCommand(ctx, "/bin/sh", "-c", "sleep 30 & echo $! > '"+pidFile+"'; wait")
	if err == nil {
		t.Fatal("expected deadline")
	}

	deadline := time.Now().Add(2 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		b, readErr := os.ReadFile(pidFile)
		if readErr == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			if pid > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid <= 0 {
		t.Fatal("child pid was not written")
	}
	for time.Now().Before(deadline) {
		if killErr := syscall.Kill(pid, 0); killErr != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child pid %d still running", pid)
}
