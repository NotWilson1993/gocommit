package main

import "testing"

func TestParseSuggestionsJSON(t *testing.T) {
	content := `{"messages":["feat: add x","fix: bug y"]}`
	msgs, err := parseSuggestions(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0] != "feat: add x" {
		t.Fatalf("unexpected first message: %q", msgs[0])
	}
}

func TestParseSuggestionsFallback(t *testing.T) {
	content := "1. add x\n2. fix y"
	msgs, err := parseSuggestions(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}
