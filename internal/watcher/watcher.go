package watcher

import (
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
	"github.com/thinkwright/claude-chronicle/internal/claude"
)

type RefreshMsg struct{}

func Watch(projectPaths []string) tea.Cmd {
	return func() tea.Msg {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}

		for _, dir := range claude.AllProjectsDirs(projectPaths) {
			_ = w.Add(dir)
		}

		// Also watch individual project dirs and UUID subdirs for session changes
		projects, _ := claude.DiscoverProjects(projectPaths)
		for _, p := range projects {
			_ = w.Add(p.DataDir)
			// Watch UUID subdirectories (newer Claude Code layout)
			entries, _ := os.ReadDir(p.DataDir)
			for _, e := range entries {
				if e.IsDir() {
					_ = w.Add(filepath.Join(p.DataDir, e.Name()))
				}
			}
		}

		// Debounce — wait for changes to settle
		debounce := time.NewTimer(time.Hour)
		debounce.Stop()

		for {
			select {
			case _, ok := <-w.Events:
				if !ok {
					return nil
				}
				debounce.Reset(500 * time.Millisecond)
			case <-debounce.C:
				return RefreshMsg{}
			case <-w.Errors:
				continue
			}
		}
	}
}
