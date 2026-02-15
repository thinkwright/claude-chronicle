package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/thinkwright/claude-chronicle/internal/claude"
	"github.com/thinkwright/claude-chronicle/internal/store"
)

type DetailPane struct {
	messages     []claude.Message
	scroll       int
	width        int
	height       int
	session      *claude.SessionEntry
	lines        []string // pre-rendered lines
	tailing      bool     // auto-scroll to bottom on new content
	prevMsgCount int      // track message count for change detection
	searchQuery  string   // current in-pane search highlight
	matchLines   []int    // line indices containing matches
	matchIdx     int      // current position in matchLines
	filters      []store.Filter // active conversation filters
}

func NewDetailPane() DetailPane {
	return DetailPane{tailing: true}
}

func (d *DetailPane) SetSession(session *claude.SessionEntry, messages []claude.Message) {
	d.session = session
	d.messages = messages
	d.prevMsgCount = len(messages)
	d.tailing = true
	d.renderLines()
	d.scrollToBottom()
}

func (d *DetailPane) SetSize(w, h int) {
	d.width = w
	d.height = h
	d.renderLines()
}

// SetFilters applies conversation-level filters. Messages that don't match
// are hidden from the rendered view. Pass nil to clear.
func (d *DetailPane) SetFilters(filters []store.Filter) {
	d.filters = filters
	d.renderLines()
	if d.tailing {
		d.scrollToBottom()
	}
}

// HasFilters returns true if conversation filters are active.
func (d *DetailPane) HasFilters() bool {
	return len(d.filters) > 0
}

