package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

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

// --- Login hardening ---

// fakeClock is a manually advanced time source for lockout tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// loginAttempt posts a password with optional extra headers and returns the
// response (body unread).
func (h *harness) loginAttempt(password string, headers map[string]string) *http.Response {
	h.t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	req, _ := http.NewRequest(http.MethodPost, h.ts.URL+"/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("login request: %v", err)
	}
	return resp
}

func testPolicy() auth.LimitPolicy {
	return auth.LimitPolicy{MaxFailures: 3, Window: 10 * time.Minute, Lockout: time.Minute}
}

func TestLoginLockoutAfterRepeatedFailures(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	h := newHarnessAuth(t, auth.WithClock(clock.now), auth.WithLimitPolicy(testPolicy()))

	// Two failures → plain 401.
	for i := 0; i < 2; i++ {
		r := h.loginAttempt("wrong", nil)
		mustStatus(t, r, http.StatusUnauthorized)
		r.Body.Close()
	}
	// The third reaches the threshold → 429 with Retry-After.
	r := h.loginAttempt("wrong", nil)
	mustStatus(t, r, http.StatusTooManyRequests)
	r.Body.Close()
	if got := r.Header.Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60", got)
	}

	// While locked, even the correct password is refused.
	clock.advance(30 * time.Second)
	r = h.loginAttempt(testPassword, nil)
	mustStatus(t, r, http.StatusTooManyRequests)
	r.Body.Close()
	if got := r.Header.Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After mid-lockout = %q, want 30", got)
	}

	// After the cooldown the correct password works again.
	clock.advance(31 * time.Second)
	r = h.loginAttempt(testPassword, nil)
	mustStatus(t, r, http.StatusOK)
	r.Body.Close()
}

func TestLoginCorrectPasswordBeforeThresholdResetsCount(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	h := newHarnessAuth(t, auth.WithClock(clock.now), auth.WithLimitPolicy(testPolicy()))

	for i := 0; i < 2; i++ {
		r := h.loginAttempt("wrong", nil)
		mustStatus(t, r, http.StatusUnauthorized)
		r.Body.Close()
	}
	r := h.loginAttempt(testPassword, nil)
	mustStatus(t, r, http.StatusOK)
	r.Body.Close()

	// The success cleared the count: two more failures still aren't a lockout.
	for i := 0; i < 2; i++ {
		r := h.loginAttempt("wrong", nil)
		mustStatus(t, r, http.StatusUnauthorized)
		r.Body.Close()
	}
}

func TestLoginFailureWindowExpires(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	h := newHarnessAuth(t, auth.WithClock(clock.now), auth.WithLimitPolicy(testPolicy()))

	for i := 0; i < 2; i++ {
		r := h.loginAttempt("wrong", nil)
		mustStatus(t, r, http.StatusUnauthorized)
		r.Body.Close()
	}
	// Failures older than the window no longer count.
	clock.advance(11 * time.Minute)
	r := h.loginAttempt("wrong", nil)
	mustStatus(t, r, http.StatusUnauthorized)
	r.Body.Close()
}

func TestLoginIgnoresForwardedHeadersByDefault(t *testing.T) {
	h := newHarnessAuth(t, auth.WithLimitPolicy(testPolicy()))

	// Rotating X-Forwarded-For must not dodge the limiter when the proxy
	// isn't trusted: every attempt is keyed on the real RemoteAddr.
	var last *http.Response
	for i := 0; i < 3; i++ {
		last = h.loginAttempt("wrong", map[string]string{"X-Forwarded-For": fmt.Sprintf("203.0.113.%d", i)})
		last.Body.Close()
	}
	if last.StatusCode != http.StatusTooManyRequests {
		t.Errorf("spoofed X-Forwarded-For evaded throttling: status %d", last.StatusCode)
	}
}

func TestLoginHonorsForwardedHeadersWhenTrusted(t *testing.T) {
	h := newHarnessAuth(t, auth.WithLimitPolicy(testPolicy()), auth.WithTrustedProxy(true))

	// Lock out one forwarded client…
	for i := 0; i < 3; i++ {
		r := h.loginAttempt("wrong", map[string]string{"X-Forwarded-For": "198.51.100.7"})
		r.Body.Close()
	}
	r := h.loginAttempt(testPassword, map[string]string{"X-Forwarded-For": "198.51.100.7"})
	mustStatus(t, r, http.StatusTooManyRequests)
	r.Body.Close()

	// …while a different client behind the same proxy is unaffected, both via
	// X-Forwarded-For and via X-Real-IP.
	r = h.loginAttempt(testPassword, map[string]string{"X-Forwarded-For": "198.51.100.8"})
	mustStatus(t, r, http.StatusOK)
	r.Body.Close()
	r = h.loginAttempt(testPassword, map[string]string{"X-Real-IP": "198.51.100.9"})
	mustStatus(t, r, http.StatusOK)
	r.Body.Close()
}

func TestSessionCookieSecureFlag(t *testing.T) {
	secure := newHarness(t)
	if !secure.cookie.Secure {
		t.Error("session cookie should be Secure by default")
	}
	resp := secure.do(http.MethodPost, "/api/auth/logout", nil)
	resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && !c.Secure {
			t.Error("clearing cookie should carry the same Secure attribute")
		}
	}

	insecure := newHarnessAuth(t, auth.WithCookieSecure(false))
	if insecure.cookie.Secure {
		t.Error("session cookie should not be Secure when disabled")
	}
	resp = insecure.do(http.MethodPost, "/api/auth/logout", nil)
	resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Secure {
			t.Error("clearing cookie should not be Secure when disabled")
		}
	}
}
