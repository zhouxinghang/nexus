# Login throttling is in-memory and resets on restart

Nexus has exactly one Owner and one password, and it is meant to be reachable
from any browser, local or remote. Without throttling, an exposed URL lets
anyone guess passwords as fast as bcrypt allows. We need per-client failure
throttling on `POST /api/auth/login`.

The limiter lives in-process (`internal/auth.Limiter`): a mutex-guarded map
from client IP to `{failures, firstFailureAt, lockedUntil}`. After N
consecutive failures within a window, that client gets `429 Too Many Requests`
with `Retry-After` until a cooldown passes. A successful login clears the
client's count. A locked client is refused before any bcrypt work happens.
Stale entries are swept once the map grows large, so it can't grow without
bound. Defaults are 5 failures in 15 minutes → 1-minute lockout, all
env-configurable.

The state is not persisted. A restart clears every lockout. We accept that:

- Nexus is one native process (ADR-0001), so there is no second instance whose
  state would need sharing, and no shared store to add.
- An attacker who can restart the Host already has more than the password.
- Persisting failures to Postgres would mean a write on every bad guess, plus
  cleanup. That costs more than it buys for a single Owner.

The client key is `RemoteAddr` by default. `X-Forwarded-For` (right-most entry)
and `X-Real-IP` are honored only when `NEXUS_TRUSTED_PROXY=true`, so a client
can't rotate spoofed headers to dodge, or poison, the throttle. Behind a proxy
with the toggle off, every client shares the proxy's bucket, which fails safe.

Alongside this, the session cookie is `Secure` by default
(`NEXUS_COOKIE_SECURE`, set `false` for plain-HTTP local dev), so the token
isn't sent over plain HTTP.

If Nexus ever runs as more than one process, revisit with a shared store.
