package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thinkwright/claude-chronicle/internal/config"
	"github.com/thinkwright/claude-chronicle/internal/store"
	"github.com/thinkwright/claude-chronicle/internal/ui"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	reindex := false
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--version", "-v":
			fmt.Printf("clog %s\n", version)
			os.Exit(0)
		case "--reindex":
			reindex = true
		case "--add-path":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--add-path requires a directory argument")
				os.Exit(1)
			}
			i++
			dir := args[i]
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				fmt.Fprintf(os.Stderr, "not a valid directory: %s\n", dir)
				os.Exit(1)
			}
			cfg := config.Load()
			if cfg.AddProjectPath(dir) {
				config.Save(cfg)
				fmt.Printf("added project path: %s\n", filepath.Clean(dir))
			} else {
				fmt.Printf("path already configured: %s\n", filepath.Clean(dir))
			}
			os.Exit(0)
		case "--remove-path":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--remove-path requires a directory argument")
				os.Exit(1)
			}
			i++
			dir := args[i]
			cfg := config.Load()
			if cfg.RemoveProjectPath(dir) {
				config.Save(cfg)
				fmt.Printf("removed project path: %s\n", filepath.Clean(dir))
			} else {
				fmt.Fprintf(os.Stderr, "path not found in config: %s\n", filepath.Clean(dir))
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	db, err := store.Open(store.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening index: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if reindex {
		if err := db.Reset(); err != nil {
			fmt.Fprintf(os.Stderr, "error resetting index: %v\n", err)
			os.Exit(1)
		}
	}

	// Ensure terminal is large enough for the dashboard layout
	const minCols, minRows = 120, 40
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		if w < minCols || h < minRows {
			cols, rows := w, h
			if cols < minCols {
				cols = minCols
			}
			if rows < minRows {
				rows = minRows
			}
			fmt.Fprintf(os.Stdout, "\x1b[8;%d;%dt", rows, cols)
		}
	}

	p := tea.NewProgram(
		ui.NewModel(db),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
