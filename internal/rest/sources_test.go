package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestSourceScreenTourCRUD(t *testing.T) {
	h := testServer(t)
	token := completeAdminLogin(t, h)

	rr := doAuth(t, h, http.MethodPost, "/api/v1/sources", token, map[string]any{
		"name": "Cam 1",
		"kind": "rtsp",
		"url":  "rtsp://camera.local/stream",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create source %d %s", rr.Code, rr.Body.String())
	}
	var src map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &src); err != nil {
		t.Fatal(err)
	}
	if src["has_password"] != false {
		t.Fatalf("has_password %+v", src["has_password"])
	}
	opts, _ := src["options"].(map[string]any)
	if opts["transport"] != "tcp" {
		t.Fatalf("options %+v", src["options"])
	}
	srcID := int(src["id"].(float64))

	rr = doAuth(t, h, http.MethodPost, "/api/v1/sources/"+strconv.Itoa(srcID)+"/probe", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("probe %d %s", rr.Code, rr.Body.String())
	}
	var probed map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &probed)
	probe, _ := probed["probe"].(map[string]any)
	if probe["status"] != "unavailable" && probe["status"] != "ok" && probe["status"] != "error" {
		t.Fatalf("probe %+v", probed["probe"])
	}

	rr = doAuth(t, h, http.MethodPost, "/api/v1/screens", token, map[string]any{
		"name":   "Main",
		"kind":   "grid",
		"layout": "2x2",
		"tiles": []map[string]any{
			{"index": 0, "source_id": srcID, "fit": "contain"},
		},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create screen %d %s", rr.Code, rr.Body.String())
	}
	var screen map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &screen)
	screenID := int(screen["id"].(float64))
	tiles, _ := screen["tiles"].([]any)
	if len(tiles) != 4 {
		t.Fatalf("padded tiles %d", len(tiles))
	}

	rr = doAuth(t, h, http.MethodGet, "/api/v1/tour", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("tour %d", rr.Code)
	}
	var tour map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &tour)
	entries, _ := tour["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("first-screen tour seed %d", len(entries))
	}

	rr = doAuth(t, h, http.MethodDelete, "/api/v1/sources/"+strconv.Itoa(srcID), token, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("delete in use %d %s", rr.Code, rr.Body.String())
	}
	var conflict map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &conflict)
	details, _ := conflict["details"].(map[string]any)
	if details == nil || details["screens"] == nil {
		t.Fatalf("conflict details %+v", conflict)
	}

	rr = doAuth(t, h, http.MethodPatch, "/api/v1/screens/"+strconv.Itoa(screenID), token, map[string]any{
		"layout": "1+5",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("layout change %d %s", rr.Code, rr.Body.String())
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &screen)
	tiles, _ = screen["tiles"].([]any)
	if len(tiles) != 6 {
		t.Fatalf("1+5 cells %d", len(tiles))
	}
	first := tiles[0].(map[string]any)
	if int(first["source_id"].(float64)) != srcID {
		t.Fatal("hotspot not preserved")
	}

	rr = doAuth(t, h, http.MethodPut, "/api/v1/tour", token, map[string]any{
		"enabled": true,
		"loop":    true,
		"entries": []map[string]any{
			{"screen_id": screenID, "dwell_ms": 5000, "transition": map[string]any{"type": "cut", "duration_ms": 0}},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("put tour %d %s", rr.Code, rr.Body.String())
	}

	rr = doAuth(t, h, http.MethodPut, "/api/v1/tour", token, map[string]any{
		"enabled": false,
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("put tour without entries want 400 got %d %s", rr.Code, rr.Body.String())
	}

	rr = doAuth(t, h, http.MethodPatch, "/api/v1/sources/"+strconv.Itoa(srcID), token, map[string]any{
		"name": "Cam 1 renamed",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("patch source name %d %s", rr.Code, rr.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &patched)
	opts, _ = patched["options"].(map[string]any)
	if opts["transport"] != "tcp" {
		t.Fatalf("patch without options must preserve transport, got %+v", patched["options"])
	}

	rr = doAuth(t, h, http.MethodGet, "/api/v1/layouts", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("layouts %d", rr.Code)
	}
	var layouts map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &layouts)
	foundFull := false
	for _, raw := range layouts["layouts"].([]any) {
		l := raw.(map[string]any)
		if l["id"] == "full" {
			foundFull = true
			if l["family"] != "full_bleed" {
				t.Fatalf("full layout family %+v", l)
			}
		}
	}
	if !foundFull {
		t.Fatal("missing full-bleed layout id")
	}

	rr = doAuth(t, h, http.MethodPost, "/api/v1/display/pause", token, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("pause %d", rr.Code)
	}
	rr = doAuth(t, h, http.MethodGet, "/api/v1/display/state", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("state %d", rr.Code)
	}

	rr = doAuth(t, h, http.MethodDelete, "/api/v1/screens/"+strconv.Itoa(screenID), token, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("screen in tour %d %s", rr.Code, rr.Body.String())
	}
}

func TestSourceRBACAndUpload(t *testing.T) {
	h := testServer(t)
	admin := completeAdminLogin(t, h)
	rr := doAuth(t, h, http.MethodPost, "/api/v1/users", admin, map[string]any{
		"username": "viewer",
		"password": "viewer123",
		"role":     "user",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create user %d", rr.Code)
	}
	opToken, _ := login(t, h, "viewer", "viewer123")
	rr = doAuth(t, h, http.MethodPost, "/api/v1/auth/change-password", opToken, map[string]any{
		"current_password": "viewer123",
		"new_password":     "viewer999",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("change %d", rr.Code)
	}
	rr = doAuth(t, h, http.MethodPost, "/api/v1/sources", opToken, map[string]any{"name": "x", "kind": "hls", "url": "http://x"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("user create source %d", rr.Code)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(fw, strings.NewReader("not-really-video"))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+admin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload %d %s", rec.Code, rec.Body.String())
	}
	var up map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &up)
	upID := int(up["id"].(float64))

	rr = doAuth(t, h, http.MethodPost, "/api/v1/sources", admin, map[string]any{
		"name":    "File",
		"kind":    "file",
		"options": map[string]any{"upload_id": upID},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("file source %d %s", rr.Code, rr.Body.String())
	}
	rr = doAuth(t, h, http.MethodDelete, "/api/v1/uploads/"+strconv.Itoa(upID), admin, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("upload in use %d %s", rr.Code, rr.Body.String())
	}
}

func TestEventsHubAndDisplayState(t *testing.T) {
	h := testServer(t)
	token := completeAdminLogin(t, h)
	rr := doAuth(t, h, http.MethodGet, "/api/v1/display/state", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("state %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "display_running") {
		t.Fatalf("body %s", rr.Body.String())
	}
}
