package argocd_test

import (
	"context"
	"testing"

	"github.com/lucasepe/kctx/internal/engine/adapters/argocd"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAppProjectAdapterExtractsPolicyContext(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "AppProject",
		"metadata": map[string]any{
			"name":      "platform",
			"namespace": "argocd",
		},
		"spec": map[string]any{
			"sourceRepos": []any{"https://token123@github.com/example/platform.git"},
			"destinations": []any{
				map[string]any{"namespace": "payments", "server": "https://kubernetes.default.svc"},
			},
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "InvalidSpecError", "status": "False", "message": "token=plain should not leak"},
			},
		},
	}}

	adapter := argocd.AppProjectAdapter{}
	if !adapter.Supports(obj) {
		t.Fatalf("Supports() = false")
	}

	entities, err := adapter.Entities(context.Background(), obj)
	if err != nil {
		t.Fatalf("Entities() error = %v", err)
	}
	if len(entities) != 4 {
		t.Fatalf("entities len = %d, want 4: %#v", len(entities), entities)
	}
	if entities[1].Name != "https://[REDACTED]@github.com/example/platform.git" {
		t.Fatalf("repository entity name = %q, want redacted", entities[1].Name)
	}

	relations, err := adapter.Relations(context.Background(), obj)
	if err != nil {
		t.Fatalf("Relations() error = %v", err)
	}
	if len(relations) != 3 {
		t.Fatalf("relations len = %d, want 3: %#v", len(relations), relations)
	}

	signals, err := adapter.Signals(context.Background(), obj)
	if err != nil {
		t.Fatalf("Signals() error = %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("signals len = %d, want 1: %#v", len(signals), signals)
	}
	if signals[0].Message != "token=[REDACTED] should not leak" {
		t.Fatalf("signal message = %q, want redacted", signals[0].Message)
	}

	status, err := adapter.Status(context.Background(), obj)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status["sourceRepos"] != "1" || status["destinations"] != "1" {
		t.Fatalf("unexpected status: %#v", status)
	}
}
