package engine_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/render"
	"github.com/lucasepe/kctx/internal/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNamespaceHealthEmptyNamespace(t *testing.T) {
	got := namespaceHealth(t, "payments")

	if got.Summary.PodsTotal != 0 || got.Summary.ServicesTotal != 0 || got.Summary.WorkloadsTotal != 0 {
		t.Fatalf("unexpected non-empty summary: %#v", got.Summary)
	}
	assertHealthSignal(t, got, "namespace_empty")
}

func TestNamespaceHealthHealthyNamespace(t *testing.T) {
	pod := backendPod("api-1", "payments", "10.244.1.10", true)
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.10", true, "api-1", "worker-01"))
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "api-data", Namespace: "payments"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}

	got := namespaceHealth(t, "payments", pod, service, slice, pvc)

	if got.Summary.PodsTotal != 1 || got.Summary.PodsReady != 1 || got.Summary.ServicesWithoutEndpoints != 0 || got.Summary.PVCsPending != 0 {
		t.Fatalf("unexpected healthy summary: %#v", got.Summary)
	}
	if len(got.Signals) != 0 {
		t.Fatalf("expected no signals, got %#v", got.Signals)
	}
}

func TestNamespaceHealthPodCrashLoopBackOff(t *testing.T) {
	pod := backendPod("api-1", "payments", "10.244.1.10", false)
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         "api",
		RestartCount: 7,
		State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
	}}

	got := namespaceHealth(t, "payments", pod)

	assertHealthSignal(t, got, "pod_crashloop")
	assertHealthSignal(t, got, "high_restart_count")
	if got.Summary.CriticalSignals != 1 {
		t.Fatalf("expected one critical signal, got %#v", got.Summary)
	}
}

func TestNamespaceHealthPodImagePullBackOff(t *testing.T) {
	pod := backendPod("api-1", "payments", "", false)
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:  "api",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
	}}

	got := namespaceHealth(t, "payments", pod)

	assertHealthSignal(t, got, "pod_image_pull_error")
}

func TestNamespaceHealthDeploymentUnavailableReplicas(t *testing.T) {
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 2, AvailableReplicas: 2},
	}

	got := namespaceHealth(t, "payments", deploy)

	assertHealthSignal(t, got, "workload_replicas_unavailable")
	if got.Summary.WorkloadsUnhealthy != 1 {
		t.Fatalf("expected unhealthy workload, got %#v", got.Summary)
	}
}

func TestNamespaceHealthServiceWithoutReadyEndpoints(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.10", false, "api-1", "worker-01"))

	got := namespaceHealth(t, "payments", service, slice)

	assertHealthSignal(t, got, "service_without_ready_endpoints")
	if got.Summary.ServicesWithoutEndpoints != 1 {
		t.Fatalf("expected service without ready endpoints, got %#v", got.Summary)
	}
}

func TestNamespaceHealthPVCPending(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "api-data", Namespace: "payments"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	}

	got := namespaceHealth(t, "payments", pvc)

	assertHealthSignal(t, got, "pvc_pending")
	if got.Summary.PVCsPending != 1 {
		t.Fatalf("expected pending PVC, got %#v", got.Summary)
	}
}

func TestNamespaceHealthRecentWarningEvents(t *testing.T) {
	pod := testPod("api-1", "payments")
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	oldWarning := podEvent("old", pod, now.Add(-time.Hour), "Warning", "FailedScheduling", "old warning")
	newWarning := podEvent("new", pod, now, "Warning", "BackOff", "new warning")
	normal := podEvent("normal", pod, now.Add(time.Minute), "Normal", "Pulled", "normal event")

	got := namespaceHealth(t, "payments", oldWarning, newWarning, normal)

	if got.Summary.WarningEvents != 2 {
		t.Fatalf("expected two warning events, got %#v", got.Summary)
	}
	if len(got.Events) != 2 || got.Events[0].Reason != "BackOff" {
		t.Fatalf("warning events not sorted newest first: %#v", got.Events)
	}
	assertHealthSignal(t, got, "pod_backoff_event")
}

func TestNamespaceHealthJSONOutputDeterministic(t *testing.T) {
	pod := backendPod("api-1", "payments", "10.244.1.10", true)
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.10", true, "api-1", "worker-01"))
	got := namespaceHealth(t, "payments", service, pod, slice)

	var first bytes.Buffer
	var second bytes.Buffer
	if err := render.JSON(&first, got); err != nil {
		t.Fatalf("render JSON first: %v", err)
	}
	if err := render.JSON(&second, got); err != nil {
		t.Fatalf("render JSON second: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("JSON output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
	for _, want := range []string{`"namespace"`, `"summary"`, `"pods"`, `"services"`} {
		if !strings.Contains(first.String(), want) {
			t.Fatalf("missing %s in JSON:\n%s", want, first.String())
		}
	}
}

func namespaceHealth(t *testing.T, namespace string, objects ...runtime.Object) *engine.NamespaceHealthResponse {
	t.Helper()
	reader := testutil.NewFakeReader(objects...)
	got, err := engine.New(reader).NamespaceHealth(context.Background(), engine.NamespaceHealthRequest{Namespace: namespace})
	if err != nil {
		t.Fatalf("NamespaceHealth() error = %v", err)
	}
	return got
}

func assertHealthSignal(t *testing.T, got *engine.NamespaceHealthResponse, reason string) {
	t.Helper()
	for _, signal := range got.Signals {
		if signal.Reason == reason {
			return
		}
	}
	t.Fatalf("missing signal %s in %#v", reason, got.Signals)
}
