package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/thinkwright/claude-chronicle/internal/store"
)

type watchEditField int

const (
	editName watchEditField = iota
	editPattern
)

type WatchlistPane struct {
	items          []store.WatchItem
	cursor         int
	width          int
	height         int
	visible        bool
	editing        bool
	editField      watchEditField
	nameInput      textinput.Model
	patInput       textinput.Model
	confirmDelete  bool
}

func NewWatchlistPane() WatchlistPane {
	ni := textinput.New()
	ni.Placeholder = "watch name"
	ni.CharLimit = 64
	ni.Prompt = "name: "
	ni.PromptStyle = lipgloss.NewStyle().Foreground(ColorCyan)
	ni.TextStyle = lipgloss.NewStyle().Foreground(ColorWhite)
	ni.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorDim)

	pi := textinput.New()
	pi.Placeholder = "regex pattern"
	pi.CharLimit = 256
	pi.Prompt = "regex: "
	pi.PromptStyle = lipgloss.NewStyle().Foreground(ColorYellow)
	pi.TextStyle = lipgloss.NewStyle().Foreground(ColorWhite)
	pi.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorDim)

	return WatchlistPane{
		nameInput: ni,
		patInput:  pi,
	}
}

func (w *WatchlistPane) SetSize(width, height int) {
	w.width = width
	w.height = height
	w.nameInput.Width = width - 12
	w.patInput.Width = width - 12
}

func (w *WatchlistPane) SetItems(items []store.WatchItem) {
	w.items = items
	if w.cursor >= len(items) && len(items) > 0 {
		w.cursor = len(items) - 1
	}
}

func (w *WatchlistPane) IsVisible() bool {
	return w.visible
}

func (w *WatchlistPane) Toggle() {
	w.visible = !w.visible
}

func (w *WatchlistPane) Show() {
	w.visible = true
}

func (w *WatchlistPane) IsEditing() bool {
	return w.editing
}

func (w *WatchlistPane) IsConfirmingDelete() bool {
	return w.confirmDelete
}

func (w *WatchlistPane) AskDelete() {
	w.confirmDelete = true
}

func (w *WatchlistPane) CancelDelete() {
	w.confirmDelete = false
}

func (w *WatchlistPane) ConfirmDelete() {
	w.confirmDelete = false
}

func (w *WatchlistPane) Up() {
	if w.cursor > 0 {
		w.cursor--
	}
}

func (w *WatchlistPane) Down() {
	if w.cursor < len(w.items)-1 {
		w.cursor++
	}
}

func (w *WatchlistPane) Selected() *store.WatchItem {
	if w.cursor >= 0 && w.cursor < len(w.items) {
		return &w.items[w.cursor]
	}
	return nil
}

func (w *WatchlistPane) StartAdd() {
	w.editing = true
	w.editField = editName
	w.nameInput.SetValue("")
	w.patInput.SetValue("")
	w.nameInput.Focus()
	w.patInput.Blur()
}

func (w *WatchlistPane) CancelEdit() {
	w.editing = false
	w.nameInput.Blur()
	w.patInput.Blur()
}

// NextField moves to the next edit field. Returns true if on the last field.
func (w *WatchlistPane) NextField() bool {
	if w.editField == editName {
		w.editField = editPattern
		w.nameInput.Blur()
		w.patInput.Focus()
		return false
	}
	return true // on pattern field, ready to submit
}

func (w *WatchlistPane) EditName() string {
	return w.nameInput.Value()
}

func (w *WatchlistPane) EditPattern() string {
	return w.patInput.Value()
}

func (w *WatchlistPane) FinishEdit() (string, string) {
	w.editing = false
	w.nameInput.Blur()
	w.patInput.Blur()
	name := w.nameInput.Value()
	pattern := w.patInput.Value()
	if name == "" {
		name = pattern // use pattern as name if no name given
	}
	return name, pattern
}

// UpdateInput passes a tea.Msg to the active textinput so it handles
// cursor positioning, insertion, and deletion natively.
func (w *WatchlistPane) UpdateInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch w.editField {
	case editName:
		w.nameInput, cmd = w.nameInput.Update(msg)
	case editPattern:
		w.patInput, cmd = w.patInput.Update(msg)
	}
	return cmd
}

func (w *WatchlistPane) View() string {
	var lines []string

	if w.editing {
		innerW := w.width - 4 // account for panel border + padding
		if innerW < 10 {
			innerW = 10
		}
		nameView := w.nameInput.View()
		if visibleLen(nameView) > innerW {
			nameView = nameView[:innerW]
		}
		patView := w.patInput.View()
		if visibleLen(patView) > innerW {
			patView = patView[:innerW]
		}
		lines = append(lines, "  "+nameView)
		lines = append(lines, "  "+patView)
		var hint string
		if w.editField == editName {
			hint = "Tab → regex field  Esc → cancel"
		} else {
			hint = "Enter → save  Esc → cancel  (?i) for case-insensitive"
		}
		if len(hint)+2 > innerW {
			if w.editField == editName {
				hint = "Tab → next  Esc → cancel"
			} else {
				hint = "Enter → save  Esc → cancel"
			}
		}
		lines = append(lines, DimStyle.Render("  "+hint))
		return strings.Join(lines, "\n")
	}

	if w.confirmDelete {
		item := w.Selected()
		name := ""
		if item != nil {
			name = item.Name
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorRed).Render(
			fmt.Sprintf("  Delete \"%s\"?  y/n", name)))
		return strings.Join(lines, "\n")
	}

	if len(w.items) == 0 {
		return DimStyle.Render("  No watches  [W] to add")
	}

	available := w.height - 1
	if available < 1 {
		available = 1
	}

	start := 0
	if w.cursor >= available {
		start = w.cursor - available + 1
	}
	end := start + available
	if end > len(w.items) {
		end = len(w.items)
	}

	for i := start; i < end; i++ {
		item := w.items[i]

		// Dot indicator
		dot := DimStyle.Render("○")
		if item.UnseenCount > 0 {
			dotColor := lipgloss.Color(item.Color)
			dot = lipgloss.NewStyle().Foreground(dotColor).Render("●")
		}
		if !item.Enabled {
			dot = DimStyle.Render("◌")
		}

		// Pattern/name
		name := item.Name
		maxLen := w.width - 20
		if maxLen < 10 {
			maxLen = 10
		}
		if len(name) > maxLen {
			name = name[:maxLen-3] + "..."
		}

		// Count badge
		countStr := ""
		if item.UnseenCount > 0 {
			countStr = lipgloss.NewStyle().Foreground(lipgloss.Color(item.Color)).Render(
				fmt.Sprintf("%d new", item.UnseenCount))
		}

		if i == w.cursor {
			sel := lipgloss.NewStyle().Background(ColorSelectBg)
			marker := sel.Foreground(ColorSelect).Render("▸ ")
			nameStr := sel.Foreground(ColorSelect).Bold(true).Render(name)
			dotStr := sel.Render(dot)
			cntStr := sel.Render(countStr)
			lines = append(lines, fmt.Sprintf("  %s%s %s  %s", marker, dotStr, nameStr, cntStr))
		} else {
			nameStr := NormalStyle.Render(name)
			lines = append(lines, fmt.Sprintf("    %s %s  %s", dot, nameStr, countStr))
		}
	}

	return strings.Join(lines, "\n")
}
