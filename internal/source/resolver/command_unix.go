//go:build unix

package resolver

import (
	"os"
	"os/exec"
	"syscall"
)

// prepareCommand puts yt-dlp (and its Python/Deno children) in their own
// process group so a deadline kill reaps the tree, and so SIGINT to this
// process does not also hit a cookie-prompt child.
func prepareCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = cancelWait
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
