package kube

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestResolveResourceCoreShortcut(t *testing.T) {
	clientset := kubefake.NewSimpleClientset()
	discovery := clientset.Discovery().(*fake.FakeDiscovery)
	discovery.Resources = []*metav1.APIResourceList{{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{{
			Name:         "pods",
			SingularName: "pod",
			Namespaced:   true,
			Kind:         "Pod",
			ShortNames:   []string{"po"},
		}},
	}}

	got, err := NewClient(clientset).ResolveResource(context.Background(), "po")
	if err != nil {
		t.Fatalf("ResolveResource() error = %v", err)
	}
	if got.GVR.Resource != "pods" || got.GVK.Kind != "Pod" || !got.Namespaced {
		t.Fatalf("resolved resource = %#v", got)
	}
}

func TestResolveResourceCustomResourceName(t *testing.T) {
	clientset := kubefake.NewSimpleClientset()
	discovery := clientset.Discovery().(*fake.FakeDiscovery)
	discovery.Resources = []*metav1.APIResourceList{{
		GroupVersion: "argoproj.io/v1alpha1",
		APIResources: []metav1.APIResource{{
			Name:         "applications",
			SingularName: "application",
			Namespaced:   true,
			Kind:         "Application",
		}},
	}}

	got, err := NewClient(clientset).ResolveResource(context.Background(), "applications.argoproj.io")
	if err != nil {
		t.Fatalf("ResolveResource() error = %v", err)
	}
	if got.GVR.Group != "argoproj.io" || got.GVK.Kind != "Application" || !got.Namespaced {
		t.Fatalf("resolved resource = %#v", got)
	}
}

func TestResolveResourceUnknownResource(t *testing.T) {
	clientset := kubefake.NewSimpleClientset()
	discovery := clientset.Discovery().(*fake.FakeDiscovery)

	if _, err := NewClient(clientset).ResolveResource(context.Background(), "missing.example.io"); err == nil {
		t.Fatal("ResolveResource() error = nil, want error")
	} else if discovery.Resources == nil {
		t.Logf("resolution failed as expected: %v", err)
	}
}
