package ui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/thinkwright/claude-chronicle/internal/claude"
	"github.com/thinkwright/claude-chronicle/internal/config"
	"github.com/thinkwright/claude-chronicle/internal/store"
	"github.com/thinkwright/claude-chronicle/internal/watcher"
)

type pane int

const (
	paneProjects pane = iota
	paneSessions
	paneDetail
	paneWatchlist
)

type tickMsg time.Time
type tailTickMsg time.Time

// Indexing messages
type indexDoneMsg struct {
	files    int
	messages int
	err      error
}

type indexProgressMsg struct {
	phase   string
	current int
	total   int
}

func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func tailTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tailTickMsg(t)
	})
}


type Model struct {
	projects    ProjectList
	sessions    SessionList
	detail      DetailPane
	search      SearchOverlay
	filterBar   FilterBar
	watchlist   WatchlistPane
	memory      MemoryModal
	hooks       HooksModal
	store       *store.Store
	focus       pane
	width       int
	height      int
	ready       bool
	frame       int
	allSessions []claude.SessionEntry
	cfg              config.Config
	showSettings     bool
	settingsCursor     int // unified cursor: 0=reindex, 1=rebuild, 2..n+1=paths, n+2=add
	settingsPathInput  textinput.Model
	settingsAddingPath bool
	settingsPathError  string
	settingsConfirmDel bool
	confirmQuit  bool
	indexing        bool   // true while background index is running
	indexStatus     string // status text for status bar
	activeWatchName string // non-empty when viewing watchlist matches
}

func NewModel(db *store.Store) Model {
	cfg := config.Load()

	pathInput := textinput.New()
	pathInput.Placeholder = "/path/to/directory"
	pathInput.CharLimit = 512
	pathInput.Prompt = "path: "
	pathInput.PromptStyle = lipgloss.NewStyle().Foreground(ColorCyan)
	pathInput.TextStyle = lipgloss.NewStyle().Foreground(ColorWhite)
	pathInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorDim)

	m := Model{
		projects:          NewProjectList(),
		sessions:          NewSessionList(),
		detail:            NewDetailPane(),
		search:            NewSearchOverlay(),
		filterBar:         NewFilterBar(),
		watchlist:         NewWatchlistPane(),
		memory:            NewMemoryModal(),
		hooks:             NewHooksModal(),
		store:             db,
		focus:             paneProjects,
		cfg:               cfg,
		settingsPathInput: pathInput,
		indexing:          true,
	}
	if cfg.WatchlistVisible {
		m.watchlist.Show()
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), tailTickCmd(), watcher.Watch(m.cfg.ProjectPaths), m.indexAllCmd())
}

func (m Model) indexAllCmd() tea.Cmd {
	return func() tea.Msg {
		progress := make(chan store.IndexProgress, 16)
		go func() {
			// Drain progress channel (we use the done message for final status)
			for range progress {
			}
		}()
		err := m.store.IndexAll(progress, m.cfg.ProjectPaths)
		files := m.store.FileCount()
		msgs := m.store.MessageCount()
		return indexDoneMsg{files: files, messages: msgs, err: err}
	}
}

func (m Model) rebuildIndexCmd() tea.Cmd {
	return func() tea.Msg {
		m.store.Reset()
		progress := make(chan store.IndexProgress, 16)
		go func() {
			for range progress {
			}
		}()
		err := m.store.IndexAll(progress, m.cfg.ProjectPaths)
		files := m.store.FileCount()
		msgs := m.store.MessageCount()
		return indexDoneMsg{files: files, messages: msgs, err: err}
	}
}

