package handler

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/source"
	"gorm.io/gorm"
)

const maxUploadBytes = 512 << 20

func (h *Handlers) ListUploads(w http.ResponseWriter, r *http.Request) {
	var items []model.Upload
	if err := h.DB.Order("id desc").Find(&items).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to list uploads", nil)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"uploads": emptyIfNil(items)})
}

func (h *Handlers) CreateUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1024)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			WriteError(w, http.StatusRequestEntityTooLarge, "too_large", "file exceeds 512 MiB limit", nil)
			return
		}
		WriteError(w, http.StatusBadRequest, "bad_request", "upload is too large or not multipart", nil)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "file field is required", nil)
		return
	}
	defer file.Close()
	if hdr.Size > maxUploadBytes {
		WriteError(w, http.StatusRequestEntityTooLarge, "too_large", "file exceeds 512 MiB limit", nil)
		return
	}
	name := filepath.Base(hdr.Filename)
	if name == "." || name == "" {
		WriteError(w, http.StatusBadRequest, "bad_request", "filename is required", nil)
		return
	}
	mime := hdr.Header.Get("Content-Type")
	if !allowedUpload(name, mime) {
		WriteError(w, http.StatusBadRequest, "bad_request", "file type is not an allowed video", nil)
		return
	}
	up := model.Upload{Name: name, MimeType: mime, Size: hdr.Size}
	if err := h.DB.Create(&up).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to create upload", nil)
		return
	}
	dir := filepath.Join(h.Cfg.DataDir, "uploads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		_ = h.DB.Delete(&up)
		WriteError(w, http.StatusInternalServerError, "io_error", "failed to create upload directory", nil)
		return
	}
	safe := sanitizeFilename(name)
	dest := filepath.Join(dir, fmt.Sprintf("%d_%s", up.ID, safe))
	out, err := os.Create(dest)
	if err != nil {
		_ = h.DB.Delete(&up)
		WriteError(w, http.StatusInternalServerError, "io_error", "failed to store file", nil)
		return
	}
	n, copyErr := io.Copy(out, file)
	_ = out.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		_ = h.DB.Delete(&up)
		WriteError(w, http.StatusInternalServerError, "io_error", "failed to store file", nil)
		return
	}
	up.Path = dest
	up.Size = n
	if err := h.DB.Save(&up).Error; err != nil {
		_ = os.Remove(dest)
		_ = h.DB.Delete(&up)
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to save upload", nil)
		return
	}
	WriteJSON(w, http.StatusCreated, up)
}

func (h *Handlers) DeleteUpload(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid upload id", nil)
		return
	}
	var up model.Upload
	if err := h.DB.First(&up, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "upload not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to load upload", nil)
		return
	}
	refs, err := source.SourcesUsingUpload(h.DB, up.ID, up.Path)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to check references", nil)
		return
	}
	if len(refs) > 0 {
		WriteError(w, http.StatusConflict, "in_use", "upload is referenced by one or more sources", map[string]any{"sources": refs})
		return
	}
	// Drop the row first: a failed delete must not leave a record pointing at
	// a file that is already gone.
	if err := h.DB.Delete(&up).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "db_error", "failed to delete upload", nil)
		return
	}
	if err := os.Remove(up.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("upload file left on disk after record delete", "id", up.ID, "path", up.Path, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func allowedUpload(name, mime string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".m4v", ".webm", ".mkv", ".mov", ".ts", ".avi", ".mpeg", ".mpg":
		return true
	}
	return strings.HasPrefix(strings.ToLower(mime), "video/")
}

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "upload"
	}
	return out
}
