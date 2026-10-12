package httpapi

import "net/http"

type loginRequest struct {
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.auth.VerifyPassword(req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	token, expires, err := s.auth.IssueToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue session")
		return
	}
	s.auth.SetSessionCookie(w, token, expires)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
