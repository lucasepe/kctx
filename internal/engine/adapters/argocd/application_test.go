package argocd_test

import (
	"context"
	"testing"

	"github.com/lucasepe/kctx/internal/engine/adapters/argocd"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func TestApplicationAdapterExtractsContextFromUnstructuredStatus(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":      "payments",
			"namespace": "argocd",
			"uid":       "app-uid",
			"labels": map[string]any{
				"team": "platform",
			},
		},
		"spec": map[string]any{
			"project": "platform",
			"destination": map[string]any{
				"namespace": "payments",
				"server":    "https://kubernetes.default.svc",
			},
			"source": map[string]any{
				"repoURL": "https://github.com/example/payments.git",
			},
		},
		"status": map[string]any{
			"health": map[string]any{
				"status": "Degraded",
			},
			"sync": map[string]any{
				"status": "OutOfSync",
			},
			"operationState": map[string]any{
				"phase":   "Failed",
				"message": "sync failed token=plain-token",
			},
			"resources": []any{
				map[string]any{
					"group":     "apps",
					"version":   "v1",
					"kind":      "Deployment",
					"namespace": "payments",
					"name":      "api",
					"health":    "Missing",
				},
				map[string]any{
					"version":   "v1",
					"kind":      "Service",
					"namespace": "payments",
					"name":      "api",
					"health":    "Healthy",
				},
			},
		},
	}}
	obj.SetUID(types.UID("app-uid"))

	adapter := argocd.ApplicationAdapter{}
	if !adapter.Supports(obj) {
		t.Fatalf("Supports() = false")
	}

	entities, err := adapter.Entities(context.Background(), obj)
	if err != nil {
		t.Fatalf("Entities() error = %v", err)
	}
	if len(entities) != 3 {
		t.Fatalf("entities len = %d, want 3: %#v", len(entities), entities)
	}
	if entities[0].Kind != "Application" || entities[0].Name != "payments" || entities[0].Labels["team"] != "platform" {
		t.Fatalf("unexpected application entity: %#v", entities[0])
	}
	if entities[1].APIVersion != "apps/v1" || entities[1].Kind != "Deployment" {
		t.Fatalf("unexpected managed entity: %#v", entities[1])
	}

	relations, err := adapter.Relations(context.Background(), obj)
	if err != nil {
		t.Fatalf("Relations() error = %v", err)
	}
	if len(relations) != 6 {
		t.Fatalf("relations len = %d, want 6: %#v", len(relations), relations)
	}

	signals, err := adapter.Signals(context.Background(), obj)
	if err != nil {
		t.Fatalf("Signals() error = %v", err)
	}
	if len(signals) != 4 {
		t.Fatalf("signals len = %d, want 4: %#v", len(signals), signals)
	}
	if signals[0].Reason != "ApplicationHealthDegraded" || signals[0].Severity != "error" {
		t.Fatalf("unexpected first signal: %#v", signals[0])
	}
	if signals[2].Message != "sync failed token=[REDACTED]" {
		t.Fatalf("operation signal message = %q, want redacted", signals[2].Message)
	}

	status, err := adapter.Status(context.Background(), obj)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status["operationMessage"] != "sync failed token=[REDACTED]" {
		t.Fatalf("operationMessage = %q, want redacted", status["operationMessage"])
	}
}
