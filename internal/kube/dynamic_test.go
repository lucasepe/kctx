package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestDynamicReaderGetListAndDecode(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-config",
			Namespace: "payments",
			Labels:    map[string]string{"app": "api"},
		},
		Data: map[string]string{"key": "value"},
	}
	client := NewClientWithDynamic(
		kubefake.NewSimpleClientset(),
		fake.NewSimpleDynamicClient(scheme, cm),
	)
	ref := ResourceRef{Group: "", Version: "v1", Resource: "configmaps"}

	got, err := client.Get(context.Background(), ref, "payments", "api-config")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	decoded, err := DecodeResource[corev1.ConfigMap](got)
	if err != nil {
		t.Fatalf("DecodeResource() error = %v", err)
	}
	if decoded.Name != "api-config" || decoded.Data["key"] != "value" {
		t.Fatalf("decoded ConfigMap = %#v", decoded)
	}

	items, err := client.List(context.Background(), ref, "payments")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	decodedItems, err := DecodeResourceList[corev1.ConfigMap](items)
	if err != nil {
		t.Fatalf("DecodeResourceList() error = %v", err)
	}
	if len(decodedItems) != 1 || decodedItems[0].Name != "api-config" {
		t.Fatalf("decoded list = %#v", decodedItems)
	}
}

func TestDynamicReaderUnavailable(t *testing.T) {
	client := NewClient(kubefake.NewSimpleClientset())
	ref := ResourceRef{Group: "", Version: "v1", Resource: "configmaps"}

	if _, err := client.Get(context.Background(), ref, "payments", "api-config"); err == nil {
		t.Fatal("Get() error = nil, want dynamic client unavailable error")
	}
}
