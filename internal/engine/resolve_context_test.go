package engine_test

import (
	"context"
	"testing"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/engine/adapters/argocd"
	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/kube"
	"github.com/lucasepe/kctx/internal/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestResolveContextDispatchesPodThroughResolver(t *testing.T) {
	pod := testPod("api-1", "payments")
	reader := testutil.NewFakeReader(pod)
	eng := engine.NewWithResolver(reader, fakeResolver{
		gvr:        corev1.SchemeGroupVersion.WithResource("pods"),
		gvk:        corev1.SchemeGroupVersion.WithKind("Pod"),
		namespaced: true,
	})

	got, err := eng.ResolveContext(context.Background(), engine.ResolveContextRequest{
		Resource:  "po",
		Namespace: "payments",
		Name:      "api-1",
	})
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	if got.Pod == nil || got.Pod.Pod.Name != "api-1" {
		t.Fatalf("expected typed Pod response, got %#v", got)
	}
	if got.Resolved.GVR.Resource != "pods" {
		t.Fatalf("resolved resource = %#v", got.Resolved)
	}
}

func TestResolveContextRequiresAdapterForDynamicResource(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	cm := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: "api-config", Namespace: "payments"},
	}
	reader := testutil.NewFakeReader()
	dynamicReader := kube.NewClientWithDynamic(kubefake.NewSimpleClientset(), fake.NewSimpleDynamicClient(scheme, cm))
	eng := engine.NewWithDynamicAndResolver(reader, dynamicReader, fakeResolver{
		gvr:        corev1.SchemeGroupVersion.WithResource("configmaps"),
		gvk:        corev1.SchemeGroupVersion.WithKind("ConfigMap"),
		namespaced: true,
	})

	_, err := eng.ResolveContext(context.Background(), engine.ResolveContextRequest{
		Resource:  "configmap",
		Namespace: "payments",
		Name:      "api-config",
	})
	if err == nil {
		t.Fatalf("ResolveContext() error = nil, want unsupported resource error")
	}
	if _, ok := err.(engine.UnsupportedResourceError); !ok {
		t.Fatalf("ResolveContext() error = %T %[1]v, want engine.UnsupportedResourceError", err)
	}
}

func TestResolveContextUsesDynamicAdapter(t *testing.T) {
	app := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":      "payments",
			"namespace": "argocd",
		},
		"status": map[string]any{
			"sync": map[string]any{"status": "OutOfSync"},
		},
	}}

	reader := testutil.NewFakeReader()
	dynamicReader := kube.NewClientWithDynamic(kubefake.NewSimpleClientset(), fake.NewSimpleDynamicClient(runtime.NewScheme(), app))
	eng := engine.NewWithDynamicAndResolver(reader, dynamicReader, fakeResolver{
		gvr:        schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
		gvk:        schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application"},
		namespaced: true,
	}, argocd.ApplicationAdapter{})

	got, err := eng.ResolveContext(context.Background(), engine.ResolveContextRequest{
		Resource:  "applications.argoproj.io",
		Namespace: "argocd",
		Name:      "payments",
	})
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	if got.Resource == nil || got.Resource.Resource.Kind != "Application" {
		t.Fatalf("expected dynamic Application response, got %#v", got)
	}
	if len(got.Resource.Signals) != 1 || got.Resource.Signals[0].Reason != "ApplicationSyncOutOfSync" {
		t.Fatalf("unexpected signals: %#v", got.Resource.Signals)
	}
}

func TestBuildGraphUsesDynamicGraphAdapter(t *testing.T) {
	app := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":      "payments",
			"namespace": "argocd",
		},
		"spec": map[string]any{
			"destination": map[string]any{
				"namespace": "payments",
				"server":    "https://kubernetes.default.svc",
			},
			"source": map[string]any{
				"repoURL": "https://github.com/example/payments.git",
			},
		},
		"status": map[string]any{
			"resources": []any{
				map[string]any{
					"group":     "apps",
					"version":   "v1",
					"kind":      "Deployment",
					"namespace": "payments",
					"name":      "api",
					"status":    "Synced",
				},
			},
		},
	}}

	reader := testutil.NewFakeReader()
	dynamicReader := kube.NewClientWithDynamic(kubefake.NewSimpleClientset(), fake.NewSimpleDynamicClient(runtime.NewScheme(), app))
	eng := engine.NewWithDynamicAndResolver(reader, dynamicReader, fakeResolver{
		gvr:        schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
		gvk:        schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application"},
		namespaced: true,
	}, argocd.ApplicationAdapter{})

	got, err := eng.BuildGraph(context.Background(), engine.BuildGraphRequest{
		Resource:  "applications.argoproj.io",
		Namespace: "argocd",
		Name:      "payments",
	})
	if err != nil {
		t.Fatalf("BuildGraph() error = %v", err)
	}
	if got.Kind != "ResourceGraph" {
		t.Fatalf("graph kind = %q, want ResourceGraph", got.Kind)
	}
	assertGraphNode(t, got.Nodes, "Application/argocd/payments")
	assertGraphNode(t, got.Nodes, "Deployment/payments/api")
	assertGraphEdge(t, got.Edges, "manages", "Application/argocd/payments", "Deployment/payments/api")
}

func assertGraphNode(t *testing.T, nodes []graph.Node, id string) {
	t.Helper()
	for _, node := range nodes {
		if node.ID == id {
			return
		}
	}
	t.Fatalf("missing graph node %q in %#v", id, nodes)
}

func assertGraphEdge(t *testing.T, edges []graph.Edge, edgeType, source, target string) {
	t.Helper()
	for _, edge := range edges {
		if edge.Type == edgeType && edge.Source == source && edge.Target == target {
			return
		}
	}
	t.Fatalf("missing graph edge %s %s -> %s in %#v", edgeType, source, target, edges)
}

type fakeResolver struct {
	gvr        schema.GroupVersionResource
	gvk        schema.GroupVersionKind
	namespaced bool
}

func (r fakeResolver) ResolveResource(ctx context.Context, input string) (*kube.ResolvedResource, error) {
	return &kube.ResolvedResource{Input: input, GVR: r.gvr, GVK: r.gvk, Namespaced: r.namespaced}, nil
}
