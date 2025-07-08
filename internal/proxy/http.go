package proxy

import (
	"io"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/GoSeoTaxi/cli-ssh2proxy/internal/sshclient"
)

const (
	lifeMax = 60 * time.Minute
)

func NewHTTP(listen string, dial sshclient.DialFunc) *http.Server {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := &http.Server{Addr: listen, Handler: h}
	go func() {
		zap.L().Info("HTTP proxy listening on", zap.String("listen", listen))
		_ = srv.ListenAndServe()
	}()
	return srv
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
