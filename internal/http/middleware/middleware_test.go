package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// okHandler is a simple 200 OK handler used as the "next" in middleware chains.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
})

// ---------------------------------------------------------------------------
// APIKeyAuth
// ---------------------------------------------------------------------------

func TestAPIKeyAuth_DisabledWhenEmpty(t *testing.T) {
	h := APIKeyAuth("")(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (auth disabled)", rec.Code)
	}
}

func TestAPIKeyAuth_RejectsWhenNoKey(t *testing.T) {
	h := APIKeyAuth("secret-key")(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAPIKeyAuth_AcceptsXAPIKeyHeader(t *testing.T) {
	h := APIKeyAuth("my-secret")(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "my-secret")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAPIKeyAuth_AcceptsBearerToken(t *testing.T) {
	h := APIKeyAuth("bearer-key")(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bearer-key")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAPIKeyAuth_RejectsWrongKey(t *testing.T) {
	h := APIKeyAuth("correct-key")(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAPIKeyAuth_ResponseBodyIsJSON(t *testing.T) {
	h := APIKeyAuth("key")(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "unauthorized") {
		t.Errorf("body %q should contain 'unauthorized'", body)
	}
}

// ---------------------------------------------------------------------------
// SecurityHeaders
// ---------------------------------------------------------------------------

func TestSecurityHeaders_SetsRequiredHeaders(t *testing.T) {
	h := SecurityHeaders()(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	checks := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"X-XSS-Protection":       "1; mode=block",
	}
	for header, want := range checks {
		got := rec.Header().Get(header)
		if got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}
}

func TestSecurityHeaders_SetsCSP(t *testing.T) {
	h := SecurityHeaders()(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header not set")
	}
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP %q missing default-src 'self'", csp)
	}
}

func TestSecurityHeaders_PassesThroughToNext(t *testing.T) {
	h := SecurityHeaders()(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want 'ok'", body)
	}
}

// ---------------------------------------------------------------------------
// UIAuth / session signing
// ---------------------------------------------------------------------------

func TestSignSession_Roundtrip(t *testing.T) {
	const username = "operator"
	const secret = "test-secret-32-bytes-long-enough"

	token := signSession(username, secret)
	if !validSession(token, username, secret) {
		t.Errorf("validSession(%q) = false, want true", token)
	}
}

func TestValidSession_ReturnsFalseForTamperedToken(t *testing.T) {
	const username = "operator"
	const secret = "test-secret"

	token := signSession(username, secret)
	// Flip the last character
	tampered := token[:len(token)-1] + "X"
	if validSession(tampered, username, secret) {
		t.Error("validSession returned true for tampered token")
	}
}

func TestValidSession_ReturnsFalseForMalformed(t *testing.T) {
	if validSession("no-colon-here", "operator", "secret") {
		t.Error("validSession returned true for malformed token (no colon)")
	}
	if validSession("", "operator", "secret") {
		t.Error("validSession returned true for empty token")
	}
}

func TestUIAuth_DisabledWhenUsernameEmpty(t *testing.T) {
	h := UIAuth("", "", "")(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (auth disabled)", rec.Code)
	}
}

func TestUIAuth_RedirectsWhenNoCookie(t *testing.T) {
	h := UIAuth("operator", "password", "secret")(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/transfers", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 redirect", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/ui/login") {
		t.Errorf("redirect location %q should contain /ui/login", loc)
	}
}

func TestUIAuth_AllowsValidSession(t *testing.T) {
	const username = "operator"
	const secret = "test-secret"
	token := signSession(username, secret)

	h := UIAuth(username, "password", secret)(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/transfers", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestUIAuth_RejectsInvalidSession(t *testing.T) {
	h := UIAuth("operator", "password", "secret")(okHandler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/transfers", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "bad-token"})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 redirect for invalid session", rec.Code)
	}
}
