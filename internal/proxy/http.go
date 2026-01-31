package proxy

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
)

const (
	lifeMax = 60 * time.Minute
)

func NewHTTP(listen, user, pass string, dial sshclient.DialFunc) (*http.Server, error) {
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
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		hj, _ := w.(http.Hijacker)
		src, _, _ := hj.Hijack()
		_, _ = io.WriteString(src, "HTTP/1.1 200 OK\r\n\r\n")
		go copyBoth(dst, src)
	})
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}

	srv := &http.Server{Addr: listen, Handler: h}
	go func() {
		zap.L().Info("HTTP proxy listening on", zap.String("listen", listen))
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zap.L().Fatal("HTTP proxy Serve error", zap.Error(err))
		}
	}()
	return srv, nil
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
