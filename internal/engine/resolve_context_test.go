package engine_test

import (
	"context"
	"testing"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/kube"
	"github.com/lucasepe/kctx/internal/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestResolveContextFallsBackToDynamicResource(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "api-config", Namespace: "payments"}}
	reader := testutil.NewFakeReader()
	dynamicReader := kube.NewClientWithDynamic(kubefake.NewSimpleClientset(), fake.NewSimpleDynamicClient(scheme, cm))
	eng := engine.NewWithDynamicAndResolver(reader, dynamicReader, fakeResolver{
		gvr:        corev1.SchemeGroupVersion.WithResource("configmaps"),
		gvk:        corev1.SchemeGroupVersion.WithKind("ConfigMap"),
		namespaced: true,
	})

	got, err := eng.ResolveContext(context.Background(), engine.ResolveContextRequest{
		Resource:  "configmap",
		Namespace: "payments",
		Name:      "api-config",
	})
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	if got.Resource == nil || got.Resource.Resource.Kind != "ConfigMap" {
		t.Fatalf("expected dynamic resource response, got %#v", got)
	}
}

type fakeResolver struct {
	gvr        schema.GroupVersionResource
	gvk        schema.GroupVersionKind
	namespaced bool
}

func (r fakeResolver) ResolveResource(ctx context.Context, input string) (*kube.ResolvedResource, error) {
	return &kube.ResolvedResource{Input: input, GVR: r.gvr, GVK: r.gvk, Namespaced: r.namespaced}, nil
}
