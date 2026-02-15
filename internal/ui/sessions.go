package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/thinkwright/claude-chronicle/internal/claude"
)

type SessionList struct {
	sessions    []claude.SessionEntry
	cursor      int
	width       int
	height      int
	projectName string
}

func NewSessionList() SessionList {
	return SessionList{}
}

func (s *SessionList) SetSessions(sessions []claude.SessionEntry, projectName string) {
	s.sessions = sessions
	s.projectName = projectName
	s.cursor = 0
}

func (s *SessionList) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *SessionList) Up() {
	if s.cursor > 0 {
		s.cursor--
	}
}

func (s *SessionList) Down() {
	if s.cursor < len(s.sessions)-1 {
		s.cursor++
	}
}

func (s *SessionList) ProjectName() string {
	return s.projectName
}

func (s *SessionList) Selected() *claude.SessionEntry {
	if len(s.sessions) == 0 {
		return nil
	}
	return &s.sessions[s.cursor]
}

// sessionSizeGlyph returns a bar character indicating relative session size.
func sessionSizeGlyph(count int) string {
	bars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇'}
	idx := 0
	switch {
	case count > 200:
		idx = 6
	case count > 100:
		idx = 5
	case count > 50:
		idx = 4
	case count > 20:
		idx = 3
	case count > 10:
		idx = 2
	case count > 5:
		idx = 1
	}
	color := ColorDim
	if idx >= 4 {
		color = ColorCyan
	} else if idx >= 2 {
		color = ColorCyanDim
	}
	return lipgloss.NewStyle().Foreground(color).Render(string(bars[idx]))
}

func (s *SessionList) View() string {
	var lines []string

	if len(s.sessions) == 0 {
		return "\n" + DimStyle.Render("  Select a project")
	}

	available := s.height - 1
	if available < 1 {
		available = 1
	}

	start := 0
	if s.cursor >= available {
		start = s.cursor - available + 1
	}
	end := start + available
	if end > len(s.sessions) {
		end = len(s.sessions)
	}

	innerW := s.width - 3
	maxPromptLen := s.width - 22

	scrollbar := RenderScrollbar(available, len(s.sessions), start)

	for idx := 0; idx < available; idx++ {
		i := start + idx
		sb := " "
		if idx < len(scrollbar) {
			sb = scrollbar[idx]
		}

		if i >= end {
			lines = append(lines, strings.Repeat(" ", innerW)+sb)
			continue
		}

		sess := s.sessions[i]
		sizeBar := sessionSizeGlyph(sess.MessageCount)

		prompt := strings.ReplaceAll(sess.FirstPrompt, "\n", " ")
		if len(prompt) > maxPromptLen {
			prompt = prompt[:maxPromptLen-3] + "..."
		}
		if prompt == "" {
			prompt = "(empty)"
		}

		age := formatAge(sess.Modified)

		var line string
		if i == s.cursor {
			sel := lipgloss.NewStyle().Background(ColorSelectBg)
			marker := sel.Foreground(ColorSelect).Render("▸")
			promptStr := sel.Foreground(ColorSelect).Bold(true).Render(prompt)
			ageStr := sel.Foreground(ColorSelect).Bold(false).Render(age)
			sizeStr := sel.Render(sizeBar)
			line = fmt.Sprintf(" %s %s %s  %s", marker, sizeStr, promptStr, ageStr)
			pad := innerW - visibleLen(line)
			if pad < 0 {
				pad = 0
			}
			lines = append(lines, line+sel.Render(strings.Repeat(" ", pad))+sb)
		} else {
			promptStr := NormalStyle.Render(prompt)
			ageStr := DimStyle.Render(age)
			line = fmt.Sprintf("   %s %s  %s", sizeBar, promptStr, ageStr)
			pad := innerW - visibleLen(line)
			if pad < 0 {
				pad = 0
			}
			lines = append(lines, line+strings.Repeat(" ", pad)+sb)
		}
	}

	return strings.Join(lines, "\n")
}

func formatAge(isoTime string) string {
	t, err := time.Parse(time.RFC3339Nano, isoTime)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.000Z", isoTime)
		if err != nil {
			return ""
		}
	}

	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}
