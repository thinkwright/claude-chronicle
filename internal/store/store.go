package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"

	_ "modernc.org/sqlite"
)

type Store struct {
	db       *sql.DB
	mu       sync.RWMutex
	reCache  map[string]*regexp.Regexp
	reCacheMu sync.RWMutex
}

func dataDir() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "clog")
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "clog")
	}
	return filepath.Join(home, ".local", "share", "clog")
}

func DBPath() string {
	return filepath.Join(dataDir(), "index.db")
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Enable WAL for concurrent reads during writes
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable FK: %w", err)
	}

	s := &Store{db: db, reCache: make(map[string]*regexp.Regexp)}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

// compiledRegex returns a cached *regexp.Regexp for the given pattern.
func (s *Store) compiledRegex(pattern string) *regexp.Regexp {
	s.reCacheMu.RLock()
	if re, ok := s.reCache[pattern]; ok {
		s.reCacheMu.RUnlock()
		return re
	}
	s.reCacheMu.RUnlock()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	s.reCacheMu.Lock()
	s.reCache[pattern] = re
	s.reCacheMu.Unlock()
	return re
}

func (s *Store) migrate() error {
	// Check schema version
	var version int
	row := s.db.QueryRow("PRAGMA user_version")
	row.Scan(&version)

	if version == 0 {
		return s.createSchema()
	}
	return nil
}

func (s *Store) createSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS files (
    id          INTEGER PRIMARY KEY,
    path        TEXT    UNIQUE NOT NULL,
    project     TEXT    NOT NULL,
    session_id  TEXT    NOT NULL,
    mtime       INTEGER NOT NULL,
    size        INTEGER NOT NULL,
    indexed_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id           INTEGER PRIMARY KEY,
    session_id   TEXT    UNIQUE NOT NULL,
    file_id      INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    project      TEXT    NOT NULL,
    first_prompt TEXT    DEFAULT '',
    git_branch   TEXT    DEFAULT '',
    model        TEXT    DEFAULT '',
    created_at   TEXT    DEFAULT '',
    modified_at  TEXT    DEFAULT '',
    message_count       INTEGER DEFAULT 0,
    total_input_tokens  INTEGER DEFAULT 0,
    total_output_tokens INTEGER DEFAULT 0,
    tool_count          INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project);
CREATE INDEX IF NOT EXISTS idx_sessions_branch ON sessions(git_branch);
CREATE INDEX IF NOT EXISTS idx_sessions_model ON sessions(model);
CREATE INDEX IF NOT EXISTS idx_sessions_modified ON sessions(modified_at);

CREATE TABLE IF NOT EXISTS messages (
    id            INTEGER PRIMARY KEY,
    session_id    TEXT    NOT NULL,
    type          TEXT    NOT NULL,
    timestamp     TEXT    NOT NULL,
    model         TEXT    DEFAULT '',
    text          TEXT    DEFAULT '',
    tool_calls    TEXT    DEFAULT '',
    input_tokens  INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_type ON messages(type);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    text, tool_calls,
    content=messages, content_rowid=id,
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, text, tool_calls) VALUES (new.id, new.text, new.tool_calls);
END;

CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text, tool_calls) VALUES ('delete', old.id, old.text, old.tool_calls);
END;

CREATE TABLE IF NOT EXISTS watchlist (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    pattern    TEXT    NOT NULL,
    enabled    BOOLEAN DEFAULT 1,
    color      TEXT    DEFAULT '#b56a6a',
    created_at TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS watchlist_matches (
    id           INTEGER PRIMARY KEY,
    watchlist_id INTEGER NOT NULL REFERENCES watchlist(id) ON DELETE CASCADE,
    message_id   INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    session_id   TEXT    NOT NULL,
    matched_text TEXT    DEFAULT '',
    seen         BOOLEAN DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_wm_watchlist ON watchlist_matches(watchlist_id);
CREATE INDEX IF NOT EXISTS idx_wm_session ON watchlist_matches(session_id);

CREATE TABLE IF NOT EXISTS saved_filters (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL,
    filter_json TEXT    NOT NULL,
    created_at  TEXT    NOT NULL
);

PRAGMA user_version = 1;
`
	_, err := s.db.Exec(schema)
	return err
}

// Reset drops all data and recreates the schema. Used by --reindex.
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tables := []string{
		"watchlist_matches", "watchlist", "saved_filters",
		"messages", "sessions", "files",
	}
	for _, t := range tables {
		if _, err := s.db.Exec("DELETE FROM " + t); err != nil {
			return err
		}
	}
	// Rebuild FTS index
	_, err := s.db.Exec("INSERT INTO messages_fts(messages_fts) VALUES('rebuild')")
	return err
}
