package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/updater/semver"
)

const (
	defaultBaseURL = "https://api.github.com"
	defaultUA      = "ssh2proxy-updater"
	maxBodyBytes   = 2 << 20
)

type Client struct {
	repo       string
	baseURL    string
	userAgent  string
	retryCount int
	httpClient *http.Client
}

type Release struct {
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	Prerelease bool    `json:"prerelease"`
	Draft      bool    `json:"draft"`
	Assets     []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

func (c *Client) UserAgent() string {
	return c.userAgent
}

func (c *Client) SetHTTPClient(httpClient *http.Client) {
	if httpClient != nil {
		c.httpClient = httpClient
	}
}

func NewClient(repo string, timeout time.Duration, userAgent string) (*Client, error) {
	return newClient(repo, timeout, userAgent, defaultBaseURL)
}

func NewClientWithBaseURL(repo string, timeout time.Duration, userAgent, baseURL string) (*Client, error) {
	return newClient(repo, timeout, userAgent, baseURL)
}

func newClient(repo string, timeout time.Duration, userAgent, baseURL string) (*Client, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return nil, errors.New("repository must not be empty")
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, fmt.Errorf("repository must be in owner/name format: %q", repo)
	}

	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = defaultUA
	}

	return &Client{
		repo:       repo,
		baseURL:    strings.TrimRight(baseURL, "/"),
		userAgent:  userAgent,
		retryCount: 3,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) LatestRelease(ctx context.Context, allowPrerelease bool) (Release, error) {
	if allowPrerelease {
		var releases []Release
		endpoint := c.releaseListURL()
		if err := c.getJSON(ctx, endpoint, &releases); err != nil {
			return Release{}, err
		}
		best, err := selectNewestBySemver(releases, true)
		if err != nil {
			return Release{}, fmt.Errorf("no valid release found in %s: %w", endpoint, err)
		}
		return best, nil
	}

	var release Release
	endpoint := c.latestReleaseURL()
	if err := c.getJSON(ctx, endpoint, &release); err != nil {
		return Release{}, err
	}
	return release, nil
}

func (c *Client) latestReleaseURL() string {
	return fmt.Sprintf("%s/repos/%s/releases/latest", c.baseURL, path.Clean(c.repo))
}

func (c *Client) releaseListURL() string {
	return fmt.Sprintf("%s/repos/%s/releases", c.baseURL, path.Clean(c.repo))
}

func (c *Client) getJSON(ctx context.Context, url string, out interface{}) error {
	var lastErr error
	for attempt := 1; attempt <= c.retryCount; attempt++ {
		err := c.tryGetJSON(ctx, url, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == c.retryCount || !isRetryable(err) {
			break
		}
		backoff := time.Duration(attempt) * 250 * time.Millisecond
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func (c *Client) tryGetJSON(ctx context.Context, url string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if readErr != nil {
		return fmt.Errorf("read response %s: %w", url, readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return &requestError{
			StatusCode: resp.StatusCode,
			URL:        url,
			Message:    msg,
		}
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode JSON from %s: %w", url, err)
	}
	return nil
}

func selectNewestBySemver(releases []Release, allowPrerelease bool) (Release, error) {
	var (
		bestRelease Release
		bestVersion semver.Version
		hasBest     bool
	)

	for _, rel := range releases {
		if rel.Draft {
			continue
		}
		if !allowPrerelease && rel.Prerelease {
			continue
		}
		v, err := semver.Parse(rel.TagName)
		if err != nil {
			continue
		}
		if !hasBest || semver.Compare(v, bestVersion) > 0 {
			bestRelease = rel
			bestVersion = v
			hasBest = true
		}
	}
	if !hasBest {
		return Release{}, errors.New("semver-tagged releases not found")
	}
	return bestRelease, nil
}

func SelectAssetForRuntime(release Release, appName, goos, goarch string) (Asset, error) {
	expected := ExpectedAssetName(appName, goos, goarch)
	for _, asset := range release.Assets {
		if asset.Name == expected {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("release %q has no asset %q", release.TagName, expected)
}

func FindAssetByName(release Release, name string) (Asset, bool) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return Asset{}, false
}

func ExpectedAssetName(appName, goos, goarch string) string {
	name := fmt.Sprintf("%s-%s_%s", strings.TrimSpace(appName), strings.TrimSpace(goos), strings.TrimSpace(goarch))
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

type requestError struct {
	StatusCode int
	URL        string
	Message    string
}

func (e *requestError) Error() string {
	return fmt.Sprintf("github api %s returned %d: %s", e.URL, e.StatusCode, e.Message)
}

func isRetryable(err error) bool {
	var rerr *requestError
	if errors.As(err, &rerr) {
		return rerr.StatusCode == http.StatusTooManyRequests || rerr.StatusCode >= 500
	}
	return true
}
