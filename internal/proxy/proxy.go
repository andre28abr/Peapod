// Package proxy is an allowlisting HTTP/HTTPS forward proxy for sandboxes:
// only requests to allowed domains (or their subdomains) are forwarded; anything
// else gets 403. Point a sandbox at it via HTTP_PROXY / HTTPS_PROXY.
//
// Destinations that resolve to private, loopback or link-local addresses are
// refused even for allowed domains (SSRF / DNS-rebinding hardening) unless
// AllowPrivate is set: a sandbox's allowlist is about the public internet, and a
// domain the user trusts today may point at 127.0.0.1 or the Docker host tomorrow.
package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Proxy enforces a domain allowlist.
type Proxy struct {
	allow        []string
	allowPrivate bool
	transport    *http.Transport
}

// New builds a proxy that permits the given domains (and their subdomains).
func New(allow []string) *Proxy {
	var clean []string
	for _, d := range allow {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			clean = append(clean, d)
		}
	}
	p := &Proxy{allow: clean}
	p.transport = &http.Transport{DialContext: p.dial}
	return p
}

// AllowPrivate lets allowed domains resolve to private/loopback addresses (for
// example a dev server on the host). Off by default.
func (p *Proxy) AllowPrivate(v bool) *Proxy {
	p.allowPrivate = v
	return p
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

var errPrivateDest = errors.New("destination resolves to a private, loopback or link-local address")

// dial resolves addr and connects only to public addresses, dialing the
// resolved IP (not the name again) so a DNS answer can't change between the
// check and the connect.
func (p *Proxy) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	lastErr := error(errPrivateDest)
	for _, ip := range ips {
		if !p.allowPrivate && !isPublicIP(ip.IP) {
			continue
		}
		c, err := d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// isPublicIP reports whether ip is a globally routable unicast address.
func isPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0: // 0.0.0.0/8
			return false
		case v4[0] == 100 && v4[1]&0xc0 == 64: // 100.64.0.0/10 carrier-grade NAT
			return false
		case v4[0] == 192 && v4[1] == 0 && v4[2] == 0: // 192.0.0.0/24
			return false
		case v4[0] == 198 && v4[1]&0xfe == 18: // 198.18.0.0/15 benchmarking
			return false
		case v4[0] >= 240: // 240.0.0.0/4 reserved + broadcast
			return false
		}
	}
	return true
}

func blocked(w http.ResponseWriter, why string) {
	http.Error(w, "blocked by peapod firewall: "+why, http.StatusForbidden)
}

// ServeHTTP implements http.Handler (used as a forward proxy).
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if r.Method == http.MethodConnect {
		host = r.URL.Host // CONNECT target is in the request URI
	}
	if !p.allowed(host) {
		blocked(w, host)
		return
	}
	if r.Method == http.MethodConnect {
		p.connect(w, r, host)
		return
	}
	r.RequestURI = ""
	resp, err := p.transport.RoundTrip(r)
	if err != nil {
		if errors.Is(err, errPrivateDest) {
			blocked(w, host+" ("+err.Error()+")")
			return
		}
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

// connect tunnels an HTTPS CONNECT request after the allowlist check. It dials
// the checked host (the CONNECT target), never a header the client controls.
func (p *Proxy) connect(w http.ResponseWriter, r *http.Request, host string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	dst, err := p.dial(ctx, "tcp", host)
	cancel()
	if err != nil {
		if errors.Is(err, errPrivateDest) {
			blocked(w, host+" ("+err.Error()+")")
			return
		}
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
