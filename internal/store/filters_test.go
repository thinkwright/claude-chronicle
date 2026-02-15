package store

import (
	"testing"
	"time"
)

func TestTokenize_SimpleWords(t *testing.T) {
	got := tokenize("deploy production")
	if len(got) != 2 || got[0] != "deploy" || got[1] != "production" {
		t.Errorf("tokenize simple = %v", got)
	}
}

func TestTokenize_QuotedPhrase(t *testing.T) {
	got := tokenize(`"deploy to production" quickly`)
	if len(got) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(got), got)
	}
	if got[0] != `"deploy to production"` {
		t.Errorf("token[0] = %q", got[0])
	}
	if got[1] != "quickly" {
		t.Errorf("token[1] = %q", got[1])
	}
}

func TestTokenize_ExtraSpaces(t *testing.T) {
	got := tokenize("  hello   world  ")
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Errorf("tokenize spaces = %v", got)
	}
}

func TestTokenize_Empty(t *testing.T) {
	got := tokenize("")
	if len(got) != 0 {
		t.Errorf("tokenize empty = %v", got)
	}
}

func TestTokenize_OnlyFilter(t *testing.T) {
	got := tokenize("model:opus")
	if len(got) != 1 || got[0] != "model:opus" {
		t.Errorf("tokenize filter = %v", got)
	}
}

func TestParseFilter_Model(t *testing.T) {
	f, ok := parseFilter("model:opus")
	if !ok {
		t.Fatal("expected ok")
	}
	if f.Field != FilterModel || f.Op != OpEquals || f.Value != "opus" {
		t.Errorf("filter = %+v", f)
	}
}

func TestParseFilter_Branch(t *testing.T) {
	f, ok := parseFilter("branch:main")
	if !ok {
		t.Fatal("expected ok")
	}
	if f.Field != FilterBranch || f.Value != "main" {
		t.Errorf("filter = %+v", f)
	}
}

func TestParseFilter_BranchGlob(t *testing.T) {
	f, ok := parseFilter("branch:feature*")
	if !ok {
		t.Fatal("expected ok")
	}
	if f.Field != FilterBranch || f.Value != "feature*" {
		t.Errorf("filter = %+v", f)
	}
}

func TestParseFilter_TokensGreater(t *testing.T) {
	f, ok := parseFilter("tokens:>10000")
	if !ok {
		t.Fatal("expected ok")
	}
	if f.Field != FilterTokens || f.Op != OpGreaterThan || f.Value != "10000" {
		t.Errorf("filter = %+v", f)
	}
}

func TestParseFilter_TokensLess(t *testing.T) {
	f, ok := parseFilter("tokens:<5000")
	if !ok {
		t.Fatal("expected ok")
	}
	if f.Field != FilterTokens || f.Op != OpLessThan || f.Value != "5000" {
		t.Errorf("filter = %+v", f)
	}
}

func TestParseFilter_AgeLessThan(t *testing.T) {
	f, ok := parseFilter("age:<1h")
	if !ok {
		t.Fatal("expected ok")
	}
	if f.Field != FilterAge || f.Op != OpLessThan || f.Value != "1h" {
		t.Errorf("filter = %+v", f)
	}
}

func TestParseFilter_InvalidNoColon(t *testing.T) {
	_, ok := parseFilter("justtext")
	if ok {
		t.Error("expected !ok for no colon")
	}
}

func TestParseFilter_InvalidEmptyValue(t *testing.T) {
	_, ok := parseFilter("model:")
	if ok {
		t.Error("expected !ok for empty value")
	}
}

func TestParseFilter_UnknownField(t *testing.T) {
	_, ok := parseFilter("unknown:value")
	if ok {
		t.Error("expected !ok for unknown field")
	}
}

func TestParse_FreeTextOnly(t *testing.T) {
	fs := Parse("deploy production")
	if fs.FreeText != "deploy production" {
		t.Errorf("freetext = %q", fs.FreeText)
	}
	if len(fs.Filters) != 0 {
		t.Errorf("filters = %v", fs.Filters)
	}
}

func TestParse_FiltersOnly(t *testing.T) {
	fs := Parse("model:opus branch:main")
	if fs.FreeText != "" {
		t.Errorf("freetext = %q, want empty", fs.FreeText)
	}
	if len(fs.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(fs.Filters))
	}
	if fs.Filters[0].Field != FilterModel || fs.Filters[0].Value != "opus" {
		t.Errorf("filter[0] = %+v", fs.Filters[0])
	}
	if fs.Filters[1].Field != FilterBranch || fs.Filters[1].Value != "main" {
		t.Errorf("filter[1] = %+v", fs.Filters[1])
	}
}

func TestParse_Combined(t *testing.T) {
	fs := Parse("deploy model:opus tokens:>10000")
	if fs.FreeText != "deploy" {
		t.Errorf("freetext = %q", fs.FreeText)
	}
	if len(fs.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(fs.Filters))
	}
}

