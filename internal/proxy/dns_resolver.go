package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	timeOutResolve      = 3 * time.Second       // timeout for DNS lookup
	dnsCacheMaxEntries  = 100000                // size cache
	dnsCacheTTLSuccess  = 30 * 60 * time.Second // 30 minutes
	dnsCacheTTLNegative = 30 * time.Second      // 0.5 minutes
)

type dnsJSON struct {
	Status  int    `json:"Status"`
	Comment string `json:"Comment"`
	Answer  []struct {
		Data string `json:"data"`
		Type int    `json:"type"`
	} `json:"Answer"`
	Authority []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Authority"`
}

var errNoRecord = errors.New("dns no record")

type noRecordError struct {
	msg string
}

func (e noRecordError) Error() string {
	if e.msg == "" {
		return errNoRecord.Error()
	}
	return errNoRecord.Error() + ": " + e.msg
}

func (e noRecordError) Unwrap() error {
	return errNoRecord
}

func newNoRecordError(msg string) error {
	return noRecordError{msg: msg}
}

func noRecordMessage(err error) (string, bool) {
	var e noRecordError
	if errors.As(err, &e) {
		return e.msg, true
	}
	return "", false
}

type dnsCacheEntry struct {
	ip        net.IP
	expiresAt time.Time
	negative  bool
	errMsg    string
}

type dnsCache struct {
	mu    sync.RWMutex
	items map[string]dnsCacheEntry
}

func newDNSCache() *dnsCache {
	return &dnsCache{items: make(map[string]dnsCacheEntry, dnsCacheMaxEntries)}
}

func (c *dnsCache) Get(key string) (dnsCacheEntry, bool) {
	now := time.Now()
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return dnsCacheEntry{}, false
	}
	if now.After(entry.expiresAt) {
		c.mu.Lock()
		if cur, ok := c.items[key]; ok && now.After(cur.expiresAt) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return dnsCacheEntry{}, false
	}
	return entry, true
}

func (c *dnsCache) Set(key string, entry dnsCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		c.items[key] = entry
		return
	}
	if len(c.items) >= dnsCacheMaxEntries {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
	c.items[key] = entry
}

func (c *dnsCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

type DNSResolver struct {
	servers    []string
	dial       func(ctx context.Context, netw, addr string) (net.Conn, error)
	v6         bool
	httpClient *http.Client
	cache      *dnsCache
}

func (r *DNSResolver) CacheLen() int {
	if r == nil || r.cache == nil {
		return 0
	}
	return r.cache.Len()
}

func (r *DNSResolver) ResolveBoot(parent context.Context, name string) (context.Context, net.IP, error) {
	if ip := net.ParseIP(name); ip != nil {
		return parent, ip, nil
	}
	return r.resolveInternal(parent, name, plainDial, http.DefaultClient)
}

func (r *DNSResolver) Resolve(parent context.Context, name string) (context.Context, net.IP, error) {
	if ip := net.ParseIP(name); ip != nil {
		return parent, ip, nil
	}
	return r.resolveInternal(parent, name, r.dial, r.httpClient)
}

func plainDial(ctx context.Context, netw, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, netw, addr)
}

