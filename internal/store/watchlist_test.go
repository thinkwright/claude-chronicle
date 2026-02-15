package store

import (
	"testing"
	"time"
)

func TestAddWatch(t *testing.T) {
	s := openTestStore(t)

	item, err := s.AddWatch("errors", "error|panic", "#ff0000")
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if item.Name != "errors" {
		t.Errorf("name = %q", item.Name)
	}
	if item.Pattern != "error|panic" {
		t.Errorf("pattern = %q", item.Pattern)
	}
	if !item.Enabled {
		t.Error("expected enabled")
	}
	if item.Color != "#ff0000" {
		t.Errorf("color = %q", item.Color)
	}
	if item.Compiled == nil {
		t.Error("expected compiled regex")
	}
}

func TestAddWatch_DefaultColor(t *testing.T) {
	s := openTestStore(t)
	item, err := s.AddWatch("test", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	if item.Color != "#b56a6a" {
		t.Errorf("default color = %q, want #b56a6a", item.Color)
	}
}

func TestAddWatch_InvalidRegex(t *testing.T) {
	s := openTestStore(t)
	_, err := s.AddWatch("bad", "[invalid", "")
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestListWatches(t *testing.T) {
	s := openTestStore(t)

	s.AddWatch("first", "error", "#ff0000")
	s.AddWatch("second", "panic", "#00ff00")

	items, err := s.ListWatches()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "first" || items[1].Name != "second" {
		t.Errorf("order: %q, %q", items[0].Name, items[1].Name)
	}
	// Each should have compiled regex
	for _, item := range items {
		if item.Compiled == nil {
			t.Errorf("item %q has nil compiled regex", item.Name)
		}
	}
}

func TestGetWatch(t *testing.T) {
	s := openTestStore(t)
	added, _ := s.AddWatch("test", "deploy", "")

	got, err := s.GetWatch(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test" || got.Pattern != "deploy" {
		t.Errorf("got = %+v", got)
	}
	if got.Compiled == nil {
		t.Error("expected compiled regex")
	}
}

func TestRemoveWatch(t *testing.T) {
	s := openTestStore(t)
	item, _ := s.AddWatch("deleteme", "test", "")

	if err := s.RemoveWatch(item.ID); err != nil {
		t.Fatal(err)
	}

	items, _ := s.ListWatches()
	if len(items) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(items))
	}
}

func TestToggleWatch(t *testing.T) {
	s := openTestStore(t)
	item, _ := s.AddWatch("toggle", "test", "")

	// Initially enabled
	if !item.Enabled {
		t.Fatal("expected enabled initially")
	}

	s.ToggleWatch(item.ID)
	got, _ := s.GetWatch(item.ID)
	if got.Enabled {
		t.Error("expected disabled after toggle")
	}

	s.ToggleWatch(item.ID)
	got, _ = s.GetWatch(item.ID)
	if !got.Enabled {
		t.Error("expected enabled after second toggle")
	}
}

func TestUpdateWatch(t *testing.T) {
	s := openTestStore(t)
	item, _ := s.AddWatch("original", "error", "")

	err := s.UpdateWatch(item.ID, "updated", "panic|crash")
	if err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetWatch(item.ID)
	if got.Name != "updated" {
		t.Errorf("name = %q, want updated", got.Name)
	}
	if got.Pattern != "panic|crash" {
		t.Errorf("pattern = %q", got.Pattern)
	}
}

