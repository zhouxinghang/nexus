package auth

import (
	"sync"
	"time"
)

// LimitPolicy is the login-throttling policy: after MaxFailures consecutive
// failed logins from one client within Window, that client is locked out for
// Lockout. A successful login clears the client's count.
type LimitPolicy struct {
	MaxFailures int
	Window      time.Duration
	Lockout     time.Duration
}

// DefaultLimitPolicy is used when no policy is configured.
var DefaultLimitPolicy = LimitPolicy{
	MaxFailures: 5,
	Window:      15 * time.Minute,
	Lockout:     time.Minute,
}

// pruneThreshold is the entry count above which stale entries are swept on
// the next recorded failure, so the map can't grow without bound.
const pruneThreshold = 1024

type limiterEntry struct {
	failures       int
	firstFailureAt time.Time
	lockedUntil    time.Time
}

// Limiter tracks failed logins per client key in memory. State is
// process-local and cleared on restart — a deliberate trade-off for a
// single-process, single-Owner app (see ADR-0006).
type Limiter struct {
	policy LimitPolicy
	// now is injectable so tests can control lockout expiry.
	now func() time.Time

	mu      sync.Mutex
	entries map[string]*limiterEntry
}

// NewLimiter builds a Limiter. A nil now uses time.Now; non-positive policy
// fields fall back to DefaultLimitPolicy.
func NewLimiter(p LimitPolicy, now func() time.Time) *Limiter {
	if p.MaxFailures <= 0 {
		p.MaxFailures = DefaultLimitPolicy.MaxFailures
	}
	if p.Window <= 0 {
		p.Window = DefaultLimitPolicy.Window
	}
	if p.Lockout <= 0 {
		p.Lockout = DefaultLimitPolicy.Lockout
	}
	if now == nil {
		now = time.Now
	}
	return &Limiter{policy: p, now: now, entries: map[string]*limiterEntry{}}
}

// Allow reports whether key may attempt a login now. When it may not, it
// returns how long until the lockout lifts.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return true, 0
	}
	now := l.now()
	if now.Before(e.lockedUntil) {
		return false, e.lockedUntil.Sub(now)
	}
	if !e.lockedUntil.IsZero() {
		// The lockout elapsed: start the client over with a clean slate.
		delete(l.entries, key)
	}
	return true, 0
}

// RecordFailure counts a verified-wrong password for key. When this failure
// reaches the threshold it locks the client and returns the lockout length.
func (l *Limiter) RecordFailure(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.entries) >= pruneThreshold {
		l.pruneLocked(now)
	}
	e, ok := l.entries[key]
	if !ok || now.Sub(e.firstFailureAt) > l.policy.Window {
		e = &limiterEntry{firstFailureAt: now}
		l.entries[key] = e
	}
	e.failures++
	if e.failures >= l.policy.MaxFailures {
		e.lockedUntil = now.Add(l.policy.Lockout)
		return true, l.policy.Lockout
	}
	return false, 0
}

// Reset clears key's failure history (called after a successful login).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// pruneLocked drops entries whose lockout has lifted and whose failure window
// has passed. Caller holds l.mu.
func (l *Limiter) pruneLocked(now time.Time) {
	for k, e := range l.entries {
		if !now.Before(e.lockedUntil) && now.Sub(e.firstFailureAt) > l.policy.Window {
			delete(l.entries, k)
		}
	}
}
