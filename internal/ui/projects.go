package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/thinkwright/claude-chronicle/internal/claude"
)

type ProjectList struct {
	projects []claude.Project
	cursor   int
	width    int
	height   int
}

func NewProjectList() ProjectList {
	return ProjectList{}
}

func (p *ProjectList) SetProjects(projects []claude.Project) {
	p.projects = projects
	if p.cursor >= len(projects) {
		p.cursor = 0
	}
}

func (p *ProjectList) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *ProjectList) Up() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *ProjectList) Down() {
	if p.cursor < len(p.projects)-1 {
		p.cursor++
	}
}

func (p *ProjectList) Selected() *claude.Project {
	if len(p.projects) == 0 {
		return nil
	}
	return &p.projects[p.cursor]
}

// projectGlyph returns a status glyph based on recency.
func projectGlyph(lastMod int64) string {
	age := time.Since(time.UnixMilli(lastMod))
	switch {
	case age < 1*time.Hour:
		return lipgloss.NewStyle().Foreground(ColorGreen).Render("◆")
	case age < 24*time.Hour:
		return lipgloss.NewStyle().Foreground(ColorCyan).Render("◆")
	case age < 7*24*time.Hour:
		return lipgloss.NewStyle().Foreground(ColorDim).Render("◇")
	default:
		return lipgloss.NewStyle().Foreground(ColorMuted).Render("◇")
	}
}

func (p *ProjectList) View() string {
	if len(p.projects) == 0 {
		return "\n" + DimStyle.Render("  No projects found")
	}

	var lines []string

	available := p.height - 1
	if available < 1 {
		available = 1
	}

	start := 0
	if p.cursor >= available {
		start = p.cursor - available + 1
	}
	end := start + available
	if end > len(p.projects) {
		end = len(p.projects)
	}

	scrollbar := RenderScrollbar(available, len(p.projects), start)
	innerW := p.width - 3

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

		proj := p.projects[i]
		glyph := projectGlyph(proj.LastModified)
		count := BadgeStyle.Render(fmt.Sprintf("(%d)", proj.SessionCount))

		name := proj.Name
		if len(name) > p.width-14 {
			name = name[:p.width-17] + "..."
		}

		var line string
		if i == p.cursor {
			sel := lipgloss.NewStyle().Background(ColorSelectBg)
			marker := sel.Foreground(ColorSelect).Render("▸")
			nameStr := sel.Foreground(ColorSelect).Bold(true).Render(name)
			countStr := sel.Foreground(ColorSelect).Bold(false).Render(fmt.Sprintf("(%d)", proj.SessionCount))
			glyphStr := sel.Render(glyph) // keep glyph original color but add bg
			line = fmt.Sprintf(" %s %s %s %s", marker, glyphStr, nameStr, countStr)
			pad := innerW - visibleLen(line)
			if pad < 0 {
				pad = 0
			}
			lines = append(lines, line+sel.Render(strings.Repeat(" ", pad))+sb)
		} else {
			nameStr := NormalStyle.Render(name)
			line = fmt.Sprintf("   %s %s %s", glyph, nameStr, count)
			pad := innerW - visibleLen(line)
			if pad < 0 {
				pad = 0
			}
			lines = append(lines, line+strings.Repeat(" ", pad)+sb)
		}
	}

	return strings.Join(lines, "\n")
}
