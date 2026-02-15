package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/thinkwright/claude-chronicle/internal/claude"
)

type SearchResult struct {
	MessageID   int64
	SessionID   string
	Project     string
	MessageType string
	Timestamp   string
	Text        string
	Highlighted string
	FirstPrompt string
	GitBranch   string
	Model       string
	Rank        float64
}

// Search executes a full-text + structured filter query and returns matching messages.
func (s *Store) Search(query string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fs := Parse(query)
	if fs.IsEmpty() {
		return nil, nil
	}

	where, params := fs.ToSQL()

	// Build the query
	var sqlStr string
	if fs.HasFTS() {
		sqlStr = fmt.Sprintf(`
			SELECT m.id, m.session_id, s.project, m.type, m.timestamp,
				m.text,
				highlight(messages_fts, 0, '<<', '>>'),
				s.first_prompt, s.git_branch, s.model,
				rank
			FROM messages m
			JOIN messages_fts ON messages_fts.rowid = m.id
			JOIN sessions s ON s.session_id = m.session_id
			WHERE %s
			ORDER BY rank
			LIMIT ?
		`, where)
	} else {
		sqlStr = fmt.Sprintf(`
			SELECT m.id, m.session_id, s.project, m.type, m.timestamp,
				m.text, m.text,
				s.first_prompt, s.git_branch, s.model,
				0
			FROM messages m
			JOIN sessions s ON s.session_id = m.session_id
			WHERE %s
			ORDER BY s.modified_at DESC, m.timestamp DESC
			LIMIT ?
		`, where)
	}

	params = append(params, limit)

	rows, err := s.db.Query(sqlStr, params...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(
			&r.MessageID, &r.SessionID, &r.Project, &r.MessageType,
			&r.Timestamp, &r.Text, &r.Highlighted,
			&r.FirstPrompt, &r.GitBranch, &r.Model, &r.Rank,
		); err != nil {
			continue
		}
		// Truncate text for display
		if len(r.Text) > 200 {
			r.Text = r.Text[:200] + "..."
		}
		if len(r.Highlighted) > 300 {
			r.Highlighted = r.Highlighted[:300] + "..."
		}
		results = append(results, r)
	}

	return results, nil
}

// SearchSessions returns sessions matching the query, for use in the session list.
func (s *Store) SearchSessions(query string, project string) ([]claude.SessionEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fs := Parse(query)
	if fs.IsEmpty() {
		return nil, nil
	}

	where, params := fs.ToSQL()

	// Add project filter if not searching globally
	if project != "" {
		// Check if there's already a project filter in the query
		hasProjectFilter := false
		for _, f := range fs.Filters {
			if f.Field == FilterProject {
				hasProjectFilter = true
				break
			}
		}
		if !hasProjectFilter {
			where += " AND s.project = ?"
			params = append(params, project)
		}
	}

	var sqlStr string
	if fs.HasFTS() {
		sqlStr = fmt.Sprintf(`
			SELECT DISTINCT s.session_id,
				COALESCE(f.path, ''),
				s.first_prompt, s.message_count,
				s.created_at, s.modified_at,
				s.git_branch, s.project
			FROM messages m
			JOIN messages_fts ON messages_fts.rowid = m.id
			JOIN sessions s ON s.session_id = m.session_id
			LEFT JOIN files f ON f.id = s.file_id
			WHERE %s
			ORDER BY s.modified_at DESC
			LIMIT 100
		`, where)
	} else {
		sqlStr = fmt.Sprintf(`
			SELECT DISTINCT s.session_id,
				COALESCE(f.path, ''),
				s.first_prompt, s.message_count,
				s.created_at, s.modified_at,
				s.git_branch, s.project
			FROM sessions s
			JOIN messages m ON m.session_id = s.session_id
			LEFT JOIN files f ON f.id = s.file_id
			WHERE %s
			ORDER BY s.modified_at DESC
			LIMIT 100
		`, where)
	}

	rows, err := s.db.Query(sqlStr, params...)
	if err != nil {
		return nil, fmt.Errorf("search sessions: %w", err)
	}
	defer rows.Close()

	var sessions []claude.SessionEntry
	for rows.Next() {
		var se claude.SessionEntry
		var fullPath sql.NullString
		if err := rows.Scan(
			&se.SessionID, &fullPath, &se.FirstPrompt, &se.MessageCount,
			&se.Created, &se.Modified, &se.GitBranch, &se.ProjectPath,
		); err != nil {
			continue
		}
		if fullPath.Valid {
			se.FullPath = fullPath.String
		}
		sessions = append(sessions, se)
	}

	return sessions, nil
}

