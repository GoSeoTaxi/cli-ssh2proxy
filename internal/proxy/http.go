package proxy

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/limits"
	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
)

const (
	lifeMax               = 60 * time.Minute
	readHeaderTimeoutHTTP = 10 * time.Second
	idleTimeoutHTTP       = 60 * time.Second
)

type HTTPServer struct {
	srv   *http.Server
	errCh chan error
}

func NewHTTP(listen, user, pass string, dial sshclient.DialFunc, sessions *limits.SessionLimiter, handshakes *limits.IPRateLimiter) (*HTTPServer, error) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user != "" || pass != "" {
			if !checkProxyAuth(r, user, pass) {
				w.Header().Set("Proxy-Authenticate", `Basic realm="ssh2proxy"`)
				http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
				return
			}
		}
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
			return
		}
		dst, err := dial(r.Context(), "tcp", r.Host)
		if err != nil {
			zap.L().Warn("http_connect_dial_failed", zap.String("host", r.Host), zap.Error(err))
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			_ = dst.Close()
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			return
		}
		src, _, err := hj.Hijack()
		if err != nil {
			_ = dst.Close()
			http.Error(w, "failed to hijack connection", http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(src, "HTTP/1.1 200 OK\r\n\r\n")
		go copyBoth(dst, src)
	})
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}
	ln = limits.WrapListener(ln, limits.ListenerOptions{
		Protocol:        "http",
		SessionLimiter:  sessions,
		RateLimiter:     handshakes,
		OverLimitAction: limits.RejectHTTP503,
		RateLimitAction: limits.RejectHTTP429,
	})

	srv := &http.Server{
		Addr:              listen,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeoutHTTP,
		IdleTimeout:       idleTimeoutHTTP,
	}
	hs := &HTTPServer{
		srv:   srv,
		errCh: make(chan error, 1),
	}
	go func() {
		defer close(hs.errCh)
		zap.L().Info("HTTP proxy listening on", zap.String("listen", listen))
		if err := srv.Serve(ln); !isHTTPServeExit(err) {
			hs.errCh <- err
		}
	}()
	return hs, nil
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if s == nil || s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func (s *HTTPServer) Errors() <-chan error {
	if s == nil {
		return nil
	}
	return s.errCh
}

func checkProxyAuth(r *http.Request, user, pass string) bool {
	header := strings.TrimSpace(r.Header.Get("Proxy-Authorization"))
	if header == "" {
		return false
	}
	lower := strings.ToLower(header)
	if !strings.HasPrefix(lower, "basic ") {
		return false
	}
	encoded := strings.TrimSpace(header[len("Basic "):])
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	u, p, ok := strings.Cut(string(raw), ":")
	if !ok {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(u), []byte(user)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(p), []byte(pass)) == 1
	return userOK && passOK
}

func copyBoth(a, b net.Conn) {
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()

	done := make(chan struct{}, 2)

	go func() { io.Copy(a, b); done <- struct{}{} }()
	go func() { io.Copy(b, a); done <- struct{}{} }()

	select {
	case <-done:
	case <-time.After(lifeMax):
	}
}

func isHTTPServeExit(err error) bool {
	return err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed)
}
