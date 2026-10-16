package portal

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Healthcheck performs only an HTTP readiness read. It does not construct the
// application or run migrations, and works in distroless container images.
func Healthcheck(addr, defaultPort string) bool {
	if addr == "" {
		addr = ":" + defaultPort
	}
	host, port, e := net.SplitHostPort(addr)
	if e != nil {
		return false
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Get("http://" + net.JoinHostPort(strings.Trim(host, "[]"), port) + "/readyz")
	if e != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode == 200
}
