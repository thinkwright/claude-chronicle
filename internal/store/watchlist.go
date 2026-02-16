package store

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type WatchItem struct {
	ID          int64
	Name        string
	Pattern     string
	Compiled    *regexp.Regexp
	Enabled     bool
	Color       string
	CreatedAt   string
	UnseenCount int
}

type WatchMatch struct {
	ID          int64
	WatchItemID int64
	MessageID   int64
	SessionID   string
	Project     string
	MatchedText string
	Seen        bool
	Timestamp   string
}

// AddWatch creates a new watchlist item. Returns error if the pattern is invalid.
func (s *Store) AddWatch(name, pattern, color string) (*WatchItem, error) {
	compiled := s.compiledRegex(pattern)
	if compiled == nil {
		return nil, fmt.Errorf("invalid regex: %s", pattern)
	}

	if color == "" {
		color = "#b56a6a"
	}

	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(
		"INSERT INTO watchlist (name, pattern, enabled, color, created_at) VALUES (?, ?, 1, ?, ?)",
		name, pattern, color, now,
	)
	if err != nil {
		return nil, err
	}

	id, _ := res.LastInsertId()
	item := &WatchItem{
		ID:        id,
		Name:      name,
		Pattern:   pattern,
		Compiled:  compiled,
		Enabled:   true,
		Color:     color,
		CreatedAt: now,
	}

	// Backfill matches against existing messages
	s.matchAllForWatch(item)

	return item, nil
}

// RemoveWatch deletes a watchlist item and its matches.
func (s *Store) RemoveWatch(id int64) error {
	_, err := s.db.Exec("DELETE FROM watchlist WHERE id = ?", id)
	return err
}

// ToggleWatch enables/disables a watchlist item.
func (s *Store) ToggleWatch(id int64) error {
	_, err := s.db.Exec("UPDATE watchlist SET enabled = NOT enabled WHERE id = ?", id)
	return err
}

// UpdateWatch modifies a watchlist item's name and pattern.
func (s *Store) UpdateWatch(id int64, name, pattern string) error {
	if s.compiledRegex(pattern) == nil {
		return fmt.Errorf("invalid regex: %s", pattern)
	}

	_, err := s.db.Exec("UPDATE watchlist SET name = ?, pattern = ? WHERE id = ?", name, pattern, id)
	if err != nil {
		return err
	}

	// Re-run matching
	s.db.Exec("DELETE FROM watchlist_matches WHERE watchlist_id = ?", id)

	item, err := s.GetWatch(id)
	if err == nil && item.Enabled {
		go s.matchAllForWatch(item)
	}
	return nil
}

// GetWatch loads a single watchlist item.
func (s *Store) GetWatch(id int64) (*WatchItem, error) {
	var item WatchItem
	err := s.db.QueryRow(`
		SELECT w.id, w.name, w.pattern, w.enabled, w.color, w.created_at,
			COALESCE((SELECT COUNT(*) FROM watchlist_matches wm WHERE wm.watchlist_id = w.id AND wm.seen = 0), 0)
		FROM watchlist w WHERE w.id = ?
	`, id).Scan(&item.ID, &item.Name, &item.Pattern, &item.Enabled, &item.Color, &item.CreatedAt, &item.UnseenCount)
	if err != nil {
		return nil, err
	}
	item.Compiled = s.compiledRegex(item.Pattern)
	return &item, nil
}

// ListWatches returns all watchlist items with unseen counts.
func (s *Store) ListWatches() ([]WatchItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT w.id, w.name, w.pattern, w.enabled, w.color, w.created_at,
			COALESCE((SELECT COUNT(*) FROM watchlist_matches wm WHERE wm.watchlist_id = w.id AND wm.seen = 0), 0)
		FROM watchlist w
		ORDER BY w.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []WatchItem
	for rows.Next() {
		var item WatchItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Pattern, &item.Enabled, &item.Color, &item.CreatedAt, &item.UnseenCount); err != nil {
			continue
		}
		item.Compiled = s.compiledRegex(item.Pattern)
		items = append(items, item)
	}
	return items, nil
}

