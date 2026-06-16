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

func TestStreamableHTTPGetReturnsMethodNotAllowed(t *testing.T) {
	server := NewHTTPServer(testProtocolServer(), testLogger())
	req := httptest.NewRequest(http.MethodGet, streamableEndpointPath, nil)
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()

	server.handleStreamableHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405: %s", rec.Code, rec.Body.String())
	}
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
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, streamableEndpointPath, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set(mcpSessionIDHeader, sessionID)
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