// SearchInSession searches within a specific session's messages.
func (s *Store) SearchInSession(sessionID string, query string) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if query == "" {
		return nil, nil
	}

	ftsQ := ftsQuery(query)

	rows, err := s.db.Query(`
		SELECT m.id, m.session_id, '' as project, m.type, m.timestamp,
			m.text,
			highlight(messages_fts, 0, '<<', '>>'),
			'' as first_prompt, '' as git_branch, m.model,
			rank
		FROM messages m
		JOIN messages_fts ON messages_fts.rowid = m.id
		WHERE m.session_id = ? AND messages_fts MATCH ?
		ORDER BY m.timestamp ASC
	`, sessionID, ftsQ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(
			&r.MessageID, &r.SessionID, &r.Project, &r.MessageType,
			&r.Timestamp, &r.Text, &r.Highlighted,
			&r.FirstPrompt, &r.GitBranch, &r.Model, &r.Rank,
		); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// SessionsByProject returns all indexed sessions for a project, matching the
// claude.SessionEntry format used by the existing UI.
func (s *Store) SessionsByProject(project string) ([]claude.SessionEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT s.session_id, COALESCE(f.path, ''), s.first_prompt, s.message_count,
			s.created_at, s.modified_at, s.git_branch, s.project
		FROM sessions s
		LEFT JOIN files f ON f.id = s.file_id
		WHERE s.project = ?
		ORDER BY s.modified_at DESC
	`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []claude.SessionEntry
	for rows.Next() {
		var se claude.SessionEntry
		var fullPath sql.NullString
		if err := rows.Scan(
			&se.SessionID, &fullPath, &se.FirstPrompt, &se.MessageCount,
			&se.Created, &se.Modified, &se.GitBranch, &se.ProjectPath,
		); err != nil {
			continue
		}
		if fullPath.Valid {
			se.FullPath = fullPath.String
		}
		sessions = append(sessions, se)
	}

	return sessions, nil
}

// SessionsByIDs returns sessions matching any of the given session IDs.
func (s *Store) SessionsByIDs(sessionIDs []string) ([]claude.SessionEntry, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	placeholders := make([]string, len(sessionIDs))
	args := make([]interface{}, len(sessionIDs))
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT s.session_id, COALESCE(f.path, ''), s.first_prompt, s.message_count,
			s.created_at, s.modified_at, s.git_branch, s.project
		FROM sessions s
		LEFT JOIN files f ON f.id = s.file_id
		WHERE s.session_id IN (%s)
		ORDER BY s.modified_at DESC
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []claude.SessionEntry
	for rows.Next() {
		var se claude.SessionEntry
		var fullPath sql.NullString
		if err := rows.Scan(
			&se.SessionID, &fullPath, &se.FirstPrompt, &se.MessageCount,
			&se.Created, &se.Modified, &se.GitBranch, &se.ProjectPath,
		); err != nil {
			continue
		}
		if fullPath.Valid {
			se.FullPath = fullPath.String
		}
		sessions = append(sessions, se)
	}

	return sessions, nil
}

// MatchCount returns the number of FTS matches for a query (for result count display).
func (s *Store) MatchCount(query string) int {
	fs := Parse(query)
	if !fs.HasFTS() {
		return 0
	}

	where, params := fs.ToSQL()
	sqlStr := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM messages m
		JOIN messages_fts ON messages_fts.rowid = m.id
		JOIN sessions s ON s.session_id = m.session_id
		WHERE %s
	`, where)

	var count int
	s.db.QueryRow(sqlStr, params...).Scan(&count)
	return count
}

// FormatHighlight converts <<matched>> markers to styled text.
func FormatHighlight(text string, startMark, endMark string) string {
	text = strings.ReplaceAll(text, "<<", startMark)
	text = strings.ReplaceAll(text, ">>", endMark)
	return text
}
