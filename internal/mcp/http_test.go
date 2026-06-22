package mcp

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	workqueue "github.com/lucasepe/kctx/internal/queue"
	"github.com/lucasepe/kctx/internal/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStreamableHTTPPostReturnsJSONRPCResponse(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithSession(t, server, sessionID, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	assertContains(t, rec.Body.String(), `"jsonrpc":"2.0"`)
	assertContains(t, rec.Body.String(), `"tools"`)
	assertContains(t, rec.Body.String(), `"get_namespace_health"`)
}

func TestStreamableHTTPPostRecordsCompletedStreamAndFinalEvent(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithSession(t, server, sessionID, `{"jsonrpc":"2.0","id":42,"method":"tools/list"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	stream, ok := server.streams.FindByRequest(context.Background(), sessionID, "42")
	if !ok {
		t.Fatal("FindByRequest() ok = false, want true")
	}
	if stream.State != StreamCompleted {
		t.Fatalf("stream State = %q, want %q", stream.State, StreamCompleted)
	}
	req, ok := server.streamRequests.FindByRequest(context.Background(), sessionID, "42")
	if !ok {
		t.Fatal("request FindByRequest() ok = false, want true")
	}
	if req.StreamID != stream.ID {
		t.Fatalf("request StreamID = %q, want %q", req.StreamID, stream.ID)
	}
	if req.State != StreamRequestCompleted {
		t.Fatalf("request State = %q, want %q", req.State, StreamRequestCompleted)
	}

	events, err := server.streamEvents.After(context.Background(), sessionID, stream.ID, "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("After() len = %d, want 1", len(events))
	}
	if events[0].StreamID != stream.ID {
		t.Fatalf("event StreamID = %q, want %q", events[0].StreamID, stream.ID)
	}
	if events[0].RequestID != "42" {
		t.Fatalf("event RequestID = %q, want 42", events[0].RequestID)
	}
	assertContains(t, string(events[0].Data), `"jsonrpc":"2.0"`)
	assertContains(t, string(events[0].Data), `"tools"`)
}

func TestStreamableHTTPPostCanReturnSSEResponse(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithAccept(t, server, sessionID, "text/event-stream", `{"jsonrpc":"2.0","id":42,"method":"tools/list"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	stream, ok := server.streams.FindByRequest(context.Background(), sessionID, "42")
	if !ok {
		t.Fatal("FindByRequest() ok = false, want true")
	}
	events, err := server.streamEvents.After(context.Background(), sessionID, stream.ID, "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("After() len = %d, want 1", len(events))
	}
	req, ok := server.streamRequests.FindByRequest(context.Background(), sessionID, "42")
	if !ok {
		t.Fatal("request FindByRequest() ok = false, want true")
	}
	if req.State != StreamRequestCompleted {
		t.Fatalf("request State = %q, want %q", req.State, StreamRequestCompleted)
	}
	assertContains(t, rec.Body.String(), "id: "+events[0].ID)
	assertContains(t, rec.Body.String(), "event: message")
	assertContains(t, rec.Body.String(), `data: {"jsonrpc":"2.0","id":42`)
	assertContains(t, string(events[0].Data), `"tools"`)
}

func TestStreamableHTTPPostSSEResponseRejectsWhenQueueUnavailable(t *testing.T) {
	q := workqueue.New(1, 1)
	server := NewHTTPServer(testProtocolServer(), testLogger(), WithRequestQueue(q))
	q.Terminate()
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithAccept(t, server, sessionID, "text/event-stream", `{"jsonrpc":"2.0","id":42,"method":"tools/list"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), "MCP request queue is unavailable")
}

func TestStreamableHTTPPostPrefersJSONWhenAcceptListsJSONFirst(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithAccept(t, server, sessionID, "application/json, text/event-stream", `{"jsonrpc":"2.0","id":42,"method":"tools/list"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	assertContains(t, rec.Body.String(), `"jsonrpc":"2.0"`)
}

func TestStreamableHTTPCORSPreflightAllowsValidOrigin(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	req := httptest.NewRequest(http.MethodOptions, streamableEndpointPath, nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Mcp-Session-Id")
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want valid origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !bytes.Contains([]byte(got), []byte(http.MethodPost)) {
		t.Fatalf("Access-Control-Allow-Methods = %q, want POST", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !bytes.Contains([]byte(got), []byte(mcpSessionIDHeader)) {
		t.Fatalf("Access-Control-Allow-Headers = %q, want %s", got, mcpSessionIDHeader)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); !bytes.Contains([]byte(got), []byte(mcpSessionIDHeader)) {
		t.Fatalf("Access-Control-Expose-Headers = %q, want %s", got, mcpSessionIDHeader)
	}
	if got := rec.Header().Get(mcpProtocolHeader); got != protocolVersion {
		t.Fatalf("%s = %q, want %q", mcpProtocolHeader, got, protocolVersion)
	}
}

func TestStreamableHTTPCORSHeadersExposeSessionID(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	req := httptest.NewRequest(http.MethodPost, streamableEndpointPath, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want valid origin", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); !bytes.Contains([]byte(got), []byte(mcpSessionIDHeader)) {
		t.Fatalf("Access-Control-Expose-Headers = %q, want %s", got, mcpSessionIDHeader)
	}
	if got := rec.Header().Get(mcpProtocolHeader); got != protocolVersion {
		t.Fatalf("%s = %q, want %q", mcpProtocolHeader, got, protocolVersion)
	}
}

func TestStreamableHTTPInitializeReturnsSessionID(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	rec := postStreamable(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(mcpSessionIDHeader); got == "" {
		t.Fatal("MCP session header is empty")
	}
	assertContains(t, rec.Body.String(), `"protocolVersion":"2025-06-18"`)
}

func TestStreamableHTTPPostRequiresSessionAfterInitialize(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	rec := postStreamable(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), "missing or expired MCP session")
}

func TestStreamableHTTPPostNotificationReturnsAccepted(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithSession(t, server, sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("POST notification body = %q, want empty", rec.Body.String())
	}
}

func TestStreamableHTTPPostJSONRPCResponseReturnsAccepted(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithSession(t, server, sessionID, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("POST response body = %q, want empty", rec.Body.String())
	}
}

func TestStreamableHTTPDeleteTerminatesSession(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	req := httptest.NewRequest(http.MethodDelete, streamableEndpointPath, nil)
	req.Header.Set(mcpSessionIDHeader, sessionID)
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	rec = postStreamableWithSession(t, server, sessionID, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400 after delete: %s", rec.Code, rec.Body.String())
	}
}

func TestStreamableHTTPDeleteRemovesStreamsAndEvents(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithSession(t, server, sessionID, `{"jsonrpc":"2.0","id":42,"method":"tools/list"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	stream, ok := server.streams.FindByRequest(context.Background(), sessionID, "42")
	if !ok {
		t.Fatal("FindByRequest() ok = false, want true before delete")
	}

	req := httptest.NewRequest(http.MethodDelete, streamableEndpointPath, nil)
	req.Header.Set(mcpSessionIDHeader, sessionID)
	deleteRec := httptest.NewRecorder()
	server.handleStreamableHTTP(deleteRec, req)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204: %s", deleteRec.Code, deleteRec.Body.String())
	}

	if _, ok := server.streams.FindByRequest(context.Background(), sessionID, "42"); ok {
		t.Fatal("FindByRequest() ok = true after delete, want false")
	}
	if _, ok := server.streamRequests.FindByRequest(context.Background(), sessionID, "42"); ok {
		t.Fatal("request FindByRequest() ok = true after delete, want false")
	}
	events, err := server.streamEvents.After(context.Background(), sessionID, stream.ID, "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("After() len = %d, want 0 after delete", len(events))
	}
}

func TestStreamableHTTPPostRejectsInvalidJSON(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	rec := postStreamable(t, server, `{`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), `"code":-32700`)
}

func TestStreamableHTTPPostRejectsTooLargeRequest(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger(), WithMaxRequestBytes(16))

	rec := postStreamable(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"padding":"too-large"}}`)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST status = %d, want 413: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), "MCP request exceeds max request bytes")
}

func TestStreamableHTTPPostRejectsTooLargeResponse(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger(), WithMaxResponseBytes(64))

	rec := postStreamable(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST status = %d, want 413: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), "MCP response exceeds max response bytes")
}

func TestStreamableHTTPGetReplaysEventsAfterLastEventID(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithSession(t, server, sessionID, `{"jsonrpc":"2.0","id":42,"method":"tools/list"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	stream, ok := server.streams.FindByRequest(context.Background(), sessionID, "42")
	if !ok {
		t.Fatal("FindByRequest() ok = false, want true")
	}
	events, err := server.streamEvents.After(context.Background(), sessionID, stream.ID, "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("After() len = %d, want 1", len(events))
	}
	secondEventID := server.streamEventIDs.Next(stream.ID)
	if err := server.streamEvents.Append(context.Background(), StreamEvent{
		ID:        secondEventID,
		SessionID: sessionID,
		StreamID:  stream.ID,
		RequestID: stream.RequestID,
		Data:      []byte(`{"jsonrpc":"2.0","id":42,"result":{"replayed":true}}`),
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	getRec := getStreamableReplay(t, server, sessionID, events[0].ID)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
	if got := getRec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	assertContains(t, getRec.Body.String(), "id: "+secondEventID)
	assertContains(t, getRec.Body.String(), "event: message")
	assertContains(t, getRec.Body.String(), `data: {"jsonrpc":"2.0","id":42,"result":{"replayed":true}}`)
}

func TestStreamableHTTPGetReturnsEmptyReplayWhenNoLaterEvents(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithSession(t, server, sessionID, `{"jsonrpc":"2.0","id":42,"method":"tools/list"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	stream, ok := server.streams.FindByRequest(context.Background(), sessionID, "42")
	if !ok {
		t.Fatal("FindByRequest() ok = false, want true")
	}
	events, err := server.streamEvents.After(context.Background(), sessionID, stream.ID, "")
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("After() len = %d, want 1", len(events))
	}

	getRec := getStreamableReplay(t, server, sessionID, events[0].ID)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", getRec.Code, getRec.Body.String())
	}
	if got := getRec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if getRec.Body.Len() != 0 {
		t.Fatalf("GET body = %q, want empty replay", getRec.Body.String())
	}
}

func TestStreamableHTTPGetRequiresSession(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	req := httptest.NewRequest(http.MethodGet, streamableEndpointPath, nil)
	req.Header.Set(lastEventIDHeader, "stream-a:00000000000000000001")
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()

	server.handleStreamableHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestStreamableHTTPGetRequiresLastEventID(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := getStreamableReplay(t, server, sessionID, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), "missing Last-Event-ID")
}

func TestStreamableHTTPGetRejectsInvalidLastEventID(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := getStreamableReplay(t, server, sessionID, "not-a-generated-event-id")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), "invalid Last-Event-ID")
}

func TestStreamableHTTPGetRejectsMissingStream(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := getStreamableReplay(t, server, sessionID, "missing-stream:00000000000000000001")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), "missing or expired MCP stream")
}

