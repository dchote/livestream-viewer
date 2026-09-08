//go:build !unix

package resolver

import (
	"os/exec"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = cancelWait
}
