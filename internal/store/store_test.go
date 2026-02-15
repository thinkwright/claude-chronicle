package store

import (
	"os"
	"path/filepath"
	"testing"
)

// openTestStore creates a real SQLite store in a temp directory.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedTestData inserts a JSONL file worth of data into the store.
// Returns the session ID and inserted message IDs.
func seedTestData(t *testing.T, s *Store) (string, []int64) {
	t.Helper()
	dir := t.TempDir()

	// Write a realistic JSONL file
	jsonl := `{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"fix the deploy bug in production"}}
{"type":"assistant","uuid":"a1","timestamp":"2025-01-01T00:00:01Z","message":{"role":"assistant","model":"claude-3-5-sonnet-20241022","content":[{"type":"text","text":"I'll look at the deployment configuration."},{"type":"tool_use","name":"Read","input":{"path":"deploy.yaml"}}],"usage":{"input_tokens":1000,"output_tokens":200}}}
{"type":"tool-result","uuid":"t1","timestamp":"2025-01-01T00:00:02Z","message":{"role":"user","content":[{"type":"tool_result","content":"apiVersion: apps/v1\nkind: Deployment"}]}}
{"type":"assistant","uuid":"a2","timestamp":"2025-01-01T00:00:03Z","message":{"role":"assistant","model":"claude-3-5-sonnet-20241022","content":[{"type":"text","text":"Found the issue. The replica count was set to 0. Let me fix it."},{"type":"tool_use","name":"Edit","input":{"path":"deploy.yaml"}}],"usage":{"input_tokens":2000,"output_tokens":500}}}
{"type":"user","uuid":"u2","timestamp":"2025-01-01T00:00:04Z","message":{"role":"user","content":"looks good, now run the tests"}}
{"type":"assistant","uuid":"a3","timestamp":"2025-01-01T00:00:05Z","message":{"role":"assistant","model":"claude-3-5-sonnet-20241022","content":[{"type":"tool_use","name":"Bash","input":{"command":"go test ./..."}}],"usage":{"input_tokens":1500,"output_tokens":100}}}
`
	path := filepath.Join(dir, "test-session-abc.jsonl")
	if err := os.WriteFile(path, []byte(jsonl), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.indexFile(path, "TestProject"); err != nil {
		t.Fatal(err)
	}

	// Get inserted message IDs
	rows, err := s.db.Query("SELECT id FROM messages WHERE session_id = ? ORDER BY id", "test-session-abc")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}

	return "test-session-abc", ids
}

func TestOpen_CreatesSchema(t *testing.T) {
	s := openTestStore(t)

	// Verify tables exist
	tables := []string{"files", "sessions", "messages", "watchlist", "watchlist_matches", "saved_filters"}
	for _, table := range tables {
		var name string
		err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	// Verify FTS5 virtual table
	var ftsName string
	err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='messages_fts'").Scan(&ftsName)
	if err != nil {
		t.Error("messages_fts virtual table not found")
	}
}

func TestOpen_WALMode(t *testing.T) {
	s := openTestStore(t)
	var mode string
	s.db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	s := openTestStore(t)
	var fk int
	s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestIndexFile_InsertsMessagesAndSession(t *testing.T) {
	s := openTestStore(t)
	sessionID, msgIDs := seedTestData(t, s)

	// Should have 6 messages (user, assistant, tool-result, assistant, user, assistant)
	if len(msgIDs) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(msgIDs))
	}

	// Verify session metadata
	var (
		project     string
		firstPrompt string
		model       string
		msgCount    int
		totalIn     int
		totalOut    int
		toolCount   int
	)
	err := s.db.QueryRow(`
		SELECT project, first_prompt, model, message_count,
			total_input_tokens, total_output_tokens, tool_count
		FROM sessions WHERE session_id = ?
	`, sessionID).Scan(&project, &firstPrompt, &model, &msgCount, &totalIn, &totalOut, &toolCount)
	if err != nil {
		t.Fatal(err)
	}

	if project != "TestProject" {
		t.Errorf("project = %q", project)
	}
	if firstPrompt != "fix the deploy bug in production" {
		t.Errorf("first_prompt = %q", firstPrompt)
	}
	if model != "sonnet" {
		t.Errorf("model = %q, want sonnet", model)
	}
	if msgCount != 6 {
		t.Errorf("message_count = %d, want 6", msgCount)
	}
	// 1000 + 2000 + 1500 = 4500
	if totalIn != 4500 {
		t.Errorf("total_input = %d, want 4500", totalIn)
	}
	// 200 + 500 + 100 = 800
	if totalOut != 800 {
		t.Errorf("total_output = %d, want 800", totalOut)
	}
	// Read, Edit, Bash = 3
	if toolCount != 3 {
		t.Errorf("tool_count = %d, want 3", toolCount)
	}
}

func TestIndexFile_FileRecord(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	if s.FileCount() != 1 {
		t.Errorf("file_count = %d, want 1", s.FileCount())
	}
	if s.MessageCount() != 6 {
		t.Errorf("message_count = %d, want 6", s.MessageCount())
	}
}

func TestIndexFile_ReindexReplacesData(t *testing.T) {
	s := openTestStore(t)
	dir := t.TempDir()

	jsonl := `{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"first version"}}
`
	path := filepath.Join(dir, "reindex-test.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)
	s.indexFile(path, "TestProject")

	if s.MessageCount() != 1 {
		t.Fatalf("initial message_count = %d, want 1", s.MessageCount())
	}

	// Update the file and re-index
	jsonl2 := `{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"updated version"}}
{"type":"assistant","uuid":"a1","timestamp":"2025-01-01T00:00:01Z","message":{"role":"assistant","model":"sonnet","content":[{"type":"text","text":"Got it."}]}}
`
	os.WriteFile(path, []byte(jsonl2), 0o644)
	s.indexFile(path, "TestProject")

	// Should have replaced, not duplicated
	if s.MessageCount() != 2 {
		t.Errorf("after reindex message_count = %d, want 2", s.MessageCount())
	}
	if s.FileCount() != 1 {
		t.Errorf("after reindex file_count = %d, want 1", s.FileCount())
	}

	// Verify content was updated
	var text string
	s.db.QueryRow("SELECT text FROM messages WHERE type = 'user'").Scan(&text)
	if text != "updated version" {
		t.Errorf("text = %q, want 'updated version'", text)
	}
}

func TestReset_ClearsAllData(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	if s.MessageCount() == 0 {
		t.Fatal("expected data before reset")
	}

	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}

	if s.FileCount() != 0 {
		t.Errorf("files after reset = %d", s.FileCount())
	}
	if s.MessageCount() != 0 {
		t.Errorf("messages after reset = %d", s.MessageCount())
	}
}

func TestCompiledRegex_Caching(t *testing.T) {
	s := openTestStore(t)

	re1 := s.compiledRegex("error|panic")
	if re1 == nil {
		t.Fatal("expected non-nil regex")
	}

	re2 := s.compiledRegex("error|panic")
	if re1 != re2 {
		t.Error("expected same cached regex pointer")
	}
}

func TestCompiledRegex_Invalid(t *testing.T) {
	s := openTestStore(t)
	re := s.compiledRegex("[invalid")
	if re != nil {
		t.Error("expected nil for invalid regex")
	}
}
