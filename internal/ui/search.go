package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/thinkwright/claude-chronicle/internal/store"
)

type SearchScope int

const (
	ScopeProject SearchScope = iota
	ScopeGlobal
	ScopeLocal // within current session
)

func (s SearchScope) String() string {
	switch s {
	case ScopeProject:
		return "PROJECT"
	case ScopeGlobal:
		return "GLOBAL"
	case ScopeLocal:
		return "LOCAL"
	}
	return ""
}

type SearchOverlay struct {
	input       textinput.Model
	active      bool
	scope       SearchScope
	width       int
	results     []store.SearchResult
	resultIdx   int
	resultCount int
}

func NewSearchOverlay() SearchOverlay {
	ti := textinput.New()
	ti.Placeholder = "search... (Tab: scope, Enter: go)"
	ti.CharLimit = 256
	ti.Prompt = "/ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorCyan)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorWhite)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorDim)
	return SearchOverlay{input: ti, scope: ScopeProject}
}

func (s *SearchOverlay) SetWidth(w int) {
	s.width = w
	s.input.Width = w - 20 // leave room for scope badge
}

func (s *SearchOverlay) Open() {
	s.active = true
	s.input.SetValue("")
	s.input.Focus()
	s.results = nil
	s.resultIdx = 0
	s.resultCount = 0
}

func (s *SearchOverlay) Close() {
	s.active = false
	s.input.Blur()
	s.input.SetValue("")
	s.results = nil
	s.resultCount = 0
}

func (s *SearchOverlay) IsActive() bool {
	return s.active
}

func (s *SearchOverlay) Value() string {
	return s.input.Value()
}

func (s *SearchOverlay) SetValue(v string) {
	s.input.SetValue(v)
}

func (s *SearchOverlay) Scope() SearchScope {
	return s.scope
}

func (s *SearchOverlay) CycleScope() {
	switch s.scope {
	case ScopeProject:
		s.scope = ScopeGlobal
	case ScopeGlobal:
		s.scope = ScopeLocal
	case ScopeLocal:
		s.scope = ScopeProject
	}
}

func (s *SearchOverlay) SetResults(results []store.SearchResult, count int) {
	s.results = results
	s.resultCount = count
	s.resultIdx = 0
}

func (s *SearchOverlay) SelectedResult() *store.SearchResult {
	if s.resultIdx >= 0 && s.resultIdx < len(s.results) {
		return &s.results[s.resultIdx]
	}
	return nil
}

func (s *SearchOverlay) ResultUp() {
	if s.resultIdx > 0 {
		s.resultIdx--
	}
}

func (s *SearchOverlay) ResultDown() {
	if s.resultIdx < len(s.results)-1 {
		s.resultIdx++
	}
}

func (s *SearchOverlay) HasResults() bool {
	return len(s.results) > 0
}

// UpdateInput forwards a key message to the underlying textinput.
func (s *SearchOverlay) UpdateInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return cmd
}

// Matches is a legacy compatibility method for substring matching.
// Used as fallback when store is not yet indexed.
func (s *SearchOverlay) Matches(text string) bool {
	query := strings.ToLower(s.input.Value())
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(text), query)
}

func (s *SearchOverlay) View() string {
	if !s.active {
		return ""
	}

	// Scope badge
	scopeStyle := lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true)
	scopeBadge := scopeStyle.Render(fmt.Sprintf("[%s]", s.scope))

	// Count indicator
	countStr := ""
	if s.resultCount > 0 {
		countStr = DimStyle.Render(fmt.Sprintf("  %d results", s.resultCount))
	}

	searchLine := fmt.Sprintf("%s %s%s", scopeBadge, s.input.View(), countStr)

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorCyan).
		Padding(0, 1).
		Width(s.width - 4)

	if len(s.results) == 0 {
		return box.Render(searchLine)
	}

	// Results dropdown
	var lines []string
	lines = append(lines, searchLine)
	lines = append(lines, DimStyle.Render(strings.Repeat("─", s.width-8)))

	maxShow := 10
	if len(s.results) < maxShow {
		maxShow = len(s.results)
	}

	for i := 0; i < maxShow; i++ {
		r := s.results[i]

		// Format: project/branch: "highlighted text..."
		prefix := lipgloss.NewStyle().Foreground(ColorCyan).Render(r.Project)
		if r.GitBranch != "" {
			prefix += DimStyle.Render("/" + r.GitBranch)
		}

		text := r.Text
		if len(text) > s.width-30 {
			text = text[:s.width-33] + "..."
		}
		text = strings.ReplaceAll(text, "\n", " ")

		if i == s.resultIdx {
			marker := SelectedStyle.Render("▸ ")
			textStr := SelectedStyle.Render(text)
			lines = append(lines, fmt.Sprintf("  %s%s: %s", marker, prefix, textStr))
		} else {
			textStr := NormalStyle.Render(text)
			lines = append(lines, fmt.Sprintf("    %s: %s", prefix, textStr))
		}
	}

	if len(s.results) > maxShow {
		lines = append(lines, DimStyle.Render(
			fmt.Sprintf("    ... and %d more", len(s.results)-maxShow)))
	}

	return box.Render(strings.Join(lines, "\n"))
}
