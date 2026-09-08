package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := defaults()
	if cfg.HTTPPort != 8099 {
		t.Fatalf("port = %d, want 8099", cfg.HTTPPort)
	}
	if cfg.DisplayEnabled {
		t.Fatal("display should be disabled by default")
	}
}

func TestTOMLOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	content := `
[http]
port = 9000
bind = "127.0.0.1"
[display]
enabled = true
[logging]
level = "debug"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPPort != 9000 {
		t.Fatalf("port = %d, want 9000", cfg.HTTPPort)
	}
	if cfg.HTTPBind != "127.0.0.1" {
		t.Fatalf("bind = %q", cfg.HTTPBind)
	}
	if !cfg.DisplayEnabled {
		t.Fatal("display should be enabled from TOML")
	}
	if cfg.LoggingLevel != "debug" {
		t.Fatalf("level = %q", cfg.LoggingLevel)
	}
	if cfg.YouTubePOTMode != "auto" || cfg.YouTubePOTPort != 4416 {
		t.Fatalf("youtube pot defaults mode=%q port=%d", cfg.YouTubePOTMode, cfg.YouTubePOTPort)
	}
}

func TestEnvOverridesTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	if err := os.WriteFile(path, []byte("[http]\nport = 9000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LSV_HTTP_PORT", "8111")
	t.Setenv("LSV_DISPLAY_ENABLED", "true")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPPort != 8111 {
		t.Fatalf("port = %d, want 8111 (env)", cfg.HTTPPort)
	}
	if !cfg.DisplayEnabled {
		t.Fatal("display should be enabled from env")
	}
}

func TestYouTubePOTConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	content := `
[youtube]
pot_mode = "external"
pot_url = "http://127.0.0.1:8080"
pot_port = 8080
pot_server_dir = "/opt/bgutil-pot"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.YouTubePOTMode != "external" || cfg.YouTubePOTURL != "http://127.0.0.1:8080" {
		t.Fatalf("toml youtube %+v", cfg)
	}
	if cfg.YouTubePOTPort != 8080 || cfg.YouTubePOTServerDir != "/opt/bgutil-pot" {
		t.Fatalf("toml youtube port/dir %+v", cfg)
	}
	t.Setenv("LSV_YOUTUBE_POT_MODE", "off")
	t.Setenv("LSV_YOUTUBE_POT_URL", "http://10.0.0.2:4416")
	t.Setenv("LSV_YOUTUBE_POT_PORT", "9000")
	t.Setenv("LSV_YOUTUBE_POT_SERVER_DIR", "/tmp/pot")
	t.Setenv("LSV_YOUTUBE_COOKIES_FROM_BROWSER", "chrome")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.YouTubePOTMode != "off" || cfg.YouTubePOTURL != "http://10.0.0.2:4416" {
		t.Fatalf("env youtube %+v", cfg)
	}
	if cfg.YouTubePOTPort != 9000 || cfg.YouTubePOTServerDir != "/tmp/pot" {
		t.Fatalf("env youtube port/dir %+v", cfg)
	}
	if cfg.YouTubeCookiesFromBrowser != "chrome" {
		t.Fatalf("cookies_from_browser %q", cfg.YouTubeCookiesFromBrowser)
	}
}