func (m Model) indexChangedCmd() tea.Cmd {
	return func() tea.Msg {
		changed, err := m.store.IndexChanged(m.cfg.ProjectPaths)
		if changed == 0 && err == nil {
			return nil
		}
		files := m.store.FileCount()
		msgs := m.store.MessageCount()
		return indexDoneMsg{files: files, messages: msgs, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		firstReady := !m.ready
		m.ready = true
		m.layoutPanes()
		m.memory.SetSize(m.width, m.height)
		m.hooks.SetSize(m.width, m.height)
		if firstReady {
			m.loadProjects()
		}
		return m, nil

	case tickMsg:
		m.frame++
		return m, tickCmd()

	case tailTickMsg:
		m.detail.Refresh()
		return m, tailTickCmd()

	case indexDoneMsg:
		m.indexing = false
		if msg.err != nil {
			m.indexStatus = fmt.Sprintf("INDEX ERR: %v", msg.err)
		} else {
			m.indexStatus = fmt.Sprintf("INDEXED %d files  %s msgs",
				msg.files, claude.FormatTokens(msg.messages))
		}
		return m, nil

	case watcher.RefreshMsg:
		m.loadProjects()
		return m, tea.Batch(watcher.Watch(m.cfg.ProjectPaths), m.indexChangedCmd())

	case tea.KeyMsg:
		if m.confirmQuit {
			return m.handleConfirmQuit(msg)
		}
		if m.memory.IsVisible() {
			return m.handleMemoryKey(msg)
		}
		if m.hooks.IsVisible() {
			return m.handleHooksKey(msg)
		}
		if m.showSettings {
			return m.handleSettingsKey(msg)
		}
		if m.search.IsActive() {
			return m.handleSearchKey(msg)
		}
		if m.filterBar.IsEditing() {
			return m.handleFilterKey(msg)
		}
		if m.watchlist.IsEditing() {
			return m.handleWatchEditKey(msg)
		}
		if m.watchlist.IsConfirmingDelete() {
			return m.handleWatchDeleteConfirm(msg)
		}
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleMemoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "M", "m":
		m.memory.Close()
	case "up", "k":
		m.memory.ScrollUp(3)
	case "down", "j":
		m.memory.ScrollDown(3)
	case "pgup":
		m.memory.ScrollUp(m.height / 2)
	case "pgdown":
		m.memory.ScrollDown(m.height / 2)
	case "left", "h":
		m.memory.PrevFile()
	case "right", "l":
		m.memory.NextFile()
	}
	return m, nil
}

func (m Model) handleHooksKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "H":
		m.hooks.Close()
	case "up", "k":
		m.hooks.ScrollUp(3)
	case "down", "j":
		m.hooks.ScrollDown(3)
	case "pgup":
		m.hooks.ScrollUp(m.height / 2)
	case "pgdown":
		m.hooks.ScrollDown(m.height / 2)
	case "left", "h":
		m.hooks.PrevSource()
	case "right", "l":
		m.hooks.NextSource()
	}
	return m, nil
}

// settingsItemCount returns the total number of navigable items in settings.
// Layout: [reindex, rebuild, ...paths, add-path]
func (m Model) settingsItemCount() int {
	return 2 + len(m.cfg.ProjectPaths) + 1
}

// settingsPathIndex returns the path list index for the current cursor, or -1 if not on a path.
func (m Model) settingsPathIndex() int {
	idx := m.settingsCursor - 2
	if idx >= 0 && idx < len(m.cfg.ProjectPaths) {
		return idx
	}
	return -1
}

func (m Model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.settingsAddingPath {
		return m.handleSettingsAddPath(msg)
	}
	if m.settingsConfirmDel {
		return m.handleSettingsDeleteConfirm(msg)
	}

	maxIdx := m.settingsItemCount() - 1

	switch msg.String() {
	case "?", "esc":
		m.showSettings = false
		m.settingsCursor = 0
		m.settingsPathError = ""
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "down", "j":
		if m.settingsCursor < maxIdx {
			m.settingsCursor++
		}
	case "enter":
		switch {
		case m.settingsCursor == 0: // reindex
			m.showSettings = false
			m.indexing = true
			return m, m.indexAllCmd()
		case m.settingsCursor == 1: // rebuild
			m.showSettings = false
			m.indexing = true
			return m, m.rebuildIndexCmd()
		case m.settingsCursor == maxIdx: // add path
			m.settingsAddingPath = true
			m.settingsPathError = ""
			m.settingsPathInput.SetValue("")
			m.settingsPathInput.Focus()
			return m, textinput.Blink
		default: // a path entry — no-op on enter (use d to delete)
		}
	case "r":
		m.showSettings = false
		m.indexing = true
		return m, m.indexAllCmd()
	case "R":
		m.showSettings = false
		m.indexing = true
		return m, m.rebuildIndexCmd()
	case "a":
		m.settingsAddingPath = true
		m.settingsPathError = ""
		m.settingsPathInput.SetValue("")
		m.settingsPathInput.Focus()
		return m, textinput.Blink
	case "d":
		if pi := m.settingsPathIndex(); pi >= 0 {
			m.settingsConfirmDel = true
		}
	}
	return m, nil
}

func (m Model) handleSettingsAddPath(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.settingsAddingPath = false
		m.settingsPathError = ""
		m.settingsPathInput.Blur()
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.settingsPathInput.Value())
		if path == "" {
			m.settingsAddingPath = false
			m.settingsPathInput.Blur()
			return m, nil
		}
		// Expand ~ to home directory
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[2:])
			}
		}
		// Validate: must be an existing directory
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			m.settingsPathError = "not a valid directory"
			return m, nil
		}
		if !m.cfg.AddProjectPath(path) {
			m.settingsPathError = "path already added"
			return m, nil
		}
		// Success
		m.settingsAddingPath = false
		m.settingsPathInput.Blur()
		m.settingsPathError = ""
		config.Save(m.cfg)
		m.loadProjects()
		return m, tea.Batch(watcher.Watch(m.cfg.ProjectPaths), m.indexChangedCmd())
	default:
		m.settingsPathError = ""
		var cmd tea.Cmd
		m.settingsPathInput, cmd = m.settingsPathInput.Update(msg)
		return m, cmd
	}
}

