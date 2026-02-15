package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// HookEntry represents a single hook command within an event group.
type HookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
	Matcher string `json:"-"` // populated from parent group
}

// hookGroup is an event group with an optional matcher and list of hooks.
type hookGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []HookEntry `json:"hooks"`
}

// HookEvent groups all hooks under a single event name (e.g. "PreToolUse").
type HookEvent struct {
	Event   string
	Groups  []hookGroup
}

// HooksSource represents hooks loaded from a single settings file.
type HooksSource struct {
	Label  string      // "global", "project", "project-local"
	Path   string      // filesystem path to settings file
	Events []HookEvent // all hook events found
}

// settingsFile is the minimal structure we parse from settings JSON.
type settingsFile struct {
	Hooks map[string][]hookGroup `json:"hooks"`
}

// LoadHooksFromFile reads hooks from a single settings.json file.
func LoadHooksFromFile(path, label string) (*HooksSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var sf settingsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, err
	}

	if len(sf.Hooks) == 0 {
		return nil, nil
	}

	src := &HooksSource{
		Label: label,
		Path:  path,
	}

	for event, groups := range sf.Hooks {
		// Stamp matcher from group onto each hook for display
		for i := range groups {
			for j := range groups[i].Hooks {
				groups[i].Hooks[j].Matcher = groups[i].Matcher
			}
		}
		src.Events = append(src.Events, HookEvent{
			Event:  event,
			Groups: groups,
		})
	}

	return src, nil
}

// LoadAllHooks loads hooks from global settings and from a project path.
// Returns all sources that have hooks (may be empty).
func LoadAllHooks(projectPath string) []HooksSource {
	var sources []HooksSource

	// Global: ~/.claude/settings.json
	globalPath := filepath.Join(ClaudeDir(), "settings.json")
	if src, err := LoadHooksFromFile(globalPath, "global"); err == nil && src != nil {
		sources = append(sources, *src)
	}

	if projectPath == "" {
		return sources
	}

	// Project: {projectPath}/.claude/settings.json
	projPath := filepath.Join(projectPath, ".claude", "settings.json")
	if src, err := LoadHooksFromFile(projPath, "project"); err == nil && src != nil {
		sources = append(sources, *src)
	}

	// Project local: {projectPath}/.claude/settings.local.json
	localPath := filepath.Join(projectPath, ".claude", "settings.local.json")
	if src, err := LoadHooksFromFile(localPath, "project-local"); err == nil && src != nil {
		sources = append(sources, *src)
	}

	return sources
}
