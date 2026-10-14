// Package auth handles the single-password login: verifying the Owner's
// password against a bcrypt hash, throttling repeated failures per client,
// issuing session JWTs carried in an HttpOnly SameSite=Strict cookie, and the
// middleware that protects the API.
package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// CookieName is the name of the session cookie.
const CookieName = "nexus_session"

// SessionTTL is the lifetime of an issued session (~7 days per spec).
const SessionTTL = 7 * 24 * time.Hour

// ErrInvalidToken is returned when a session token is missing, malformed, or
// expired.
var ErrInvalidToken = errors.New("invalid session token")

// Authenticator verifies passwords, throttles failed logins, and
// mints/validates session tokens.
type Authenticator struct {
	passwordHash []byte
	signingKey   []byte
	// now is injectable so tests can control token expiry and lockouts.
	now func() time.Time
	// cookieSecure marks the session cookie Secure (HTTPS-only).
	cookieSecure bool
	// trustProxy honors X-Forwarded-For / X-Real-IP when resolving the
	// client IP; off by default so headers can't spoof the throttle key.
	trustProxy bool
	policy     LimitPolicy
	limiter    *Limiter
}

// Option configures an Authenticator.
type Option func(*Authenticator)

// WithClock injects the time source used for tokens and lockouts.
func WithClock(now func() time.Time) Option {
	return func(a *Authenticator) { a.now = now }
}

// WithCookieSecure sets the session cookie's Secure flag (default true).
func WithCookieSecure(secure bool) Option {
	return func(a *Authenticator) { a.cookieSecure = secure }
}

// WithTrustedProxy makes ClientIP honor forwarded headers (default false).
func WithTrustedProxy(trust bool) Option {
	return func(a *Authenticator) { a.trustProxy = trust }
}

// WithLimitPolicy sets the failed-login throttling policy.
func WithLimitPolicy(p LimitPolicy) Option {
	return func(a *Authenticator) { a.policy = p }
}

// New builds an Authenticator from the bcrypt password hash and JWT signing
// key. By default the cookie is Secure, forwarded headers are ignored, and
// DefaultLimitPolicy applies.
func New(passwordHash string, signingKey []byte, opts ...Option) *Authenticator {
	a := &Authenticator{
		passwordHash: []byte(passwordHash),
		signingKey:   signingKey,
		now:          time.Now,
		cookieSecure: true,
		policy:       DefaultLimitPolicy,
	}
	for _, o := range opts {
		o(a)
	}
	a.limiter = NewLimiter(a.policy, func() time.Time { return a.now() })
	return a
}

// Limiter exposes the failed-login limiter.
func (a *Authenticator) Limiter() *Limiter { return a.limiter }

// ClientIP resolves the throttle key for a request. It uses RemoteAddr unless
// the trusted-proxy toggle is on, in which case it prefers the right-most
// X-Forwarded-For entry (the address the trusted proxy itself appended; any
// entries to its left are client-supplied and spoofable), then X-Real-IP.
func (a *Authenticator) ClientIP(r *http.Request) string {
	if a.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// VerifyPassword reports whether the supplied password matches the configured
// bcrypt hash.
func (a *Authenticator) VerifyPassword(password string) bool {
	return bcrypt.CompareHashAndPassword(a.passwordHash, []byte(password)) == nil
}

// IssueToken mints a signed session JWT valid for SessionTTL.
func (a *Authenticator) IssueToken() (string, time.Time, error) {
	now := a.now()
	expires := now.Add(SessionTTL)
	claims := jwt.RegisteredClaims{
		Subject:   "owner",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expires),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(a.signingKey)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expires, nil
}

// ParseToken validates a session token and returns nil when it is valid.
func (a *Authenticator) ParseToken(tokenString string) error {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithTimeFunc(a.now),
	)
	_, err := parser.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return a.signingKey, nil
	})
	if err != nil {
		return ErrInvalidToken
	}
	return nil
}

// SetSessionCookie writes the session cookie onto the response.
func (a *Authenticator) SetSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearSessionCookie expires the session cookie, ending the session.
func (a *Authenticator) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

type contextKey string

const authedKey contextKey = "authed"

// Middleware rejects any request without a valid session cookie.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if err := a.ParseToken(cookie.Value); err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), authedKey, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
