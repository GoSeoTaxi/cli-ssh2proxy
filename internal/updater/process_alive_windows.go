//go:build windows

package updater

func isProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	// Portable process liveness checks are limited on Windows in this package,
	// so lock reclamation relies on mtime unless PID is invalid.
	return true, nil
}
