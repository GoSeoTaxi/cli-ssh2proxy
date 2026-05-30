package apply

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultUserAgent = "ssh2proxy-updater"
	maxDownloadBytes = 256 << 20 // 256 MiB hard cap for release assets.
)

var ErrDownloadTooLarge = errors.New("download exceeds hard cap")

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type InstallParams struct {
	HTTPClient        HTTPClient
	UserAgent         string
	ExecutablePath    string
	BinaryAssetName   string
	BinaryAssetURL    string
	ChecksumsAssetURL string
	ProcessID         int
	Args              []string
}

type InstallResult struct {
	Updated              bool
	WindowsHelperStarted bool
}

func DownloadVerifyAndInstall(ctx context.Context, params InstallParams) (InstallResult, error) {
	if params.HTTPClient == nil {
		return InstallResult{}, errors.New("http client is required")
	}
	if strings.TrimSpace(params.ExecutablePath) == "" {
		return InstallResult{}, errors.New("executable path is required")
	}
	if strings.TrimSpace(params.BinaryAssetName) == "" {
		return InstallResult{}, errors.New("binary asset name is required")
	}
	if strings.TrimSpace(params.BinaryAssetURL) == "" {
		return InstallResult{}, errors.New("binary asset URL is required")
	}
	if strings.TrimSpace(params.ChecksumsAssetURL) == "" {
		return InstallResult{}, errors.New("checksums asset URL is required")
	}
	if params.ProcessID <= 0 {
		return InstallResult{}, errors.New("process id must be positive")
	}

	ua := strings.TrimSpace(params.UserAgent)
	if ua == "" {
		ua = defaultUserAgent
	}

	exePath, err := filepath.Abs(params.ExecutablePath)
	if err != nil {
		return InstallResult{}, fmt.Errorf("resolve executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	checksumBytes, err := downloadToBytes(ctx, params.HTTPClient, params.ChecksumsAssetURL, ua)
	if err != nil {
		return InstallResult{}, fmt.Errorf("download checksums: %w", err)
	}
	checksums, err := ParseChecksums(checksumBytes)
	if err != nil {
		return InstallResult{}, fmt.Errorf("parse checksums: %w", err)
	}
	expectedHash, ok := checksums[params.BinaryAssetName]
	if !ok {
		return InstallResult{}, fmt.Errorf("checksums.txt does not contain %q", params.BinaryAssetName)
	}

	tmpFile, err := os.CreateTemp(exeDir, filepath.Base(exePath)+".update-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()
	if closeErr := tmpFile.Close(); closeErr != nil {
		_ = os.Remove(tmpPath)
		return InstallResult{}, fmt.Errorf("close temporary file: %w", closeErr)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if err := downloadToPath(ctx, params.HTTPClient, params.BinaryAssetURL, ua, tmpPath); err != nil {
		return InstallResult{}, fmt.Errorf("download binary asset: %w", err)
	}
	if err := VerifyFileSHA256(tmpPath, expectedHash); err != nil {
		return InstallResult{}, fmt.Errorf("checksum verification failed: %w", err)
	}

	if runtime.GOOS == "windows" {
		if err := stageWindowsUpdate(exePath, tmpPath, params.ProcessID, params.Args); err != nil {
			return InstallResult{}, err
		}
		return InstallResult{Updated: true, WindowsHelperStarted: true}, nil
	}

	if err := replaceUnixExecutable(exePath, tmpPath); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Updated: true}, nil
}

func ParseChecksums(content []byte) (map[string]string, error) {
	out := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("invalid checksums line %d: %q", lineNo, line)
		}

		hash := strings.ToLower(strings.TrimSpace(fields[0]))
		if len(hash) != 64 || !isHex(hash) {
			return nil, fmt.Errorf("invalid checksum hash on line %d: %q", lineNo, fields[0])
		}
		name := strings.TrimSpace(strings.Join(fields[1:], " "))
		name = strings.TrimPrefix(name, "*")
		name = strings.TrimPrefix(name, "./")
		if name == "" {
			return nil, fmt.Errorf("empty file name on line %d", lineNo)
		}

		out[name] = hash
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	if len(out) == 0 {
		return nil, errors.New("checksums file is empty")
	}
	return out, nil
}

func VerifyFileSHA256(filePath, expectedHex string) error {
	expected := strings.ToLower(strings.TrimSpace(expectedHex))
	if len(expected) != 64 || !isHex(expected) {
		return fmt.Errorf("invalid expected checksum %q", expectedHex)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash %s: %w", filePath, err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if sum != expected {
		return fmt.Errorf("expected %s, got %s", expected, sum)
	}
	return nil
}

func replaceUnixExecutable(executablePath, downloadedPath string) error {
	mode := os.FileMode(0o755)
	if info, err := os.Stat(executablePath); err == nil {
		mode = info.Mode().Perm()
	}
	if mode&0o111 == 0 {
		mode |= 0o111
	}
	if err := os.Chmod(downloadedPath, mode); err != nil {
		return fmt.Errorf("set executable permissions: %w", err)
	}
	if err := os.Rename(downloadedPath, executablePath); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	return nil
}

func stageWindowsUpdate(executablePath, downloadedPath string, pid int, args []string) error {
	stagedPath := executablePath + ".new"
	_ = os.Remove(stagedPath)
	if err := os.Rename(downloadedPath, stagedPath); err != nil {
		return fmt.Errorf("stage windows update: %w", err)
	}
	if err := startWindowsHelper(executablePath, stagedPath, pid, args); err != nil {
		return fmt.Errorf("launch windows update helper: %w", err)
	}
	return nil
}

func startWindowsHelper(executablePath, stagedPath string, pid int, args []string) error {
	scriptPath := executablePath + ".update.ps1"
	psArgs := make([]string, 0, len(args))
	for _, arg := range args {
		psArgs = append(psArgs, fmt.Sprintf("'%s'", psSingleQuoted(arg)))
	}

	script := strings.Join([]string{
		fmt.Sprintf("$target = '%s'", psSingleQuoted(executablePath)),
		fmt.Sprintf("$staged = '%s'", psSingleQuoted(stagedPath)),
		fmt.Sprintf("$pidToWait = %d", pid),
		fmt.Sprintf("$argsToPass = @(%s)", strings.Join(psArgs, ",")),
		"while (Get-Process -Id $pidToWait -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 250 }",
		"Move-Item -LiteralPath $staged -Destination $target -Force",
		"Start-Process -FilePath $target -ArgumentList $argsToPass",
		"Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue",
	}, "\n")

	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return fmt.Errorf("write helper script: %w", err)
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

func psSingleQuoted(v string) string {
	return strings.ReplaceAll(v, "'", "''")
}

func downloadToBytes(ctx context.Context, client HTTPClient, url, userAgent string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := readAllWithHardLimit(resp.Body, maxDownloadBytes)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func downloadToPath(ctx context.Context, client HTTPClient, url, userAgent, filePath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := copyWithHardLimit(f, resp.Body, maxDownloadBytes); err != nil {
		return err
	}
	return nil
}

func readAllWithHardLimit(src io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("invalid hard cap %d", maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(src, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%w: max=%d", ErrDownloadTooLarge, maxBytes)
	}
	return body, nil
}

func copyWithHardLimit(dst io.Writer, src io.Reader, maxBytes int64) error {
	if maxBytes <= 0 {
		return fmt.Errorf("invalid hard cap %d", maxBytes)
	}
	written, err := io.Copy(dst, io.LimitReader(src, maxBytes+1))
	if err != nil {
		return err
	}
	if written > maxBytes {
		return fmt.Errorf("%w: max=%d", ErrDownloadTooLarge, maxBytes)
	}
	return nil
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
