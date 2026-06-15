package mcp

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSSEServerMessageEndpointEnqueuesToolCallResponse(t *testing.T) {
	server := NewSSEServer(testProtocolServer(), testLogger())
	session := server.sessions.add(context.Background(), "test-session")

	rec := postMessage(t, server, "test-session", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_namespace_health","arguments":{"namespace":"payments"}}}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202: %s", rec.Code, rec.Body.String())
	}

	select {
	case message := <-session.messages:
		assertContains(t, string(message), `"jsonrpc":"2.0"`)
		assertContains(t, string(message), `"kind":"NamespaceHealth"`)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for queued SSE response")
	}
}

func TestSSEServerMessageEndpointAcknowledgesBeforeToolResponseIsConsumed(t *testing.T) {
	server := NewSSEServer(testProtocolServer(), testLogger())
	server.sessions.add(context.Background(), "test-session")

	rec := postMessage(t, server, "test-session", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_namespace_health","arguments":{"namespace":"payments"}}}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
}

func TestSSEServerMessageEndpointRejectsInvalidSession(t *testing.T) {
	server := NewSSEServer(testProtocolServer(), testLogger())

	rec := postMessage(t, server, "missing-session", `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), "invalid or expired MCP session")
}

func TestSSEServerMessageEndpointRequiresPOST(t *testing.T) {
	server := NewSSEServer(testProtocolServer(), testLogger())
	req := httptest.NewRequest(http.MethodGet, sseMessagePath+"?sessionId=test-session", nil)
	rec := httptest.NewRecorder()

	server.handleMessage(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405: %s", rec.Code, rec.Body.String())
	}
}

func TestSSEServerInvalidJSONIsSentOnSessionStream(t *testing.T) {
	server := NewSSEServer(testProtocolServer(), testLogger())
	session := server.sessions.add(context.Background(), "test-session")

	rec := postMessage(t, server, "test-session", `{`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202: %s", rec.Code, rec.Body.String())
	}

	select {
	case message := <-session.messages:
		assertContains(t, string(message), `"code":-32700`)
		assertContains(t, string(message), `"message":"invalid JSON-RPC message"`)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for queued parse error")
	}
}

func TestSSESessionRemoveCancelsInFlightContext(t *testing.T) {
	store := newSessionStore()
	session := store.add(context.Background(), "test-session")

	store.remove("test-session")

	select {
	case <-session.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session context was not canceled")
	}
	if _, ok := store.get("test-session"); ok {
		t.Fatal("session was not removed from store")
	}
}

func TestSSESessionEnqueueFailsWhenSessionIsClosed(t *testing.T) {
	store := newSessionStore()
	session := store.add(context.Background(), "test-session")
	store.remove("test-session")

	err := session.enqueue(successResponse([]byte("1"), map[string]any{}))
	if err == nil {
		t.Fatal("enqueue error = nil, want cancellation error")
	}
}

func TestSSEServerAllowsConcurrentPostsToSameSession(t *testing.T) {
	server := NewSSEServer(testProtocolServer(), testLogger())
	session := server.sessions.add(context.Background(), "test-session")

	const requests = 4
	var wg sync.WaitGroup
	errCh := make(chan string, requests)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := postMessage(t, server, "test-session", `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
			if rec.Code != http.StatusAccepted {
				errCh <- rec.Body.String()
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for errText := range errCh {
		t.Fatalf("POST status was not accepted: %s", errText)
	}

	for i := 0; i < requests; i++ {
		select {
		case message := <-session.messages:
			assertContains(t, string(message), `"jsonrpc":"2.0"`)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for queued response %d", i+1)
		}
	}
}

func TestSSEServerHealthEndpoints(t *testing.T) {
	handler := NewSSEServer(testProtocolServer(), testLogger()).Handler()

	for _, path := range []string{"/livez", "/readyz", "/version"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, rec.Code)
		}
	}
}

func testProtocolServer() *Server {
	reader := testutil.NewFakeReader(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "payments"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	})
	return New(engine.New(reader), testLogger(), WithRequestTimeout(5*time.Second), WithKubeAPIBudget(100))
}

func postMessage(t *testing.T, server *SSEServer, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, sseMessagePath+"?sessionId="+sessionID, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.handleMessage(rec, req)
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
