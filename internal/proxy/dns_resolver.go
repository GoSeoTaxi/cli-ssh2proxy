// internal/proxy/dns_resolver.go
package proxy

import (
	"context"
	"net"
)

type DNSResolver struct {
	servers []string
	dial    func(ctx context.Context, netw, addr string) (net.Conn, error)
}

func newDNSResolver(servers []string,
	dial func(ctx context.Context, netw, addr string) (net.Conn, error)) *DNSResolver {
	return &DNSResolver{servers: servers, dial: dial}
}

func (r *DNSResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	var lastErr error
	for _, srv := range r.servers {
		d := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return r.dial(ctx, "udp", srv)
			},
		}
		ips, err := d.LookupIP(ctx, "ip", name)
		if err == nil && len(ips) > 0 {
			return ctx, ips[0], nil
		}
		lastErr = err
	}
	return ctx, nil, lastErr
}
