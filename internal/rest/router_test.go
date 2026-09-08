package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

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
	// The check must actually reach the database, not report a constant.
	if body["database"] != true {
		t.Fatalf("expected database check to pass, body %+v", body)
	}
	if body["ready"] != true {
		t.Fatalf("expected ready with the display disabled, body %+v", body)
	}
}

// A display failure keeps the API up so the operator can read the error, so
// health degrades without a 503 that would make a service manager restart the
// process in a loop.
func TestHealthDegradesWhenDisplayEnabledButNotRunning(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DatabasePath:   filepath.Join(dir, "test.sqlite"),
		DataDir:        dir,
		JWTSecret:      "test-secret",
		HTTPPort:       8099,
		DisplayEnabled: true,
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg, nil, nil, nil, resolver.Tools{
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
	})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 so the API stays reachable", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "degraded" || body["ready"] != false {
		t.Fatalf("body %+v", body)
	}
}

// Login is unauthenticated and each attempt costs a bcrypt comparison.
func TestLoginIsRateLimited(t *testing.T) {
	h := testServer(t)
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})

	attempt := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	for i := 0; i < 5; i++ {
		if code := attempt(); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401 within the burst", i, code)
		}
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d after the burst, want 429", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("429 should carry Retry-After")
	}
}

func TestOversizedJSONBodyIsRejected(t *testing.T) {
	h := testServer(t)
	token := completeAdminLogin(t, h)
	// Valid JSON, but far larger than any legitimate payload.
	huge := `{"placeholder_color":"#` + strings.Repeat("0", 4<<20) + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/config", strings.NewReader(huge))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code == http.StatusOK || rr.Code == http.StatusNoContent {
		t.Fatalf("status %d; an oversized body should not be accepted", rr.Code)
	}
}

// GET /system/info drives the setup UI, so a missing binary must be reported.
func TestSystemInfoReportsMissingTools(t *testing.T) {
	h := testServer(t)
	token := completeAdminLogin(t, h)
	rr := doAuth(t, h, http.MethodGet, "/api/v1/system/info", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["ffprobe"] != false {
		t.Fatalf("ffprobe reported %v with no binary on PATH", body["ffprobe"])
	}
	if body["yt_dlp"] != false {
		t.Fatalf("yt_dlp reported %v with no binary on PATH", body["yt_dlp"])
	}
	if v, ok := body["yt_dlp_version"]; ok && v != "" {
		t.Fatalf("yt_dlp_version = %v with no binary on PATH", v)
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

func TestYouTubeCookiesAPI(t *testing.T) {
	h := testServer(t)
	admin := completeAdminLogin(t, h)

	rr := doAuth(t, h, http.MethodGet, "/api/v1/system/youtube", admin, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var st map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st["configured"] != false {
		t.Fatalf("status %+v", st)
	}
	pot, _ := st["pot"].(map[string]any)
	if pot["mode"] != "off" {
		t.Fatalf("pot %+v", pot)
	}

	cookies := "# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tLOGIN_INFO\tsecret-value\n"
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "cookies.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(cookies)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/youtube/cookies", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+admin)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upload status %d body %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-value") || strings.Contains(rr.Body.String(), "LOGIN_INFO") {
		t.Fatalf("response leaked cookies: %s", rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st["configured"] != true {
		t.Fatalf("status %+v", st)
	}

	rr = doAuth(t, h, http.MethodGet, "/api/v1/system/info", admin, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("info %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "secret-value") {
		t.Fatal("system info leaked cookies")
	}
	var info map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	yt, _ := info["youtube"].(map[string]any)
	if yt["configured"] != true {
		t.Fatalf("info youtube %+v", yt)
	}

	rr = doAuth(t, h, http.MethodPut, "/api/v1/system/youtube/po-token", admin, map[string]any{"po_token": "web.gvs+abc"})
	if rr.Code != http.StatusOK {
		t.Fatalf("po-token %d body %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "web.gvs+abc") {
		t.Fatal("PO token echoed")
	}

	rr = doAuth(t, h, http.MethodDelete, "/api/v1/system/youtube/cookies", admin, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete %d", rr.Code)
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st["configured"] != false {
		t.Fatalf("after delete %+v", st)
	}
}

func TestYouTubeCookiesRejectsRoleUser(t *testing.T) {
	h := testServer(t)
	admin := completeAdminLogin(t, h)
	rr := doAuth(t, h, http.MethodPost, "/api/v1/users", admin, map[string]any{
		"username": "viewer",
		"password": "viewer123",
		"role":     "user",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rr.Code, rr.Body.String())
	}
	token, _ := login(t, h, "viewer", "viewer123")
	rr = doAuth(t, h, http.MethodPost, "/api/v1/auth/change-password", token, map[string]any{
		"current_password": "viewer123",
		"new_password":     "viewer999",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("change-password %d", rr.Code)
	}
	rr = doAuth(t, h, http.MethodGet, "/api/v1/system/youtube", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("user GET youtube %d", rr.Code)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "cookies.txt")
	_, _ = fw.Write([]byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t0\tA\tB\n"))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/youtube/cookies", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("user upload status %d, want 403", rr.Code)
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
