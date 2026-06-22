package mcp

import (
	"bytes"
	"testing"
)

func TestStreamIDFromEventID(t *testing.T) {
	for _, tc := range []struct {
		name    string
		eventID string
		want    string
		ok      bool
	}{
		{name: "valid", eventID: "stream-a:00000000000000000001", want: "stream-a", ok: true},
		{name: "stream id with colon", eventID: "stream:a:42", want: "stream:a", ok: true},
		{name: "missing separator", eventID: "stream-a", ok: false},
		{name: "missing stream", eventID: ":1", ok: false},
		{name: "missing sequence", eventID: "stream-a:", ok: false},
		{name: "non numeric sequence", eventID: "stream-a:nope", ok: false},
		{name: "zero sequence", eventID: "stream-a:0", ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := streamIDFromEventID(tc.eventID)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("streamID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWriteSSEEvent(t *testing.T) {
	var buf bytes.Buffer
	err := writeSSEEvent(&buf, StreamEvent{
		ID:   "stream-a:00000000000000000001",
		Data: []byte("{\"jsonrpc\":\"2.0\"}"),
	})
	if err != nil {
		t.Fatalf("writeSSEEvent() error = %v", err)
	}
	want := "id: stream-a:00000000000000000001\n" +
		"event: message\n" +
		"data: {\"jsonrpc\":\"2.0\"}\n\n"
	if buf.String() != want {
		t.Fatalf("SSE event = %q, want %q", buf.String(), want)
	}
}

func TestWriteSSEEventSplitsMultilineData(t *testing.T) {
	var buf bytes.Buffer
	err := writeSSEEvent(&buf, StreamEvent{
		ID:   "stream-a:00000000000000000001",
		Data: []byte("line one\nline two"),
	})
	if err != nil {
		t.Fatalf("writeSSEEvent() error = %v", err)
	}
	want := "id: stream-a:00000000000000000001\n" +
		"event: message\n" +
		"data: line one\n" +
		"data: line two\n\n"
	if buf.String() != want {
		t.Fatalf("SSE event = %q, want %q", buf.String(), want)
	}
}
