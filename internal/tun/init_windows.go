//go:build windows

package tun

import (
	"log"
	"syscall"
)

func init() {
	dll, err := ensureWintun()
	if err != nil {
		log.Fatalf("wintun: %v", err)
	}

	_, _ = syscall.LoadDLL(dll)
}
