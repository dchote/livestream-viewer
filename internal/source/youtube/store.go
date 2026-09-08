// Package youtube stores the process-level YouTube session used by yt-dlp.
package youtube

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dchote/livestream-viewer/internal/source/youtube/pot"
)

const (
	cookiesName = "youtube.cookies"
	tokenName   = "youtube.po_token"

	// MaxCookiesBytes is well above a typical Netscape export and far below
	// a video file someone might drop on the wrong control.
	MaxCookiesBytes = 256 << 10
	maxPOTokenBytes = 16 << 10
)

// CookiesPath is data/secrets/youtube.cookies.
func CookiesPath(dataDir string) string {
	return filepath.Join(dataDir, "secrets", cookiesName)
}

// POTokenPath is data/secrets/youtube.po_token.
func POTokenPath(dataDir string) string {
	return filepath.Join(dataDir, "secrets", tokenName)
}

// Status is the secret-redacted view of the YouTube session.
type Status struct {
	Configured    bool       `json:"configured"`
	YouTubeHosts  int        `json:"youtube_hosts"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
	POToken       bool       `json:"po_token"`
	LastErrorCode string     `json:"last_error_code"`
	LastError     string     `json:"last_error"`
	POT           pot.Status `json:"pot"`
}

// LoadStatus reports whether cookies and a PO token are on disk. lastError*
// are filled by the caller from live ingest health.
func LoadStatus(dataDir string) Status {
	st := Status{}
	path := CookiesPath(dataDir)
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		st.Configured = true
		mod := info.ModTime().UTC()
		st.UpdatedAt = &mod
		if b, err := os.ReadFile(path); err == nil {
			st.YouTubeHosts = countYouTubeHosts(b)
		}
	}
	if b, err := os.ReadFile(POTokenPath(dataDir)); err == nil && strings.TrimSpace(string(b)) != "" {
		st.POToken = true
	}
	return st
}

// WriteCookies validates a Netscape cookie export and writes it atomically.
func WriteCookies(dataDir string, raw []byte) (int, error) {
	n, err := ValidateCookies(raw)
	if err != nil {
		return 0, err
	}
	if err := writeSecret(CookiesPath(dataDir), raw); err != nil {
		return 0, err
	}
	return n, nil
}

// DeleteCookies removes the cookie jar. Missing is success.
func DeleteCookies(dataDir string) error {
	return removeSecret(CookiesPath(dataDir))
}

// WritePOToken stores a trimmed PO token, or deletes the file when empty.
func WritePOToken(dataDir, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return removeSecret(POTokenPath(dataDir))
	}
	if len(token) > maxPOTokenBytes {
		return fmt.Errorf("PO token is too large")
	}
	return writeSecret(POTokenPath(dataDir), []byte(token+"\n"))
}

// ReadPOToken returns the stored token, or "" if none.
func ReadPOToken(dataDir string) string {
	return ReadPOTokenFile(POTokenPath(dataDir))
}

// ReadPOTokenFile returns a trimmed PO token from path, or "".
func ReadPOTokenFile(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// CookiesConfigured reports that a non-empty cookie file exists.
func CookiesConfigured(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

// ValidateCookies accepts a Netscape cookie file that names at least one
// youtube.com host. The raw bytes are not logged.
func ValidateCookies(raw []byte) (int, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("cookie file is empty")
	}
	if len(raw) > MaxCookiesBytes {
		return 0, fmt.Errorf("cookie file exceeds %d KiB", MaxCookiesBytes/1024)
	}
	trim := bytes.TrimSpace(raw)
	lower := bytes.ToLower(trim[:min(64, len(trim))])
	if bytes.HasPrefix(lower, []byte("<!doctype")) || bytes.HasPrefix(lower, []byte("<html")) || bytes.HasPrefix(trim, []byte("{")) {
		return 0, fmt.Errorf("file is not a Netscape cookie export")
	}
	n := countYouTubeHosts(raw)
	if n == 0 {
		return 0, fmt.Errorf("cookie file has no youtube.com rows; export cookies while signed in to YouTube")
	}
	return n, nil
}

func countYouTubeHosts(raw []byte) int {
	n := 0
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		host := strings.ToLower(strings.TrimLeft(fields[0], "."))
		if host == "youtube.com" || strings.HasSuffix(host, ".youtube.com") {
			n++
		}
	}
	return n
}

func writeSecret(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("secrets dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write secret: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace secret: %w", err)
	}
	return nil
}

func removeSecret(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