// messageMatchesFilters returns true if a message passes all active filters.
func (d *DetailPane) messageMatchesFilters(msg claude.Message) bool {
	for _, f := range d.filters {
		switch f.Field {
		case store.FilterType:
			if string(msg.Type) != f.Value {
				return false
			}
		case store.FilterModel:
			model := claude.FormatModel(msg.Model)
			if !strings.Contains(model, strings.ToLower(f.Value)) {
				return false
			}
		case store.FilterTool:
			found := false
			for _, tc := range msg.ToolCalls {
				if strings.EqualFold(tc, f.Value) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		case store.FilterTokens:
			// Parse threshold
			var threshold int
			fmt.Sscanf(f.Value, "%d", &threshold)
			total := msg.InputTokens + msg.OutputTokens
			switch f.Op {
			case store.OpGreaterThan:
				if total <= threshold {
					return false
				}
			case store.OpLessThan:
				if total >= threshold {
					return false
				}
			default:
				if total != threshold {
					return false
				}
			}
		}
		// FilterBranch, FilterProject, FilterAge don't apply at message level — skip
	}
	return true
}

// Refresh re-reads the JSONL file for the current session.
// Returns true if new content was found.
func (d *DetailPane) Refresh() bool {
	if d.session == nil {
		return false
	}

	messages, err := claude.LoadMessages(d.session.FullPath)
	if err != nil || len(messages) == d.prevMsgCount {
		return false
	}

	d.messages = messages
	d.prevMsgCount = len(messages)
	d.renderLines()

	if d.tailing {
		d.scrollToBottom()
	}
	return true
}

func (d *DetailPane) scrollToBottom() {
	maxScroll := len(d.lines) - d.height - 1
	if maxScroll < 0 {
		maxScroll = 0
	}
	d.scroll = maxScroll
	d.tailing = true
}

func (d *DetailPane) IsAtBottom() bool {
	maxScroll := len(d.lines) - d.height - 1
	if maxScroll < 0 {
		return true
	}
	return d.scroll >= maxScroll
}

func (d *DetailPane) ScrollToTop() {
	d.scroll = 0
	d.tailing = false
}

func (d *DetailPane) ScrollUp(n int) {
	d.scroll -= n
	if d.scroll < 0 {
		d.scroll = 0
	}
	// User scrolled up — pause auto-tail
	d.tailing = false
}

func (d *DetailPane) ScrollDown(n int) {
	d.scroll += n
	maxScroll := len(d.lines) - d.height - 1
	if maxScroll < 0 {
		maxScroll = 0
	}
	if d.scroll > maxScroll {
		d.scroll = maxScroll
	}
	// If user scrolled back to bottom, re-enable tailing
	if d.scroll >= maxScroll {
		d.tailing = true
	}
}

func (d *DetailPane) IsTailing() bool {
	return d.tailing
}

// SetSearch sets the search query and finds all matching lines.
func (d *DetailPane) SetSearch(query string) {
	d.searchQuery = query
	d.matchLines = nil
	d.matchIdx = 0

	if query == "" {
		return
	}

	lower := strings.ToLower(query)
	for i, line := range d.lines {
		if strings.Contains(strings.ToLower(stripAnsi(line)), lower) {
			d.matchLines = append(d.matchLines, i)
		}
	}

	// Jump to first match
	if len(d.matchLines) > 0 {
		d.scrollToLine(d.matchLines[0])
	}
}

// ClearSearch removes the search highlight.
func (d *DetailPane) ClearSearch() {
	d.searchQuery = ""
	d.matchLines = nil
	d.matchIdx = 0
}

// NextMatch scrolls to the next search match.
func (d *DetailPane) NextMatch() {
	if len(d.matchLines) == 0 {
		return
	}
	d.matchIdx++
	if d.matchIdx >= len(d.matchLines) {
		d.matchIdx = 0 // wrap around
	}
	d.scrollToLine(d.matchLines[d.matchIdx])
}

// PrevMatch scrolls to the previous search match.
func (d *DetailPane) PrevMatch() {
	if len(d.matchLines) == 0 {
		return
	}
	d.matchIdx--
	if d.matchIdx < 0 {
		d.matchIdx = len(d.matchLines) - 1 // wrap around
	}
	d.scrollToLine(d.matchLines[d.matchIdx])
}

// MatchInfo returns current match index and total matches.
func (d *DetailPane) MatchInfo() (current, total int) {
	if len(d.matchLines) == 0 {
		return 0, 0
	}
	return d.matchIdx + 1, len(d.matchLines)
}

func (d *DetailPane) scrollToLine(line int) {
	d.scroll = line - d.height/3
	if d.scroll < 0 {
		d.scroll = 0
	}
	maxScroll := len(d.lines) - d.height + 4
	if maxScroll < 0 {
		maxScroll = 0
	}
	if d.scroll > maxScroll {
		d.scroll = maxScroll
	}
	d.tailing = false
}

func (d *DetailPane) renderLines() {
	d.lines = nil
	if d.session == nil || len(d.messages) == 0 {
		return
	}

	contentWidth := d.width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}

	sepWidth := min(contentWidth, 40)

	makeSep := func(style lipgloss.Style, ts string) string {
		tsStr := formatMsgTime(ts)
		if tsStr == "" {
			return style.Render("  ┃") + DimStyle.Render(strings.Repeat("╌", sepWidth))
		}
		dashLen := sepWidth - len(tsStr) - 1 // 1 for space before timestamp
		if dashLen < 4 {
			dashLen = 4
		}
		return style.Render("  ┃") + DimStyle.Render(strings.Repeat("╌", dashLen)+" "+tsStr)
	}

	for i, msg := range d.messages {
		// Skip messages that don't match active filters
		if len(d.filters) > 0 && !d.messageMatchesFilters(msg) {
			continue
		}

		switch msg.Type {
		case claude.TypeUser:
			if i > 0 {
				d.lines = append(d.lines, makeSep(UserMsgStyle, msg.Timestamp))
			}
			tag := UserMsgStyle.Render("  ┃ ▶ USER")
			d.lines = append(d.lines, tag)
			for _, line := range wrapText(msg.Text, contentWidth) {
				d.lines = append(d.lines, UserMsgStyle.Render("  ┃ ")+NormalStyle.Render(line))
			}
			d.lines = append(d.lines, "")

		case claude.TypeAssistant:
			if i > 0 {
				d.lines = append(d.lines, makeSep(AssistantMsgStyle, msg.Timestamp))
			}
			model := claude.FormatModel(msg.Model)
			// Model-colored dot
			modelDot := lipgloss.NewStyle().Foreground(ColorGreen).Render("●")
			if strings.Contains(model, "opus") {
				modelDot = lipgloss.NewStyle().Foreground(ColorAccent).Render("●")
			} else if strings.Contains(model, "haiku") {
				modelDot = lipgloss.NewStyle().Foreground(ColorDim).Render("●")
			}

			tag := AssistantMsgStyle.Render("  ┃ ") + modelDot + AssistantMsgStyle.Render(fmt.Sprintf(" CLAUDE [%s]", model))
			d.lines = append(d.lines, tag)

			if len(msg.ToolCalls) > 0 {
				tools := ToolMsgStyle.Render("  ┃   ⚙ " + strings.Join(msg.ToolCalls, " · "))
				d.lines = append(d.lines, tools)
			}

			text := msg.Text
			if len(text) > 500 {
				text = text[:500] + "..."
			}
			for _, line := range wrapText(text, contentWidth) {
				d.lines = append(d.lines, AssistantMsgStyle.Render("  ┃ ")+NormalStyle.Render(line))
			}

			if msg.OutputTokens > 0 {
				tokens := DimStyle.Render(fmt.Sprintf("  ┃   ⊘ %s in / %s out",
					claude.FormatTokens(msg.InputTokens), claude.FormatTokens(msg.OutputTokens)))
				d.lines = append(d.lines, tokens)
			}
			d.lines = append(d.lines, "")

		case claude.TypeToolResult:
			if msg.Text == "" {
				continue
			}
			if i > 0 {
				d.lines = append(d.lines, makeSep(ToolMsgStyle, msg.Timestamp))
			}
			tag := ToolMsgStyle.Render("  ┃ ⚙ TOOL RESULT")
			d.lines = append(d.lines, tag)
			text := msg.Text
			if len(text) > 200 {
				text = text[:200] + "..."
			}
			for _, line := range wrapText(text, contentWidth) {
				d.lines = append(d.lines, ToolMsgStyle.Render("  ┃ ")+DimStyle.Render(line))
			}
			d.lines = append(d.lines, "")

		case claude.TypeSystem:
			if msg.Text == "" {
				continue
			}
			if i > 0 {
				d.lines = append(d.lines, makeSep(SystemMsgStyle, msg.Timestamp))
			}
			tag := SystemMsgStyle.Render("  ┃ ◌ SYSTEM")
			d.lines = append(d.lines, tag)
			d.lines = append(d.lines, SystemMsgStyle.Render("  ┃ "+msg.Text))
			d.lines = append(d.lines, "")
		}
	}
}

