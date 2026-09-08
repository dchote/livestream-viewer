//go:build unix

package pot

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// prepareManaged puts the Deno POT server in its own process group so a
// cancelled context reaps Deno children the same way yt-dlp is reaped.
func prepareManaged(cmd *exec.Cmd) {
	cmd.WaitDelay = time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		pid := cmd.Process.Pid
		if pid <= 0 {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-pid, syscall.SIGKILL)
		if err == nil {
			return nil
		}
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}
