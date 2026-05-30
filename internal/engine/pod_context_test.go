package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func TestResolvePodContextMinimalPod(t *testing.T) {
	pod := testPod("api", "payments")
	pod.Status.Phase = corev1.PodRunning
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}

	got := resolve(t, pod)

	if got.Pod.Name != "api" || got.Pod.Namespace != "payments" {
		t.Fatalf("unexpected pod entity: %#v", got.Pod)
	}
	if !got.Status.Ready {
		t.Fatal("expected pod to be ready")
	}
	if len(got.Owners) != 0 || len(got.Services) != 0 || len(got.Events) != 0 {
		t.Fatalf("expected no correlated objects, got owners=%d services=%d events=%d", len(got.Owners), len(got.Services), len(got.Events))
	}
}

func TestResolvePodContextOwnerChain(t *testing.T) {
	pod := testPod("api-abc", "payments")
	pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-7d9f8", UID: "rs-uid"}}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-7d9f8",
			Namespace: "payments",
			UID:       "rs-uid",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "deploy-uid"},
			},
		},
	}
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments", UID: "deploy-uid"}}

	got := resolve(t, pod, rs, deploy)

	if len(got.Owners) != 2 {
		t.Fatalf("expected 2 owners, got %d", len(got.Owners))
	}
	if got.Owners[0].Kind != "ReplicaSet" || got.Owners[1].Kind != "Deployment" {
		t.Fatalf("unexpected owners: %#v", got.Owners)
	}
	assertRelation(t, got, "owned_by", "Pod", "ReplicaSet")
	assertRelation(t, got, "owned_by", "ReplicaSet", "Deployment")
}

func TestResolvePodContextServiceSelector(t *testing.T) {
	pod := testPod("api", "payments")
	pod.Labels = map[string]string{"app": "api", "tier": "backend"}
	matching := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
	}
	other := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "payments"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}},
	}

	got := resolve(t, pod, matching, other)

	if len(got.Services) != 1 || got.Services[0].Name != "api" {
		t.Fatalf("unexpected services: %#v", got.Services)
	}
	assertRelation(t, got, "selected_by", "Pod", "Service")
}

func TestResolvePodContextDependencies(t *testing.T) {
	pod := testPod("api", "payments")
	pod.Spec.Volumes = []corev1.Volume{
		{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"}}}},
		{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "db-credentials"}}},
		{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "api-data"}}},
	}
	pod.Spec.Containers = []corev1.Container{{
		Name: "api",
		EnvFrom: []corev1.EnvFromSource{
			{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "runtime-config"}}},
			{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "runtime-secret"}}},
		},
		Env: []corev1.EnvVar{
			{Name: "TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "token-secret"}, Key: "token"}}},
		},
	}}

	got := resolve(t, pod)

	want := map[string]bool{
		"ConfigMap/api-config":     false,
		"ConfigMap/runtime-config": false,
		"Secret/db-credentials":    false,
		"Secret/runtime-secret":    false,
		"Secret/token-secret":      false,
		"PVC/api-data":             false,
	}
	for _, volume := range got.Volumes {
		key := volume.Type + "/" + volume.Name
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, seen := range want {
		if !seen {
			t.Fatalf("missing dependency %s in %#v", key, got.Volumes)
		}
	}
	assertRelation(t, got, "mounts_pvc", "Pod", "PVC")
	assertRelation(t, got, "uses_configmap", "Pod", "ConfigMap")
	assertRelation(t, got, "uses_secret", "Pod", "Secret")
}

func TestResolvePodContextCrashLoopSignal(t *testing.T) {
	pod := testPod("api", "payments")
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         "api",
		RestartCount: 7,
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
		},
	}}

	got := resolve(t, pod)

	assertSignal(t, got, "container_crashloop")
	assertSignal(t, got, "container_restart_cycle")
	assertSignal(t, got, "high_restart_count")
	assertSignal(t, got, "pod_not_ready")
}

func TestResolvePodContextOOMKilledSignal(t *testing.T) {
	pod := testPod("api", "payments")
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:         "api",
		Ready:        true,
		RestartCount: 1,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"},
		},
	}}

	got := resolve(t, pod)

	assertSignal(t, got, "container_oom_killed")
	assertSignal(t, got, "container_restart_cycle")
	assertSignal(t, got, "container_restarted")
}

