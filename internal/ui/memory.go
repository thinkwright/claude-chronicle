package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/thinkwright/claude-chronicle/internal/claude"
)

// MemoryModal displays project memory files in a centered overlay.
type MemoryModal struct {
	visible     bool
	files       []claude.MemoryFile
	fileIdx     int // which file is selected
	scroll      int // scroll offset within current file
	lines       []string
	width       int
	height      int
	projectName string
}

func NewMemoryModal() MemoryModal {
	return MemoryModal{}
}

func (m *MemoryModal) IsVisible() bool {
	return m.visible
}

func (m *MemoryModal) Show(projectName string, files []claude.MemoryFile) {
	m.visible = true
	m.files = files
	m.fileIdx = 0
	m.scroll = 0
	m.projectName = projectName
	m.renderLines()
}

func (m *MemoryModal) Close() {
	m.visible = false
}

func (m *MemoryModal) SetSize(w, h int) {
	m.width = w
	m.height = h
	if m.visible {
		m.renderLines()
	}
}

func (m *MemoryModal) ScrollUp(n int) {
	m.scroll -= n
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m *MemoryModal) ScrollDown(n int) {
	m.scroll += n
	maxScroll := len(m.lines) - m.contentHeight()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

// NextFile switches to the next memory file.
func (m *MemoryModal) NextFile() {
	if len(m.files) <= 1 {
		return
	}
	m.fileIdx = (m.fileIdx + 1) % len(m.files)
	m.scroll = 0
	m.renderLines()
}

// PrevFile switches to the previous memory file.
func (m *MemoryModal) PrevFile() {
	if len(m.files) <= 1 {
		return
	}
	m.fileIdx--
	if m.fileIdx < 0 {
		m.fileIdx = len(m.files) - 1
	}
	m.scroll = 0
	m.renderLines()
}

func (m *MemoryModal) contentHeight() int {
	h := m.height * 70 / 100
	if h < 5 {
		h = 5
	}
	return h
}

func (m *MemoryModal) renderLines() {
	m.lines = nil
	if len(m.files) == 0 {
		return
	}

	contentW := m.modalWidth() - 6 // modal padding
	if contentW < 20 {
		contentW = 20
	}

	content := m.files[m.fileIdx].Content
	m.lines = WrapText(content, contentW)
}

func (m *MemoryModal) modalWidth() int {
	w := m.width * 60 / 100
	if w > 90 {
		w = 90
	}
	if w < 40 {
		w = 40
	}
	return w
}

// View renders the centered modal overlay.
func (m *MemoryModal) View() string {
	if !m.visible {
		return ""
	}

	modalW := m.modalWidth()
	contentH := m.contentHeight()
	innerW := modalW - 4 // border + padding

	bc := lipgloss.NewStyle().Foreground(ColorAccent)
	tc := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(ColorDim)

	var rows []string

	// Top border with title
	title := fmt.Sprintf(" MEMORY — %s ", strings.ToUpper(m.projectName))
	titleVisLen := utf8.RuneCountInString(title)
	fillLen := innerW - 3 - titleVisLen // ┏(1) ━(1) ╸(1) title ╺(1) fill ┓(1) = innerW+2 total
	if fillLen < 0 {
		fillLen = 0
	}
	topBorder := bc.Render("┏━╸") + tc.Render(title) + bc.Render("╺"+strings.Repeat("━", fillLen)+"┓")
	rows = append(rows, topBorder)

	side := bc.Render("┃")

	// File tabs (if multiple files)
	if len(m.files) > 1 {
		var tabs []string
		for i, f := range m.files {
			name := f.Name
			if len(name) > 20 {
				name = name[:17] + "..."
			}
			if i == m.fileIdx {
				tabs = append(tabs, lipgloss.NewStyle().
					Foreground(ColorSelect).Bold(true).Render(" "+name+" "))
			} else {
				tabs = append(tabs, dim.Render(" "+name+" "))
			}
		}
		tabLine := "  " + strings.Join(tabs, dim.Render("│"))
		pad := innerW - visibleLen(tabLine)
		if pad < 0 {
			pad = 0
		}
		rows = append(rows, side+tabLine+strings.Repeat(" ", pad)+side)

		// Separator
		sep := dim.Render("  " + strings.Repeat("─", innerW-2))
		pad = innerW - visibleLen(sep)
		if pad < 0 {
			pad = 0
		}
		rows = append(rows, side+sep+strings.Repeat(" ", pad)+side)
	}

	// Content
	if len(m.files) == 0 {
		emptyMsg := dim.Render("  No memory files found for this project")
		pad := innerW - visibleLen(emptyMsg)
		if pad < 0 {
			pad = 0
		}
		rows = append(rows, side+emptyMsg+strings.Repeat(" ", pad)+side)
		for i := 0; i < contentH-1; i++ {
			rows = append(rows, side+strings.Repeat(" ", innerW)+side)
		}
	} else {
		for i := 0; i < contentH; i++ {
			lineIdx := m.scroll + i
			content := ""
			if lineIdx < len(m.lines) {
				raw := m.lines[lineIdx]
				// Style markdown headers
				if strings.HasPrefix(raw, "## ") {
					content = "  " + lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render(raw[3:])
				} else if strings.HasPrefix(raw, "# ") {
					content = "  " + lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(raw[2:])
				} else if strings.HasPrefix(raw, "- ") {
					content = "  " + dim.Render("·") + " " + lipgloss.NewStyle().Foreground(ColorWhite).Render(raw[2:])
				} else if raw == "" {
					content = ""
				} else {
					content = "  " + lipgloss.NewStyle().Foreground(ColorWhite).Render(raw)
				}
			}
			pad := innerW - visibleLen(content)
			if pad < 0 {
				pad = 0
			}
			rows = append(rows, side+content+strings.Repeat(" ", pad)+side)
		}
	}

	// Footer with hints
	var hints []string
	if len(m.files) > 1 {
		hints = append(hints, "←/→ switch file")
	}
	hints = append(hints, "↑/↓ scroll", "Esc close")
	footer := dim.Render("  " + strings.Join(hints, "  "))
	pad := innerW - visibleLen(footer)
	if pad < 0 {
		pad = 0
	}
	rows = append(rows, side+footer+strings.Repeat(" ", pad)+side)

	// Scroll position
	if len(m.lines) > contentH {
		pct := (m.scroll * 100) / max(len(m.lines)-contentH, 1)
		pos := dim.Render(fmt.Sprintf("  %d%%", pct))
		posPad := innerW - visibleLen(pos)
		if posPad < 0 {
			posPad = 0
		}
		rows = append(rows, side+strings.Repeat(" ", posPad)+pos+side)
	}

	// Bottom border
	bottomBorder := bc.Render("┗" + strings.Repeat("━", innerW) + "┛")
	rows = append(rows, bottomBorder)

	modal := strings.Join(rows, "\n")

	// Center horizontally
	leftPad := (m.width - modalW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	padStr := strings.Repeat(" ", leftPad)

	var centered []string
	for _, row := range strings.Split(modal, "\n") {
		centered = append(centered, padStr+row)
	}

	// Center vertically
	modalHeight := len(centered)
	topPad := (m.height - modalHeight) / 2
	if topPad < 0 {
		topPad = 0
	}

	var final []string
	for i := 0; i < topPad; i++ {
		final = append(final, "")
	}
	final = append(final, centered...)

	return strings.Join(final, "\n")
}
