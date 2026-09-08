package pot

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// PluginVersion is the pinned bgutil yt-dlp plugin.
	PluginVersion = "2.0.0"

	pluginZipURL = "https://github.com/Brainicism/bgutil-ytdlp-pot-provider/releases/download/2.0.0/bgutil-ytdlp-pot-provider.zip"
	pluginZipSHA = "bce874dfa25896c2798e0f4f8147b7b22e785479eb1e459ab232bf2506c95016"
)

// HTTPGet is overridden in tests.
var HTTPGet = httpGet

// InstallPlugin places the bgutil yt-dlp plugin under dataDir/yt-dlp-plugins
// and returns that directory. yt-dlp must be invoked with --plugin-dirs on
// that path; a Homebrew install does not include the plugin.
//
// The plugin is embedded in the binary and copied locally. A GitHub download
// is only a last resort if the embed is missing (tests, broken builds).
func InstallPlugin(dataDir string) (string, error) {
	if dataDir == "" {
		return "", fmt.Errorf("data dir is empty")
	}
	dest := filepath.Join(dataDir, "yt-dlp-plugins")
	if pluginDirReady(dest) && pluginDirVersion(dest) == PluginVersion {
		return dest, nil
	}
	if embeddedPluginReady() {
		if err := copyEmbeddedPlugin(dest); err != nil {
			return "", err
		}
		return dest, nil
	}
	if src := findBundledPlugin(); src != "" {
		if err := copyPluginDir(src, dest); err != nil {
			return "", err
		}
		return dest, nil
	}
	if err := downloadPlugin(dest); err != nil {
		return "", err
	}
	return dest, nil
}

func pluginDirReady(dir string) bool {
	return hasPluginTree(filepath.Join(dir, "bgutil"))
}

func hasPluginTree(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "yt_dlp_plugins", "extractor", "getpot_bgutil_http.py"))
	return err == nil
}

func pluginDirVersion(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func findBundledPlugin() string {
	var candidates []string
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "pluginfs"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "pluginfs"),
			filepath.Join(cwd, "internal", "source", "youtube", "pot", "pluginfs"),
		)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "yt-dlp-plugins"),
			filepath.Join(dir, "..", "internal", "source", "youtube", "pot", "pluginfs"),
		)
	}
	for _, c := range candidates {
		if hasPluginTree(c) {
			return c
		}
	}
	return ""
}

func copyPluginDir(src, dest string) error {
	if !hasPluginTree(src) {
		return fmt.Errorf("no yt_dlp_plugins in %s", src)
	}
	// yt-dlp only scans plugin-dirs/*/yt_dlp_plugins (or zip files), not
	// plugin-dirs/yt_dlp_plugins itself.
	nested := filepath.Join(dest, "bgutil", "yt_dlp_plugins")
	if err := os.RemoveAll(nested); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(src, "yt_dlp_plugins"), nested); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "VERSION"), []byte(PluginVersion+"\n"), 0o644)
}

func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		in, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, in, 0o644)
	})
}

func downloadPlugin(dest string) error {
	raw, err := HTTPGet(pluginZipURL)
	if err != nil {
		return fmt.Errorf("download yt-dlp PO token plugin: %w", err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != pluginZipSHA {
		return fmt.Errorf("yt-dlp PO token plugin checksum mismatch")
	}
	tmp, err := os.MkdirTemp("", "lsv-yt-plugin-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := extractPluginZip(raw, tmp); err != nil {
		return err
	}
	return copyPluginDir(tmp, dest)
}

func extractPluginZip(raw []byte, dest string) error {
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return fmt.Errorf("plugin zip: %w", err)
	}
	for _, f := range r.File {
		name := filepath.ToSlash(filepath.Clean(f.Name))
		if name == "." || strings.HasPrefix(name, "..") || strings.Contains(name, "/../") {
			continue
		}
		if !strings.HasPrefix(name, "yt_dlp_plugins/") && name != "yt_dlp_plugins" {
			continue
		}
		out := filepath.Join(dest, filepath.FromSlash(name))
		if !strings.HasPrefix(out, dest+string(os.PathSeparator)) && out != dest {
			continue
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(rc, 1<<20))
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(out, body, 0o644); err != nil {
			return err
		}
	}
	if !hasPluginTree(dest) {
		return fmt.Errorf("plugin zip missing getpot_bgutil_http.py")
	}
	return nil
}

func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}
