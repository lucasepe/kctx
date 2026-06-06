package engine_test

import (
	"context"
	"testing"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestDynamicEngineResolveResourceContextGenericFallback(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-config",
			Namespace: "payments",
			UID:       "cm-uid",
			Labels:    map[string]string{"app": "api"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "api",
				UID:        "deploy-uid",
			}},
		},
	}
	reader := kube.NewClientWithDynamic(
		kubefake.NewSimpleClientset(),
		fake.NewSimpleDynamicClient(scheme, cm),
	)

	got, err := engine.NewDynamicEngine(reader).ResolveResourceContext(context.Background(), engine.ResolveResourceContextRequest{
		Resource:  kube.ResourceRef{Group: "", Version: "v1", Resource: "configmaps"},
		Namespace: "payments",
		Name:      "api-config",
	})
	if err != nil {
		t.Fatalf("ResolveResourceContext() error = %v", err)
	}
	if got.Resource.Kind != "ConfigMap" || got.Resource.Name != "api-config" || got.Resource.UID != "cm-uid" {
		t.Fatalf("unexpected resource: %#v", got.Resource)
	}
	if len(got.Owners) != 1 || got.Owners[0].Kind != "Deployment" || got.Owners[0].Name != "api" {
		t.Fatalf("unexpected owners: %#v", got.Owners)
	}
	if len(got.Relations) != 1 || got.Relations[0].Type != "owned_by" {
		t.Fatalf("unexpected relations: %#v", got.Relations)
	}
}
