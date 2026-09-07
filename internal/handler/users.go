package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/dchote/livestream-viewer/internal/model"
)

func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	var users []model.User
	if err := h.DB.Order("id asc").Find(&users).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list users", nil)
		return
	}
	out := make([]map[string]any, 0, len(users))
	for i := range users {
		out = append(out, publicUser(&users[i]))
	}
	WriteJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		WriteError(w, http.StatusBadRequest, "bad_request", "username is required", nil)
		return
	}
	if len(req.Password) < 8 {
		WriteError(w, http.StatusBadRequest, "bad_request", "password must be at least 8 characters", nil)
		return
	}
	role := req.Role
	if role == "" {
		role = model.RoleUser
	}
	if !model.ValidRole(role) {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid role", nil)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "failed to hash password", nil)
		return
	}
	u := model.User{
		Username:           username,
		PasswordHash:       string(hash),
		Role:               role,
		MustChangePassword: true,
	}
	if err := h.DB.Create(&u).Error; err != nil {
		if isUniqueConstraint(err) {
			WriteError(w, http.StatusConflict, "username_exists", "username already exists", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create user", nil)
		return
	}
	WriteJSON(w, http.StatusCreated, publicUser(&u))
}

func (h *Handlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid user id", nil)
		return
	}
	actor := UserFromContext(r.Context())
	if actor != nil && actor.ID == id {
		WriteError(w, http.StatusBadRequest, "bad_request", "cannot change own role", nil)
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON", nil)
		return
	}
	if !model.ValidRole(req.Role) {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid role", nil)
		return
	}
	var user model.User
	if err := h.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "user not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load user", nil)
		return
	}
	user.Role = req.Role
	if err := h.DB.Save(&user).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "failed to update user", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid user id", nil)
		return
	}
	actor := UserFromContext(r.Context())
	if actor != nil && actor.ID == id {
		WriteError(w, http.StatusBadRequest, "bad_request", "cannot delete self", nil)
		return
	}
	res := h.DB.Delete(&model.User{}, id)
	if res.Error != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "failed to delete user", nil)
		return
	}
	if res.RowsAffected == 0 {
		WriteError(w, http.StatusNotFound, "not_found", "user not found", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseIDParam(r *http.Request) (uint, error) {
	n, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	return uint(n), err
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "constraint")
}