// formatMsgTime parses an ISO timestamp and returns a compact time string.
func formatMsgTime(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.000Z", ts)
		if err != nil {
			return ""
		}
	}
	return t.Local().Format("Jan 2, 2006 3:04 PM")
}

// Title returns the pane title string for the border header.
func (d *DetailPane) Title() string {
	title := "CONVERSATION LOG"
	if d.session != nil {
		if d.tailing {
			title += "  ● LIVE"
		} else {
			pos := d.scroll + 1
			total := len(d.lines)
			if total == 0 {
				total = 1
			}
			pct := (d.scroll * 100) / max(total-d.height+4, 1)
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			title += fmt.Sprintf("  ↑ L%d/%d (%d%%)  [g] live", pos, total, pct)
		}
	}
	if len(d.filters) > 0 {
		title += "  ⚡ FILTERED"
	}
	if d.searchQuery != "" {
		cur, total := d.MatchInfo()
		if total > 0 {
			title += fmt.Sprintf("  MATCH %d/%d", cur, total)
		} else {
			title += "  NO MATCHES"
		}
	}
	return title
}

func (d *DetailPane) View() string {
	var lines []string

	if d.session == nil {
		return "\n" + DimStyle.Render("  Select a session to view")
	}

	available := d.height
	if available < 1 {
		available = 1
	}

	end := d.scroll + available
	if end > len(d.lines) {
		end = len(d.lines)
	}

	// Render scrollbar alongside content
	scrollbar := RenderScrollbar(available, len(d.lines), d.scroll)
	innerW := d.width - 3 // content width inside panel border (w-2), minus 1 for scrollbar gutter

	for idx := 0; idx < available; idx++ {
		contentIdx := d.scroll + idx
		content := ""
		if contentIdx < len(d.lines) {
			content = d.lines[contentIdx]
			if d.searchQuery != "" {
				content = highlightMatches(content, d.searchQuery)
			}
		}
		sb := " "
		if idx < len(scrollbar) {
			sb = scrollbar[idx]
		}
		// Right-align scrollbar char at the gutter edge
		pad := innerW - visibleLen(content)
		if pad < 0 {
			pad = 0
		}
		lines = append(lines, content+strings.Repeat(" ", pad)+sb)
	}

	return strings.Join(lines, "\n")
}

