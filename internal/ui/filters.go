package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/thinkwright/claude-chronicle/internal/store"
)

type FilterBar struct {
	filters []store.Filter
	active  bool
	editing bool
	input   textinput.Model
	width   int
}

func NewFilterBar() FilterBar {
	ti := textinput.New()
	ti.Placeholder = "type:user  model:opus  tool:Bash  tokens:>10000"
	ti.CharLimit = 256
	ti.Prompt = "filter log: "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorYellow)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorWhite)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorDim)
	return FilterBar{input: ti}
}

func (f *FilterBar) SetWidth(w int) {
	f.width = w
	f.input.Width = w - 20
}

func (f *FilterBar) OpenEditor() {
	f.editing = true
	// Pre-fill with current filters
	f.input.SetValue(f.String())
	f.input.Focus()
}

func (f *FilterBar) CloseEditor() {
	f.editing = false
	f.input.Blur()
}

func (f *FilterBar) IsEditing() bool {
	return f.editing
}

func (f *FilterBar) ApplyFromInput() {
	raw := f.input.Value()
	f.editing = false
	f.input.Blur()

	if raw == "" {
		f.filters = nil
		return
	}

	fs := store.Parse(raw)
	f.filters = fs.Filters
}

func (f *FilterBar) Clear() {
	f.filters = nil
	f.editing = false
	f.input.SetValue("")
}

func (f *FilterBar) HasFilters() bool {
	return len(f.filters) > 0
}

func (f *FilterBar) Filters() []store.Filter {
	return f.filters
}

// FilterQuery returns the raw filter expression for use with the store.
func (f *FilterBar) FilterQuery() string {
	return f.String()
}

func (f *FilterBar) String() string {
	var parts []string
	for _, fl := range f.filters {
		parts = append(parts, filterToString(fl))
	}
	return strings.Join(parts, " ")
}

func (f *FilterBar) Value() string {
	return f.input.Value()
}

func (f *FilterBar) SetValue(v string) {
	f.input.SetValue(v)
}

// UpdateInput passes a tea.Msg to the textinput for native cursor handling.
func (f *FilterBar) UpdateInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return cmd
}

func (f *FilterBar) View() string {
	if f.editing {
		box := lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorYellow).
			Padding(0, 1).
			Width(f.width - 4)
		return box.Render(f.input.View())
	}

	if !f.HasFilters() {
		return ""
	}

	// Render filter chips
	var chips []string
	chipStyle := lipgloss.NewStyle().
		Foreground(ColorYellow).
		Bold(true)

	for _, fl := range f.filters {
		chips = append(chips, chipStyle.Render(filterToString(fl)))
	}

	hint := DimStyle.Render("  [f:edit  F:clear]")
	label := lipgloss.NewStyle().Foreground(ColorYellowDim).Render("  FILTERS: ")
	return label + strings.Join(chips, "  ") + hint
}

func filterToString(f store.Filter) string {
	field := ""
	switch f.Field {
	case store.FilterModel:
		field = "model"
	case store.FilterBranch:
		field = "branch"
	case store.FilterProject:
		field = "project"
	case store.FilterType:
		field = "type"
	case store.FilterTool:
		field = "tool"
	case store.FilterTokens:
		field = "tokens"
	case store.FilterAge:
		field = "age"
	}

	op := ""
	switch f.Op {
	case store.OpGreaterThan:
		op = ">"
	case store.OpLessThan:
		op = "<"
	}

	return fmt.Sprintf("%s:%s%s", field, op, f.Value)
}