func (m Model) handleSettingsDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if pi := m.settingsPathIndex(); pi >= 0 {
			m.cfg.RemoveProjectPath(m.cfg.ProjectPaths[pi])
			// If we deleted the last path, move cursor up to "add path" (which is now at a lower index)
			maxIdx := m.settingsItemCount() - 1
			if m.settingsCursor > maxIdx {
				m.settingsCursor = maxIdx
			}
			config.Save(m.cfg)
			m.loadProjects()
			m.settingsConfirmDel = false
			return m, tea.Batch(watcher.Watch(m.cfg.ProjectPaths), m.indexChangedCmd())
		}
		m.settingsConfirmDel = false
	default:
		m.settingsConfirmDel = false
	}
	return m, nil
}

func (m Model) handleConfirmQuit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "q", "enter":
		return m, tea.Quit
	default:
		m.confirmQuit = false
	}
	return m, nil
}

func (m Model) saveConfig() {
	m.cfg.WatchlistVisible = m.watchlist.IsVisible()
	config.Save(m.cfg)
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.search.Close()
		m.doFilterSessions()
		return m, nil
	case "tab":
		m.search.CycleScope()
		m.doSearch()
		return m, nil
	case "enter":
		query := m.search.Value()
		if m.search.HasResults() {
			r := m.search.SelectedResult()
			if r != nil {
				m.navigateToResult(r)
			}
		}
		m.search.Close()
		if query != "" {
			m.detail.SetSearch(query)
		}
		return m, nil
	case "up":
		m.search.ResultUp()
		return m, nil
	case "down":
		m.search.ResultDown()
		return m, nil
	}

	// Let the textinput handle all other keys (typing, backspace, arrows, etc.)
	prev := m.search.Value()
	cmd := m.search.UpdateInput(msg)
	if m.search.Value() != prev {
		m.doSearch()
	}
	return m, cmd
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterBar.CloseEditor()
		return m, nil
	case "enter":
		m.filterBar.ApplyFromInput()
		m.doApplyFilters()
		return m, nil
	default:
		cmd := m.filterBar.UpdateInput(msg)
		return m, cmd
	}
}

func (m Model) handleWatchEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.watchlist.CancelEdit()
		return m, nil
	case "tab":
		if m.watchlist.NextField() {
			// Already on last field, do nothing (enter will submit)
		}
		return m, textinput.Blink
	case "enter":
		if m.watchlist.NextField() {
			// On last field — submit
			name, pattern := m.watchlist.FinishEdit()
			if pattern != "" && m.store != nil {
				m.store.AddWatch(name, pattern, "")
				m.refreshWatchlist()
			}
		}
		return m, textinput.Blink
	default:
		// Pass all other keys to the active textinput
		cmd := m.watchlist.UpdateInput(msg)
		return m, cmd
	}
}

func (m Model) handleWatchDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		item := m.watchlist.Selected()
		if item != nil && m.store != nil {
			m.store.RemoveWatch(item.ID)
			m.refreshWatchlist()
		}
		m.watchlist.ConfirmDelete()
	default:
		m.watchlist.CancelDelete()
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		m.confirmQuit = true
		return m, nil

	case "?":
		m.showSettings = true

	case "M":
		proj := m.projects.Selected()
		if proj != nil {
			files, _ := claude.LoadMemory(proj.DataDir)
			m.memory.SetSize(m.width, m.height)
			m.memory.Show(proj.Name, files)
		}

	case "H":
		proj := m.projects.Selected()
		projName := ""
		projPath := ""
		if proj != nil {
			projName = proj.Name
			projPath = proj.Path
		}
		sources := claude.LoadAllHooks(projPath)
		m.hooks.SetSize(m.width, m.height)
		m.hooks.Show(projName, sources)

	case "esc":
		// Clear search highlights and restore session list
		m.detail.ClearSearch()
		m.doFilterSessions()

	case "tab":
		m.focus = m.nextPane(1)

	case "shift+tab":
		m.focus = m.nextPane(-1)

	case "/":
		m.search.Open()
		return m, textinput.Blink

	case "f":
		m.filterBar.OpenEditor()
		return m, textinput.Blink

	case "F":
		m.filterBar.Clear()
		m.detail.SetFilters(nil)

	case "w":
		m.watchlist.Toggle()
		if m.watchlist.IsVisible() {
			m.refreshWatchlist()
		}
		m.layoutPanes()
		m.saveConfig()

	case "W":
		m.watchlist.Show()
		m.watchlist.StartAdd()
		m.focus = paneWatchlist
		m.layoutPanes()
		return m, textinput.Blink

	case "up", "k":
		switch m.focus {
		case paneProjects:
			m.projects.Up()
			m.doSelectProject()
		case paneSessions:
			m.sessions.Up()
		case paneDetail:
			m.detail.ScrollUp(3)
		case paneWatchlist:
			m.watchlist.Up()
		}

	case "down", "j":
		switch m.focus {
		case paneProjects:
			m.projects.Down()
			m.doSelectProject()
		case paneSessions:
			m.sessions.Down()
		case paneDetail:
			m.detail.ScrollDown(3)
		case paneWatchlist:
			m.watchlist.Down()
		}

	case "pgup":
		if m.focus == paneDetail {
			m.detail.ScrollUp(m.height / 2)
		}

	case "pgdown":
		if m.focus == paneDetail {
			m.detail.ScrollDown(m.height / 2)
		}

	case "g":
		if m.focus == paneDetail {
			m.detail.ClearSearch()
			m.detail.scrollToBottom()
		}

	case "G":
		if m.focus == paneDetail {
			m.detail.ScrollToTop()
		}

	case "n":
		m.detail.NextMatch()

	case "N":
		m.detail.PrevMatch()

	case "a":
		if m.focus == paneWatchlist {
			m.watchlist.StartAdd()
			return m, textinput.Blink
		}

	case "d":
		if m.focus == paneWatchlist {
			item := m.watchlist.Selected()
			if item != nil {
				m.watchlist.AskDelete()
			}
		}

	case " ":
		if m.focus == paneWatchlist {
			item := m.watchlist.Selected()
			if item != nil && m.store != nil {
				m.store.ToggleWatch(item.ID)
				m.refreshWatchlist()
			}
		}

	case "m":
		if m.focus == paneWatchlist {
			item := m.watchlist.Selected()
			if item != nil && m.store != nil {
				m.store.MarkWatchSeen(item.ID)
				m.refreshWatchlist()
			}
		}

	case "enter":
		switch m.focus {
		case paneSessions:
			m.doSelectSession()
			m.focus = paneDetail
		case paneProjects:
			m.doSelectProject()
			m.focus = paneSessions
		case paneWatchlist:
			m.doSelectWatchlist()
			m.focus = paneSessions
		}
	}

	return m, nil
}

