//go:build !unix

package pot

import (
	"os/exec"
	"time"
)

func prepareManaged(cmd *exec.Cmd) {
	cmd.WaitDelay = time.Second
}