// HeaderStats returns session metrics styled for the header bar.
// Returns empty string if no session is loaded.
func (d *DetailPane) HeaderStats() string {
	if d.session == nil {
		return ""
	}

	var totalIn, totalOut, toolCount int
	var model string
	for _, m := range d.messages {
		totalIn += m.InputTokens
		totalOut += m.OutputTokens
		toolCount += len(m.ToolCalls)
		if m.Model != "" {
			model = claude.FormatModel(m.Model)
		}
	}

	bg := lipgloss.NewStyle().Background(ColorBarBg)
	sep := bg.Foreground(ColorDim).Render(" │ ")

	parts := []string{
		bg.Foreground(ColorCyan).Render(
			fmt.Sprintf("MSGS %d", len(d.messages))),
		bg.Foreground(ColorGreen).Render(
			fmt.Sprintf("TOK %s", claude.FormatTokens(totalIn+totalOut))),
		bg.Foreground(ColorYellow).Render(
			fmt.Sprintf("TOOLS %d", toolCount)),
	}
	if model != "" {
		modelColor := ColorGreen
		if strings.Contains(model, "opus") {
			modelColor = ColorAccent
		} else if strings.Contains(model, "haiku") {
			modelColor = ColorDim
		}
		parts = append(parts, bg.Foreground(modelColor).Bold(true).Render(model))
	}
	if d.session.GitBranch != "" {
		parts = append(parts, bg.Foreground(ColorWhite).Render(d.session.GitBranch))
	}
	return strings.Join(parts, sep)
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}

		for len(paragraph) > width {
			cut := width
			for i := width; i > width/2; i-- {
				if paragraph[i] == ' ' {
					cut = i
					break
				}
			}
			lines = append(lines, paragraph[:cut])
			paragraph = strings.TrimLeft(paragraph[cut:], " ")
		}
		if paragraph != "" {
			lines = append(lines, paragraph)
		}
	}

	return lines
}

// highlightMatches applies search highlighting to a styled line.
// It works on the plain-text segments between ANSI escape sequences,
// replacing case-insensitive matches with the highlighted version.
func highlightMatches(line, query string) string {
	if query == "" {
		return line
	}
	lowerQuery := strings.ToLower(query)
	qLen := len(lowerQuery)

	var out strings.Builder
	i := 0
	for i < len(line) {
		// Pass through ANSI escape sequences untouched
		if line[i] == '\x1b' && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) && line[j] != 'm' {
				j++
			}
			if j < len(line) {
				j++ // include the 'm'
			}
			out.WriteString(line[i:j])
			i = j
			continue
		}

		// Check for case-insensitive match at this position
		if i+qLen <= len(line) && strings.ToLower(line[i:i+qLen]) == lowerQuery {
			out.WriteString(SearchHighlightStyle.Render(line[i : i+qLen]))
			i += qLen
			continue
		}

		out.WriteByte(line[i])
		i++
	}
	return out.String()
}
