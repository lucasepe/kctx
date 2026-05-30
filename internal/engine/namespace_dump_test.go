package engine_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/model"
	"github.com/lucasepe/kctx/internal/render"
	"github.com/lucasepe/kctx/internal/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestDumpNamespaceEmptyNamespace(t *testing.T) {
	got := dumpNamespace(t, "payments", testNamespace("payments"))

	if got.Namespace != "payments" {
		t.Fatalf("unexpected namespace: %s", got.Namespace)
	}
	if got.Summary.Pods != 0 || got.Summary.Services != 0 || got.Summary.Signals != 0 {
		t.Fatalf("unexpected empty summary: %#v", got.Summary)
	}
	assertDumpEntity(t, got, "Namespace/payments")
}

func TestDumpNamespacePodOwnershipChains(t *testing.T) {
	pod := testPod("api-abc", "payments")
	pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-7d9f8"}}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-7d9f8",
			Namespace: "payments",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "api"},
			},
		},
	}
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"}}

	got := dumpNamespace(t, "payments", testNamespace("payments"), pod, rs, deploy)

	assertDumpRelation(t, got, "owns", "ReplicaSet/payments/api-7d9f8", "Pod/payments/api-abc")
	assertDumpRelation(t, got, "owns", "Deployment/payments/api", "ReplicaSet/payments/api-7d9f8")
}

func TestDumpNamespaceServiceSelectorRelations(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	pod := backendPod("api-1", "payments", "10.244.1.10", true)

	got := dumpNamespace(t, "payments", testNamespace("payments"), service, pod)

	assertDumpRelation(t, got, "selects", "Service/payments/payments-api", "Pod/payments/api-1")
}

func TestDumpNamespaceEndpointSliceRelations(t *testing.T) {
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	pod := backendPod("api-1", "payments", "10.244.1.10", true)
	slice := endpointSlice("payments-api", "payments", sliceEndpoint("10.244.1.10", true, "api-1", "worker-01"))

	got := dumpNamespace(t, "payments", testNamespace("payments"), service, pod, slice)

	assertDumpRelation(t, got, "has_endpoint", "Service/payments/payments-api", "EndpointSlice/payments/payments-api-slice")
	assertDumpRelation(t, got, "endpoint_targets", "EndpointSlice/payments/payments-api-slice", "Pod/payments/api-1")
}

func TestDumpNamespacePVCRelations(t *testing.T) {
	pod := testPod("api-1", "payments")
	pod.Spec.Volumes = []corev1.Volume{{
		Name:         "data",
		VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "api-data"}},
	}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "api-data", Namespace: "payments"}}

	got := dumpNamespace(t, "payments", testNamespace("payments"), pod, pvc)

	assertDumpRelation(t, got, "mounts_pvc", "Pod/payments/api-1", "PersistentVolumeClaim/payments/api-data")
}

func TestDumpNamespaceSecretAndConfigMapRelations(t *testing.T) {
	pod := testPod("api-1", "payments")
	pod.Spec.Volumes = []corev1.Volume{
		{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"}}}},
		{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "db-credentials"}}},
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "api-config", Namespace: "payments"}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "db-credentials", Namespace: "payments"}, Type: corev1.SecretTypeOpaque}

	got := dumpNamespace(t, "payments", testNamespace("payments"), pod, cm, secret)

	assertDumpRelation(t, got, "uses_configmap", "Pod/payments/api-1", "ConfigMap/payments/api-config")
	assertDumpRelation(t, got, "uses_secret", "Pod/payments/api-1", "Secret/payments/db-credentials")
}

func TestDumpNamespaceSignalGeneration(t *testing.T) {
	pod := backendPod("api-1", "payments", "10.244.1.10", false)
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:  "api",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"},
		},
	}}
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "api-data", Namespace: "payments"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	}

	got := dumpNamespace(t, "payments", testNamespace("payments"), pod, service, pvc)

	assertDumpSignal(t, got, "pod_crashloop")
	assertDumpSignal(t, got, "pod_not_ready")
	assertDumpSignal(t, got, "service_without_ready_endpoints")
	assertDumpSignal(t, got, "pvc_pending")
	entity := dumpEntity(t, got, "Pod/payments/api-1")
	if entity.LastState != "terminated" || entity.LastReason != "OOMKilled" {
		t.Fatalf("expected pod last state in dump entity, got %#v", entity)
	}
}

func TestDumpNamespaceAggregatesWarningEventSignals(t *testing.T) {
	pod := backendPod("api-1", "payments", "10.244.1.10", false)
	first := podEvent("one", pod, metav1.Now().Time, "Warning", "Unhealthy", "Liveness probe failed: timeout")
	second := podEvent("two", pod, metav1.Now().Time.Add(time.Minute), "Warning", "Unhealthy", "Liveness probe failed: timeout")
	third := podEvent("three", pod, metav1.Now().Time.Add(2*time.Minute), "Warning", "BackOff", "Back-off restarting failed container api")

	got := dumpNamespace(t, "payments", testNamespace("payments"), pod, first, second, third)

	assertDumpSignal(t, got, "probe_warning_events")
	assertDumpSignal(t, got, "pod_backoff_events")
	assertNoDumpSignal(t, got, "warning_event_present")
}