func (m Model) nextPane(dir int) pane {
	panes := []pane{paneProjects, paneSessions, paneDetail}
	if m.watchlist.IsVisible() {
		panes = []pane{paneProjects, paneSessions, paneDetail, paneWatchlist}
	}
	for i, p := range panes {
		if p == m.focus {
			next := (i + dir + len(panes)) % len(panes)
			return panes[next]
		}
	}
	return paneProjects
}

func (m *Model) loadProjects() {
	projects, err := claude.DiscoverProjects(m.cfg.ProjectPaths)
	if err != nil {
		return
	}
	m.projects.SetProjects(projects)
	m.doSelectProject()
}

func (m *Model) doSelectProject() {
	proj := m.projects.Selected()
	if proj == nil {
		return
	}
	sessions, err := claude.LoadSessions(proj.DataDir)
	if err != nil {
		return
	}
	m.activeWatchName = ""
	m.allSessions = sessions
	m.doFilterSessions()
}

func (m *Model) doSelectWatchlist() {
	item := m.watchlist.Selected()
	if item == nil || m.store == nil {
		return
	}

	matches, err := m.store.MatchesForWatch(item.ID, 10000)
	if err != nil || len(matches) == 0 {
		m.activeWatchName = item.Name
		m.allSessions = nil
		m.doFilterSessions()
		return
	}

	seen := make(map[string]bool)
	var sessionIDs []string
	for _, match := range matches {
		if !seen[match.SessionID] {
			seen[match.SessionID] = true
			sessionIDs = append(sessionIDs, match.SessionID)
		}
	}

	sessions, err := m.store.SessionsByIDs(sessionIDs)
	if err != nil {
		return
	}

	m.activeWatchName = item.Name
	m.allSessions = sessions
	m.doFilterSessions()
	m.store.MarkWatchSeen(item.ID)
	m.refreshWatchlist()
}

func (m *Model) doFilterSessions() {
	name := ""
	if m.activeWatchName != "" {
		name = "WATCH: " + m.activeWatchName
	} else if proj := m.projects.Selected(); proj != nil {
		name = proj.Name
	}

	if !m.search.IsActive() && m.search.Value() == "" {
		m.sessions.SetSessions(m.allSessions, name)
		return
	}

	var filtered []claude.SessionEntry
	for _, s := range m.allSessions {
		if m.search.Matches(s.FirstPrompt) {
			filtered = append(filtered, s)
		}
	}
	m.sessions.SetSessions(filtered, name)
}

func (m *Model) doSearch() {
	query := m.search.Value()
	if query == "" {
		m.search.SetResults(nil, 0)
		m.doFilterSessions()
		return
	}

	if m.store == nil {
		// Fallback to substring matching if store not available
		m.doFilterSessions()
		return
	}

	switch m.search.Scope() {
	case ScopeLocal:
		// Search within current session
		sess := m.sessions.Selected()
		if sess != nil {
			results, err := m.store.SearchInSession(sess.SessionID, query)
			if err == nil {
				m.search.SetResults(results, len(results))
			}
		}
	case ScopeProject:
		// Search within current project
		proj := m.projects.Selected()
		projectName := ""
		if proj != nil {
			projectName = proj.Name
		}
		results, err := m.store.Search(query, 50)
		if err == nil {
			// Filter results to current project
			var filtered []store.SearchResult
			for _, r := range results {
				if projectName == "" || r.Project == projectName {
					filtered = append(filtered, r)
				}
			}
			count := m.store.MatchCount(query)
			m.search.SetResults(filtered, count)
		}
		// Also filter session list
		sessions, err := m.store.SearchSessions(query, projectName)
		if err == nil && len(sessions) > 0 {
			name := ""
			if proj != nil {
				name = proj.Name
			}
			m.sessions.SetSessions(sessions, name)
		}
	case ScopeGlobal:
		// Search across all projects
		results, err := m.store.Search(query, 50)
		if err == nil {
			count := m.store.MatchCount(query)
			m.search.SetResults(results, count)
		}
		sessions, err := m.store.SearchSessions(query, "")
		if err == nil && len(sessions) > 0 {
			m.sessions.SetSessions(sessions, "ALL PROJECTS")
		}
	}
}

