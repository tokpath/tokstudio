package modelruntime

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"github.com/maximhq/bifrost/core/network"
	"github.com/maximhq/bifrost/core/schemas"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The embedded SDK does not expose a provider DialContext hook. Its documented
// HTTP proxy setting lets us enforce the actual dial boundary without changing
// TLS SNI or trusting a second DNS lookup after the platform URL check.
type egressProxy struct {
	server        *http.Server
	listener      net.Listener
	token         string
	allowLoopback bool
	mu            sync.Mutex
	connections   map[net.Conn]bool
}

func newEgressProxy(allowLoopback bool) (*egressProxy, error) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	raw := make([]byte, 32)
	if _, e = rand.Read(raw); e != nil {
		listener.Close()
		return nil, e
	}
	p := &egressProxy{listener: listener, token: hex.EncodeToString(raw), allowLoopback: allowLoopback, connections: map[net.Conn]bool{}}
	p.server = &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = p.server.Serve(listener) }()
	return p, nil
}
func (p *egressProxy) config() *schemas.ProxyConfig {
	return &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://" + p.listener.Addr().String()), Username: schemas.NewSecretVar("runtime"), Password: schemas.NewSecretVar(p.token)}
}
func (p *egressProxy) close() {
	_ = p.server.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for conn := range p.connections {
		_ = conn.Close()
	}
}
func (p *egressProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, password, ok := r.BasicAuth()
	if !ok {
		clone := r.Clone(r.Context())
		clone.Header = r.Header.Clone()
		clone.Header.Set("Authorization", r.Header.Get("Proxy-Authorization"))
		user, password, ok = clone.BasicAuth()
	}
	if !ok || user != "runtime" || subtle.ConstantTimeCompare([]byte(password), []byte(p.token)) != 1 {
		http.Error(w, "proxy authorization required", 407)
		return
	}
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT required", 405)
		return
	}
	host, port, e := net.SplitHostPort(r.Host)
	if e != nil || host == "" || port == "" {
		http.Error(w, "invalid destination", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ips, e := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if e != nil || len(ips) == 0 {
		http.Error(w, "destination unavailable", 502)
		return
	}
	for _, ip := range ips {
		if !network.IsPublicIP(ip) && !(p.allowLoopback && ip.IsLoopback() && (strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil)) {
			http.Error(w, "destination denied", 403)
			return
		}
	}
	var upstream net.Conn
	for _, ip := range ips {
		upstream, e = (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
		if e == nil {
			break
		}
	}
	if e != nil || upstream == nil {
		http.Error(w, "destination unavailable", 502)
		return
	}
	defer upstream.Close()
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "tunnel unavailable", 500)
		return
	}
	client, buffer, e := hijacker.Hijack()
	if e != nil {
		return
	}
	defer client.Close()
	p.mu.Lock()
	p.connections[client] = true
	p.connections[upstream] = true
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.connections, client); delete(p.connections, upstream); p.mu.Unlock() }()
	if _, e = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); e != nil {
		return
	}
	if e = buffer.Flush(); e != nil {
		return
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(upstream, buffer); _ = upstream.Close(); close(done) }()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
	<-done
}