func TestResolvePodContextWarningEvents(t *testing.T) {
	pod := testPod("api", "payments")
	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour)
	eventOld := podEvent("old", pod, old, "Warning", "FailedScheduling", "0/3 nodes are available")
	eventNew := podEvent("new", pod, now, "Warning", "Unhealthy", "Liveness probe failed")
	unrelated := podEvent("other", testPod("worker", "payments"), now.Add(time.Minute), "Normal", "Pulled", "Pulled image")
	normal := podEvent("normal", pod, now.Add(2*time.Minute), "Normal", "Started", "Container started")

	got := resolve(t, pod, eventOld, eventNew, unrelated, normal)

	if len(got.Events) != 2 {
		t.Fatalf("expected 2 pod events, got %d", len(got.Events))
	}
	if got.Events[0].Reason != "Unhealthy" || got.Events[1].Reason != "FailedScheduling" {
		t.Fatalf("events not sorted most recent first: %#v", got.Events)
	}
	assertSignal(t, got, "transient_probe_warnings")
	assertSignal(t, got, "resolved_scheduling_warnings")
	assertNoRelation(t, got, "event_for")
}

func TestResolvePodContextWarningEventsDedupedAndLimited(t *testing.T) {
	pod := testPod("api", "payments")
	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	objects := []runtime.Object{pod}
	for i := 0; i < 12; i++ {
		message := "warning"
		if i%2 == 0 {
			message = "repeated warning"
		}
		objects = append(objects, podEvent("event-"+string(rune('a'+i)), pod, now.Add(time.Duration(i)*time.Minute), "Warning", "Unhealthy", message))
	}

	got := resolve(t, objects...)

	if len(got.Events) != 2 {
		t.Fatalf("expected deduped warning events, got %d: %#v", len(got.Events), got.Events)
	}
	assertNoRelation(t, got, "event_for")
}

func resolve(t *testing.T, objects ...runtime.Object) *engine.ResolvePodContextResponse {
	t.Helper()
	var target *corev1.Pod
	for _, object := range objects {
		if pod, ok := object.(*corev1.Pod); ok && target == nil {
			target = pod
		}
	}
	if target == nil {
		t.Fatal("resolve helper requires a pod object")
	}
	reader := testutil.NewFakeReader(objects...)
	got, err := engine.New(reader).ResolvePodContext(context.Background(), engine.ResolvePodContextRequest{Namespace: target.Namespace, Name: target.Name})
	if err != nil {
		t.Fatalf("ResolvePodContext() error = %v", err)
	}
	return got
}

func testPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(namespace + "-" + name + "-uid")},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
}

func podEvent(name string, pod *corev1.Pod, at time.Time, eventType, reason, message string) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: pod.Namespace},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Namespace: pod.Namespace,
			Name:      pod.Name,
			UID:       pod.UID,
		},
		Type:          eventType,
		Reason:        reason,
		Message:       message,
		LastTimestamp: metav1.NewTime(at),
	}
}

func assertRelation(t *testing.T, got *engine.ResolvePodContextResponse, relationType, sourceKind, targetKind string) {
	t.Helper()
	for _, relation := range got.Relations {
		if relation.Type == relationType && relation.Source.Kind == sourceKind && relation.Target.Kind == targetKind {
			return
		}
	}
	t.Fatalf("missing relation %s %s->%s in %#v", relationType, sourceKind, targetKind, got.Relations)
}

func assertNoRelation(t *testing.T, got *engine.ResolvePodContextResponse, relationType string) {
	t.Helper()
	for _, relation := range got.Relations {
		if relation.Type == relationType {
			t.Fatalf("unexpected relation %s in %#v", relationType, got.Relations)
		}
	}
}

func assertSignal(t *testing.T, got *engine.ResolvePodContextResponse, reason string) {
	t.Helper()
	for _, signal := range got.Signals {
		if signal.Reason == reason {
			return
		}
	}
	t.Fatalf("missing signal %s in %#v", reason, got.Signals)
}