func (m *Model) navigateToResult(r *store.SearchResult) {
	// Find the session and load it
	if r.SessionID == "" {
		return
	}
	// Load messages for this session from the JSONL file
	// First find the full path from allSessions or the store
	for i, sess := range m.allSessions {
		if sess.SessionID == r.SessionID {
			m.sessions.cursor = i
			messages, err := claude.LoadMessages(sess.FullPath)
			if err == nil {
				m.detail.SetSession(&m.allSessions[i], messages)
				m.focus = paneDetail
			}
			return
		}
	}
}

func (m *Model) doApplyFilters() {
	if !m.filterBar.HasFilters() {
		m.detail.SetFilters(nil)
		return
	}
	m.detail.SetFilters(m.filterBar.Filters())
}

func (m *Model) refreshWatchlist() {
	if m.store == nil {
		return
	}
	items, err := m.store.ListWatches()
	if err == nil {
		m.watchlist.SetItems(items)
	}
}

func (m *Model) doSelectSession() {
	sess := m.sessions.Selected()
	if sess == nil {
		return
	}
	messages, err := claude.LoadMessages(sess.FullPath)
	if err != nil {
		return
	}
	m.detail.SetSession(sess, messages)
}

func (m *Model) layoutPanes() {
	leftW := m.width * 30 / 100
	if leftW < 28 {
		leftW = 28
	}
	rightW := m.width - leftW

	// Reserve space for filter bar and status bar
	extraLines := 6
	if m.filterBar.HasFilters() {
		extraLines += 1
	}

	bodyH := m.height - extraLines

	if m.watchlist.IsVisible() {
		watchH := bodyH * 25 / 100
		if watchH < 5 {
			watchH = 5
		}
		projH := (bodyH - watchH) * 45 / 100
		sessH := bodyH - watchH - projH - 4 // subtract border overhead (3 panels × 2 rows - overlap)

		m.projects.SetSize(leftW, projH)
		m.sessions.SetSize(leftW, sessH)
		m.watchlist.SetSize(leftW, watchH)
		m.detail.SetSize(rightW, bodyH)
	} else {
		projH := bodyH * 40 / 100
		sessH := bodyH - projH - 2 // subtract border overhead (2 panels × 2 rows - overlap)

		m.projects.SetSize(leftW, projH)
		m.sessions.SetSize(leftW, sessH)
		m.detail.SetSize(rightW, bodyH)
	}

	m.search.SetWidth(m.width)
	m.filterBar.SetWidth(m.width)
}

func (m Model) View() string {
	if !m.ready {
		return ""
	}

	var b strings.Builder

	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	leftW := m.width * 30 / 100
	if leftW < 28 {
		leftW = 28
	}
	rightW := m.width - leftW

	extraLines := 5
	if m.filterBar.HasFilters() || m.filterBar.IsEditing() {
		extraLines += 1
	}
	bodyH := m.height - extraLines

	var leftParts []string

	sessTitle := fmt.Sprintf("SESSIONS (%s)", strings.ToUpper(m.sessions.ProjectName()))
	detailTitle := m.detail.Title()

	if m.watchlist.IsVisible() {
		watchH := bodyH * 25 / 100
		if watchH < 5 {
			watchH = 5
		}
		projH := (bodyH - watchH) * 45 / 100
		sessH := bodyH - watchH - projH - 4 // borders

		projBox := m.panelBox("PROJECTS", m.projects.View(), leftW, projH, m.focus == paneProjects)
		sessBox := m.panelBox(sessTitle, m.sessions.View(), leftW, sessH, m.focus == paneSessions)
		watchBox := m.panelBox("WATCHLIST", m.watchlist.View(), leftW, watchH, m.focus == paneWatchlist)
		leftParts = append(leftParts, projBox, sessBox, watchBox)
	} else {
		projH := bodyH * 40 / 100
		sessH := bodyH - projH - 2

		projBox := m.panelBox("PROJECTS", m.projects.View(), leftW, projH, m.focus == paneProjects)
		sessBox := m.panelBox(sessTitle, m.sessions.View(), leftW, sessH, m.focus == paneSessions)
		leftParts = append(leftParts, projBox, sessBox)
	}

	leftPane := lipgloss.JoinVertical(lipgloss.Left, leftParts...)
	rightPane := m.panelBox(detailTitle, m.detail.View(), rightW, bodyH, m.focus == paneDetail)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	b.WriteString(body)
	b.WriteString("\n")

	// Overlays
	if m.confirmQuit {
		// handled below as full-screen overlay
	} else if m.showSettings {
		// handled below as full-screen overlay
	} else if m.filterBar.IsEditing() {
		b.WriteString(m.filterBar.View())
		b.WriteString("\n")
	} else if m.search.IsActive() {
		b.WriteString(m.search.View())
		b.WriteString("\n")
	}

	// Filter bar (persistent when filters active)
	if m.filterBar.HasFilters() && !m.filterBar.IsEditing() {
		b.WriteString(m.filterBar.View())
		b.WriteString("\n")
	}

	b.WriteString(m.renderStatusBar())

	// Modal overlays — render on top of everything
	if m.confirmQuit {
		return overlayCenter(b.String(), m.renderConfirmQuit(), m.width, m.height)
	}
	if m.showSettings {
		return m.renderSettings()
	}
	if m.memory.IsVisible() {
		return m.memory.View()
	}
	if m.hooks.IsVisible() {
		return m.hooks.View()
	}

	return b.String()
}

