package proxy

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8": true, "1.1.1.1": true, "2606:4700:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false,
		"::1": false, "fd00::1": false, "fe80::1": false, "::ffff:10.0.0.1": false,
	}
	for s, want := range cases {
		if got := isPublicIP(net.ParseIP(s)); got != want {
			t.Errorf("isPublicIP(%s) = %v, want %v", s, got, want)
		}
	}
}

// TestPrivateDestinationsBlocked: even an allowed domain must not reach a
// private or loopback address (SSRF / DNS-rebinding hardening) — on both the
// CONNECT (HTTPS) and the plain-HTTP paths. Opting in lifts the block.
func TestPrivateDestinationsBlocked(t *testing.T) {
	p := New([]string{"localhost"})

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodConnect, "localhost:1", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("CONNECT to loopback: status %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost:1/", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("GET via proxy to loopback: status %d, want 403", rec.Code)
	}

	// With AllowPrivate the dial is attempted (and fails on the closed port with
	// a 502, not a 403).
	rec = httptest.NewRecorder()
	New([]string{"localhost"}).AllowPrivate(true).ServeHTTP(rec, httptest.NewRequest(http.MethodConnect, "localhost:1", nil))
	if rec.Code == http.StatusForbidden {
		t.Errorf("AllowPrivate must not 403 (got %d)", rec.Code)
	}
}

// TestAllowed is the heart of the domain firewall: only listed domains and their
// subdomains pass; look-alikes and substrings must be rejected.
func TestAllowed(t *testing.T) {
	p := New([]string{"pypi.org", " GitHub.com "}) // New trims + lowercases
	cases := []struct {
		host string
		ok   bool
	}{
		{"pypi.org", true},                // exact
		{"sub.pypi.org", true},            // subdomain
		{"PyPI.org:443", true},            // case-insensitive + port stripped
		{"github.com", true},              // trimmed entry
		{"api.github.com", true},          // subdomain of trimmed entry
		{"files.pythonhosted.org", false}, // unrelated
		{"evil.com", false},               // unrelated
		{"notpypi.org", false},            // substring, not a subdomain
		{"pypi.org.evil.com", false},      // suffix spoof
	}
	for _, c := range cases {
		if got := p.allowed(c.host); got != c.ok {
			t.Errorf("allowed(%q) = %v, want %v", c.host, got, c.ok)
		}
	}
}

// TestServeHTTPBlocks verifies a disallowed host gets 403 (and is never fetched).
func TestServeHTTPBlocks(t *testing.T) {
	p := New([]string{"allowed.test"})
	req := httptest.NewRequest(http.MethodGet, "http://blocked.test/pkg", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("blocked host: status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
