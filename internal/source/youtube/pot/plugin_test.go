package pot

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallPluginFromBundled(t *testing.T) {
	t.Parallel()
	dest := t.TempDir()
	dir, err := InstallPlugin(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !pluginDirReady(dir) {
		t.Fatalf("plugin missing in %s", dir)
	}
	if pluginDirVersion(dir) != PluginVersion {
		t.Fatalf("version %q", pluginDirVersion(dir))
	}
	again, err := InstallPlugin(dest)
	if err != nil {
		t.Fatal(err)
	}
	if again != dir {
		t.Fatalf("second install %q want %q", again, dir)
	}
}

func TestExtractPluginZip(t *testing.T) {
	t.Parallel()
	src := findBundledPlugin()
	if src == "" {
		t.Fatal("bundled plugin missing")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || info.IsDir() {
			return nil
		}
		if !strings.HasPrefix(rel, "yt_dlp_plugins/") {
			return nil
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := extractPluginZip(buf.Bytes(), dest); err != nil {
		t.Fatal(err)
	}
	if !hasPluginTree(dest) {
		t.Fatal("extracted plugin missing")
	}
}