func (m Model) renderHeader() string {
	bg := lipgloss.NewStyle().Background(ColorBarBg)

	pulse := 0.7 + 0.3*math.Sin(float64(m.frame)*0.06)

	// Animated star glyph
	starFrames := []string{"✦", "✧", "✶", "✧", "✦", "⊹"}
	starIdx := (m.frame / 6) % len(starFrames)
	sgr := int(80 * pulse)
	sgg := int(255 * pulse)
	sgb := int(120 * pulse)
	if sgr > 255 { sgr = 255 }
	if sgg > 255 { sgg = 255 }
	if sgb > 255 { sgb = 255 }
	starColor := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", sgr, sgg, sgb))
	star := bg.Foreground(starColor).Bold(true).Render(starFrames[starIdx])
	titleLabel := bg.Foreground(ColorCyan).Bold(true).Render("CLAUDE CHRONICLE")

	// Left column width
	leftW := m.width * 30 / 100
	if leftW < 28 {
		leftW = 28
	}

	// Build left side: " ✦ CLAUDE CHRONICLE" padded to leftW
	leftContent := bg.Render(" ") + star + bg.Render(" ") + titleLabel
	leftVisW := 1 + 1 + 1 + 16 // space + star + space + "CLAUDE CHRONICLE"
	leftPad := leftW - leftVisW
	if leftPad < 0 {
		leftPad = 0
	}
	leftCol := leftContent + bg.Render(strings.Repeat(" ", leftPad))

	// Right column: stats + spacer + clock
	stats := bg.Render(" ") + m.detail.HeaderStats()
	now := time.Now()
	clockText := fmt.Sprintf("TIME %s  ", now.Format("15:04:05"))
	clock := bg.Foreground(ColorBarText).Render(clockText)

	usedWidth := leftW + visibleLen(stats) + len(clockText)
	spacerLen := max(m.width-usedWidth, 1)
	spacer := bg.Render(strings.Repeat(" ", spacerLen))

	return leftCol + stats + spacer + clock
}

func (m Model) panelBox(title, content string, w, h int, focused bool) string {
	return RenderPanel(title, content, w, h, focused)
}