func TestDumpNamespaceNoDuplicateNodeSchedulingRelations(t *testing.T) {
	pod := backendPod("api-1", "payments", "10.244.1.10", true)
	pod.Spec.NodeName = "worker-01"

	got := dumpNamespace(t, "payments", testNamespace("payments"), pod)

	assertDumpRelation(t, got, "scheduled_on", "Pod/payments/api-1", "Node/worker-01")
	assertNoDumpRelation(t, got, "runs_on")
}

func TestDumpNamespaceSecretDataExcluded(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-credentials", Namespace: "payments"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"password": []byte("super-secret-value")},
	}

	got := dumpNamespace(t, "payments", testNamespace("payments"), secret)
	var out bytes.Buffer
	if err := render.JSON(&out, got); err != nil {
		t.Fatalf("render JSON: %v", err)
	}
	text := out.String()
	if strings.Contains(text, "super-secret-value") || strings.Contains(text, "password") {
		t.Fatalf("secret data leaked in dump JSON:\n%s", text)
	}
	assertDumpEntity(t, got, "Secret/payments/db-credentials")
}

func TestDumpNamespaceDeterministicSorting(t *testing.T) {
	podB := testPod("b", "payments")
	podA := testPod("a", "payments")
	serviceB := testService("b-svc", "payments", map[string]string{"app": "b"})
	serviceA := testService("a-svc", "payments", map[string]string{"app": "a"})

	got := dumpNamespace(t, "payments", testNamespace("payments"), podB, serviceB, podA, serviceA)

	for i := 1; i < len(got.Entities); i++ {
		prev := got.Entities[i-1].Kind + "/" + got.Entities[i-1].Name
		next := got.Entities[i].Kind + "/" + got.Entities[i].Name
		if prev > next {
			t.Fatalf("entities not sorted: %#v", got.Entities)
		}
	}
	for i := 1; i < len(got.Relations); i++ {
		prev := got.Relations[i-1].Source + "/" + got.Relations[i-1].Type + "/" + got.Relations[i-1].Target
		next := got.Relations[i].Source + "/" + got.Relations[i].Type + "/" + got.Relations[i].Target
		if prev > next {
			t.Fatalf("relations not sorted: %#v", got.Relations)
		}
	}
}

func TestDumpNamespaceJSONSerializationStable(t *testing.T) {
	pod := backendPod("api-1", "payments", "10.244.1.10", true)
	service := testService("payments-api", "payments", map[string]string{"app": "api"})
	got := dumpNamespace(t, "payments", testNamespace("payments"), pod, service)

	var first bytes.Buffer
	var second bytes.Buffer
	if err := render.JSON(&first, got); err != nil {
		t.Fatalf("render JSON first: %v", err)
	}
	if err := render.JSON(&second, got); err != nil {
		t.Fatalf("render JSON second: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("JSON output is not stable:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
}

func dumpNamespace(t *testing.T, namespace string, objects ...runtime.Object) *engine.DumpNamespaceResponse {
	t.Helper()
	reader := testutil.NewFakeReader(objects...)
	got, err := engine.New(reader).DumpNamespace(context.Background(), engine.DumpNamespaceRequest{Namespace: namespace})
	if err != nil {
		t.Fatalf("DumpNamespace() error = %v", err)
	}
	return got
}

func testNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}
}

func assertDumpEntity(t *testing.T, got *engine.DumpNamespaceResponse, id string) {
	t.Helper()
	_ = dumpEntity(t, got, id)
}

func dumpEntity(t *testing.T, got *engine.DumpNamespaceResponse, id string) model.DumpEntity {
	t.Helper()
	for _, entity := range got.Entities {
		if entity.ID == id {
			return entity
		}
	}
	t.Fatalf("missing entity %s in %#v", id, got.Entities)
	return model.DumpEntity{}
}

func assertDumpRelation(t *testing.T, got *engine.DumpNamespaceResponse, relationType, source, target string) {
	t.Helper()
	for _, relation := range got.Relations {
		if relation.Type == relationType && relation.Source == source && relation.Target == target {
			return
		}
	}
	t.Fatalf("missing relation %s %s -> %s in %#v", relationType, source, target, got.Relations)
}

func assertDumpSignal(t *testing.T, got *engine.DumpNamespaceResponse, code string) {
	t.Helper()
	for _, signal := range got.Signals {
		if signal.Code == code {
			return
		}
	}
	t.Fatalf("missing signal %s in %#v", code, got.Signals)
}

func assertNoDumpSignal(t *testing.T, got *engine.DumpNamespaceResponse, code string) {
	t.Helper()
	for _, signal := range got.Signals {
		if signal.Code == code {
			t.Fatalf("unexpected signal %s in %#v", code, got.Signals)
		}
	}
}

func assertNoDumpRelation(t *testing.T, got *engine.DumpNamespaceResponse, relationType string) {
	t.Helper()
	for _, relation := range got.Relations {
		if relation.Type == relationType {
			t.Fatalf("unexpected relation %s in %#v", relationType, got.Relations)
		}
	}
}
