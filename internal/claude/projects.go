package claude

import (
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
func decodePath(encoded string) string {
	if len(encoded) == 0 {
		return ""
	}
	// Leading hyphen becomes leading slash, remaining hyphens become slashes
	return "/" + strings.ReplaceAll(encoded[1:], "-", "/")
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
			decoded := decodePath(e.Name())

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
