package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Project struct {
	Name         string
	Path         string // original filesystem path
	EncodedName  string // folder name under ~/.claude/projects/
	DataDir      string // full path to project data dir
	SessionCount int
	LastModified int64 // unix timestamp
}

func ClaudeDir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// AllProjectsDirs returns the default projects directory plus any custom paths.
func AllProjectsDirs(extraPaths []string) []string {
	dirs := []string{ProjectsDir()}
	for _, p := range extraPaths {
		p = filepath.Clean(p)
		if p != dirs[0] {
			dirs = append(dirs, p)
		}
	}
	return dirs
}

func ProjectsDir() string {
	return filepath.Join(ClaudeDir(), "projects")
}

// decodePath converts a folder name like "-Users-jane-Dev-myproject"
// back to "/Users/jane/Dev/myproject".
// Because the encoding replaces "/" with "-", original paths containing
// hyphens are ambiguous. We validate the result and fall back to
// extracting the real path from JSONL data when the naive decode fails.
func decodePath(encoded string, dataDir string) string {
	if len(encoded) == 0 {
		return ""
	}
	// Naive decode: leading hyphen becomes /, remaining hyphens become /
	naive := "/" + strings.ReplaceAll(encoded[1:], "-", "/")

	// If the decoded path exists on disk, it's correct
	if _, err := os.Stat(naive); err == nil {
		return naive
	}

	// Fallback: extract real path from a JSONL file's cwd field
	if real := extractCwdFromDir(dataDir); real != "" {
		return real
	}

	return naive
}

// extractCwdFromDir reads the first JSONL file in dataDir to find the cwd field.
func extractCwdFromDir(dataDir string) string {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			if cwd := extractCwdFromFile(filepath.Join(dataDir, e.Name())); cwd != "" {
				return cwd
			}
		}
	}
	// Check UUID subdirectories (newer Claude Code layout)
	for _, e := range entries {
		if e.IsDir() {
			subEntries, _ := os.ReadDir(filepath.Join(dataDir, e.Name()))
			for _, sub := range subEntries {
				if !sub.IsDir() && strings.HasSuffix(sub.Name(), ".jsonl") {
					if cwd := extractCwdFromFile(filepath.Join(dataDir, e.Name(), sub.Name())); cwd != "" {
						return cwd
					}
				}
			}
		}
	}
	return ""
}

// extractCwdFromFile reads the first few lines of a JSONL file looking for the cwd field.
func extractCwdFromFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0), 512*1024)
	for i := 0; i < 5 && scanner.Scan(); i++ {
		var raw struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(scanner.Bytes(), &raw) == nil && raw.Cwd != "" {
			return raw.Cwd
		}
	}
	return ""
}

// shortName extracts the last path component as a display name.
func shortName(path string) string {
	return filepath.Base(path)
}

// countJSONLInDir counts .jsonl files in a directory and returns the latest mtime.
func countJSONLInDir(dir string) (count int, lastMod int64) {
	entries, _ := os.ReadDir(dir)
	for _, f := range entries {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
			count++
			if info, err := f.Info(); err == nil {
				mt := info.ModTime().UnixMilli()
				if mt > lastMod {
					lastMod = mt
				}
			}
		}
	}
	return
}

func DiscoverProjects(extraPaths []string) ([]Project, error) {
	dirs := AllProjectsDirs(extraPaths)

	seen := make(map[string]int) // decoded path -> index in projects slice
	var projects []Project

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // directory may not exist; skip silently
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}

			dataDir := filepath.Join(dir, e.Name())
			decoded := decodePath(e.Name(), dataDir)

			// Try index first, fall back to scanning JSONL files
			sessionCount := 0
			var lastMod int64

			idx, err := LoadSessionsIndex(dataDir)
			if err == nil {
				for _, s := range idx.Entries {
					if !s.IsSidechain {
						sessionCount++
					}
					if s.FileMtime > lastMod {
						lastMod = s.FileMtime
					}
				}
			} else {
				// No index -- count .jsonl files at root level
				c, lm := countJSONLInDir(dataDir)
				sessionCount += c
				if lm > lastMod {
					lastMod = lm
				}
				// Also check UUID subdirectories (newer Claude Code layout)
				subEntries, _ := os.ReadDir(dataDir)
				for _, sub := range subEntries {
					if sub.IsDir() {
						c, lm := countJSONLInDir(filepath.Join(dataDir, sub.Name()))
						sessionCount += c
						if lm > lastMod {
							lastMod = lm
						}
					}
				}
			}

			if sessionCount == 0 {
				continue
			}

			proj := Project{
				Name:         shortName(decoded),
				Path:         decoded,
				EncodedName:  e.Name(),
				DataDir:      dataDir,
				SessionCount: sessionCount,
				LastModified: lastMod,
			}

			// Deduplication: prefer the entry with the more recent modification
			if existingIdx, ok := seen[decoded]; ok {
				if lastMod > projects[existingIdx].LastModified {
					projects[existingIdx] = proj
				}
			} else {
				seen[decoded] = len(projects)
				projects = append(projects, proj)
			}
		}
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastModified > projects[j].LastModified
	})

	return projects, nil
}
