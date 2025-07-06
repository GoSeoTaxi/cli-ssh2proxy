package tun

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"
)

//go:embed bins/tun2socks-linux_amd64
var binLinux []byte

//go:embed bins/tun2socks-darwin_arm64
var binDarwinARM []byte

//go:embed bins/tun2socks-darwin_amd64
var binDarwinAMD []byte

//go:embed bins/tun2socks-windows_amd64.exe
var binWin []byte

const confTpl = `
device: {{.Device}}
proxy: socks5://{{.Proxy}}
ifconfig:
  - 10.0.0.2/30
  - 10.0.0.1
route:
  - 0.0.0.0/1
  - 128.0.0.0/1
loglevel: info
`

type confData struct {
	Device string
	Proxy  string
}

func RunExternal(socksAddr string) (*exec.Cmd, error) {
	data, name, err := pickBinary()
	if err != nil {
		return nil, err
	}

	binPath, err := writeTemp(name, data, runtime.GOOS != "windows")
	if err != nil {
		return nil, err
	}

	cfgPath, err := writeConfig(confData{
		Device: pickDevice(),
		Proxy:  socksAddr,
	})
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(binPath, "-config", cfgPath)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func pickBinary() (data []byte, name string, err error) {
	switch runtime.GOOS {
	case "linux":
		return binLinux, "tun2socks", nil
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return binDarwinARM, "tun2socks", nil
		}
		return binDarwinAMD, "tun2socks", nil
	case "windows":
		return binWin, "tun2socks.exe", nil
	default:
		return nil, "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func writeTemp(name string, data []byte, chmod bool) (string, error) {
	tmpDir := os.TempDir()
	path := filepath.Join(tmpDir, name)
	if err := os.WriteFile(path, data, 0o755); err != nil {
		return "", err
	}
	if chmod {
		_ = os.Chmod(path, 0o755)
	}
	return path, nil
}

func writeConfig(d confData) (string, error) {
	tmp, err := os.CreateTemp("", "t2s-*.yaml")
	if err != nil {
		return "", err
	}
	tmpl := template.Must(template.New("").Parse(confTpl))
	if err = tmpl.Execute(tmp, d); err != nil {
		return "", err
	}
	tmp.Close()
	return tmp.Name(), nil
}

func pickDevice() string {
	switch runtime.GOOS {
	case "windows":
		return "wintun://auto"
	case "darwin":
		return "utun"
	default:
		return "tun"
	}
}
