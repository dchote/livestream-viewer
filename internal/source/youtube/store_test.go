package youtube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleCookies(hosts ...string) []byte {
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n")
	for _, h := range hosts {
		b.WriteString(h + "\tTRUE\t/\tTRUE\t0\tLOGIN_INFO\tabc\n")
	}
	return []byte(b.String())
}

func TestWriteCookiesRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	n, err := WriteCookies(dir, sampleCookies(".youtube.com", "www.youtube.com"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("hosts %d, want 2", n)
	}
	st := LoadStatus(dir)
	if !st.Configured || st.YouTubeHosts != 2 || st.UpdatedAt == nil {
		t.Fatalf("status %+v", st)
	}
	info, err := os.Stat(CookiesPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", info.Mode().Perm())
	}
	if err := DeleteCookies(dir); err != nil {
		t.Fatal(err)
	}
	st = LoadStatus(dir)
	if st.Configured {
		t.Fatal("deleted cookies still configured")
	}
}

func TestValidateCookiesRejectsJunk(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  []byte
	}{
		{"empty", nil},
		{"html", []byte("<!DOCTYPE html><html>")},
		{"json", []byte(`{"cookies":[]}`)},
		{"no youtube", []byte("# Netscape HTTP Cookie File\n.example.com\tTRUE\t/\tTRUE\t0\tA\tB\n")},
		{"too large", bytesRepeat('x', MaxCookiesBytes+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateCookies(tc.raw); err == nil {
				t.Fatal("expected reject")
			}
		})
	}
}

func TestPOTokenRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if ReadPOToken(dir) != "" {
		t.Fatal("empty dir")
	}
	if err := WritePOToken(dir, "  web.gvs+abc  "); err != nil {
		t.Fatal(err)
	}
	if got := ReadPOToken(dir); got != "web.gvs+abc" {
		t.Fatalf("token %q", got)
	}
	if !LoadStatus(dir).POToken {
		t.Fatal("expected po_token true")
	}
	if err := WritePOToken(dir, ""); err != nil {
		t.Fatal(err)
	}
	if ReadPOToken(dir) != "" || LoadStatus(dir).POToken {
		t.Fatal("empty token should clear the file")
	}
}

func TestCookiesConfigured(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := CookiesPath(dir)
	if CookiesConfigured(path) {
		t.Fatal("missing file")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, sampleCookies(".youtube.com"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !CookiesConfigured(path) {
		t.Fatal("expected configured")
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
