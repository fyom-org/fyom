//go:build !windows

package playback

import (
	"os/exec"
	"syscall"
)

// setDetach configures the command to run in a new process group so it is
// not killed when the parent process exits.
func setDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