// MatchNewMessages runs all enabled watchlist patterns against the given message IDs.
// Returns the number of new matches found.
func (s *Store) MatchNewMessages(messageIDs []int64) (int, error) {
	if len(messageIDs) == 0 {
		return 0, nil
	}

	items, err := s.ListWatches()
	if err != nil {
		return 0, err
	}

	total := 0
	for _, item := range items {
		if !item.Enabled || item.Compiled == nil {
			continue
		}

		// Build IN clause
		placeholders := make([]string, len(messageIDs))
		args := make([]interface{}, len(messageIDs))
		for i, id := range messageIDs {
			placeholders[i] = "?"
			args[i] = id
		}

		rows, err := s.db.Query(fmt.Sprintf(
			"SELECT id, session_id, text FROM messages WHERE id IN (%s)",
			strings.Join(placeholders, ",")),
			args...,
		)
		if err != nil {
			continue
		}

		for rows.Next() {
			var msgID int64
			var sessionID, text string
			if rows.Scan(&msgID, &sessionID, &text) != nil {
				continue
			}

			loc := item.Compiled.FindStringIndex(text)
			if loc == nil {
				continue
			}

			// Extract context snippet around match
			snippet := extractSnippet(text, loc[0], loc[1], 100)

			s.db.Exec(`
				INSERT INTO watchlist_matches (watchlist_id, message_id, session_id, matched_text, seen)
				VALUES (?, ?, ?, ?, 0)
			`, item.ID, msgID, sessionID, snippet)
			total++
		}
		rows.Close()
	}

	return total, nil
}

// MatchesForWatch returns matches for a specific watchlist item.
func (s *Store) MatchesForWatch(watchID int64, limit int) ([]WatchMatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT wm.id, wm.watchlist_id, wm.message_id, wm.session_id, wm.matched_text, wm.seen,
			COALESCE(m.timestamp, ''),
			COALESCE(s.project, '')
		FROM watchlist_matches wm
		LEFT JOIN messages m ON m.id = wm.message_id
		LEFT JOIN sessions s ON s.session_id = wm.session_id
		WHERE wm.watchlist_id = ?
		ORDER BY m.timestamp DESC
		LIMIT ?
	`, watchID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []WatchMatch
	for rows.Next() {
		var m WatchMatch
		if err := rows.Scan(&m.ID, &m.WatchItemID, &m.MessageID, &m.SessionID,
			&m.MatchedText, &m.Seen, &m.Timestamp, &m.Project); err != nil {
			continue
		}
		matches = append(matches, m)
	}
	return matches, nil
}

// MarkWatchSeen marks all matches for a watchlist item as seen.
func (s *Store) MarkWatchSeen(watchID int64) error {
	_, err := s.db.Exec("UPDATE watchlist_matches SET seen = 1 WHERE watchlist_id = ?", watchID)
	return err
}

// MarkSessionSeen marks all watchlist matches for a specific session as seen.
func (s *Store) MarkSessionSeen(sessionID string) error {
	_, err := s.db.Exec("UPDATE watchlist_matches SET seen = 1 WHERE session_id = ?", sessionID)
	return err
}

// TotalUnseenCount returns the total unseen watchlist match count.
func (s *Store) TotalUnseenCount() int {
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM watchlist_matches WHERE seen = 0").Scan(&count)
	return count
}

// matchAllForWatch runs a single watchlist pattern against all indexed messages.
func (s *Store) matchAllForWatch(item *WatchItem) {
	if item.Compiled == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query("SELECT id, session_id, text FROM messages")
	if err != nil {
		return
	}
	defer rows.Close()

	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO watchlist_matches (watchlist_id, message_id, session_id, matched_text, seen)
		VALUES (?, ?, ?, ?, 0)
	`)
	if err != nil {
		return
	}
	defer stmt.Close()

	batch := 0
	for rows.Next() {
		var msgID int64
		var sessionID, text string
		if rows.Scan(&msgID, &sessionID, &text) != nil {
			continue
		}

		loc := item.Compiled.FindStringIndex(text)
		if loc == nil {
			continue
		}

		snippet := extractSnippet(text, loc[0], loc[1], 100)
		stmt.Exec(item.ID, msgID, sessionID, snippet)
		batch++

		if batch%1000 == 0 {
			if err := tx.Commit(); err != nil {
				return
			}
			stmt.Close()
			tx, err = s.db.Begin()
			if err != nil {
				return
			}
			stmt, err = tx.Prepare(`
				INSERT INTO watchlist_matches (watchlist_id, message_id, session_id, matched_text, seen)
				VALUES (?, ?, ?, ?, 0)
			`)
			if err != nil {
				tx.Rollback()
				return
			}
		}
	}

	tx.Commit()
}

func extractSnippet(text string, matchStart, matchEnd, contextLen int) string {
	start := matchStart - contextLen/2
	if start < 0 {
		start = 0
	}
	end := matchEnd + contextLen/2
	if end > len(text) {
		end = len(text)
	}

	snippet := text[start:end]
	snippet = strings.ReplaceAll(snippet, "\n", " ")

	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}
	return snippet
}
