package claude

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestJSONL creates a temp JSONL file and returns its path.
func writeTestJSONL(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")
	var content string
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMessages_UserMessage(t *testing.T) {
	path := writeTestJSONL(t,
		`{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"hello world"}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	m := msgs[0]
	if m.Type != TypeUser {
		t.Errorf("type = %q, want %q", m.Type, TypeUser)
	}
	if m.Text != "hello world" {
		t.Errorf("text = %q, want %q", m.Text, "hello world")
	}
	if m.UUID != "u1" {
		t.Errorf("uuid = %q, want %q", m.UUID, "u1")
	}
	if m.Timestamp != "2025-01-01T00:00:00Z" {
		t.Errorf("timestamp = %q", m.Timestamp)
	}
}

func TestLoadMessages_UserMessageContentBlocks(t *testing.T) {
	path := writeTestJSONL(t,
		`{"type":"user","uuid":"u2","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":[{"type":"text","text":"block one"},{"type":"text","text":"block two"}]}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Text != "block one\nblock two" {
		t.Errorf("text = %q, want %q", msgs[0].Text, "block one\nblock two")
	}
}

func TestLoadMessages_AssistantMessage(t *testing.T) {
	path := writeTestJSONL(t,
		`{"type":"assistant","uuid":"a1","timestamp":"2025-01-01T00:00:01Z","message":{"role":"assistant","model":"claude-3-5-sonnet-20241022","content":[{"type":"text","text":"I can help with that."},{"type":"tool_use","name":"Read","input":{"path":"foo.go"}}],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":20,"cache_creation_input_tokens":10}}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	m := msgs[0]
	if m.Type != TypeAssistant {
		t.Errorf("type = %q, want %q", m.Type, TypeAssistant)
	}
	if m.Text != "I can help with that." {
		t.Errorf("text = %q", m.Text)
	}
	if len(m.ToolCalls) != 1 || m.ToolCalls[0] != "Read" {
		t.Errorf("tool_calls = %v, want [Read]", m.ToolCalls)
	}
	if m.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("model = %q", m.Model)
	}
	// Token aggregation: 100 + 20 + 10 = 130 input
	if m.InputTokens != 130 {
		t.Errorf("input_tokens = %d, want 130", m.InputTokens)
	}
	if m.OutputTokens != 50 {
		t.Errorf("output_tokens = %d, want 50", m.OutputTokens)
	}
}

func TestLoadMessages_AssistantMultipleToolCalls(t *testing.T) {
	path := writeTestJSONL(t,
		`{"type":"assistant","uuid":"a2","timestamp":"2025-01-01T00:00:01Z","message":{"role":"assistant","model":"opus","content":[{"type":"tool_use","name":"Read","input":{}},{"type":"text","text":"middle"},{"type":"tool_use","name":"Write","input":{}}]}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	m := msgs[0]
	if len(m.ToolCalls) != 2 {
		t.Fatalf("tool_calls len = %d, want 2", len(m.ToolCalls))
	}
	if m.ToolCalls[0] != "Read" || m.ToolCalls[1] != "Write" {
		t.Errorf("tool_calls = %v", m.ToolCalls)
	}
	if m.Text != "middle" {
		t.Errorf("text = %q, want %q", m.Text, "middle")
	}
}

func TestLoadMessages_ToolResultStringContent(t *testing.T) {
	path := writeTestJSONL(t,
		`{"type":"tool-result","uuid":"t1","timestamp":"2025-01-01T00:00:02Z","message":{"role":"user","content":[{"type":"tool_result","content":"file contents here"}]}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != TypeToolResult {
		t.Errorf("type = %q", msgs[0].Type)
	}
	if msgs[0].Text != "file contents here" {
		t.Errorf("text = %q", msgs[0].Text)
	}
}

func TestLoadMessages_ToolResultNestedContent(t *testing.T) {
	path := writeTestJSONL(t,
		`{"type":"tool-result","uuid":"t2","timestamp":"2025-01-01T00:00:02Z","message":{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"nested output"}]}]}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Text != "nested output" {
		t.Errorf("text = %q, want %q", msgs[0].Text, "nested output")
	}
}

func TestLoadMessages_ToolResultTruncated(t *testing.T) {
	long := string(make([]byte, 300)) // 300 null bytes won't be useful but tests truncation
	// Use a real long string
	longStr := ""
	for i := 0; i < 250; i++ {
		longStr += "x"
	}
	_ = long
	path := writeTestJSONL(t,
		`{"type":"tool-result","uuid":"t3","timestamp":"2025-01-01T00:00:02Z","message":{"role":"user","content":[{"type":"tool_result","content":"`+longStr+`"}]}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if len(msgs[0].Text) > 204 { // 200 + "..."
		t.Errorf("text not truncated, len = %d", len(msgs[0].Text))
	}
}

func TestLoadMessages_SkipsUnknownTypes(t *testing.T) {
	path := writeTestJSONL(t,
		`{"type":"progress","uuid":"p1","timestamp":"2025-01-01T00:00:00Z"}`,
		`{"type":"queue-operation","uuid":"q1","timestamp":"2025-01-01T00:00:00Z"}`,
		`{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"real message"}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	// progress/queue-operation return nil from parseMessage, only user msg kept
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (skipping unknown types), got %d", len(msgs))
	}
	if msgs[0].Text != "real message" {
		t.Errorf("text = %q", msgs[0].Text)
	}
}

func TestLoadMessages_SkipsEmptyLines(t *testing.T) {
	path := writeTestJSONL(t,
		``,
		`{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"hello"}}`,
		``,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1, got %d", len(msgs))
	}
}

func TestLoadMessages_SkipsMalformedJSON(t *testing.T) {
	path := writeTestJSONL(t,
		`not json at all`,
		`{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"valid"}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1, got %d", len(msgs))
	}
}

func TestLoadMessages_EmptyUserContentSkipped(t *testing.T) {
	path := writeTestJSONL(t,
		`{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":""}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 (empty user text skipped), got %d", len(msgs))
	}
}

func TestLoadMessages_MixedConversation(t *testing.T) {
	path := writeTestJSONL(t,
		`{"type":"user","uuid":"u1","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"fix the bug"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2025-01-01T00:00:01Z","message":{"role":"assistant","model":"sonnet","content":[{"type":"text","text":"Looking at the code..."}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2025-01-01T00:00:02Z","message":{"role":"assistant","model":"sonnet","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"tool-result","uuid":"t1","timestamp":"2025-01-01T00:00:03Z","message":{"role":"user","content":[{"type":"tool_result","content":"main.go\ngo.mod"}]}}`,
		`{"type":"assistant","uuid":"a3","timestamp":"2025-01-01T00:00:04Z","message":{"role":"assistant","model":"sonnet","content":[{"type":"text","text":"I found the files."}]}}`,
	)
	msgs, err := LoadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}

	types := make([]MessageType, len(msgs))
	for i, m := range msgs {
		types[i] = m.Type
	}
	expected := []MessageType{TypeUser, TypeAssistant, TypeAssistant, TypeToolResult, TypeAssistant}
	for i, e := range expected {
		if types[i] != e {
			t.Errorf("msg[%d].Type = %q, want %q", i, types[i], e)
		}
	}
}

func TestLoadMessages_MissingFile(t *testing.T) {
	_, err := LoadMessages("/nonexistent/path/file.jsonl")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFormatModel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude-3-opus-20240229", "opus"},
		{"claude-3-5-sonnet-20241022", "sonnet"},
		{"claude-3-haiku-20240307", "haiku"},
		{"claude-opus-4-20250514", "opus"},
		{"gpt-4", "gpt-4"},
		{"", ""},
	}
	for _, tt := range tests {
		got := FormatModel(tt.input)
		if got != tt.want {
			t.Errorf("FormatModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{10000, "10.0K"},
		{999999, "1000.0K"},
		{1000000, "1.0M"},
		{2500000, "2.5M"},
	}
	for _, tt := range tests {
		got := FormatTokens(tt.input)
		if got != tt.want {
			t.Errorf("FormatTokens(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 200); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	long := "abcdefghij"
	if got := truncate(long, 5); got != "abcde..." {
		t.Errorf("truncate long = %q, want %q", got, "abcde...")
	}
	if got := truncate("exact", 5); got != "exact" {
		t.Errorf("truncate exact = %q", got)
	}
}
