// Package pot supervises the BgUtils PO token provider used by yt-dlp.
package pot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dchote/livestream-viewer/internal/backoff"
)

const (
	// DefaultPort is the upstream BgUtils listen port.
	DefaultPort = 4416
	// DefaultBaseURL is what the yt-dlp plugin probes with no extractor-args.
	DefaultBaseURL = "http://127.0.0.1:4416"

	ModeAuto     = "auto"
	ModeManaged  = "managed"
	ModeExternal = "external"
	ModeOff      = "off"

	defaultPingInterval = 2 * time.Second
	defaultPingTimeout  = time.Second
)

// Status is the secret-free view of the provider.
type Status struct {
	Mode      string `json:"mode"`
	Running   bool   `json:"running"`
	BaseURL   string `json:"base_url,omitempty"`
	Version   string `json:"version,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

// Config selects how the provider is reached. Empty Mode is auto.
type Config struct {
	Mode      string
	URL       string
	Port      int
	ServerDir string

	LookPath func(file string) (string, error)
	Command  func(ctx context.Context, name string, args []string, dir string) error
	Ping     func(ctx context.Context, baseURL string) (string, error)

	PingInterval time.Duration
	PingTimeout  time.Duration
}

// Provider runs (or watches) the PO token HTTP server.
type Provider struct {
	cfg      Config
	resolved Resolved

	mu      sync.Mutex
	running bool
	version string
	lastErr string
	onReady func()
}

// New resolves the mode immediately so BaseURL is available before Start.
func New(cfg Config) *Provider {
	if cfg.LookPath == nil {
		cfg.LookPath = exec.LookPath
	}
	if cfg.Command == nil {
		cfg.Command = runManaged
	}
	if cfg.Ping == nil {
		timeout := cfg.PingTimeout
		if timeout <= 0 {
			timeout = defaultPingTimeout
		}
		cfg.Ping = func(ctx context.Context, baseURL string) (string, error) {
			return pingHTTP(ctx, baseURL, timeout)
		}
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = defaultPingInterval
	}
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = defaultPingTimeout
	}
	p := &Provider{cfg: cfg, resolved: Resolve(cfg)}
	if p.resolved.Err != "" {
		p.lastErr = p.resolved.Err
	}
	return p
}

// BaseURL is where yt-dlp should look, or "" when the provider is off.
func (p *Provider) BaseURL() string {
	if p == nil {
		return ""
	}
	return p.resolved.BaseURL
}

// Status is a snapshot for GET /system/youtube.
func (p *Provider) Status() Status {
	if p == nil {
		return Status{Mode: ModeOff}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return Status{
		Mode:      p.resolved.Mode,
		Running:   p.running,
		BaseURL:   p.resolved.BaseURL,
		Version:   p.version,
		LastError: p.lastErr,
	}
}

// OnReady registers a callback for false→true transitions. Replaces any previous.
func (p *Provider) OnReady(fn func()) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.onReady = fn
	p.mu.Unlock()
}

// Start blocks until ctx is cancelled. Safe to call when the resolved mode is off.
func (p *Provider) Start(ctx context.Context) {
	if p == nil {
		return
	}
	slog.Info("youtube pot provider",
		"mode", p.resolved.Mode,
		"base_url", p.resolved.BaseURL,
		"err", p.resolved.Err)
	if p.resolved.Mode == ModeOff {
		return
	}
	if p.resolved.Mode == ModeManaged && p.resolved.Err == "" {
		go p.runManagedLoop(ctx)
	}
	p.runHealth(ctx)
}

func (p *Provider) runManagedLoop(ctx context.Context) {
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		dir := filepath.Join(p.resolved.ServerDir, "node_modules")
		args := []string{
			"run",
			"--allow-env",
			"--allow-net",
			"--allow-ffi=.",
			"--allow-read=.",
			"../src/main.ts",
			"--port", fmt.Sprintf("%d", p.resolved.Port),
			"--host", "127.0.0.1",
		}
		slog.Info("youtube pot provider starting", "port", p.resolved.Port, "dir", p.resolved.ServerDir)
		err := p.cfg.Command(ctx, p.resolved.Deno, args, dir)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			p.setError(fmt.Sprintf("provider exited: %v", err))
			slog.Warn("youtube pot provider exited", "err", err)
		} else {
			p.setError("provider exited")
		}
		p.markDown()
		d := backoff.Delay(attempt, 1000)
		attempt++
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (p *Provider) runHealth(ctx context.Context) {
	p.poll(ctx)
	t := time.NewTicker(p.cfg.PingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			p.markDown()
			return
		case <-t.C:
			p.poll(ctx)
		}
	}
}

func (p *Provider) poll(ctx context.Context) {
	if p.resolved.BaseURL == "" {
		return
	}
	ver, err := p.cfg.Ping(ctx, p.resolved.BaseURL)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		p.setError(err.Error())
		p.markDown()
		return
	}
	p.markUp(ver)
}

func (p *Provider) markUp(version string) {
	p.mu.Lock()
	was := p.running
	p.running = true
	p.version = version
	p.lastErr = ""
	fn := p.onReady
	p.mu.Unlock()
	if !was && fn != nil {
		fn()
	}
}

func (p *Provider) markDown() {
	p.mu.Lock()
	p.running = false
	p.mu.Unlock()
}

func (p *Provider) setError(msg string) {
	p.mu.Lock()
	p.lastErr = msg
	p.mu.Unlock()
}

// Resolved is the effective provider mode after auto selection.
type Resolved struct {
	Mode      string
	BaseURL   string
	Port      int
	ServerDir string
	Deno      string
	Err       string
}

// Resolve picks managed, external, or off. auto never appears in the result.
func Resolve(cfg Config) Resolved {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = ModeAuto
	}
	port := cfg.Port
	if port <= 0 {
		port = DefaultPort
	}
	url := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	serverOK := serverDirReady(cfg.ServerDir)
	var deno string
	var denoErr error
	if cfg.LookPath != nil {
		deno, denoErr = cfg.LookPath("deno")
	} else {
		deno, denoErr = exec.LookPath("deno")
	}
	denoOK := denoErr == nil && deno != ""

	pickManaged := func() Resolved {
		r := Resolved{Mode: ModeManaged, Port: port, ServerDir: cfg.ServerDir, Deno: deno}
		r.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
		if !denoOK {
			r.Err = "deno is not installed"
			return r
		}
		if !serverOK {
			r.Err = "PO token provider server directory is missing"
			return r
		}
		return r
	}

	switch mode {
	case ModeOff:
		return Resolved{Mode: ModeOff}
	case ModeManaged:
		return pickManaged()
	case ModeExternal:
		if url == "" {
			url = DefaultBaseURL
		}
		return Resolved{Mode: ModeExternal, BaseURL: url, Port: port}
	case ModeAuto:
		if denoOK && serverOK {
			return pickManaged()
		}
		if url == "" {
			url = fmt.Sprintf("http://127.0.0.1:%d", port)
		}
		// Stay on external even when nothing answers yet. A sidecar that
		// starts after this process is the documented standalone path;
		// going Off here would ignore it until a restart.
		r := Resolved{Mode: ModeExternal, BaseURL: url, Port: port}
		if !pingOK(cfg, url) {
			r.Err = fmt.Sprintf("waiting for PO token provider at %s", url)
		}
		return r
	default:
		return Resolved{Mode: ModeOff, Err: fmt.Sprintf("unknown youtube pot mode %q", cfg.Mode)}
	}
}

func pingOK(cfg Config, url string) bool {
	if cfg.Ping == nil || url == "" {
		return false
	}
	timeout := cfg.PingTimeout
	if timeout <= 0 {
		timeout = defaultPingTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := cfg.Ping(ctx, url)
	return err == nil
}

func serverDirReady(dir string) bool {
	if dir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "main.ts")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err != nil {
		return false
	}
	return true
}

func runManaged(ctx context.Context, name string, args []string, dir string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	prepareManaged(cmd)
	return cmd.Run()
}

type pingBody struct {
	Version string `json:"version"`
}

func parsePing(body []byte) (string, error) {
	var p pingBody
	if err := json.Unmarshal(body, &p); err != nil {
		return "", fmt.Errorf("ping: %w", err)
	}
	p.Version = strings.TrimSpace(p.Version)
	if p.Version == "" {
		return "", fmt.Errorf("ping missing version")
	}
	return p.Version, nil
}

func pingHTTP(ctx context.Context, baseURL string, timeout time.Duration) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/ping", nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ping HTTP %d", resp.StatusCode)
	}
	return parsePing(b)
}