func (r *DNSResolver) resolveInternal(parent context.Context, name string,
	dialFn func(ctx context.Context, netw, addr string) (net.Conn, error),
	httpCl *http.Client) (context.Context, net.IP, error) {

	wantType, wantNet := "A", "ip4"
	if r.v6 {
		wantType, wantNet = "AAAA", "ip"
	}

	cacheKey := dnsCacheKey(name, wantType)

	ctx, cancel := context.WithTimeout(parent, timeOutResolve)
	keepCtx := false
	defer func() {
		if !keepCtx {
			cancel()
		}
	}()

	if r.cache != nil {
		if entry, ok := r.cache.Get(cacheKey); ok {
			if entry.negative {
				if entry.errMsg == "" {
					return ctx, nil, errNoRecord
				}
				return ctx, nil, newNoRecordError(entry.errMsg)
			}
			if entry.ip != nil {
				keepCtx = true
				return ctx, cloneIP(entry.ip), nil
			}
		}
	}

	if len(r.servers) == 0 {
		ips, err := net.DefaultResolver.LookupIP(ctx, wantNet, name)
		if err != nil {
			if isDNSNotFound(err) {
				return ctx, nil, newNoRecordError(fmt.Sprintf("no %s record from system resolver", wantType))
			}
			return ctx, nil, err
		}
		if len(ips) == 0 {
			return ctx, nil, newNoRecordError(fmt.Sprintf("no %s record from system resolver", wantType))
		}
		if r.cache != nil {
			r.cache.Set(cacheKey, dnsCacheEntry{
				ip:        cloneIP(ips[0]),
				expiresAt: time.Now().Add(dnsCacheTTLSuccess),
			})
		}
		keepCtx = true
		return ctx, ips[0], nil
	}

	var lastErr error

	for _, srv := range r.servers {
		ip, err := func() (net.IP, error) {
			childCtx, cancelChild := context.WithTimeout(ctx, timeOutResolve)
			defer cancelChild()

			if strings.HasPrefix(srv, "https://") {
				url := fmt.Sprintf("%s?name=%s&type=%s", srv, name, wantType)

				req, errReq := http.NewRequestWithContext(childCtx, http.MethodGet, url, nil)
				if errReq != nil {
					return nil, errReq
				}
				// Cloudflare returns 400 if Accept includes dns-message without wire-format "dns=" param.
				if strings.Contains(srv, "cloudflare") {
					req.Header.Set("Accept", "application/dns-json")
				} else {
					req.Header.Set("Accept", "application/dns-json, application/dns-message")
				}

				resp, err := httpCl.Do(req)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 2048))
					fields := []zap.Field{
						zap.Int("status", resp.StatusCode),
						zap.String("server", srv),
						zap.String("url", url),
					}
					if readErr != nil {
						fields = append(fields, zap.Error(readErr))
					}
					if len(bodyBytes) > 0 {
						fields = append(fields, zap.String("body", strings.TrimSpace(string(bodyBytes))))
					}
					if strings.Contains(srv, "cloudflare") {
						zap.L().Warn("doh_non_ok_cloudflare", fields...)
					} else {
						zap.L().Warn("doh_non_ok", fields...)
					}
					return nil, fmt.Errorf("DoH non-OK %d from %s", resp.StatusCode, srv)
				}

				var dj dnsJSON
				if err = json.NewDecoder(resp.Body).Decode(&dj); err != nil {
					return nil, err
				}
				for _, ans := range dj.Answer {
					if (ans.Type == 1 && !r.v6) || (ans.Type == 28 && r.v6) {
						if ip := net.ParseIP(ans.Data); ip != nil {
							return ip, nil
						}
					}
				}
				if dj.Status != 0 || dj.Comment != "" || len(dj.Authority) > 0 {
					zap.L().Warn("doh_no_answer",
						zap.String("server", srv),
						zap.String("name", name),
						zap.String("type", wantType),
						zap.Int("status", dj.Status),
						zap.String("comment", dj.Comment),
						zap.Int("authority_count", len(dj.Authority)),
					)
				}
				if dj.Status == 0 || dj.Status == 3 {
					return nil, newNoRecordError(fmt.Sprintf("no %s record in DoH response from %s", wantType, srv))
				}
				return nil, fmt.Errorf("DoH status %d from %s", dj.Status, srv)
			}

			res := &net.Resolver{
				PreferGo: true,
				Dial: func(c context.Context, _, _ string) (net.Conn, error) {
					return dialFn(c, "tcp", srv)
				},
			}
			ips, err := res.LookupIP(childCtx, wantNet, name)
			if err != nil {
				if isDNSNotFound(err) {
					return nil, newNoRecordError(fmt.Sprintf("no %s record from %s", wantType, srv))
				}
				return nil, err
			}
			if len(ips) == 0 {
				return nil, newNoRecordError(fmt.Sprintf("no %s record from %s", wantType, srv))
			}
			return ips[0], nil
		}()

		if err == nil {
			if r.cache != nil {
				r.cache.Set(cacheKey, dnsCacheEntry{
					ip:        cloneIP(ip),
					expiresAt: time.Now().Add(dnsCacheTTLSuccess),
				})
			}
			keepCtx = true
			return ctx, ip, nil
		}
		lastErr = err
	}

	if r.cache != nil && errors.Is(lastErr, errNoRecord) {
		msg, _ := noRecordMessage(lastErr)
		r.cache.Set(cacheKey, dnsCacheEntry{
			negative:  true,
			errMsg:    msg,
			expiresAt: time.Now().Add(dnsCacheTTLNegative),
		})
	}
	return ctx, nil, lastErr
}

func NewDNSResolver(servers []string, v6 bool, dial func(ctx context.Context, netw, addr string) (net.Conn, error)) *DNSResolver {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dial(ctx, network, addr)
		},
		ForceAttemptHTTP2: true,
	}

	return &DNSResolver{
		servers:    servers,
		dial:       dial,
		v6:         v6,
		httpClient: &http.Client{Transport: tr, Timeout: timeOutResolve},
		cache:      newDNSCache(),
	}
}

func dnsCacheKey(name, recordType string) string {
	cleanName := strings.ToLower(strings.TrimSuffix(name, "."))
	return recordType + "|" + cleanName
}

func cloneIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	cp := make(net.IP, len(ip))
	copy(cp, ip)
	return cp
}

func isDNSNotFound(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsNotFound
	}
	return false
}
