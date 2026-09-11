package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peapod/internal/driver/mock"
	"peapod/internal/sandbox"
)

func newTestHandler() http.Handler {
	return Handler(sandbox.NewManager(mock.New()))
}

func post(h http.Handler, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7070"+path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestCSRFGuard: a foreign web page must not be able to drive the local API. A
// text/plain POST (what a cross-site fetch sends without preflight) and a
// cross-site JSON POST are refused; the dashboard's own same-origin call passes.
func TestCSRFGuard(t *testing.T) {
	h := newTestHandler()
	body := `{"image":"alpine"}`

	if rec := post(h, "/api/sandboxes", body, map[string]string{"Content-Type": "text/plain"}); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain POST: status %d, want 415", rec.Code)
	}
	if rec := post(h, "/api/sandboxes", body, map[string]string{
		"Content-Type": "application/json", "Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example",
	}); rec.Code != http.StatusForbidden {
		t.Errorf("cross-site JSON POST: status %d, want 403", rec.Code)
	}
	if rec := post(h, "/api/destroy", `{"id":"x"}`, map[string]string{
		"Content-Type": "application/json", "Origin": "https://evil.example",
	}); rec.Code != http.StatusForbidden {
		t.Errorf("foreign-Origin destroy: status %d, want 403", rec.Code)
	}

	rec := post(h, "/api/sandboxes", body, map[string]string{
		"Content-Type": "application/json", "Sec-Fetch-Site": "same-origin", "Origin": "http://127.0.0.1:7070",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("same-origin JSON POST: status %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"pp_`) {
		t.Errorf("create response missing id: %s", rec.Body.String())
	}
}

// TestGetSandboxes: read endpoints stay open — harmless, and the page needs them.
func TestGetSandboxes(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7070/api/sandboxes", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sandboxes"`) {
		t.Errorf("GET /api/sandboxes: %d %s", rec.Code, rec.Body.String())
	}
}