func (m Model) renderSettings() string {
	modalW := m.width * 50 / 100
	if modalW > 70 {
		modalW = 70
	}
	if modalW < 40 {
		modalW = 40
	}
	innerW := modalW - 4

	bc := lipgloss.NewStyle().Foreground(ColorCyan)
	tc := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	dim := lipgloss.NewStyle().Foreground(ColorDim)

	var rows []string

	// Top border with title
	title := " SETTINGS "
	titleVisLen := len(title)
	fillLen := innerW - 3 - titleVisLen
	if fillLen < 0 {
		fillLen = 0
	}
	rows = append(rows, bc.Render("┏━╸")+tc.Render(title)+bc.Render("╺"+strings.Repeat("━", fillLen)+"┓"))

	side := bc.Render("┃")

	// Database stats
	fileCount := 0
	msgCount := 0
	if m.store != nil {
		fileCount = m.store.FileCount()
		msgCount = m.store.MessageCount()
	}

	addLine := func(content string) {
		pad := innerW - visibleLen(content)
		if pad < 0 {
			pad = 0
		}
		rows = append(rows, side+content+strings.Repeat(" ", pad)+side)
	}

	addLine("")
	addLine("  " + lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("DATABASE"))
	addLine("")
	addLine(fmt.Sprintf("  Indexed files:    %s",
		lipgloss.NewStyle().Foreground(ColorGreen).Render(fmt.Sprintf("%d", fileCount))))
	addLine(fmt.Sprintf("  Indexed messages: %s",
		lipgloss.NewStyle().Foreground(ColorGreen).Render(fmt.Sprintf("%d", msgCount))))
	// Navigable action items with unified cursor
	selMark := lipgloss.NewStyle().Foreground(ColorSelect)
	selText := lipgloss.NewStyle().Foreground(ColorSelect).Bold(true)

	addAction := func(idx int, label string) {
		if m.settingsCursor == idx {
			addLine(fmt.Sprintf("  %s %s", selMark.Render("▸"), selText.Render(label)))
		} else {
			addLine(fmt.Sprintf("    %s", NormalStyle.Render(label)))
		}
	}

	addLine("")
	addAction(0, "[r] Reindex all files")
	addAction(1, "[R] Rebuild database")

	// Custom project paths
	addLine("")
	addLine("  " + dim.Render(strings.Repeat("─", innerW-4)))
	addLine("")
	addLine("  " + lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("CUSTOM PATHS"))
	addLine("")

	if m.settingsAddingPath {
		m.settingsPathInput.Width = innerW - 10
		addLine("  " + m.settingsPathInput.View())
		if m.settingsPathError != "" {
			addLine("  " + lipgloss.NewStyle().Foreground(ColorRed).Render(m.settingsPathError))
		}
	} else if len(m.cfg.ProjectPaths) > 0 {
		for i, p := range m.cfg.ProjectPaths {
			pathText := p
			if len(pathText) > innerW-6 {
				pathText = "..." + pathText[len(pathText)-(innerW-9):]
			}
			cursorIdx := 2 + i
			if m.settingsConfirmDel && m.settingsCursor == cursorIdx {
				addLine(fmt.Sprintf("  %s %s",
					lipgloss.NewStyle().Foreground(ColorRed).Render("▸"),
					lipgloss.NewStyle().Foreground(ColorRed).Render(pathText)))
			} else if m.settingsCursor == cursorIdx {
				addLine(fmt.Sprintf("  %s %s", selMark.Render("▸"), selText.Render(pathText)))
			} else {
				addLine(fmt.Sprintf("    %s %s",
					lipgloss.NewStyle().Foreground(ColorGreen).Render("▸"),
					NormalStyle.Render(pathText)))
			}
		}
		if m.settingsConfirmDel {
			if pi := m.settingsPathIndex(); pi >= 0 {
				addLine("")
				name := filepath.Base(m.cfg.ProjectPaths[pi])
				addLine("  " + lipgloss.NewStyle().Foreground(ColorRed).Render(
					fmt.Sprintf("Remove \"%s\"?  y/n", name)))
			}
		}
	} else {
		addLine("  " + dim.Render("No custom paths"))
	}

	addLine("")
	addIdx := 2 + len(m.cfg.ProjectPaths) // "add path" is always the last item
	addAction(addIdx, "[a] Add path")

	addLine("")

	// Footer — context-sensitive, kept short to avoid overflow
	if m.settingsAddingPath {
		addLine(dim.Render("  Enter: add  Esc: cancel"))
	} else if m.settingsConfirmDel {
		addLine(dim.Render("  y: confirm  n: cancel"))
	} else {
		addLine(dim.Render("  ↑↓ navigate  Enter select  Esc close"))
	}

	// Bottom border
	rows = append(rows, bc.Render("┗"+strings.Repeat("━", innerW)+"┛"))

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

func (m Model) renderConfirmQuit() string {
	bc := lipgloss.NewStyle().Foreground(ColorYellow)
	tc := lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)
	dim := lipgloss.NewStyle().Foreground(ColorDim)

	innerW := 30

	side := bc.Render("┃")

	var rows []string

	// Top border with title
	title := " QUIT "
	fillLen := innerW - 3 - len(title)
	if fillLen < 0 {
		fillLen = 0
	}
	rows = append(rows, bc.Render("┏━╸")+tc.Render(title)+bc.Render("╺"+strings.Repeat("━", fillLen)+"┓"))

	rows = append(rows, side+strings.Repeat(" ", innerW)+side)

	q := "  Exit clog?"
	qStyled := lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(q)
	rows = append(rows, side+qStyled+strings.Repeat(" ", max(innerW-visibleLen(qStyled), 0))+side)

	rows = append(rows, side+strings.Repeat(" ", innerW)+side)

	opts := fmt.Sprintf("  %s yes  %s no", SelectedStyle.Render("[y/q]"), dim.Render("[n]"))
	rows = append(rows, side+opts+strings.Repeat(" ", max(innerW-visibleLen(opts), 0))+side)

	rows = append(rows, side+strings.Repeat(" ", innerW)+side)
	rows = append(rows, bc.Render("┗"+strings.Repeat("━", innerW)+"┛"))

	return strings.Join(rows, "\n")
}

