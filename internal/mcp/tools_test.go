package mcp

import (
	"strings"
	"testing"
)

func TestToolResultExceedsMaxBytes(t *testing.T) {
	server := New(nil, testLogger(), WithMaxToolResultBytes(16))

	result, err := server.successToolResult(map[string]string{"value": strings.Repeat("x", 64)})
	if err != nil {
		t.Fatalf("successToolResult() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want true")
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "limit_exceeded") {
		t.Fatalf("tool result does not contain limit_exceeded error: %#v", result.Content)
	}
}

func TestToolResultUsesShortTextForLargeStructuredContent(t *testing.T) {
	server := New(nil, testLogger(), WithMaxToolResultBytes(1024), WithStructuredContentMaxBytes(16))

	value := map[string]string{"value": strings.Repeat("x", 64)}
	result, err := server.successToolResult(value)
	if err != nil {
		t.Fatalf("successToolResult() error = %v", err)
	}
	if result.IsError {
		t.Fatal("IsError = true, want false")
	}
	if result.StructuredContent == nil {
		t.Fatal("StructuredContent = nil, want value")
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "Large structured result") {
		t.Fatalf("content text = %#v, want large result summary", result.Content)
	}
}
