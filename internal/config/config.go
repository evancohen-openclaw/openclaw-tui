package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds TUI startup configuration from CLI flags, config file, and env.
type Config struct {
	URL         string `json:"url,omitempty"`
	Token       string `json:"token,omitempty"`
	Password    string `json:"password,omitempty"`
	Session     string `json:"session,omitempty"`
	Theme       string `json:"theme,omitempty"`       // "dark", "light", or ""
	TLSInsecure bool   `json:"tlsInsecure,omitempty"` // skip TLS cert verification for self-signed certs
	Version     string `json:"-"`
}

// DefaultConfigDir returns the default config directory.
func DefaultConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "openclaw-tui")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "openclaw-tui")
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.json")
}

// Load reads configuration from file, then overlays env vars.
// CLI flags take priority and should be applied after this.
func Load(path string) (Config, error) {
	var cfg Config

	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file is fine, use defaults
			cfg.applyEnv()
			cfg.applyDefaults()
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.applyEnv()
	cfg.applyDefaults()
	return cfg, nil
}

// Save writes the config to disk (creates directories as needed).
func (c Config) Save(path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Only persist fields the user would want saved
	saved := Config{
		URL:      c.URL,
		Token:    c.Token,
		Password: c.Password,
		Session:  c.Session,
		Theme:    c.Theme,
	}

	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, append(data, '\n'), 0600)
}

// applyEnv overlays environment variables onto empty fields.
func (c *Config) applyEnv() {
	if c.URL == "" {
		if v := os.Getenv("OPENCLAW_GATEWAY_URL"); v != "" {
			c.URL = v
		}
	}
	if c.Token == "" {
		if v := os.Getenv("OPENCLAW_GATEWAY_TOKEN"); v != "" {
			c.Token = v
		}
	}
	if c.Password == "" {
		if v := os.Getenv("OPENCLAW_GATEWAY_PASSWORD"); v != "" {
			c.Password = v
		}
	}
	if c.Theme == "" {
		if v := os.Getenv("OPENCLAW_THEME"); v != "" {
			c.Theme = strings.ToLower(v)
		}
	}
}

// applyDefaults fills in remaining empty fields with defaults.
func (c *Config) applyDefaults() {
	if c.URL == "" {
		c.URL = "ws://127.0.0.1:18789"
	}
}
