package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the tier-1 bootstrap configuration. Precedence: flags > LSV_ env > TOML > defaults.
type Config struct {
	ConfigPath     string
	DatabasePath   string
	DataDir        string
	HTTPPort       int
	HTTPBind       string
	JWTSecret      string
	DisplayEnabled bool
	DisplayDriver  string
	DisplayDevice  int
	LoggingLevel   string
	FrontendEmbed  bool
}

type fileConfig struct {
	Database databaseSection `toml:"database"`
	Data     dataSection     `toml:"data"`
	HTTP     httpSection     `toml:"http"`
	Display  displaySection  `toml:"display"`
	Logging  loggingSection  `toml:"logging"`
}

type databaseSection struct {
	Path string `toml:"path"`
}

type dataSection struct {
	Dir string `toml:"dir"`
}

type httpSection struct {
	Port      int    `toml:"port"`
	Bind      string `toml:"bind"`
	JWTSecret string `toml:"jwt_secret"`
}

type displaySection struct {
	Enabled bool   `toml:"enabled"`
	Driver  string `toml:"driver"`
	Device  int    `toml:"device"`
}

type loggingSection struct {
	Level string `toml:"level"`
}

func defaults() Config {
	return Config{
		DatabasePath:   "data/livestream-viewer.sqlite",
		DataDir:        "data",
		HTTPPort:       8099,
		HTTPBind:       "0.0.0.0",
		DisplayEnabled: false,
		LoggingLevel:   "info",
		FrontendEmbed:  true,
	}
}

// Load reads configuration from TOML, environment variables, and flags.
// Flag values already parsed via ParseFlags should be passed in; Load also
// applies env and file on top of defaults.
func Load(configPath string) (*Config, error) {
	cfg := defaults()
	cfg.ConfigPath = configPath

	path := configPath
	if path == "" {
		path = findDefaultTOML()
	}
	if path != "" {
		if err := loadTOML(path, &cfg); err != nil {
			return nil, err
		}
		cfg.ConfigPath = path
	}

	applyEnv(&cfg)
	return &cfg, nil
}

// ParseFlags defines and parses command-line flags, then overlays them on cfg.
func ParseFlags(cfg *Config) {
	configPath := flag.String("config", cfg.ConfigPath, "Path to livestream-viewer.toml")
	display := flag.Bool("display", cfg.DisplayEnabled, "Start the display engine")
	frontendEmbed := flag.Bool("frontend-embed", cfg.FrontendEmbed, "Serve the embedded web UI")
	httpPort := flag.Int("http-port", cfg.HTTPPort, "Management UI and API port")
	bind := flag.String("bind", cfg.HTTPBind, "HTTP bind address")
	flag.Parse()

	if *configPath != "" && *configPath != cfg.ConfigPath {
		if loaded, err := Load(*configPath); err == nil {
			*cfg = *loaded
		}
	}
	// Flags win.
	if isFlagPassed("display") {
		cfg.DisplayEnabled = *display
	}
	if isFlagPassed("frontend-embed") {
		cfg.FrontendEmbed = *frontendEmbed
	}
	if isFlagPassed("http-port") {
		cfg.HTTPPort = *httpPort
	}
	if isFlagPassed("bind") {
		cfg.HTTPBind = *bind
	}
	if isFlagPassed("config") {
		cfg.ConfigPath = *configPath
	}
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func findDefaultTOML() string {
	candidates := []string{
		"configs/livestream-viewer.toml",
		"livestream-viewer.toml",
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "livestream-viewer.toml"),
			filepath.Join(dir, "configs", "livestream-viewer.toml"),
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func loadTOML(path string, cfg *Config) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read config: %w", err)
	}
	var fc fileConfig
	if err := toml.Unmarshal(b, &fc); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	if fc.Database.Path != "" {
		cfg.DatabasePath = fc.Database.Path
	}
	if fc.Data.Dir != "" {
		cfg.DataDir = fc.Data.Dir
	}
	if fc.HTTP.Port != 0 {
		cfg.HTTPPort = fc.HTTP.Port
	}
	if fc.HTTP.Bind != "" {
		cfg.HTTPBind = fc.HTTP.Bind
	}
	if fc.HTTP.JWTSecret != "" {
		cfg.JWTSecret = fc.HTTP.JWTSecret
	}
	cfg.DisplayEnabled = fc.Display.Enabled
	if fc.Display.Driver != "" {
		cfg.DisplayDriver = fc.Display.Driver
	}
	cfg.DisplayDevice = fc.Display.Device
	if fc.Logging.Level != "" {
		cfg.LoggingLevel = fc.Logging.Level
	}
	return nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("LSV_DATABASE_PATH"); v != "" {
		cfg.DatabasePath = v
	}
	if v := os.Getenv("LSV_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("LSV_HTTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.HTTPPort = n
		}
	}
	if v := os.Getenv("LSV_HTTP_BIND"); v != "" {
		cfg.HTTPBind = v
	}
	if v := os.Getenv("LSV_HTTP_JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}
	if v := os.Getenv("LSV_DISPLAY_ENABLED"); v != "" {
		cfg.DisplayEnabled = parseBool(v)
	}
	if v := os.Getenv("LSV_DISPLAY_DRIVER"); v != "" {
		cfg.DisplayDriver = v
	}
	if v := os.Getenv("LSV_DISPLAY_DEVICE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DisplayDevice = n
		}
	}
	if v := os.Getenv("LSV_LOGGING_LEVEL"); v != "" {
		cfg.LoggingLevel = v
	}
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// EnsureDirs creates the data directory and the parent of the database file.
func (c *Config) EnsureDirs() error {
	if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(c.DataDir, "uploads"), 0o755); err != nil {
		return fmt.Errorf("uploads dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(c.DataDir, "thumbnails"), 0o755); err != nil {
		return fmt.Errorf("thumbnails dir: %w", err)
	}
	if dir := filepath.Dir(c.DatabasePath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("database dir: %w", err)
		}
	}
	return nil
}

// ResolveJWTSecret returns the configured secret, or a persisted generated one.
func (c *Config) ResolveJWTSecret() (string, error) {
	if c.JWTSecret != "" {
		return c.JWTSecret, nil
	}
	path := filepath.Join(c.DataDir, ".jwt_secret")
	if b, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(b))
		if s != "" {
			c.JWTSecret = s
			return s, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("jwt secret: %w", err)
	}
	s := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(s+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("persist jwt secret: %w", err)
	}
	c.JWTSecret = s
	return s, nil
}

// Addr returns the HTTP listen address.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.HTTPBind, c.HTTPPort)
}
