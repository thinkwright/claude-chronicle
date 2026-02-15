package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type MessageType string

const (
	TypeUser       MessageType = "user"
	TypeAssistant  MessageType = "assistant"
	TypeToolResult MessageType = "tool-result"
	TypeSystem     MessageType = "system"
)

type Message struct {
	Type      MessageType
	UUID      string
	Timestamp string
	Model     string
	Role      string

	// Parsed content
	Text      string   // plain text content
	ToolCalls []string // tool names used (assistant messages)
	ToolName  string   // for tool results, the originating tool

	// Token usage (assistant messages)
	InputTokens  int
	OutputTokens int
}

// rawMessage is used for initial JSON parsing to determine type.
type rawMessage struct {
	Type      string          `json:"type"`
	UUID      string          `json:"uuid"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

type messageContent struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *struct {
		InputTokens              int `json:"input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		OutputTokens             int `json:"output_tokens"`
	} `json:"usage"`
}

type contentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Name  string `json:"name"`
	Input json.RawMessage `json:"input"`
}

func LoadMessages(jsonlPath string) ([]Message, error) {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw rawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		msg := parseMessage(raw)
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	return messages, scanner.Err()
}

func parseMessage(raw rawMessage) *Message {
	switch MessageType(raw.Type) {
	case TypeUser:
		return parseUserMessage(raw)
	case TypeAssistant:
		return parseAssistantMessage(raw)
	case TypeToolResult:
		return parseToolResultMessage(raw)
	case TypeSystem:
		return parseSystemMessage(raw)
	default:
		return nil
	}
}

func parseUserMessage(raw rawMessage) *Message {
	var mc messageContent
	if err := json.Unmarshal(raw.Message, &mc); err != nil {
		return nil
	}

	text := extractText(mc.Content)
	if text == "" {
		return nil
	}

	return &Message{
		Type:      TypeUser,
		UUID:      raw.UUID,
		Timestamp: raw.Timestamp,
		Role:      "user",
		Text:      text,
	}
}

func parseAssistantMessage(raw rawMessage) *Message {
	var mc messageContent
	if err := json.Unmarshal(raw.Message, &mc); err != nil {
		return nil
	}

	var blocks []contentBlock
	if err := json.Unmarshal(mc.Content, &blocks); err != nil {
		return nil
	}

	var textParts []string
	var toolCalls []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				textParts = append(textParts, b.Text)
			}
		case "tool_use":
			toolCalls = append(toolCalls, b.Name)
		}
	}

	msg := &Message{
		Type:      TypeAssistant,
		UUID:      raw.UUID,
		Timestamp: raw.Timestamp,
		Model:     mc.Model,
		Role:      "assistant",
		Text:      strings.Join(textParts, "\n"),
		ToolCalls: toolCalls,
	}

	if mc.Usage != nil {
		msg.InputTokens = mc.Usage.InputTokens + mc.Usage.CacheReadInputTokens + mc.Usage.CacheCreationInputTokens
		msg.OutputTokens = mc.Usage.OutputTokens
	}

	return msg
}

func parseToolResultMessage(raw rawMessage) *Message {
	var mc messageContent
	if err := json.Unmarshal(raw.Message, &mc); err != nil {
		return nil
	}

	text := extractToolResultText(mc.Content)

	return &Message{
		Type:      TypeToolResult,
		UUID:      raw.UUID,
		Timestamp: raw.Timestamp,
		Role:      "tool",
		Text:      text,
	}
}

func parseSystemMessage(raw rawMessage) *Message {
	// System messages have content at the top level, not nested in message
	var sys struct {
		Content string `json:"content"`
		Subtype string `json:"subtype"`
	}
	// Try parsing the raw line again for top-level fields
	// For system messages, content is a top-level field
	return &Message{
		Type:      TypeSystem,
		UUID:      raw.UUID,
		Timestamp: raw.Timestamp,
		Role:      "system",
		Text:      sys.Content,
	}
}

func extractText(raw json.RawMessage) string {
	// content can be a string or array of blocks
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func extractToolResultText(raw json.RawMessage) string {
	var blocks []struct {
		Type    string `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	for _, b := range blocks {
		if b.Type == "tool_result" {
			// content is either a string or array of {type, text}
			var s string
			if err := json.Unmarshal(b.Content, &s); err == nil {
				return truncate(s, 200)
			}
			var inner []contentBlock
			if err := json.Unmarshal(b.Content, &inner); err == nil {
				for _, ib := range inner {
					if ib.Type == "text" {
						return truncate(ib.Text, 200)
					}
				}
			}
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func FormatModel(model string) string {
	switch {
	case strings.Contains(model, "opus"):
		return "opus"
	case strings.Contains(model, "sonnet"):
		return "sonnet"
	case strings.Contains(model, "haiku"):
		return "haiku"
	default:
		return model
	}
}

func FormatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
