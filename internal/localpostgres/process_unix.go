//go:build !windows

package localpostgres

import (
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

func configureCommand(cmd *exec.Cmd) {}

func containProcess(process *os.Process) (func(), error) { return func() {}, nil }

func lockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}
