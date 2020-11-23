// +build !windows


package dccli

import (
	"os/exec"
	"syscall"
)

func AssignProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
}
