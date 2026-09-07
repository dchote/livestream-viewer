package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"errors"
	"github.com/dchote/livestream-viewer/internal/config"
	"github.com/dchote/livestream-viewer/internal/database"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
)

func testServer(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		DatabasePath: filepath.Join(dir, "test.sqlite"),
		DataDir:      dir,
		JWTSecret:    "test-secret",
		HTTPPort:     8099,
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return New(db, cfg, nil, nil, nil, resolver.Tools{
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
	})
}

func TestHealth(t *testing.T) {
	h := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body %+v", body)
	}
}

func TestOpenAPIAndDocs(t *testing.T) {
	h := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("openapi status %d", rr.Code)
	}
	if len(rr.Body.Bytes()) < 100 {
		t.Fatal("openapi too short")
	}

	req = httptest.NewRequest(http.MethodGet, "/docs", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("docs status %d", rr.Code)
	}
}

func TestLayoutsRequiresAuth(t *testing.T) {
	h := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/layouts", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rr.Code)
	}
}

func TestLoginRequiresPasswordChange(t *testing.T) {
	h := testServer(t)
	token, user := login(t, h, "admin", "admin")
	if user["must_change_password"] != true {
		t.Fatalf("must_change_password = %v", user["must_change_password"])
	}

	rr := doAuth(t, h, http.MethodGet, "/api/v1/layouts", token, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("layouts status %d, want 403", rr.Code)
	}
	var errBody map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody["error"] != "password_change_required" {
		t.Fatalf("error = %v", errBody["error"])
	}

	rr = doAuth(t, h, http.MethodGet, "/api/v1/auth/me", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("me status %d", rr.Code)
	}

	rr = doAuth(t, h, http.MethodPost, "/api/v1/auth/change-password", token, map[string]any{
		"current_password": "admin",
		"new_password":     "admin",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("same password status %d", rr.Code)
	}

	rr = doAuth(t, h, http.MethodPost, "/api/v1/auth/change-password", token, map[string]any{
		"current_password": "admin",
		"new_password":     "newpass12",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("change-password status %d body %s", rr.Code, rr.Body.String())
	}
	var changed map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &changed); err != nil {
		t.Fatal(err)
	}
	if changed["must_change_password"] != false {
		t.Fatalf("must_change_password after change = %v", changed["must_change_password"])
	}

	rr = doAuth(t, h, http.MethodGet, "/api/v1/layouts", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("layouts after change status %d", rr.Code)
	}
}

func TestUserManagementRBAC(t *testing.T) {
	h := testServer(t)
	adminToken := completeAdminLogin(t, h)

	rr := doAuth(t, h, http.MethodGet, "/api/v1/users", adminToken, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list users status %d body %s", rr.Code, rr.Body.String())
	}
	var listed map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	users, ok := listed["users"].([]any)
	if !ok || len(users) != 1 {
		t.Fatalf("users = %+v", listed["users"])
	}

	rr = doAuth(t, h, http.MethodPost, "/api/v1/users", adminToken, map[string]any{
		"username": "operator",
		"password": "operator1",
		"role":     "user",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create user status %d body %s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["role"] != "user" || created["must_change_password"] != true {
		t.Fatalf("created %+v", created)
	}
	userID := int(created["id"].(float64))

	rr = doAuth(t, h, http.MethodPatch, "/api/v1/users/1", adminToken, map[string]any{"role": "user"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("change own role status %d", rr.Code)
	}

	rr = doAuth(t, h, http.MethodDelete, "/api/v1/users/1", adminToken, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("delete self status %d", rr.Code)
	}

	opToken, opUser := login(t, h, "operator", "operator1")
	if opUser["must_change_password"] != true {
		t.Fatal("created user should must-change-password")
	}
	rr = doAuth(t, h, http.MethodPost, "/api/v1/auth/change-password", opToken, map[string]any{
		"current_password": "operator1",
		"new_password":     "operator9",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("operator change-password status %d body %s", rr.Code, rr.Body.String())
	}

	rr = doAuth(t, h, http.MethodGet, "/api/v1/layouts", opToken, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("operator layouts status %d", rr.Code)
	}
	rr = doAuth(t, h, http.MethodGet, "/api/v1/users", opToken, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator list users status %d, want 403", rr.Code)
	}
	rr = doAuth(t, h, http.MethodPost, "/api/v1/sources", opToken, map[string]any{"name": "cam"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator create source status %d, want 403", rr.Code)
	}

	rr = doAuth(t, h, http.MethodPatch, "/api/v1/users/"+strconv.Itoa(userID), adminToken, map[string]any{"role": "admin"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("promote status %d body %s", rr.Code, rr.Body.String())
	}
	rr = doAuth(t, h, http.MethodDelete, "/api/v1/users/"+strconv.Itoa(userID), adminToken, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete user status %d body %s", rr.Code, rr.Body.String())
	}
}

func completeAdminLogin(t *testing.T, h http.Handler) string {
	t.Helper()
	token, _ := login(t, h, "admin", "admin")
	rr := doAuth(t, h, http.MethodPost, "/api/v1/auth/change-password", token, map[string]any{
		"current_password": "admin",
		"new_password":     "newpass12",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("admin change-password status %d body %s", rr.Code, rr.Body.String())
	}
	return token
}

func login(t *testing.T, h http.Handler, username, password string) (string, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status %d body %s", rr.Code, rr.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatal("missing token")
	}
	user, _ := data["user"].(map[string]any)
	return token, user
}

func doAuth(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}
