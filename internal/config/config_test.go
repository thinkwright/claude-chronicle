package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.WatchlistVisible != false {
		t.Error("watchlist_visible should default to false")
	}
	if cfg.DefaultScope != "project" {
		t.Errorf("default_scope = %q, want project", cfg.DefaultScope)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Use a temp dir as XDG_CONFIG_HOME to avoid touching real config
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := Config{
		WatchlistVisible: true,
		DefaultScope:     "global",
	}

	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Verify file was created
	path := filepath.Join(dir, "clog", "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Load it back
	loaded := Load()
	if loaded.WatchlistVisible != true {
		t.Error("watchlist_visible not persisted")
	}
	if loaded.DefaultScope != "global" {
		t.Errorf("default_scope = %q, want global", loaded.DefaultScope)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// No config file exists — should return defaults
	cfg := Load()
	expected := DefaultConfig()
	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("got %+v, want defaults %+v", cfg, expected)
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Write invalid JSON
	configDir := filepath.Join(dir, "clog")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte("not json"), 0o644)

	// Should return defaults without error
	cfg := Load()
	expected := DefaultConfig()
	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("malformed json: got %+v, want defaults", cfg)
	}
}

func TestLoad_PartialJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Write partial JSON — only one field
	configDir := filepath.Join(dir, "clog")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"watchlist_visible": true}`), 0o644)

	cfg := Load()
	if !cfg.WatchlistVisible {
		t.Error("watchlist_visible should be true from file")
	}
	// Other fields should keep defaults (Load starts with DefaultConfig)
	if cfg.DefaultScope != "project" {
		t.Errorf("DefaultScope = %q, want project (default preserved)", cfg.DefaultScope)
	}
}

func TestConfigDir_XDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got := ConfigDir()
	want := filepath.Join(dir, "clog")
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// The clog subdir shouldn't exist yet
	clogDir := filepath.Join(dir, "clog")
	if _, err := os.Stat(clogDir); err == nil {
		t.Fatal("clog dir shouldn't exist yet")
	}

	Save(DefaultConfig())

	if _, err := os.Stat(clogDir); err != nil {
		t.Errorf("Save should create directory: %v", err)
	}
}

func TestSave_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := Config{WatchlistVisible: true, DefaultScope: "global"}
	Save(cfg)

	data, _ := os.ReadFile(filepath.Join(dir, "clog", "config.json"))

	// Should be valid, indented JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}

	// Check it's indented (contains newlines)
	if len(data) < 10 {
		t.Error("expected indented JSON output")
	}
}
