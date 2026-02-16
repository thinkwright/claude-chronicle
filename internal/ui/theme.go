package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// Nostromo MU/TH/UR 6000 color palette
var (
	ColorCyan      = lipgloss.Color("#5a9ab5")
	ColorCyanDim   = lipgloss.Color("#3a6678")
	ColorAccent    = lipgloss.Color("#7fcfdf") // bright focus accent — distinct from base cyan
	ColorGreen     = lipgloss.Color("#5aaa7a")
	ColorGreenDim  = lipgloss.Color("#3a6648")
	ColorRed       = lipgloss.Color("#b56a6a")
	ColorYellow    = lipgloss.Color("#b5a05a")
	ColorYellowDim = lipgloss.Color("#5a5030")
	ColorDim       = lipgloss.Color("#3a5565")
	ColorMuted     = lipgloss.Color("#1a2a35")
	ColorBg        = lipgloss.Color("#000000")
	ColorBarBg     = lipgloss.Color("#0f1e28") // status/header bar background
	ColorBarText   = lipgloss.Color("#d0dde5") // white text for status bars
	ColorRowAlt    = lipgloss.Color("#0a1418") // alternating row tint
	ColorWhite     = lipgloss.Color("#8899a5")
	ColorSelect   = lipgloss.Color("#c8d84a") // vivid yellow-green for selected items
	ColorSelectBg = lipgloss.Color("#1a2a1a") // subtle dark green row background for selection

	// Styles
	HeaderStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)

	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true).
			Padding(0, 1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(ColorSelect).
			Bold(true)

	NormalStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)

	DimStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	UserMsgStyle = lipgloss.NewStyle().
			Foreground(ColorCyan)

	AssistantMsgStyle = lipgloss.NewStyle().
				Foreground(ColorGreen)

	ToolMsgStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	SystemMsgStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	BadgeStyle = lipgloss.NewStyle().
			Foreground(ColorYellowDim)

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorDim).
			Background(ColorBarBg).
			Padding(0, 1)

	SearchHighlightStyle = lipgloss.NewStyle().
				Foreground(ColorBg).
				Background(ColorYellow).
				Bold(true)
)

// ─── Custom Border Rendering ──────────────────────────────────────────
// Renders panels with inline title in the top border:
//   ┏━━╸ PROJECTS ╺━━━━━━━━━━━━━┓
//   ┃                             ┃
//   ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
// Heavy top/bottom (━), thin sides (┃), custom corners.

// RenderPanel draws a panel with an inline title in the top border.
func RenderPanel(title string, content string, w, h int, focused bool) string {
	borderColor := ColorCyanDim
	titleColor := ColorCyan
	if focused {
		borderColor = lipgloss.Color("#70cc90") // bright green border
		titleColor = lipgloss.Color("#a0ffbb")  // near-white green title
	}

	bc := lipgloss.NewStyle().Foreground(borderColor)
	tc := lipgloss.NewStyle().Foreground(titleColor).Bold(true)

	innerW := w - 2 // subtract left+right border chars

	var topBorder, bottomBorder, side string

	titleText := " " + title + " "
	titleVisLen := utf8.RuneCountInString(titleText)

	if focused {
		// Double-line border for focused pane: ╔═╗ ║ ╚═╝
		fillLen := w - 5 - titleVisLen
		if fillLen < 0 {
			fillLen = 0
		}
		topBorder = bc.Render("╔═╸") + tc.Render(titleText) + bc.Render("╺"+strings.Repeat("═", fillLen)+"╗")
		bottomBorder = bc.Render("╚" + strings.Repeat("═", innerW) + "╝")
		side = bc.Render("║")
	} else {
		// Single-line border for unfocused: ┏━┓ ┃ ┗━┛
		fillLen := w - 5 - titleVisLen
		if fillLen < 0 {
			fillLen = 0
		}
		topBorder = bc.Render("┏━╸") + tc.Render(titleText) + bc.Render("╺"+strings.Repeat("━", fillLen)+"┓")
		bottomBorder = bc.Render("┗" + strings.Repeat("━", innerW) + "┛")
		side = bc.Render("┃")
	}

	// ── Content lines ──
	lines := strings.Split(content, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}

	var rows []string
	rows = append(rows, topBorder)
	for _, line := range lines {
		visible := visibleLen(line)
		if visible > innerW {
			line = truncateToWidth(line, innerW)
			visible = innerW
		}
		pad := ""
		if visible < innerW {
			pad = strings.Repeat(" ", innerW-visible)
		}
		rows = append(rows, side+line+pad+side)
	}
	rows = append(rows, bottomBorder)

	return strings.Join(rows, "\n")
}

// ─── Scrollbar ────────────────────────────────────────────────────────

// RenderScrollbar returns a vertical slice of scrollbar characters for the given
// viewport. height is the visible rows, totalLines is the total content lines,
// and offset is the current scroll position. Returns a []string of length height,
// each entry being a single scrollbar character.
func RenderScrollbar(height, totalLines, offset int) []string {
	track := make([]string, height)

	if totalLines <= height || height < 1 {
		// No scrollbar needed — all content visible
		for i := range track {
			track[i] = " "
		}
		return track
	}

	// Thumb size: proportional to viewport/content ratio, min 1 row
	thumbSize := (height * height) / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}

	// Thumb position
	maxOffset := totalLines - height
	if maxOffset < 1 {
		maxOffset = 1
	}
	thumbPos := (offset * (height - thumbSize)) / maxOffset

	thumbChar := lipgloss.NewStyle().Foreground(ColorAccent).Render("┃")
	trackChar := lipgloss.NewStyle().Foreground(ColorMuted).Render("╎")

	for i := range track {
		if i >= thumbPos && i < thumbPos+thumbSize {
			track[i] = thumbChar
		} else {
			track[i] = trackChar
		}
	}

	return track
}

// ─── Sparkline ────────────────────────────────────────────────────────

// Sparkline renders a sparkline from values using braille bar characters.
func Sparkline(values []float64, width int) string {
	bars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	if len(values) == 0 {
		return ""
	}

	// Find max for normalization
	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	// Sample or pad to width
	sampled := make([]float64, width)
	for i := 0; i < width; i++ {
		srcIdx := i * len(values) / width
		if srcIdx >= len(values) {
			srcIdx = len(values) - 1
		}
		sampled[i] = values[srcIdx]
	}

	var b strings.Builder
	for _, v := range sampled {
		idx := int(v / maxVal * float64(len(bars)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(bars) {
			idx = len(bars) - 1
		}
		b.WriteRune(bars[idx])
	}
	return b.String()
}

