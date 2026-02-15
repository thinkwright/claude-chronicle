package watcher

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
	"github.com/thinkwright/claude-chronicle/internal/claude"
)

type RefreshMsg struct{}

func Watch() tea.Cmd {
	return func() tea.Msg {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}

		dir := claude.ProjectsDir()
		_ = w.Add(dir)

		// Also watch individual project dirs for session changes
		projects, _ := claude.DiscoverProjects()
		for _, p := range projects {
			_ = w.Add(p.DataDir)
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
