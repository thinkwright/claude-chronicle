package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	WatchlistVisible bool     `json:"watchlist_visible"`
	DefaultScope     string   `json:"default_search_scope"` // "project", "global", "local"
	ProjectPaths     []string `json:"project_paths,omitempty"`
}

// AddProjectPath adds a directory to the custom paths list. Returns false if already present.
func (c *Config) AddProjectPath(path string) bool {
	clean := filepath.Clean(path)
	for _, p := range c.ProjectPaths {
		if filepath.Clean(p) == clean {
			return false
		}
	}
	c.ProjectPaths = append(c.ProjectPaths, clean)
	return true
}

// RemoveProjectPath removes a directory from the custom paths list. Returns false if not found.
func (c *Config) RemoveProjectPath(path string) bool {
	clean := filepath.Clean(path)
	for i, p := range c.ProjectPaths {
		if filepath.Clean(p) == clean {
			c.ProjectPaths = append(c.ProjectPaths[:i], c.ProjectPaths[i+1:]...)
			return true
		}
	}
	return false
}

func DefaultConfig() Config {
	return Config{
		WatchlistVisible: false,
		DefaultScope:     "project",
	}
}

func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "clog")
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".config", "clog")
	}
	return filepath.Join(home, ".config", "clog")
}

func configPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func Load() Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg) // ignore errors; fall back to defaults
	return cfg
}

func Save(cfg Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0o644)
}
