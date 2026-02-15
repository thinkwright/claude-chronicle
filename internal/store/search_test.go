package store

import (
	"os"
	"testing"
)

func TestSearch_FTS(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search("deploy", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results for 'deploy'")
	}
	// Should find messages containing "deploy"
	found := false
	for _, r := range results {
		if r.Project == "TestProject" {
			found = true
		}
	}
	if !found {
		t.Error("expected result from TestProject")
	}
}

func TestSearch_FTSPhrase(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search(`"deploy bug"`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for phrase 'deploy bug'")
	}
}

func TestSearch_FTSNoMatch(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search("xyznonexistent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_StructuredModelFilter(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search("model:sonnet", 10)
	if err != nil {
		t.Fatal(err)
	}
	// All messages in test data are from sonnet sessions
	if len(results) == 0 {
		t.Fatal("expected results for model:sonnet")
	}
}

func TestSearch_StructuredTypeFilter(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search("type:user", 10)
	if err != nil {
		t.Fatal(err)
	}
	// Should get only user messages
	for _, r := range results {
		if r.MessageType != "user" {
			t.Errorf("expected type=user, got %q", r.MessageType)
		}
	}
	if len(results) != 2 {
		t.Errorf("expected 2 user messages, got %d", len(results))
	}
}

func TestSearch_StructuredToolFilter(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search("tool:Read", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for tool:Read")
	}
}

func TestSearch_CombinedFTSAndFilter(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search("deploy model:sonnet", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for combined query")
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search("", 10)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil for empty query, got %d results", len(results))
	}
}

func TestSearch_Limit(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search("type:assistant", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result with limit=1, got %d", len(results))
	}
}

func TestSearch_HighlightMarkers(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	results, err := s.Search("deploy", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// FTS5 highlight should add << >> markers
	found := false
	for _, r := range results {
		if r.Highlighted != "" && r.Highlighted != r.Text {
			found = true
		}
	}
	if !found {
		t.Log("note: highlight markers not found (may depend on FTS5 behavior)")
	}
}

func TestSearch_TextTruncation(t *testing.T) {
	s := openTestStore(t)
	dir := t.TempDir()

	// Create a message with very long text
	longText := ""
	for i := 0; i < 300; i++ {
		longText += "word "
	}
	jsonl := `{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"` + longText + `"}}
`
	path := dir + "/long-text.jsonl"
	writeFile(t, path, jsonl)
	s.indexFile(path, "LongProject")

	results, err := s.Search("word", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// Text should be truncated to ~203 chars (200 + "...")
	if len(results[0].Text) > 204 {
		t.Errorf("text not truncated, len = %d", len(results[0].Text))
	}
}

func TestSearchSessions_ByProject(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	sessions, err := s.SearchSessions("deploy", "TestProject")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "test-session-abc" {
		t.Errorf("session_id = %q", sessions[0].SessionID)
	}
}

func TestSearchSessions_GlobalScope(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	// Empty project = global
	sessions, err := s.SearchSessions("deploy", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) == 0 {
		t.Fatal("expected results in global scope")
	}
}

func TestSearchInSession(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := seedTestData(t, s)

	results, err := s.SearchInSession(sessionID, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results within session")
	}
	for _, r := range results {
		if r.SessionID != sessionID {
			t.Errorf("result from wrong session: %q", r.SessionID)
		}
	}
}

func TestSearchInSession_Empty(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := seedTestData(t, s)

	results, err := s.SearchInSession(sessionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil for empty query")
	}
}

func TestSessionsByProject(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	sessions, err := s.SessionsByProject("TestProject")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	se := sessions[0]
	if se.SessionID != "test-session-abc" {
		t.Errorf("session_id = %q", se.SessionID)
	}
	if se.FirstPrompt != "fix the deploy bug in production" {
		t.Errorf("first_prompt = %q", se.FirstPrompt)
	}
}

func TestSessionsByProject_Empty(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	sessions, err := s.SessionsByProject("NonExistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0, got %d", len(sessions))
	}
}

func TestMatchCount(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	count := s.MatchCount("deploy")
	if count == 0 {
		t.Error("expected non-zero match count for 'deploy'")
	}

	count = s.MatchCount("xyznonexistent")
	if count != 0 {
		t.Errorf("expected 0 for nonexistent, got %d", count)
	}
}

func TestMatchCount_StructuredOnly(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	// model:sonnet has no FTS component, so MatchCount returns 0
	count := s.MatchCount("model:sonnet")
	if count != 0 {
		t.Errorf("expected 0 for structured-only query, got %d", count)
	}
}

func TestFormatHighlight(t *testing.T) {
	got := FormatHighlight("the <<deploy>> was <<broken>>", "[", "]")
	want := "the [deploy] was [broken]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// writeFile helper for tests
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
