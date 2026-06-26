package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
