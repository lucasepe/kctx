package mcp

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// streamIDFromEventID extracts the logical stream ID from a generated event ID.
func streamIDFromEventID(eventID string) (string, bool) {
	idx := strings.LastIndex(eventID, ":")
	if idx <= 0 || idx == len(eventID)-1 {
		return "", false
	}
	streamID := eventID[:idx]
	sequenceText := eventID[idx+1:]
	sequence, err := strconv.ParseUint(sequenceText, 10, 64)
	if err != nil || sequence == 0 {
		return "", false
	}
	return streamID, true
}

// writeSSEEvent writes one MCP message as a Server-Sent Event.
func writeSSEEvent(w io.Writer, event StreamEvent) error {
	if event.ID == "" {
		return errStreamEventMissingFields
	}
	if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "event: message\n"); err != nil {
		return err
	}
	for _, line := range bytes.Split(event.Data, []byte("\n")) {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}
