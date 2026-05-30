package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/updater/apply"
	ghrelease "github.com/GoSeoTaxi/cli-ssh2proxy/internal/updater/github"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/updater/semver"
)

const (
	defaultAppName = "ssh2proxy"
	releaseRepo    = "GoSeoTaxi/cli-ssh2proxy"
	lockMaxAge     = 15 * time.Minute
)

var (
	ErrLockTaken   = errors.New("updater lock is already taken")
	nowFn          = time.Now
	processAliveFn = isProcessAlive
)

type Options struct {
	Enabled         bool
	AllowPrerelease bool
	Timeout         time.Duration
	CurrentVersion  string
	AppName         string
	Args            []string
	ProcessID       int
	ExecutablePath  string
	APIBaseURL      string
	HTTPClient      *http.Client
}

type Result struct {
	Checked              bool
	UpdateAvailable      bool
	Updated              bool
	CurrentVersion       string
	LatestVersion        string
	WindowsHelperStarted bool
}

func CheckAndUpdate(ctx context.Context, opts Options) (Result, error) {
	if !opts.Enabled {
		return Result{}, nil
	}

	trimmedCurrent := strings.TrimSpace(opts.CurrentVersion)
	if trimmedCurrent == "" || trimmedCurrent == "dev" {
		return Result{}, nil
	}
	currentVersion, err := semver.Parse(trimmedCurrent)
	if err != nil {
		return Result{}, fmt.Errorf("parse current version %q: %w", trimmedCurrent, err)
	}

	exePath := strings.TrimSpace(opts.ExecutablePath)
	if exePath == "" {
		exePath, err = os.Executable()
		if err != nil {
			return Result{}, fmt.Errorf("detect executable path: %w", err)
		}
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return Result{}, fmt.Errorf("resolve executable path: %w", err)
	}

	lock, err := acquireLock(exePath + ".update.lock")
	if err != nil {
		if errors.Is(err, ErrLockTaken) {
			return Result{}, nil
		}
		return Result{}, err
	}
	defer lock.Release()

	if opts.ProcessID <= 0 {
		opts.ProcessID = os.Getpid()
	}
	appName := strings.TrimSpace(opts.AppName)
	if appName == "" {
		appName = defaultAppName
	}

	var client *ghrelease.Client
	if strings.TrimSpace(opts.APIBaseURL) != "" {
		client, err = ghrelease.NewClientWithBaseURL(releaseRepo, opts.Timeout, "ssh2proxy-updater/"+trimmedCurrent, opts.APIBaseURL)
	} else {
		client, err = ghrelease.NewClient(releaseRepo, opts.Timeout, "ssh2proxy-updater/"+trimmedCurrent)
	}
	if err != nil {
		return Result{}, fmt.Errorf("create github client: %w", err)
	}
	client.SetHTTPClient(opts.HTTPClient)
	release, err := client.LatestRelease(ctx, opts.AllowPrerelease)
	if err != nil {
		return Result{}, fmt.Errorf("load latest release: %w", err)
	}

	latestVersion, err := semver.Parse(release.TagName)
	if err != nil {
		return Result{}, fmt.Errorf("parse release tag %q: %w", release.TagName, err)
	}

	result := Result{
		Checked:        true,
		CurrentVersion: currentVersion.String(),
		LatestVersion:  latestVersion.String(),
	}

	if semver.Compare(latestVersion, currentVersion) <= 0 {
		return result, nil
	}
	result.UpdateAvailable = true

	binaryAsset, err := ghrelease.SelectAssetForRuntime(release, appName, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return result, err
	}
	checksumAsset, ok := ghrelease.FindAssetByName(release, "checksums.txt")
	if !ok {
		return result, fmt.Errorf("release %q does not contain checksums.txt", release.TagName)
	}

	installResult, err := apply.DownloadVerifyAndInstall(ctx, apply.InstallParams{
		HTTPClient:        client.HTTPClient(),
		UserAgent:         client.UserAgent(),
		ExecutablePath:    exePath,
		BinaryAssetName:   binaryAsset.Name,
		BinaryAssetURL:    binaryAsset.BrowserDownloadURL,
		ChecksumsAssetURL: checksumAsset.BrowserDownloadURL,
		ProcessID:         opts.ProcessID,
		Args:              opts.Args,
	})
	if err != nil {
		return result, err
	}

	result.Updated = installResult.Updated
	result.WindowsHelperStarted = installResult.WindowsHelperStarted
	return result, nil
}

type lockHandle struct {
	path string
	file *os.File
}

func acquireLock(path string) (*lockHandle, error) {
	lock, err := createLock(path)
	if err == nil {
		return lock, nil
	}
	if !errors.Is(err, ErrLockTaken) {
		return nil, err
	}

	stale, err := isStaleLock(path)
	if err != nil {
		return nil, err
	}
	if !stale {
		return nil, ErrLockTaken
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrLockTaken
		}
		return nil, fmt.Errorf("remove stale update lock %s: %w", path, err)
	}

	lock, err = createLock(path)
	if err != nil {
		if errors.Is(err, ErrLockTaken) {
			return nil, ErrLockTaken
		}
		return nil, err
	}
	return lock, nil
}

func (l *lockHandle) Release() {
	if l == nil {
		return
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	if l.path != "" {
		_ = os.Remove(l.path)
	}
}

func createLock(path string) (*lockHandle, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, ErrLockTaken
		}
		return nil, fmt.Errorf("acquire update lock %s: %w", path, err)
	}
	if _, err := f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write update lock %s: %w", path, err)
	}
	return &lockHandle{path: path, file: f}, nil
}

func isStaleLock(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("open update lock %s: %w", path, err)
	}
	defer f.Close()

	initialInfo, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("stat update lock %s: %w", path, err)
	}

	pid, hasPID, err := readLockPID(f)
	if err != nil {
		return false, fmt.Errorf("read update lock %s: %w", path, err)
	}

	staleByAge := nowFn().Sub(initialInfo.ModTime()) > lockMaxAge
	staleByPID := false
	if !hasPID {
		staleByPID = true
	} else {
		alive, err := processAliveFn(pid)
		if err != nil {
			return false, fmt.Errorf("check update lock pid %d: %w", pid, err)
		}
		staleByPID = !alive
	}

	if !staleByAge && !staleByPID {
		return false, nil
	}

	currentInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat update lock before reclaim %s: %w", path, err)
	}
	if !os.SameFile(initialInfo, currentInfo) {
		return false, nil
	}

	return true, nil
}

func readLockPID(f *os.File) (int, bool, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, false, err
	}
	body, err := io.ReadAll(io.LimitReader(f, 64))
	if err != nil {
		return 0, false, err
	}
	pidText := strings.TrimSpace(string(body))
	if pidText == "" {
		return 0, false, nil
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil || pid <= 0 {
		return 0, false, nil
	}
	return pid, true, nil
}
