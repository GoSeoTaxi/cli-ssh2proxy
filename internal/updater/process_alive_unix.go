//go:build !windows

package updater

import "syscall"

func isProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, 0)
	switch err {
	case nil:
		return true, nil
	case syscall.EPERM:
		return true, nil
	case syscall.ESRCH:
		return false, nil
	default:
		return false, err
	}
}
