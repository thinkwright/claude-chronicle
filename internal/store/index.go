package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thinkwright/claude-chronicle/internal/claude"
)

type IndexProgress struct {
	Phase   string // "discovering", "indexing", "done"
	Current int
	Total   int
	File    string
}

// skipTypes are JSONL message types we don't index.
var skipTypes = map[claude.MessageType]bool{
	"progress":              true,
	"queue-operation":       true,
	"file-history-snapshot": true,
}

// IndexAll indexes every JSONL file across all projects.
// Sends progress updates on the channel. Closes the channel when done.
func (s *Store) IndexAll(progress chan<- IndexProgress) error {
	defer close(progress)

	progress <- IndexProgress{Phase: "discovering"}

	projects, err := claude.DiscoverProjects()
	if err != nil {
		return fmt.Errorf("discover projects: %w", err)
	}

	// Collect all JSONL files
	type fileEntry struct {
		path    string
		project string
	}
	var files []fileEntry

	for _, proj := range projects {
		entries, err := os.ReadDir(proj.DataDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				files = append(files, fileEntry{
					path:    filepath.Join(proj.DataDir, e.Name()),
					project: proj.Name,
				})
			}
		}
	}

	progress <- IndexProgress{Phase: "indexing", Total: len(files)}

	for i, f := range files {
		progress <- IndexProgress{
			Phase:   "indexing",
			Current: i + 1,
			Total:   len(files),
			File:    filepath.Base(f.path),
		}

		if err := s.indexFile(f.path, f.project); err != nil {
			// Log but continue — one bad file shouldn't stop everything
			continue
		}
	}

	progress <- IndexProgress{Phase: "done", Current: len(files), Total: len(files)}
	return nil
}

// IndexChanged re-indexes only files whose mtime or size changed.
// Returns the number of new/updated files.
func (s *Store) IndexChanged() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	projects, err := claude.DiscoverProjects()
	if err != nil {
		return 0, err
	}

	changed := 0

	for _, proj := range projects {
		entries, err := os.ReadDir(proj.DataDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}

			path := filepath.Join(proj.DataDir, e.Name())
			info, err := e.Info()
			if err != nil {
				continue
			}

			mtime := info.ModTime().UnixMilli()
			size := info.Size()

			// Check if this file is already indexed with same mtime+size
			var existingMtime, existingSize int64
			err = s.db.QueryRow(
				"SELECT mtime, size FROM files WHERE path = ?", path,
			).Scan(&existingMtime, &existingSize)

			if err == nil && existingMtime == mtime && existingSize == size {
				continue // unchanged
			}

			if err := s.indexFile(path, proj.Name); err != nil {
				continue
			}
			changed++
		}
	}

	return changed, nil
}

func (s *Store) indexFile(path, project string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	mtime := info.ModTime().UnixMilli()
	size := info.Size()
	now := time.Now().UnixMilli()

	// Load messages using existing parser
	messages, err := claude.LoadMessages(path)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clean up old data for this file
	var oldFileID int64
	err = tx.QueryRow("SELECT id FROM files WHERE path = ?", path).Scan(&oldFileID)
	if err == nil {
		// Delete old messages (cascade will handle watchlist_matches)
		tx.Exec("DELETE FROM messages WHERE session_id = ?", sessionID)
		tx.Exec("DELETE FROM sessions WHERE session_id = ?", sessionID)
		tx.Exec("DELETE FROM files WHERE id = ?", oldFileID)
	}

	// Insert file record
	res, err := tx.Exec(
		"INSERT INTO files (path, project, session_id, mtime, size, indexed_at) VALUES (?, ?, ?, ?, ?, ?)",
		path, project, sessionID, mtime, size, now,
	)
	if err != nil {
		return err
	}
	fileID, _ := res.LastInsertId()

	// Insert messages, aggregate session stats
	var (
		firstPrompt string
		gitBranch   string
		model       string
		createdAt   string
		modifiedAt  string
		totalIn     int
		totalOut    int
		toolCount   int
		msgCount    int
	)

	msgStmt, err := tx.Prepare(`
		INSERT INTO messages (session_id, type, timestamp, model, text, tool_calls, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer msgStmt.Close()

	for _, msg := range messages {
		if skipTypes[msg.Type] {
			continue
		}

		// Truncate very long texts for FTS (keep full text in original JSONL)
		text := msg.Text
		if len(text) > 50000 {
			text = text[:50000]
		}

		tools := strings.Join(msg.ToolCalls, ", ")

		_, err := msgStmt.Exec(
			sessionID, string(msg.Type), msg.Timestamp, msg.Model,
			text, tools, msg.InputTokens, msg.OutputTokens,
		)
		if err != nil {
			continue
		}

		// Aggregate session stats
		msgCount++
		totalIn += msg.InputTokens
		totalOut += msg.OutputTokens
		toolCount += len(msg.ToolCalls)

		if msg.Model != "" {
			model = claude.FormatModel(msg.Model)
		}
		if msg.Timestamp != "" {
			if createdAt == "" {
				createdAt = msg.Timestamp
			}
			modifiedAt = msg.Timestamp
		}
		if firstPrompt == "" && msg.Type == claude.TypeUser && msg.Text != "" {
			firstPrompt = msg.Text
			if len(firstPrompt) > 120 {
				firstPrompt = firstPrompt[:120]
			}
		}
		if gitBranch == "" && msg.Type == claude.TypeSystem {
			// Git branch is often in system messages — the claude package
			// handles this in extractSessionMeta but we check the raw text here
		}
	}

	// Insert session metadata
	_, err = tx.Exec(`
		INSERT INTO sessions (session_id, file_id, project, first_prompt, git_branch, model,
			created_at, modified_at, message_count, total_input_tokens, total_output_tokens, tool_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, fileID, project, firstPrompt, gitBranch, model,
		createdAt, modifiedAt, msgCount, totalIn, totalOut, toolCount,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// FileCount returns the number of indexed files.
func (s *Store) FileCount() int {
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
	return count
}

// MessageCount returns the total number of indexed messages.
func (s *Store) MessageCount() int {
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	return count
}

// LastIndexedAt returns the most recent indexed_at timestamp (unix millis),
// or 0 if no files are indexed.
func (s *Store) LastIndexedAt() int64 {
	var ts int64
	s.db.QueryRow("SELECT COALESCE(MAX(indexed_at), 0) FROM files").Scan(&ts)
	return ts
}

// IndexAge returns the duration since the last indexing operation.
// Returns 0 if no files are indexed.
func (s *Store) IndexAge() time.Duration {
	ts := s.LastIndexedAt()
	if ts == 0 {
		return 0
	}
	return time.Since(time.UnixMilli(ts))
}
