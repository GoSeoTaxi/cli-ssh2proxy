package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	timeOutResolve = 3 * time.Second
)

type dnsJSON struct {
	Answer []struct {
		Data string `json:"data"`
		Type int    `json:"type"`
	} `json:"Answer"`
}

type DNSResolver struct {
	servers    []string
	dial       func(ctx context.Context, netw, addr string) (net.Conn, error)
	v6         bool
	httpClient *http.Client
}

func (r *DNSResolver) ResolveBoot(parent context.Context, name string) (context.Context, net.IP, error) {
	return r.resolveInternal(parent, name, plainDial, http.DefaultClient)
}

func (r *DNSResolver) Resolve(parent context.Context, name string) (context.Context, net.IP, error) {
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

	ctx, cancel := context.WithTimeout(parent, timeOutResolve)
	defer cancel()

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
				req.Header.Set("Accept", "application/dns-json, application/dns-message")

				resp, err := httpCl.Do(req)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
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
				return nil, fmt.Errorf("no %s record in DoH response from %s", wantType, srv)
			}

			res := &net.Resolver{
				PreferGo: true,
				Dial: func(c context.Context, _, _ string) (net.Conn, error) {
					return dialFn(c, "tcp", srv)
				},
			}
			ips, err := res.LookupIP(childCtx, wantNet, name)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no %s record from %s", wantType, srv)
			}
			return ips[0], nil
		}()

		if err == nil {
			return ctx, ip, nil
		}
		lastErr = err
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
	}
}
