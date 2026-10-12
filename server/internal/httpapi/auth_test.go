package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"nexus/internal/auth"
)

func TestLoginSucceedsWithCorrectPassword(t *testing.T) {
	h := newHarness(t)
	// newHarness already logged in; a cookie should be present.
	if h.cookie == nil || h.cookie.Value == "" {
		t.Fatal("expected a session cookie after login")
	}
	if !h.cookie.HttpOnly {
		t.Error("session cookie should be HttpOnly")
	}
	if h.cookie.SameSite != http.SameSiteStrictMode {
		t.Error("session cookie should be SameSite=Strict")
	}
}

func TestLoginFailsWithWrongPassword(t *testing.T) {
	h := newHarness(t)
	body, _ := json.Marshal(map[string]string{"password": "wrong"})
	resp, err := http.Post(h.ts.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	mustStatus(t, resp, http.StatusUnauthorized)
}

func TestProtectedEndpointRejectsUnauthenticated(t *testing.T) {
	h := newHarness(t)
	resp := h.doWithCookie(http.MethodGet, "/api/auth/me", nil, nil)
	mustStatus(t, resp, http.StatusUnauthorized)
}

func TestProtectedEndpointRejectsInvalidToken(t *testing.T) {
	h := newHarness(t)
	bad := &http.Cookie{Name: auth.CookieName, Value: "not-a-jwt"}
	resp := h.doWithCookie(http.MethodGet, "/api/auth/me", nil, bad)
	mustStatus(t, resp, http.StatusUnauthorized)
}

func TestMeReturnsAuthenticatedState(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodGet, "/api/auth/me", nil)
	mustStatus(t, resp, http.StatusOK)
	var out map[string]interface{}
	decode(t, resp, &out)
	if out["authenticated"] != true {
		t.Errorf("expected authenticated=true, got %v", out)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/auth/logout", nil)
	mustStatus(t, resp, http.StatusOK)
	// The logout response should expire the cookie.
	var cleared bool
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("expected logout to expire the session cookie")
	}
}
