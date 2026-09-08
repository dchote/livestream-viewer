package pot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	if err := os.MkdirAll(filepath.Join(ready, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ready, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ready, "src", "main.ts"), []byte("export {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	look := func(ok bool) func(string) (string, error) {
		return func(file string) (string, error) {
			if ok && file == "deno" {
				return "/usr/bin/deno", nil
			}
			return "", errors.New("missing")
		}
	}

	cases := []struct {
		name string
		cfg  Config
		mode string
		url  string
		err  bool
	}{
		{
			name: "auto managed when deno and server exist",
			cfg:  Config{Mode: ModeAuto, ServerDir: ready, LookPath: look(true)},
			mode: ModeManaged,
			url:  DefaultBaseURL,
		},
		{
			name: "auto waits for a sidecar when nothing is installed",
			cfg:  Config{Mode: ModeAuto, LookPath: look(false)},
			mode: ModeExternal,
			url:  DefaultBaseURL,
			err:  true,
		},
		{
			name: "auto external when default URL pings",
			cfg: Config{
				Mode:     ModeAuto,
				LookPath: look(false),
				Ping:     func(context.Context, string) (string, error) { return "2.0.0", nil },
			},
			mode: ModeExternal,
			url:  DefaultBaseURL,
		},
		{
			name: "auto external when only a URL is set",
			cfg:  Config{Mode: ModeAuto, URL: "http://127.0.0.1:8080", LookPath: look(false)},
			mode: ModeExternal,
			url:  "http://127.0.0.1:8080",
			err:  true,
		},
		{
			name: "managed missing deno",
			cfg:  Config{Mode: ModeManaged, ServerDir: ready, LookPath: look(false)},
			mode: ModeManaged,
			url:  DefaultBaseURL,
			err:  true,
		},
		{
			name: "external default URL",
			cfg:  Config{Mode: ModeExternal, LookPath: look(false)},
			mode: ModeExternal,
			url:  DefaultBaseURL,
		},
		{
			name: "off",
			cfg:  Config{Mode: ModeOff, ServerDir: ready, LookPath: look(true)},
			mode: ModeOff,
		},
		{
			name: "custom managed port",
			cfg:  Config{Mode: ModeManaged, Port: 8080, ServerDir: ready, LookPath: look(true)},
			mode: ModeManaged,
			url:  "http://127.0.0.1:8080",
		},
		{
			name: "unknown mode is off",
			cfg:  Config{Mode: "sidecar", LookPath: look(true)},
			mode: ModeOff,
			err:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.cfg)
			if got.Mode != tc.mode {
				t.Fatalf("mode %q want %q (err %q)", got.Mode, tc.mode, got.Err)
			}
			if got.BaseURL != tc.url {
				t.Fatalf("url %q want %q", got.BaseURL, tc.url)
			}
			if tc.err && got.Err == "" {
				t.Fatal("expected an error")
			}
			if !tc.err && got.Err != "" {
				t.Fatalf("unexpected err %q", got.Err)
			}
		})
	}
}

func TestParsePing(t *testing.T) {
	t.Parallel()
	ver, err := parsePing([]byte(`{"server_uptime":1.5,"version":"2.0.0"}`))
	if err != nil {
		t.Fatal(err)
	}
	if ver != "2.0.0" {
		t.Fatalf("version %q", ver)
	}
	if _, err := parsePing([]byte(`{"server_uptime":1}`)); err == nil {
		t.Fatal("expected missing version")
	}
	if _, err := parsePing([]byte(`not json`)); err == nil {
		t.Fatal("expected json error")
	}
}

func TestHealthMarksRunningAndReady(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ping" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"server_uptime":3,"version":"2.0.0"}`))
	}))
	defer srv.Close()

	ready := make(chan struct{}, 1)
	p := New(Config{
		Mode:         ModeExternal,
		URL:          srv.URL,
		PingInterval: 20 * time.Millisecond,
		LookPath:     func(string) (string, error) { return "", errors.New("missing") },
	})
	p.OnReady(func() { ready <- struct{}{} })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Start(ctx)

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ready")
	}
	st := p.Status()
	if !st.Running || st.Version != "2.0.0" || st.Mode != ModeExternal {
		t.Fatalf("status %+v", st)
	}
	if p.BaseURL() != srv.URL {
		t.Fatalf("base URL %q", p.BaseURL())
	}
}

