package pot

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func copyEmbeddedPlugin(dest string) error {
	nested := filepath.Join(dest, "bgutil", "yt_dlp_plugins")
	if err := os.RemoveAll(nested); err != nil {
		return err
	}
	if err := copyFSTree(bundledPluginFS, "pluginfs/yt_dlp_plugins", nested); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "VERSION"), []byte(PluginVersion+"\n"), 0o644)
}

func embeddedPluginReady() bool {
	_, err := fs.Stat(bundledPluginFS, "pluginfs/yt_dlp_plugins/extractor/getpot_bgutil_http.py")
	return err == nil
}

func copyFSTree(src fs.FS, root, dest string) error {
	return fs.WalkDir(src, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(path))
		if err != nil {
			return err
		}
		if rel == "." {
			if d.IsDir() {
				return os.MkdirAll(dest, 0o755)
			}
			return nil
		}
		if strings.HasPrefix(rel, "..") {
			return nil
		}
		out := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		body, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, body, 0o644)
	})
}
