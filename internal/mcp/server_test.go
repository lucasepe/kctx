package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServerInitializeAndListTools(t *testing.T) {
	out := serveInputWithOptions(t, strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n"), WithVersion("v9.9.9"))

	responses := decodeResponses(t, out)
	if len(responses) != 2 {
		t.Fatalf("responses len = %d, want 2\n%s", len(responses), out)
	}
	assertJSONContains(t, responses[0], `"protocolVersion":"2025-06-18"`)
	assertJSONContains(t, responses[0], `"tools":{"listChanged":false}`)
	assertJSONContains(t, responses[0], `"version":"v9.9.9"`)
	assertJSONContains(t, responses[1], `"name":"get_namespace_health"`)
	assertJSONContains(t, responses[1], `"name":"explain_resource"`)
	assertJSONContains(t, responses[1], `"name":"trace_service"`)
	assertJSONContains(t, responses[1], `"name":"get_pod_graph"`)
	assertJSONContains(t, responses[1], `"name":"dump_namespace"`)
}

func TestToolsListContract(t *testing.T) {
	expectedRequired := map[string][]string{
		"get_namespace_health": {"namespace"},
		"explain_resource":     {"resource", "name"},
		"trace_service":        {"namespace", "name"},
		"get_pod_graph":        {"namespace", "name"},
		"dump_namespace":       {"namespace"},
	}

	list := tools()
	if len(list) != len(expectedRequired) {
		t.Fatalf("tool count = %d, want %d", len(list), len(expectedRequired))
	}

	seen := map[string]bool{}
	for _, tool := range list {
		if tool.Name == "" {
			t.Fatal("tool name is empty")
		}
		if tool.Description == "" {
			t.Fatalf("%s description is empty", tool.Name)
		}
		if seen[tool.Name] {
			t.Fatalf("duplicate tool name %q", tool.Name)
		}
		seen[tool.Name] = true

		schema := tool.InputSchema
		if schema["type"] != "object" {
			t.Fatalf("%s schema type = %v, want object", tool.Name, schema["type"])
		}
		if schema["additionalProperties"] != false {
			t.Fatalf("%s schema additionalProperties = %v, want false", tool.Name, schema["additionalProperties"])
		}
		if _, ok := schema["properties"].(map[string]any); !ok {
			t.Fatalf("%s schema properties missing or invalid", tool.Name)
		}
		if got := stringsFromAnySlice(t, schema["required"]); !sameStrings(got, expectedRequired[tool.Name]) {
			t.Fatalf("%s required = %v, want %v", tool.Name, got, expectedRequired[tool.Name])
		}

		if tool.Annotations["readOnlyHint"] != true {
			t.Fatalf("%s readOnlyHint = %v, want true", tool.Name, tool.Annotations["readOnlyHint"])
		}
		if tool.Annotations["destructiveHint"] != false {
			t.Fatalf("%s destructiveHint = %v, want false", tool.Name, tool.Annotations["destructiveHint"])
		}
		if tool.Annotations["idempotentHint"] != true {
			t.Fatalf("%s idempotentHint = %v, want true", tool.Name, tool.Annotations["idempotentHint"])
		}
	}

	for name := range expectedRequired {
		if !seen[name] {
			t.Fatalf("expected tool %q was not registered", name)
		}
	}
}

func TestServerCallNamespaceHealth(t *testing.T) {
	out := serveInput(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_namespace_health","arguments":{"namespace":"payments"}}}`)

	responses := decodeResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("responses len = %d, want 1\n%s", len(responses), out)
	}
	assertJSONContains(t, responses[0], `"isError":false`)
	assertJSONContains(t, responses[0], `"kind":"NamespaceHealth"`)
	assertJSONContains(t, responses[0], `"namespace":"payments"`)
}

func TestServerCallGetPodGraph(t *testing.T) {
	out := serveInput(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_pod_graph","arguments":{"namespace":"payments","name":"api-1"}}}`)

	responses := decodeResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("responses len = %d, want 1\n%s", len(responses), out)
	}
	assertJSONContains(t, responses[0], `"isError":false`)
	assertJSONContains(t, responses[0], `"kind":"PodGraph"`)
}

func TestServerCallDumpNamespace(t *testing.T) {
	out := serveInput(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"dump_namespace","arguments":{"namespace":"payments"}}}`)

	responses := decodeResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("responses len = %d, want 1\n%s", len(responses), out)
	}
	assertJSONContains(t, responses[0], `"isError":false`)
	assertJSONContains(t, responses[0], `"kind":"NamespaceDump"`)
	assertJSONContains(t, responses[0], `"namespace":"payments"`)
}

func TestServerCallExplainResourceRejectsEventLimitOverMaximum(t *testing.T) {
	out := serveInput(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"explain_resource","arguments":{"resource":"pod","namespace":"payments","name":"api-1","eventLimit":501}}}`)

	responses := decodeResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("responses len = %d, want 1\n%s", len(responses), out)
	}
	assertJSONContains(t, responses[0], `"isError":true`)
	assertJSONContains(t, responses[0], `"code":"bad_request"`)
	assertJSONContains(t, responses[0], `eventLimit must be less than or equal to 500`)
}

