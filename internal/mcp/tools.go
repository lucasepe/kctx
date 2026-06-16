package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/lucasepe/kctx/internal/apperror"
	"github.com/lucasepe/kctx/internal/model"
)

// Tool describes one MCP tool exposed by kctx. The field names match the MCP
// tool listing shape so the value can be encoded directly in tools/list.
type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content           []textContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError"`
}

// tools returns the deterministic initial MCP tool set. Keep this list small:
// these are the highest-value operations for grounding AI SRE agents.
func tools() []Tool {
	return []Tool{
		{
			Name:        "get_namespace_health",
			Title:       "Get Namespace Health",
			Description: "Return a compact, read-only health snapshot for one Kubernetes namespace.",
			InputSchema: objectSchema(
				map[string]any{
					"namespace": stringSchema("Kubernetes namespace to inspect."),
				},
				"namespace",
			),
			Annotations: readOnlyAnnotations(),
		},
		{
			Name:        "explain_resource",
			Title:       "Explain Resource",
			Description: "Return normalized context for a Pod or supported adapter-backed Kubernetes resource.",
			InputSchema: objectSchema(
				map[string]any{
					"resource":   stringSchema("Resource token such as pod, po, application.argoproj.io, or certificates.cert-manager.io."),
					"namespace":  stringSchema("Kubernetes namespace. Omit only for cluster-scoped resources."),
					"name":       stringSchema("Resource name."),
					"eventLimit": integerSchema("Maximum recent warning Events to include for Pod context."),
				},
				"resource", "name",
			),
			Annotations: readOnlyAnnotations(),
		},
		{
			Name:        "trace_service",
			Title:       "Trace Service",
			Description: "Trace a Service to endpoints, selected Pods, owners, Nodes, relations, and factual signals.",
			InputSchema: objectSchema(
				map[string]any{
					"namespace": stringSchema("Kubernetes namespace containing the Service."),
					"name":      stringSchema("Service name."),
				},
				"namespace", "name",
			),
			Annotations: readOnlyAnnotations(),
		},
		{
			Name:        "get_pod_graph",
			Title:       "Get Pod Graph",
			Description: "Build a dependency and ownership graph around one Pod.",
			InputSchema: objectSchema(
				map[string]any{
					"namespace": stringSchema("Kubernetes namespace containing the Pod."),
					"name":      stringSchema("Pod name."),
				},
				"namespace", "name",
			),
			Annotations: readOnlyAnnotations(),
		},
		{
			Name:        "dump_namespace",
			Title:       "Dump Namespace",
			Description: "Return a deterministic namespace snapshot of entities, relations, signals, and recent warning Events.",
			InputSchema: objectSchema(
				map[string]any{
					"namespace": stringSchema("Kubernetes namespace to dump."),
				},
				"namespace",
			),
			Annotations: readOnlyAnnotations(),
		},
	}
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func integerSchema(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
		"minimum":     0,
		"maximum":     maxEventLimit,
	}
}

func readOnlyAnnotations() map[string]any {
	return map[string]any{
		"readOnlyHint":    true,
		"destructiveHint": false,
		"idempotentHint":  true,
	}
}

func (s *Server) successToolResult(value any) (toolResult, error) {
	return s.jsonToolResult(value, false)
}

func (s *Server) errorToolResult(err error) toolResult {
	result, marshalErr := s.jsonToolResult(apperror.Envelope(err), true)
	if marshalErr != nil {
		return toolResult{
			Content: []textContent{{
				Type: "text",
				Text: fmt.Sprintf(`{"schemaVersion":"kctx.io/v1alpha1","kind":"Error","error":{"code":"internal_error","message":%q}}`, marshalErr.Error()),
			}},
			IsError: true,
		}
	}
	return result
}

func (s *Server) badRequestToolResult(message string) toolResult {
	result, marshalErr := s.jsonToolResult(model.NewErrorEnvelope(model.ErrorBadRequest, message), true)
	if marshalErr != nil {
		return toolResult{
			Content: []textContent{{
				Type: "text",
				Text: fmt.Sprintf(`{"schemaVersion":"kctx.io/v1alpha1","kind":"Error","error":{"code":"internal_error","message":%q}}`, marshalErr.Error()),
			}},
			IsError: true,
		}
	}
	return result
}

func (s *Server) jsonToolResult(value any, isError bool) (toolResult, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return toolResult{}, fmt.Errorf("encode tool result: %w", err)
	}
	if !isError && s.maxToolResultBytes > 0 && int64(len(data)) > s.maxToolResultBytes {
		envelope := model.NewErrorEnvelopeWithDetails(model.ErrorLimitExceeded, "MCP tool result exceeds max response bytes", map[string]string{
			"maxBytes": fmt.Sprintf("%d", s.maxToolResultBytes),
			"bytes":    fmt.Sprintf("%d", len(data)),
		})
		return s.jsonToolResult(envelope, true)
	}

	result := toolResult{
		Content: []textContent{{
			Type: "text",
			Text: string(data),
		}},
		StructuredContent: value,
		IsError:           isError,
	}
	if !isError && s.structuredContentMaxBytes > 0 && int64(len(data)) > s.structuredContentMaxBytes {
		result.Content = []textContent{{
			Type: "text",
			Text: fmt.Sprintf("Large structured result returned in structuredContent (%d bytes compact JSON).", len(data)),
		}}
	}
	return result, nil
}