func TestReadyFiresOnEachUpTransition(t *testing.T) {
	t.Parallel()
	var up atomic.Bool
	up.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"version":"2.0.0"}`))
	}))
	defer srv.Close()

	var n atomic.Int32
	p := New(Config{
		Mode:         ModeExternal,
		URL:          srv.URL,
		PingInterval: 15 * time.Millisecond,
		LookPath:     func(string) (string, error) { return "", errors.New("missing") },
	})
	p.OnReady(func() { n.Add(1) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Start(ctx)

	waitN := func(want int32) {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if n.Load() >= want {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("ready count %d want >= %d", n.Load(), want)
	}
	waitN(1)
	up.Store(false)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && p.Status().Running {
		time.Sleep(5 * time.Millisecond)
	}
	if p.Status().Running {
		t.Fatal("still running after ping failure")
	}
	up.Store(true)
	waitN(2)
}

func TestAutoHealthLoopAdoptsSidecar(t *testing.T) {
	t.Parallel()
	ready := make(chan struct{}, 1)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 2 {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"version":"2.0.0"}`))
	}))
	defer srv.Close()

	p := New(Config{
		Mode:         ModeAuto,
		URL:          srv.URL,
		PingInterval: 20 * time.Millisecond,
		LookPath:     func(string) (string, error) { return "", errors.New("missing") },
	})
	if p.BaseURL() != srv.URL || p.Status().Mode != ModeExternal {
		t.Fatalf("auto should watch the sidecar URL, got %+v %q", p.Status(), p.BaseURL())
	}
	p.OnReady(func() { ready <- struct{}{} })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Start(ctx)
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("auto never adopted the sidecar")
	}
}

func TestOffDoesNotStart(t *testing.T) {
	t.Parallel()
	p := New(Config{Mode: ModeOff, LookPath: func(string) (string, error) { return "/bin/deno", nil }})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	p.Start(ctx)
	st := p.Status()
	if st.Running || st.Mode != ModeOff || p.BaseURL() != "" {
		t.Fatalf("status %+v url %q", st, p.BaseURL())
	}
}

func TestNilProvider(t *testing.T) {
	t.Parallel()
	var p *Provider
	if p.BaseURL() != "" {
		t.Fatal("nil base URL")
	}
	st := p.Status()
	if st.Mode != ModeOff || st.Running {
		t.Fatalf("status %+v", st)
	}
	p.OnReady(func() { t.Fatal("should not run") })
	p.Start(context.Background())
}

func TestParsePingIgnoresWhitespace(t *testing.T) {
	t.Parallel()
	ver, err := parsePing([]byte(`{"version":" 2.0.0 "}`))
	if err != nil || ver != "2.0.0" {
		t.Fatalf("ver %q err %v", ver, err)
	}
}

func TestManagedStartCommand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.ts"), []byte("export {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	started := make(chan []string, 1)
	p := New(Config{
		Mode:         ModeManaged,
		Port:         4416,
		ServerDir:    dir,
		PingInterval: time.Hour,
		LookPath:     func(string) (string, error) { return "/usr/bin/deno", nil },
		Ping:         func(context.Context, string) (string, error) { return "", errors.New("down") },
		Command: func(ctx context.Context, name string, args []string, cwd string) error {
			started <- append([]string{name, cwd}, args...)
			<-ctx.Done()
			return ctx.Err()
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Start(ctx)
	select {
	case got := <-started:
		if got[0] != "/usr/bin/deno" {
			t.Fatalf("bin %q", got[0])
		}
		if !strings.HasSuffix(got[1], "node_modules") {
			t.Fatalf("cwd %q", got[1])
		}
		joined := strings.Join(got[2:], " ")
		if !strings.Contains(joined, "../src/main.ts") || !strings.Contains(joined, "--port 4416") {
			t.Fatalf("args %v", got[2:])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("managed command was not started")
	}
}