func TestParse_Empty(t *testing.T) {
	fs := Parse("")
	if !fs.IsEmpty() {
		t.Errorf("expected empty filter set")
	}
}

func TestFilterSet_HasFTS(t *testing.T) {
	fs := Parse("deploy model:opus")
	if !fs.HasFTS() {
		t.Error("expected HasFTS true")
	}
	fs2 := Parse("model:opus")
	if fs2.HasFTS() {
		t.Error("expected HasFTS false")
	}
}

func TestFilterSet_IsEmpty(t *testing.T) {
	if !Parse("").IsEmpty() {
		t.Error("empty string should be empty")
	}
	if Parse("hello").IsEmpty() {
		t.Error("should not be empty")
	}
	if Parse("model:opus").IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestToSQL_FreeTextOnly(t *testing.T) {
	fs := Parse("deploy")
	where, params := fs.ToSQL()
	if where != "messages_fts MATCH ?" {
		t.Errorf("where = %q", where)
	}
	if len(params) != 1 || params[0] != "deploy" {
		t.Errorf("params = %v", params)
	}
}

func TestToSQL_ModelFilter(t *testing.T) {
	fs := Parse("model:opus")
	where, params := fs.ToSQL()
	if where != "s.model LIKE ?" {
		t.Errorf("where = %q", where)
	}
	if len(params) != 1 || params[0] != "%opus%" {
		t.Errorf("params = %v", params)
	}
}

func TestToSQL_BranchExact(t *testing.T) {
	fs := Parse("branch:main")
	where, params := fs.ToSQL()
	if where != "s.git_branch = ?" {
		t.Errorf("where = %q", where)
	}
	if len(params) != 1 || params[0] != "main" {
		t.Errorf("params = %v", params)
	}
}

func TestToSQL_BranchGlob(t *testing.T) {
	fs := Parse("branch:feature*")
	where, params := fs.ToSQL()
	if where != "s.git_branch LIKE ?" {
		t.Errorf("where = %q", where)
	}
	if len(params) != 1 || params[0] != "feature%" {
		t.Errorf("params = %v", params)
	}
}

func TestToSQL_TokensGreater(t *testing.T) {
	fs := Parse("tokens:>10000")
	where, params := fs.ToSQL()
	if where != "(m.input_tokens + m.output_tokens) > ?" {
		t.Errorf("where = %q", where)
	}
	if len(params) != 1 || params[0] != 10000 {
		t.Errorf("params = %v", params)
	}
}

func TestToSQL_TypeFilter(t *testing.T) {
	fs := Parse("type:user")
	where, params := fs.ToSQL()
	if where != "m.type = ?" {
		t.Errorf("where = %q", where)
	}
	if len(params) != 1 || params[0] != "user" {
		t.Errorf("params = %v", params)
	}
}

func TestToSQL_ToolFilter(t *testing.T) {
	fs := Parse("tool:Bash")
	where, params := fs.ToSQL()
	if where != "m.tool_calls LIKE ?" {
		t.Errorf("where = %q", where)
	}
	if len(params) != 1 || params[0] != "%Bash%" {
		t.Errorf("params = %v", params)
	}
}

func TestToSQL_Empty(t *testing.T) {
	fs := Parse("")
	where, params := fs.ToSQL()
	if where != "1=1" {
		t.Errorf("where = %q, want 1=1", where)
	}
	if params != nil {
		t.Errorf("params = %v, want nil", params)
	}
}

func TestToSQL_Combined(t *testing.T) {
	fs := Parse("deploy model:opus")
	where, params := fs.ToSQL()
	if where != "messages_fts MATCH ? AND s.model LIKE ?" {
		t.Errorf("where = %q", where)
	}
	if len(params) != 2 {
		t.Fatalf("params len = %d, want 2", len(params))
	}
}

func TestParseAge(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		err   bool
	}{
		{"30m", 30 * time.Minute, false},
		{"1h", 1 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},
		{"x", 0, true},         // too short
		{"abc", 0, true},       // invalid number
		{"10x", 0, true},       // unknown unit
	}
	for _, tt := range tests {
		got, err := parseAge(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("parseAge(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAge(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseAge(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFtsQuery_Plain(t *testing.T) {
	if got := ftsQuery("deploy production"); got != "deploy production" {
		t.Errorf("ftsQuery plain = %q", got)
	}
}

func TestFtsQuery_OR(t *testing.T) {
	got := ftsQuery("error|panic")
	if got != "error OR panic" {
		t.Errorf("ftsQuery OR = %q", got)
	}
}

func TestFtsQuery_ORWithSpaces(t *testing.T) {
	got := ftsQuery("error | panic | crash")
	if got != "error OR panic OR crash" {
		t.Errorf("ftsQuery OR = %q", got)
	}
}
