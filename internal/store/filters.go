package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type FilterField int

const (
	FilterModel FilterField = iota
	FilterBranch
	FilterProject
	FilterType
	FilterTool
	FilterTokens
	FilterAge
)

type FilterOp int

const (
	OpEquals FilterOp = iota
	OpGreaterThan
	OpLessThan
	OpLike
)

type Filter struct {
	Field FilterField
	Op    FilterOp
	Value string
}

type FilterSet struct {
	FreeText string
	Filters  []Filter
}

// Parse parses a query string into free-text search terms and structured filters.
// Examples:
//
//	"deploy production" → FreeText: "deploy production"
//	"model:opus branch:main" → Filters: [{FilterModel, OpEquals, "opus"}, ...]
//	"deploy model:opus tokens:>10000" → FreeText: "deploy", Filters: [...]
//	"age:<1h" → Filters: [{FilterAge, OpLessThan, "1h"}]
func Parse(query string) *FilterSet {
	fs := &FilterSet{}
	var freeWords []string

	tokens := tokenize(query)
	for _, tok := range tokens {
		if f, ok := parseFilter(tok); ok {
			fs.Filters = append(fs.Filters, f)
		} else {
			freeWords = append(freeWords, tok)
		}
	}

	fs.FreeText = strings.Join(freeWords, " ")
	return fs
}

// tokenize splits a query string respecting quoted phrases.
func tokenize(query string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false

	for _, r := range query {
		switch {
		case r == '"':
			inQuote = !inQuote
			current.WriteRune(r)
		case unicode.IsSpace(r) && !inQuote:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// parseFilter attempts to parse a token as field:value or field:>value.
func parseFilter(token string) (Filter, bool) {
	idx := strings.Index(token, ":")
	if idx < 1 || idx == len(token)-1 {
		return Filter{}, false
	}

	field := strings.ToLower(token[:idx])
	value := token[idx+1:]

	var f Filter

	switch field {
	case "model":
		f.Field = FilterModel
	case "branch":
		f.Field = FilterBranch
	case "project":
		f.Field = FilterProject
	case "type":
		f.Field = FilterType
	case "tool":
		f.Field = FilterTool
	case "tokens":
		f.Field = FilterTokens
	case "age":
		f.Field = FilterAge
	default:
		return Filter{}, false
	}

	// Parse operator prefix
	if strings.HasPrefix(value, ">") {
		f.Op = OpGreaterThan
		f.Value = value[1:]
	} else if strings.HasPrefix(value, "<") {
		f.Op = OpLessThan
		f.Value = value[1:]
	} else {
		f.Op = OpEquals
		f.Value = value
	}

	return f, true
}

// ToSQL generates a SQL WHERE clause and parameters from the filter set.
// The query joins messages (m), sessions (s), and optionally messages_fts.
// Returns the WHERE clause (without "WHERE") and the parameter list.
func (fs *FilterSet) ToSQL() (string, []interface{}) {
	var conditions []string
	var params []interface{}

	// FTS5 match
	if fs.FreeText != "" {
		conditions = append(conditions, "messages_fts MATCH ?")
		params = append(params, ftsQuery(fs.FreeText))
	}

	for _, f := range fs.Filters {
		cond, p := filterToSQL(f)
		if cond != "" {
			conditions = append(conditions, cond)
			params = append(params, p...)
		}
	}

	if len(conditions) == 0 {
		return "1=1", nil
	}
	return strings.Join(conditions, " AND "), params
}

// HasFTS returns true if this filter set includes a free-text search.
func (fs *FilterSet) HasFTS() bool {
	return fs.FreeText != ""
}

// IsEmpty returns true if there are no filters or free text.
func (fs *FilterSet) IsEmpty() bool {
	return fs.FreeText == "" && len(fs.Filters) == 0
}

func filterToSQL(f Filter) (string, []interface{}) {
	switch f.Field {
	case FilterModel:
		return "s.model LIKE ?", []interface{}{"%" + f.Value + "%"}

	case FilterBranch:
		if strings.Contains(f.Value, "*") {
			// Convert glob to SQL LIKE
			pattern := strings.ReplaceAll(f.Value, "*", "%")
			return "s.git_branch LIKE ?", []interface{}{pattern}
		}
		return "s.git_branch = ?", []interface{}{f.Value}

	case FilterProject:
		return "s.project LIKE ?", []interface{}{"%" + f.Value + "%"}

	case FilterType:
		return "m.type = ?", []interface{}{f.Value}

	case FilterTool:
		return "m.tool_calls LIKE ?", []interface{}{"%" + f.Value + "%"}

	case FilterTokens:
		n, err := strconv.Atoi(f.Value)
		if err != nil {
			return "", nil
		}
		col := "(m.input_tokens + m.output_tokens)"
		switch f.Op {
		case OpGreaterThan:
			return fmt.Sprintf("%s > ?", col), []interface{}{n}
		case OpLessThan:
			return fmt.Sprintf("%s < ?", col), []interface{}{n}
		default:
			return fmt.Sprintf("%s = ?", col), []interface{}{n}
		}

	case FilterAge:
		dur, err := parseAge(f.Value)
		if err != nil {
			return "", nil
		}
		cutoff := time.Now().Add(-dur).Format(time.RFC3339)
		switch f.Op {
		case OpLessThan:
			// age:<1h means modified within the last hour
			return "s.modified_at > ?", []interface{}{cutoff}
		case OpGreaterThan:
			// age:>1h means modified more than 1 hour ago
			return "s.modified_at < ?", []interface{}{cutoff}
		default:
			return "s.modified_at > ?", []interface{}{cutoff}
		}
	}

	return "", nil
}

// parseAge parses a duration like "1h", "30m", "7d", "2w".
func parseAge(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid age: %s", s)
	}

	unit := s[len(s)-1]
	num, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, err
	}

	switch unit {
	case 'm':
		return time.Duration(num) * time.Minute, nil
	case 'h':
		return time.Duration(num) * time.Hour, nil
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(num) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit: %c", unit)
	}
}

// ftsQuery converts user input into an FTS5 query string.
// Handles OR with |, quoted phrases, and plain terms.
func ftsQuery(input string) string {
	// FTS5 already handles quoted phrases and basic operators
	// We just need to handle the | separator as OR
	if strings.Contains(input, "|") {
		parts := strings.Split(input, "|")
		var quoted []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				quoted = append(quoted, p)
			}
		}
		return strings.Join(quoted, " OR ")
	}
	return input
}
