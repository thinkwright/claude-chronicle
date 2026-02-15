package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	WatchlistVisible bool   `json:"watchlist_visible"`
	DefaultScope     string `json:"default_search_scope"` // "project", "global", "local"
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
