package engine_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/render"
	"github.com/lucasepe/kctx/internal/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildPodGraphOwnerChain(t *testing.T) {
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

	got := buildGraph(t, pod, rs, deploy)

	assertNode(t, got, "Pod/payments/api-abc")
	assertNode(t, got, "ReplicaSet/payments/api-7d9f8")
	assertNode(t, got, "Deployment/payments/api")
	assertEdge(t, got, "owns", "Deployment/payments/api", "ReplicaSet/payments/api-7d9f8")
	assertEdge(t, got, "owns", "ReplicaSet/payments/api-7d9f8", "Pod/payments/api-abc")
}

func TestBuildPodGraphServiceSelector(t *testing.T) {
	pod := testPod("api", "payments")
	pod.Labels = map[string]string{"app": "api"}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
	}

	got := buildGraph(t, pod, service)

	assertNode(t, got, "Service/payments/api")
	assertEdge(t, got, "selects", "Service/payments/api", "Pod/payments/api")
}

func TestBuildPodGraphDependencies(t *testing.T) {
	pod := testPod("api", "payments")
	pod.Spec.Volumes = []corev1.Volume{
		{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"}}}},
		{Name: "secret", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "db-credentials"}}},
		{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "api-data"}}},
	}

	got := buildGraph(t, pod)

	assertNode(t, got, "ConfigMap/payments/api-config")
	assertNode(t, got, "Secret/payments/db-credentials")
	assertNode(t, got, "PersistentVolumeClaim/payments/api-data")
	assertEdge(t, got, "uses_configmap", "Pod/payments/api", "ConfigMap/payments/api-config")
	assertEdge(t, got, "uses_secret", "Pod/payments/api", "Secret/payments/db-credentials")
	assertEdge(t, got, "mounts_pvc", "Pod/payments/api", "PersistentVolumeClaim/payments/api-data")
}

func TestBuildPodGraphNodeScheduling(t *testing.T) {
	pod := testPod("api", "payments")
	pod.Spec.NodeName = "worker-01"
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-01"}}

	got := buildGraph(t, pod, node)

	assertNode(t, got, "Node/worker-01")
	assertEdge(t, got, "scheduled_on", "Node/worker-01", "Pod/payments/api")
}

func TestBuildPodGraphDeduplicatesNodesAndEdges(t *testing.T) {
	pod := testPod("api", "payments")
	pod.Spec.Volumes = []corev1.Volume{
		{Name: "config-a", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"}}}},
		{Name: "config-b", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"}}}},
	}
	pod.Spec.Containers = []corev1.Container{{
		Name: "api",
		EnvFrom: []corev1.EnvFromSource{
			{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"}}},
		},
	}}

	got := buildGraph(t, pod)

	if countNodes(got, "ConfigMap/payments/api-config") != 1 {
		t.Fatalf("expected one ConfigMap node, got %#v", got.Nodes)
	}
	if countEdges(got, "uses_configmap", "Pod/payments/api", "ConfigMap/payments/api-config") != 1 {
		t.Fatalf("expected one ConfigMap edge, got %#v", got.Edges)
	}
}

func TestMermaidGraphRendering(t *testing.T) {
	g := &graph.Graph{
		Nodes: []graph.Node{
			{ID: "Deployment/payments/api", Kind: "Deployment", Namespace: "payments", Name: "api"},
			{ID: "Pod/payments/api-abc", Kind: "Pod", Namespace: "payments", Name: "api-abc"},
			{ID: "ReplicaSet/payments/api-7d9f8", Kind: "ReplicaSet", Namespace: "payments", Name: "api-7d9f8"},
		},
		Edges: []graph.Edge{
			{Type: "owns", Source: "Deployment/payments/api", Target: "ReplicaSet/payments/api-7d9f8"},
			{Type: "owns", Source: "ReplicaSet/payments/api-7d9f8", Target: "Pod/payments/api-abc"},
		},
	}
	var out bytes.Buffer

	if err := render.MermaidGraph(&out, g); err != nil {
		t.Fatalf("MermaidGraph() error = %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"graph TD",
		"Deployment_payments_api[Deployment api]",
		"Deployment_payments_api -->|owns| ReplicaSet_payments_api_7d9f8",
		"ReplicaSet_payments_api_7d9f8 -->|owns| Pod_payments_api_abc",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in mermaid output:\n%s", want, text)
		}
	}
}

func TestDOTGraphRendering(t *testing.T) {
	g := &graph.Graph{
		Nodes: []graph.Node{
			{ID: "Pod/payments/api-abc", Kind: "Pod", Namespace: "payments", Name: "api-abc"},
			{ID: "ReplicaSet/payments/api-7d9f8", Kind: "ReplicaSet", Namespace: "payments", Name: "api-7d9f8"},
		},
		Edges: []graph.Edge{
			{Type: "owns", Source: "ReplicaSet/payments/api-7d9f8", Target: "Pod/payments/api-abc"},
		},
	}
	var out bytes.Buffer

	if err := render.DOTGraph(&out, g); err != nil {
		t.Fatalf("DOTGraph() error = %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"digraph G {",
		"\"ReplicaSet/payments/api-7d9f8\" -> \"Pod/payments/api-abc\" [label=\"owns\"]",
		"}",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in DOT output:\n%s", want, text)
		}
	}
}

func buildGraph(t *testing.T, objects ...runtime.Object) *graph.Graph {
	t.Helper()
	var target *corev1.Pod
	for _, object := range objects {
		if pod, ok := object.(*corev1.Pod); ok && target == nil {
			target = pod
		}
	}
	if target == nil {
		t.Fatal("buildGraph helper requires a pod object")
	}
	reader := testutil.NewFakeReader(objects...)
	got, err := engine.New(reader).BuildPodGraph(context.Background(), engine.BuildPodGraphRequest{Namespace: target.Namespace, Name: target.Name})
	if err != nil {
		t.Fatalf("BuildPodGraph() error = %v", err)
	}
	return got
}

func assertNode(t *testing.T, got *graph.Graph, id string) {
	t.Helper()
	if countNodes(got, id) == 0 {
		t.Fatalf("missing node %s in %#v", id, got.Nodes)
	}
}

func assertEdge(t *testing.T, got *graph.Graph, edgeType, source, target string) {
	t.Helper()
	if countEdges(got, edgeType, source, target) == 0 {
		t.Fatalf("missing edge %s %s -> %s in %#v", edgeType, source, target, got.Edges)
	}
}

func countNodes(got *graph.Graph, id string) int {
	count := 0
	for _, node := range got.Nodes {
		if node.ID == id {
			count++
		}
	}
	return count
}

func countEdges(got *graph.Graph, edgeType, source, target string) int {
	count := 0
	for _, edge := range got.Edges {
		if edge.Type == edgeType && edge.Source == source && edge.Target == target {
			count++
		}
	}
	return count
}