func TestServerCallToolValidationErrorUsesToolResult(t *testing.T) {
	out := serveInput(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_namespace_health","arguments":{}}}`)

	responses := decodeResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("responses len = %d, want 1\n%s", len(responses), out)
	}
	assertJSONContains(t, responses[0], `"isError":true`)
	assertJSONContains(t, responses[0], `"kind":"Error"`)
	assertJSONContains(t, responses[0], `"code":"bad_request"`)
	assertJSONContains(t, responses[0], `"namespace is required"`)
}

func TestServerJSONRPCErrorCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "invalid JSON",
			input: `{`,
			want:  []string{`"code":-32700`, `"message":"invalid JSON-RPC message"`},
		},
		{
			name:  "invalid JSON-RPC request",
			input: `{"jsonrpc":"2.0","id":1}`,
			want:  []string{`"code":-32600`, `"message":"invalid JSON-RPC request"`},
		},
		{
			name:  "unknown method",
			input: `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`,
			want:  []string{`"code":-32601`, `"message":"method not found"`},
		},
		{
			name:  "missing tool name",
			input: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"arguments":{}}}`,
			want:  []string{`"code":-32602`, `"message":"tool name is required"`},
		},
		{
			name:  "unknown tool",
			input: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"restart_pod","arguments":{}}}`,
			want:  []string{`"code":-32602`, `"message":"unknown tool \"restart_pod\""`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := serveInput(t, tt.input)
			responses := decodeResponses(t, out)
			if len(responses) != 1 {
				t.Fatalf("responses len = %d, want 1\n%s", len(responses), out)
			}
			for _, want := range tt.want {
				assertJSONContains(t, responses[0], want)
			}
		})
	}
}

func TestServerEngineErrorsUseToolResult(t *testing.T) {
	out := serveInput(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_pod_graph","arguments":{"namespace":"payments","name":"missing"}}}`)

	responses := decodeResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("responses len = %d, want 1\n%s", len(responses), out)
	}
	assertJSONContains(t, responses[0], `"isError":true`)
	assertJSONContains(t, responses[0], `"kind":"Error"`)
	assertJSONContains(t, responses[0], `"code":"not_found"`)
}

func TestServerNotificationDoesNotRespond(t *testing.T) {
	out := serveInput(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if strings.TrimSpace(out) != "" {
		t.Fatalf("notification produced response:\n%s", out)
	}
}

func serveInput(t *testing.T, input string) string {
	t.Helper()
	return serveInputWithOptions(t, input)
}

func serveInputWithOptions(t *testing.T, input string, opts ...Option) string {
	t.Helper()
	reader := testutil.NewFakeReader(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "payments"},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "payments"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	})
	allOpts := []Option{WithRequestTimeout(5 * time.Second), WithKubeAPIBudget(100)}
	allOpts = append(allOpts, opts...)
	srv := New(engine.New(reader), slog.Default(), allOpts...)
	var out bytes.Buffer
	if err := srv.Serve(context.Background(), strings.NewReader(input+"\n"), &out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	return out.String()
}

func decodeResponses(t *testing.T, out string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	responses := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("response is not JSON: %v\n%s", err, line)
		}
		responses = append(responses, resp)
	}
	return responses
}

func assertJSONContains(t *testing.T, value any, want string) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("response does not contain %q:\n%s", want, data)
	}
}

func stringsFromAnySlice(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]string)
	if ok {
		return items
	}
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("value is not a string slice: %#v", value)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("slice item is not string: %#v", item)
		}
		out = append(out, text)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
