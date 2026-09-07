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
