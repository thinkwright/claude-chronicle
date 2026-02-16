package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/thinkwright/claude-chronicle/internal/claude"
)

// HooksModal displays Claude Code hooks configuration in a centered overlay.
type HooksModal struct {
	visible     bool
	sources     []claude.HooksSource
	sourceIdx   int // which source tab is selected
	scroll      int
	lines       []string // rendered content lines
	width       int
	height      int
	projectName string
}

func NewHooksModal() HooksModal {
	return HooksModal{}
}

func (h *HooksModal) IsVisible() bool {
	return h.visible
}

func (h *HooksModal) Show(projectName string, sources []claude.HooksSource) {
	h.visible = true
	h.sources = sources
	h.sourceIdx = 0
	h.scroll = 0
	h.projectName = projectName
	h.renderLines()
}

func (h *HooksModal) Close() {
	h.visible = false
}

func (h *HooksModal) SetSize(w, ht int) {
	h.width = w
	h.height = ht
	if h.visible {
		h.renderLines()
	}
}

func (h *HooksModal) ScrollUp(n int) {
	h.scroll -= n
	if h.scroll < 0 {
		h.scroll = 0
	}
}

func (h *HooksModal) ScrollDown(n int) {
	h.scroll += n
	mx := len(h.lines) - h.contentHeight()
	if mx < 0 {
		mx = 0
	}
	if h.scroll > mx {
		h.scroll = mx
	}
}

func (h *HooksModal) NextSource() {
	if len(h.sources) <= 1 {
		return
	}
	h.sourceIdx = (h.sourceIdx + 1) % len(h.sources)
	h.scroll = 0
	h.renderLines()
}

func (h *HooksModal) PrevSource() {
	if len(h.sources) <= 1 {
		return
	}
	h.sourceIdx--
	if h.sourceIdx < 0 {
		h.sourceIdx = len(h.sources) - 1
	}
	h.scroll = 0
	h.renderLines()
}

func (h *HooksModal) contentHeight() int {
	ht := h.height * 70 / 100
	if ht < 5 {
		ht = 5
	}
	return ht
}

func (h *HooksModal) modalWidth() int {
	w := h.width * 60 / 100
	if w > 90 {
		w = 90
	}
	if w < 50 {
		w = 50
	}
	return w
}

// renderLines builds the text content for the currently selected source.
func (h *HooksModal) renderLines() {
	h.lines = nil

	if len(h.sources) == 0 {
		h.lines = append(h.lines, "No hooks configured.")
		h.lines = append(h.lines, "")
		h.lines = append(h.lines, "Hooks are defined in:")
		h.lines = append(h.lines, "  ~/.claude/settings.json           (global)")
		h.lines = append(h.lines, "  <project>/.claude/settings.json   (project)")
		h.lines = append(h.lines, "  <project>/.claude/settings.local.json")
		return
	}

	src := h.sources[h.sourceIdx]
	contentW := h.modalWidth() - 8
	if contentW < 30 {
		contentW = 30
	}

	h.lines = append(h.lines, fmt.Sprintf("Source: %s", src.Path))
	h.lines = append(h.lines, "")

	for _, ev := range src.Events {
		h.lines = append(h.lines, fmt.Sprintf("## %s", ev.Event))
		h.lines = append(h.lines, "")

		for _, g := range ev.Groups {
			if g.Matcher != "" {
				h.lines = append(h.lines, fmt.Sprintf("  matcher: %s", g.Matcher))
			}

			for _, hook := range g.Hooks {
				typeStr := hook.Type
				if typeStr == "" {
					typeStr = "command"
				}

				h.lines = append(h.lines, fmt.Sprintf("  - type: %s", typeStr))

				// Show command or prompt, word-wrapped
				content := hook.Command
				if content == "" {
					content = hook.Prompt
				}
				if content != "" {
					label := "cmd"
					if hook.Command == "" {
						label = "prompt"
					}
					// Wrap long commands
					prefix := fmt.Sprintf("    %s: ", label)
					wrapped := WrapText(content, contentW-len(prefix))
					for i, wl := range wrapped {
						if i == 0 {
							h.lines = append(h.lines, prefix+wl)
						} else {
							h.lines = append(h.lines, strings.Repeat(" ", len(prefix))+wl)
						}
					}
				}

				if hook.Timeout > 0 {
					h.lines = append(h.lines, fmt.Sprintf("    timeout: %ds", hook.Timeout))
				}
			}
			h.lines = append(h.lines, "")
		}
	}
}

