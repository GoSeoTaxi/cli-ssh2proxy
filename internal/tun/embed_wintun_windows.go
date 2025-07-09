//go:build windows
// +build windows

package tun

import (
	"embed"
	"io/ioutil"
	"os"
	"path/filepath"
)

//go:embed wintun/wintun.dll
var wintunDLL embed.FS

func ensureWintun() (string, error) {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	dst := filepath.Join(dir, "wintun.dll")
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	data, _ := wintunDLL.ReadFile("wintun/wintun.dll")
	return dst, ioutil.WriteFile(dst, data, 0o644)
}
