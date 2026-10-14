package httpapi

import (
	"math"
	"net/http"
	"strconv"
	"time"
)

type loginRequest struct {
	Password string `json:"password"`
}

// handleLogin verifies the Owner password with per-client throttling: a
// locked-out client is refused with 429 before any bcrypt work, a
// verified-wrong password counts as a failure, and success clears the count.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	limiter := s.auth.Limiter()
	client := s.auth.ClientIP(r)
	if ok, wait := limiter.Allow(client); !ok {
		writeTooManyAttempts(w, wait)
		return
	}

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.auth.VerifyPassword(req.Password) {
		if locked, wait := limiter.RecordFailure(client); locked {
			writeTooManyAttempts(w, wait)
			return
		}
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	limiter.Reset(client)

	token, expires, err := s.auth.IssueToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue session")
		return
	}
	s.auth.SetSessionCookie(w, token, expires)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func writeTooManyAttempts(w http.ResponseWriter, wait time.Duration) {
	secs := int(math.Ceil(wait.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"owner":         true,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.ClearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