// View renders the centered modal overlay.
func (h *HooksModal) View() string {
	if !h.visible {
		return ""
	}

	modalW := h.modalWidth()
	contentH := h.contentHeight()
	innerW := modalW - 4

	bc := lipgloss.NewStyle().Foreground(ColorYellow)
	tc := lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)
	dim := lipgloss.NewStyle().Foreground(ColorDim)

	var rows []string

	// Top border with title
	title := fmt.Sprintf(" HOOKS — %s ", strings.ToUpper(h.projectName))
	titleVisLen := utf8.RuneCountInString(title)
	fillLen := innerW - 3 - titleVisLen
	if fillLen < 0 {
		fillLen = 0
	}
	topBorder := bc.Render("┏━╸") + tc.Render(title) + bc.Render("╺"+strings.Repeat("━", fillLen)+"┓")
	rows = append(rows, topBorder)

	side := bc.Render("┃")

	// Source tabs
	if len(h.sources) > 1 {
		var tabs []string
		for i, src := range h.sources {
			name := src.Label
			count := 0
			for _, ev := range src.Events {
				for _, g := range ev.Groups {
					count += len(g.Hooks)
				}
			}
			label := fmt.Sprintf("%s (%d)", name, count)
			if i == h.sourceIdx {
				tabs = append(tabs, lipgloss.NewStyle().
					Foreground(ColorSelect).Bold(true).Render(" "+label+" "))
			} else {
				tabs = append(tabs, dim.Render(" "+label+" "))
			}
		}
		tabLine := "  " + strings.Join(tabs, dim.Render("│"))
		pad := innerW - visibleLen(tabLine)
		if pad < 0 {
			pad = 0
		}
		rows = append(rows, side+tabLine+strings.Repeat(" ", pad)+side)

		sep := dim.Render("  " + strings.Repeat("─", innerW-2))
		pad = innerW - visibleLen(sep)
		if pad < 0 {
			pad = 0
		}
		rows = append(rows, side+sep+strings.Repeat(" ", pad)+side)
	}

	// Content
	if len(h.sources) == 0 {
		for i := 0; i < contentH; i++ {
			content := ""
			if i < len(h.lines) {
				content = "  " + dim.Render(h.lines[i])
			}
			pad := innerW - visibleLen(content)
			if pad < 0 {
				pad = 0
			}
			rows = append(rows, side+content+strings.Repeat(" ", pad)+side)
		}
	} else {
		for i := 0; i < contentH; i++ {
			lineIdx := h.scroll + i
			content := ""
			if lineIdx < len(h.lines) {
				raw := h.lines[lineIdx]
				if strings.HasPrefix(raw, "## ") {
					content = "  " + lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render(raw[3:])
				} else if strings.HasPrefix(raw, "Source: ") {
					content = "  " + dim.Render(raw)
				} else if strings.HasPrefix(raw, "  matcher:") {
					content = "  " + lipgloss.NewStyle().Foreground(ColorYellow).Render(raw)
				} else if strings.HasPrefix(raw, "  - type:") {
					content = "  " + lipgloss.NewStyle().Foreground(ColorGreen).Render(raw)
				} else if strings.HasPrefix(raw, "    cmd:") || strings.HasPrefix(raw, "    prompt:") {
					content = "  " + lipgloss.NewStyle().Foreground(ColorWhite).Render(raw)
				} else if strings.HasPrefix(raw, "    timeout:") {
					content = "  " + dim.Render(raw)
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

	// Footer
	var hints []string
	if len(h.sources) > 1 {
		hints = append(hints, "←/→ switch source")
	}
	hints = append(hints, "↑/↓ scroll", "Esc close")
	footer := dim.Render("  " + strings.Join(hints, "  "))
	pad := innerW - visibleLen(footer)
	if pad < 0 {
		pad = 0
	}
	rows = append(rows, side+footer+strings.Repeat(" ", pad)+side)

	// Scroll position
	if len(h.lines) > contentH {
		pct := (h.scroll * 100) / max(len(h.lines)-contentH, 1)
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
	leftPad := (h.width - modalW) / 2
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
	topPad := (h.height - modalHeight) / 2
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
