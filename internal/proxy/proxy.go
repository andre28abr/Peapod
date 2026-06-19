// Package proxy is an allowlisting HTTP/HTTPS forward proxy for sandboxes:
// only requests to allowed domains (or their subdomains) are forwarded; anything
// else gets 403. Point a sandbox at it via HTTP_PROXY / HTTPS_PROXY.
package proxy

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Proxy enforces a domain allowlist.
type Proxy struct {
	allow []string
}

// New builds a proxy that permits the given domains (and their subdomains).
func New(allow []string) *Proxy {
	var clean []string
	for _, d := range allow {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			clean = append(clean, d)
		}
	}
	return &Proxy{allow: clean}
}

func (p *Proxy) allowed(hostport string) bool {
	host := strings.ToLower(hostport)
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	for _, d := range p.allow {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// ServeHTTP implements http.Handler (used as a forward proxy).
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if r.Method == http.MethodConnect {
		host = r.URL.Host // CONNECT target is in the request URI
	}
	if !p.allowed(host) {
		http.Error(w, "blocked by peapod firewall: "+host, http.StatusForbidden)
		return
	}
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	r.RequestURI = ""
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// connect tunnels an HTTPS CONNECT request after the allowlist check.
func (p *Proxy) connect(w http.ResponseWriter, r *http.Request) {
	dst, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		dst.Close()
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	src, _, err := hj.Hijack()
	if err != nil {
		dst.Close()
		return
	}
	_, _ = src.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	go func() { _, _ = io.Copy(dst, src); dst.Close() }()
	_, _ = io.Copy(src, dst)
	src.Close()
}

// ListenAndServe runs the proxy on addr.
func (p *Proxy) ListenAndServe(addr string) error {
	srv := &http.Server{Addr: addr, Handler: p, ReadHeaderTimeout: 10 * time.Second}
	return srv.ListenAndServe()
}