func TestUpdateWatch_InvalidRegex(t *testing.T) {
	s := openTestStore(t)
	item, _ := s.AddWatch("test", "error", "")

	err := s.UpdateWatch(item.ID, "test", "[invalid")
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestMatchNewMessages(t *testing.T) {
	s := openTestStore(t)
	_, msgIDs := seedTestData(t, s)

	// Add a watch that should match "deploy"
	s.AddWatch("deploy-watch", "deploy", "#ff0000")

	// Wait for background backfill to complete
	time.Sleep(200 * time.Millisecond)

	// Now test MatchNewMessages with the message IDs
	count, err := s.MatchNewMessages(msgIDs)
	if err != nil {
		t.Fatal(err)
	}
	// "deploy" appears in at least the first user message and first assistant message
	if count == 0 {
		t.Error("expected matches for 'deploy' pattern")
	}
}

func TestMatchNewMessages_NoIDs(t *testing.T) {
	s := openTestStore(t)
	count, err := s.MatchNewMessages(nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestMatchNewMessages_DisabledWatch(t *testing.T) {
	s := openTestStore(t)
	_, msgIDs := seedTestData(t, s)

	item, _ := s.AddWatch("disabled", "deploy", "")
	time.Sleep(100 * time.Millisecond)
	s.ToggleWatch(item.ID) // disable

	count, err := s.MatchNewMessages(msgIDs)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("expected 0 matches from disabled watch")
	}
}

func TestMatchesForWatch(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	item, _ := s.AddWatch("deploy-match", "deploy", "")
	// Wait for backfill
	time.Sleep(200 * time.Millisecond)

	matches, err := s.MatchesForWatch(item.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("expected matches")
	}
	for _, m := range matches {
		if m.WatchItemID != item.ID {
			t.Errorf("wrong watch ID: %d", m.WatchItemID)
		}
		if m.MatchedText == "" {
			t.Error("expected non-empty matched text snippet")
		}
	}
}

func TestMarkWatchSeen(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	item, _ := s.AddWatch("seen-test", "deploy", "")
	time.Sleep(200 * time.Millisecond)

	// Should have unseen matches
	got, _ := s.GetWatch(item.ID)
	if got.UnseenCount == 0 {
		t.Fatal("expected unseen matches after backfill")
	}

	// Mark seen
	s.MarkWatchSeen(item.ID)

	got, _ = s.GetWatch(item.ID)
	if got.UnseenCount != 0 {
		t.Errorf("unseen = %d after mark seen", got.UnseenCount)
	}
}

func TestMarkSessionSeen(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := seedTestData(t, s)

	s.AddWatch("session-seen", "deploy", "")
	time.Sleep(200 * time.Millisecond)

	s.MarkSessionSeen(sessionID)

	count := s.TotalUnseenCount()
	if count != 0 {
		t.Errorf("total unseen = %d after marking session seen", count)
	}
}

func TestTotalUnseenCount(t *testing.T) {
	s := openTestStore(t)
	seedTestData(t, s)

	// No watches = 0
	if s.TotalUnseenCount() != 0 {
		t.Error("expected 0 with no watches")
	}

	s.AddWatch("unseen", "deploy", "")
	time.Sleep(200 * time.Millisecond)

	count := s.TotalUnseenCount()
	if count == 0 {
		t.Error("expected unseen matches")
	}
}

func TestExtractSnippet(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		start, end int
		contextLen int
		wantPrefix bool // should have "..." prefix
		wantSuffix bool // should have "..." suffix
	}{
		{
			name:       "middle of text",
			text:       "aaa bbb ccc ddd eee fff ggg hhh iii jjj kkk lll mmm",
			start:      20, end: 23,
			contextLen: 10,
			wantPrefix: true,
			wantSuffix: true,
		},
		{
			name:       "start of text",
			text:       "error at line 5",
			start:      0, end: 5,
			contextLen: 20,
			wantPrefix: false,
			wantSuffix: false,
		},
		{
			name:       "end of text",
			text:       "something went wrong",
			start:      15, end: 20,
			contextLen: 10,
			wantPrefix: true,
			wantSuffix: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSnippet(tt.text, tt.start, tt.end, tt.contextLen)
			if tt.wantPrefix && got[:3] != "..." {
				t.Errorf("expected ... prefix, got %q", got[:10])
			}
			if !tt.wantPrefix && got[:3] == "..." {
				t.Errorf("unexpected ... prefix")
			}
			if tt.wantSuffix && got[len(got)-3:] != "..." {
				t.Errorf("expected ... suffix, got %q", got[len(got)-10:])
			}
		})
	}
}

func TestExtractSnippet_NewlinesRemoved(t *testing.T) {
	text := "line one\nline two\nline three"
	got := extractSnippet(text, 0, 8, 100)
	if got != "line one line two line three" {
		t.Errorf("newlines not removed: %q", got)
	}
}
