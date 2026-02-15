package claude

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MemoryFile represents a single memory file from a project.
type MemoryFile struct {
	Name    string // display name (e.g. "MEMORY.md", "patterns.md")
	Path    string // full filesystem path
	Content string // file contents
}

// LoadMemory reads all memory files for a project data directory.
// Returns MEMORY.md first (if it exists), then other .md files sorted by name.
func LoadMemory(dataDir string) ([]MemoryFile, error) {
	memDir := filepath.Join(dataDir, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return nil, err
	}

	var main []MemoryFile
	var others []MemoryFile

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		path := filepath.Join(memDir, e.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		mf := MemoryFile{
			Name:    e.Name(),
			Path:    path,
			Content: string(content),
		}

		if strings.EqualFold(e.Name(), "MEMORY.md") {
			main = append(main, mf)
		} else {
			others = append(others, mf)
		}
	}

	sort.Slice(others, func(i, j int) bool {
		return others[i].Name < others[j].Name
	})

	return append(main, others...), nil
}
