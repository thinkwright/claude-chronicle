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
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
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

func DiscoverProjects() ([]Project, error) {
	dir := ProjectsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var projects []Project
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
			// No index — count .jsonl files and get mtime
			files, _ := os.ReadDir(dataDir)
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
					sessionCount++
					if info, err := f.Info(); err == nil {
						mt := info.ModTime().UnixMilli()
						if mt > lastMod {
							lastMod = mt
						}
					}
				}
			}
		}

		if sessionCount == 0 {
			continue
		}

		projects = append(projects, Project{
			Name:         shortName(decoded),
			Path:         decoded,
			EncodedName:  e.Name(),
			DataDir:      dataDir,
			SessionCount: sessionCount,
			LastModified:  lastMod,
		})
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastModified > projects[j].LastModified
	})

	return projects, nil
}
