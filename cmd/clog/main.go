package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thinkwright/claude-chronicle/internal/store"
	"github.com/thinkwright/claude-chronicle/internal/ui"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	reindex := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--version", "-v":
			fmt.Printf("clog %s\n", version)
			os.Exit(0)
		case "--reindex":
			reindex = true
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
