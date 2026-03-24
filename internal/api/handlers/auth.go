// Package handlers contains HTTP handler functions for the VoidDB REST API.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/voiddb/void/internal/api/middleware"
	"github.com/voiddb/void/internal/auth"
)

// AuthHandler groups authentication-related endpoints.
type AuthHandler struct {
	svc *auth.Service
}

// NewAuthHandler creates an AuthHandler backed by svc.
func NewAuthHandler(svc *auth.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// --- POST /v1/auth/login -----------------------------------------------------

// loginRequest is the JSON body for the login endpoint.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login handles POST /v1/auth/login.
// @Summary Login
// @Description Authenticate with username and password, receive JWT tokens.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pair, err := h.svc.Login(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

// --- POST /v1/auth/refresh ---------------------------------------------------

// refreshRequest carries the refresh token.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh handles POST /v1/auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pair, err := h.svc.RefreshToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

// --- GET /v1/auth/me ---------------------------------------------------------

// Me handles GET /v1/auth/me. Returns the currently authenticated user.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	user, err := h.svc.GetUser(claims.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// --- User management (admin only) --------------------------------------------

// createUserRequest is the body for creating a user.
type createUserRequest struct {
	Username string    `json:"username"`
	Password string    `json:"password"`
	Role     auth.Role `json:"role"`
}

// CreateUser handles POST /v1/users.
func (h *AuthHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.CreateUser(req.Username, req.Password, req.Role); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": req.Username})
}

// ListUsers handles GET /v1/users.
func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users := h.svc.ListUsers()
	writeJSON(w, http.StatusOK, users)
}

// DeleteUser handles DELETE /v1/users/{id}.
func (h *AuthHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := h.svc.DeleteUser(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// changePasswordRequest carries the new password.
type changePasswordRequest struct {
	Password string `json:"password"`
}

// ChangePassword handles PUT /v1/users/{id}/password.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.ChangePassword(id, req.Password); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
