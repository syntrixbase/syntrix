package rest

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/syntrixbase/syntrix/internal/core/identity"
	"github.com/syntrixbase/syntrix/pkg/model"
)

func (h *Handler) handleSignUp(w http.ResponseWriter, r *http.Request) {
	var req identity.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Invalid request body")
		return
	}

	tokenPair, err := h.auth.SignUp(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Signup failed")
		return
	}

	writeJSON(w, http.StatusOK, tokenPair)
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req identity.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Username and password are required")
		return
	}

	tokenPair, err := h.auth.SignIn(r.Context(), req)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "Invalid credentials")
			return
		}
		if errors.Is(err, identity.ErrAccountDisabled) {
			writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "Account is disabled")
			return
		}
		if errors.Is(err, identity.ErrAccountLocked) {
			writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "Account is locked")
			return
		}
		if model.IsCanceled(err) {
			w.WriteHeader(499)
			return
		}
		writeError(w, http.StatusInternalServerError, ErrCodeInternalError, "Login failed")
		return
	}

	writeJSON(w, http.StatusOK, tokenPair)
}

func (h *Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req identity.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Invalid request body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Missing refresh token")
		return
	}

	tokenPair, err := h.auth.Refresh(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized, "Invalid or expired refresh token")
		return
	}

	writeJSON(w, http.StatusOK, tokenPair)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Invalid request body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, ErrCodeBadRequest, "Missing refresh token")
		return
	}

	if err := h.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		writeInternalError(w, err, "Logout failed")
		return
	}

	w.WriteHeader(http.StatusOK)
}
