package certmanager_test

import (
	"context"
	"testing"
	"time"

	"github.com/lucasepe/kctx/internal/engine/adapters/certmanager"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCertificateAdapterExtractsHealthyContext(t *testing.T) {
	obj := certificateObject("True", "Ready", "Certificate is up to date", time.Now().Add(90*24*time.Hour).UTC().Format(time.RFC3339))

	adapter := certmanager.CertificateAdapter{}
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
	if entities[1].Kind != "Secret" || entities[1].Name != "api-tls" {
		t.Fatalf("unexpected Secret entity: %#v", entities[1])
	}

	relations, err := adapter.Relations(context.Background(), obj)
	if err != nil {
		t.Fatalf("Relations() error = %v", err)
	}
	if len(relations) != 2 {
		t.Fatalf("relations len = %d, want 2: %#v", len(relations), relations)
	}

	signals, err := adapter.Signals(context.Background(), obj)
	if err != nil {
		t.Fatalf("Signals() error = %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("signals len = %d, want 0: %#v", len(signals), signals)
	}
}

func TestCertificateAdapterSignalsUnhealthyAndExpiringCertificate(t *testing.T) {
	obj := certificateObject("False", "DoesNotExist", "secret token=plain is missing", time.Now().Add(24*time.Hour).UTC().Format(time.RFC3339))
	obj.Object["status"].(map[string]any)["conditions"] = append(obj.Object["status"].(map[string]any)["conditions"].([]any),
		map[string]any{"type": "Issuing", "status": "True", "reason": "DoesNotExist", "message": "Issuing replacement certificate"},
	)

	adapter := certmanager.CertificateAdapter{}
	signals, err := adapter.Signals(context.Background(), obj)
	if err != nil {
		t.Fatalf("Signals() error = %v", err)
	}
	if len(signals) != 3 {
		t.Fatalf("signals len = %d, want 3: %#v", len(signals), signals)
	}
	if signals[0].Message != "secret token=[REDACTED] is missing" {
		t.Fatalf("ready signal message = %q, want redacted", signals[0].Message)
	}

	status, err := adapter.Status(context.Background(), obj)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status["ready"] != "False" || status["readyReason"] != "DoesNotExist" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status["readyMessage"] != "secret token=[REDACTED] is missing" {
		t.Fatalf("readyMessage = %q, want redacted", status["readyMessage"])
	}
}

func certificateObject(readyStatus, readyReason, readyMessage, notAfter string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Certificate",
		"metadata": map[string]any{
			"name":      "api",
			"namespace": "payments",
			"labels": map[string]any{
				"app": "api",
			},
		},
		"spec": map[string]any{
			"secretName": "api-tls",
			"issuerRef": map[string]any{
				"name": "letsencrypt",
				"kind": "Issuer",
			},
		},
		"status": map[string]any{
			"notAfter":    notAfter,
			"renewalTime": time.Now().Add(12 * time.Hour).UTC().Format(time.RFC3339),
			"conditions": []any{
				map[string]any{"type": "Ready", "status": readyStatus, "reason": readyReason, "message": readyMessage},
			},
		},
	}}
}