func TestStreamableHTTPGetRejectsMissingEventCursor(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	sessionID := initializeStreamable(t, server)

	rec := postStreamableWithSession(t, server, sessionID, `{"jsonrpc":"2.0","id":42,"method":"tools/list"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	stream, ok := server.streams.FindByRequest(context.Background(), sessionID, "42")
	if !ok {
		t.Fatal("FindByRequest() ok = false, want true")
	}

	rec = getStreamableReplay(t, server, sessionID, formatStreamEventID(stream.ID, 99))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), "missing or expired MCP event cursor")
}

func TestStreamableHTTPRejectsInvalidOrigin(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	req := httptest.NewRequest(http.MethodPost, streamableEndpointPath, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for rejected origin", got)
	}
}

func TestStreamableHTTPAllowsSameOrigin(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	req := httptest.NewRequest(http.MethodPost, streamableEndpointPath, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Host = "kctx.internal:8080"
	req.Header.Set("Origin", "https://kctx.internal:8080")
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestStreamableHTTPAllowsLoopbackOrigin(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	req := httptest.NewRequest(http.MethodPost, streamableEndpointPath, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestStreamableHTTPRejectsUnsupportedProtocolVersion(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	req := httptest.NewRequest(http.MethodPost, streamableEndpointPath, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Header.Set(mcpProtocolHeader, "2024-11-05")
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestStreamableHTTPAllowsSupportedProtocolVersion(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())

	req := httptest.NewRequest(http.MethodPost, streamableEndpointPath, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Header.Set(mcpProtocolHeader, protocolVersion)
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPServerHealthEndpoints(t *testing.T) {
	handler := NewHTTPServer(testProtocolServer(), testLogger()).Handler()

	for _, path := range []string{"/livez", "/readyz", "/version"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, rec.Code)
		}
	}
}

func TestHTTPServerHandlerExposesStreamableEndpoint(t *testing.T) {
	handler := NewHTTPServer(testProtocolServer(), testLogger()).Handler()

	req := httptest.NewRequest(http.MethodPost, streamableEndpointPath, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d, want 200: %s", streamableEndpointPath, rec.Code, rec.Body.String())
	}
}

func testProtocolServer() *Server {
	reader := testutil.NewFakeReader(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "payments"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	})
	return New(engine.New(reader), testLogger(), WithRequestTimeout(5*time.Second), WithKubeAPIBudget(100))
}

func initializeStreamable(t *testing.T, server *HTTPServer) string {
	t.Helper()
	rec := postStreamable(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	sessionID := rec.Header().Get(mcpSessionIDHeader)
	if sessionID == "" {
		t.Fatal("initialize did not return MCP session header")
	}
	return sessionID
}

func postStreamable(t *testing.T, server *HTTPServer, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postStreamableWithSession(t, server, "", body)
}

func postStreamableWithSession(t *testing.T, server *HTTPServer, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postStreamableWithAccept(t, server, sessionID, "application/json, text/event-stream", body)
}

func postStreamableWithAccept(t *testing.T, server *HTTPServer, sessionID, accept, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, streamableEndpointPath, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", accept)
	if sessionID != "" {
		req.Header.Set(mcpSessionIDHeader, sessionID)
	}
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)
	return rec
}

func getStreamableReplay(t *testing.T, server *HTTPServer, sessionID, lastEventID string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, streamableEndpointPath, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if sessionID != "" {
		req.Header.Set(mcpSessionIDHeader, sessionID)
	}
	if lastEventID != "" {
		req.Header.Set(lastEventIDHeader, lastEventID)
	}
	rec := httptest.NewRecorder()
	server.handleStreamableHTTP(rec, req)
	return rec
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !bytes.Contains([]byte(got), []byte(want)) {
		t.Fatalf("value does not contain %q:\n%s", want, got)
	}
}