// overlayCenter composites a small modal on top of a rendered background,
// replacing lines in the center while keeping the dashboard visible around it.
func overlayCenter(bg, modal string, width, height int) string {
	bgLines := strings.Split(bg, "\n")
	modalLines := strings.Split(modal, "\n")

	// Pad background to full height
	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}

	modalH := len(modalLines)
	modalW := 0
	for _, ml := range modalLines {
		if w := visibleLen(ml); w > modalW {
			modalW = w
		}
	}

	topOff := (height - modalH) / 2
	leftOff := (width - modalW) / 2
	if topOff < 0 {
		topOff = 0
	}
	if leftOff < 0 {
		leftOff = 0
	}

	for i, ml := range modalLines {
		row := topOff + i
		if row < len(bgLines) {
			bgLines[row] = spliceAnsiLine(bgLines[row], ml, leftOff, width)
		}
	}

	return strings.Join(bgLines, "\n")
}

// ansiSegments splits a string into segments: each segment is either an ANSI
// escape sequence (visible=false) or a single visible rune (visible=true).
type ansiSeg struct {
	text    string
	visible bool
}

func splitAnsiSegments(s string) []ansiSeg {
	var segs []ansiSeg
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Scan to end of escape sequence (letter terminates)
			j := i + 1
			for j < len(s) && !((s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z')) {
				j++
			}
			if j < len(s) {
				j++ // include the terminating letter
			}
			segs = append(segs, ansiSeg{s[i:j], false})
			i = j
		} else {
			// Decode one UTF-8 rune
			_, size := utf8.DecodeRuneInString(s[i:])
			segs = append(segs, ansiSeg{s[i : i+size], true})
			i += size
		}
	}
	return segs
}

// spliceAnsiLine composites modalLine on top of bgLine starting at visible
// column leftOff. Preserves background on both sides of the modal.
func spliceAnsiLine(bgLine, modalLine string, leftOff, totalWidth int) string {
	modalVisW := visibleLen(modalLine)
	segs := splitAnsiSegments(bgLine)

	var out strings.Builder
	col := 0

	// 1. Write background up to leftOff visible columns
	for _, seg := range segs {
		if col >= leftOff {
			break
		}
		if !seg.visible {
			out.WriteString(seg.text)
		} else {
			out.WriteString(seg.text)
			col++
		}
	}
	// Pad if background was shorter than leftOff
	for col < leftOff {
		out.WriteByte(' ')
		col++
	}

	// 2. Reset styling, write modal
	out.WriteString("\x1b[0m")
	out.WriteString(modalLine)

	// 3. Skip background segments covered by the modal, then write the rest
	rightStart := leftOff + modalVisW
	bgCol := 0
	writing := false
	for _, seg := range segs {
		if !seg.visible {
			if writing {
				out.WriteString(seg.text)
			}
			continue
		}
		bgCol++
		if bgCol <= rightStart {
			continue
		}
		if !writing {
			writing = true
		}
		out.WriteString(seg.text)
	}

	return out.String()
}

func (m Model) renderStatusBar() string {
	bg := lipgloss.NewStyle().Background(ColorBarBg)

	leftText := "  [/] Search  [f] Filter  [w] Watch  [M] Memory  [H] Hooks  [Tab] Switch  [?] Settings  [q] Quit"
	left := bg.Foreground(ColorBarText).Render(leftText)

	// Right-side indicators (clustered together)
	var rightParts []string
	rightLen := 0

	// Watchlist unseen indicator (when pane is hidden)
	if !m.watchlist.IsVisible() && m.store != nil {
		unseen := m.store.TotalUnseenCount()
		if unseen > 0 {
			watchText := fmt.Sprintf("WATCH: %d new", unseen)
			rightParts = append(rightParts, bg.Foreground(ColorRed).Render(watchText))
			rightLen += len(watchText)
		}
	}

	// Index status indicator
	if m.indexing {
		indexText := "INDEXING..."
		rightParts = append(rightParts, bg.Foreground(ColorYellow).Render(indexText))
		rightLen += len(indexText)
	} else if m.indexStatus != "" {
		rightParts = append(rightParts, bg.Foreground(ColorBarText).Render(m.indexStatus))
		rightLen += len(m.indexStatus)
	}

	// Stale index nag (>7 days since last full index)
	if !m.indexing && m.store != nil {
		age := m.store.IndexAge()
		if age > 7*24*time.Hour {
			days := int(age.Hours() / 24)
			nagText := fmt.Sprintf("INDEX: %dd old — [?]→[r] to refresh", days)
			rightParts = append(rightParts, bg.Foreground(ColorYellowDim).Render(nagText))
			rightLen += runewidth.StringWidth(nagText)
		}
	}

	if len(rightParts) == 0 {
		spacerLen := max(m.width-len(leftText), 1)
		spacer := bg.Render(strings.Repeat(" ", spacerLen))
		return left + spacer
	}

	sep := bg.Foreground(ColorDim).Render(" │ ")
	sepLen := 3
	rightTotal := rightLen + sepLen*(len(rightParts)-1) + 2
	right := strings.Join(rightParts, sep) + bg.Render("  ")

	spacerLen := max(m.width-len(leftText)-rightTotal, 1)
	spacer := bg.Render(strings.Repeat(" ", spacerLen))
	return left + spacer + right
}

func visibleLen(s string) int {
	return runewidth.StringWidth(stripAnsi(s))
}

func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
