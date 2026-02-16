package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type SessionsIndex struct {
	Version int            `json:"version"`
	Entries []SessionEntry `json:"entries"`
}

type SessionEntry struct {
	SessionID    string `json:"sessionId"`
	FullPath     string `json:"fullPath"`
	FileMtime    int64  `json:"fileMtime"`
	FirstPrompt  string `json:"firstPrompt"`
	MessageCount int    `json:"messageCount"`
	Created      string `json:"created"`
	Modified     string `json:"modified"`
	GitBranch    string `json:"gitBranch"`
	ProjectPath  string `json:"projectPath"`
	IsSidechain  bool   `json:"isSidechain"`
}

func LoadSessionsIndex(projectDataDir string) (*SessionsIndex, error) {
	path := filepath.Join(projectDataDir, "sessions-index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var idx SessionsIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}

	return &idx, nil
}

func LoadSessions(projectDataDir string) ([]SessionEntry, error) {
	// Try sessions-index.json first
	idx, err := LoadSessionsIndex(projectDataDir)
	if err == nil {
		var sessions []SessionEntry
		for _, e := range idx.Entries {
			if !e.IsSidechain {
				sessions = append(sessions, e)
			}
		}
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].FileMtime > sessions[j].FileMtime
		})
		return sessions, nil
	}

	// Fallback: scan .jsonl files directly
	return scanJSONLFiles(projectDataDir)
}

// scanJSONLFiles discovers sessions by reading .jsonl files directly.
// Checks both root-level files and UUID subdirectories.
func scanJSONLFiles(dir string) ([]SessionEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []SessionEntry

	// Scan root-level JSONL files
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if entry, ok := sessionFromFile(filepath.Join(dir, e.Name()), e); ok {
			sessions = append(sessions, entry)
		}
	}

	// Scan UUID subdirectories
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(dir, e.Name())
		subEntries, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}
		for _, se := range subEntries {
			if se.IsDir() || !strings.HasSuffix(se.Name(), ".jsonl") {
				continue
			}
			if entry, ok := sessionFromFile(filepath.Join(subDir, se.Name()), se); ok {
				sessions = append(sessions, entry)
			}
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].FileMtime > sessions[j].FileMtime
	})

	return sessions, nil
}

// sessionFromFile builds a SessionEntry from a single .jsonl file.
func sessionFromFile(fullPath string, e os.DirEntry) (SessionEntry, bool) {
	sessionID := strings.TrimSuffix(e.Name(), ".jsonl")

	info, err := e.Info()
	if err != nil {
		return SessionEntry{}, false
	}

	entry := SessionEntry{
		SessionID: sessionID,
		FullPath:  fullPath,
		FileMtime: info.ModTime().UnixMilli(),
		Modified:  info.ModTime().Format("2006-01-02T15:04:05.000Z"),
	}

	extractSessionMeta(fullPath, &entry)

	if entry.MessageCount == 0 {
		return SessionEntry{}, false
	}

	return entry, true
}

// extractSessionMeta reads a JSONL file to get first prompt, timestamp,
// git branch, and message count. Once metadata fields are found, it stops
// JSON-parsing and only counts remaining lines for performance.
func extractSessionMeta(path string, entry *SessionEntry) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0), 1024*1024) // 1MB max line

	count := 0
	metaDone := false
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		count++

		// Once we have all metadata, just count lines — skip expensive JSON parsing
		if metaDone {
			continue
		}

		var raw struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			GitBranch string `json:"gitBranch"`
			Message   struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		// Grab timestamp from first message
		if entry.Created == "" && raw.Timestamp != "" {
			entry.Created = raw.Timestamp
		}

		// Grab git branch
		if entry.GitBranch == "" && raw.GitBranch != "" {
			entry.GitBranch = raw.GitBranch
		}

		// Grab first user prompt
		if entry.FirstPrompt == "" && raw.Type == "user" {
			entry.FirstPrompt = extractContentText(raw.Message.Content)
		}

		// Update modified timestamp
		if raw.Timestamp != "" {
			entry.Modified = raw.Timestamp
		}

		// Check if all metadata fields are populated
		if entry.Created != "" && entry.GitBranch != "" && entry.FirstPrompt != "" {
			metaDone = true
		}
	}

	entry.MessageCount = count
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n])
}

func extractContentText(raw json.RawMessage) string {
	// Try as string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return truncateRunes(s, 120)
	}

	// Try as array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return truncateRunes(b.Text, 120)
			}
		}
	}

	return ""
}
