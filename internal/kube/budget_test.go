package kube

import (
	"context"
	"errors"
	"testing"

	"github.com/lucasepe/kctx/internal/limits"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestClientConsumesKubeAPIBudget(t *testing.T) {
	client := NewClient(kubefake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "payments"},
	}))
	ctx := limits.ContextWithBudget(context.Background(), limits.NewBudget(1))

	if _, err := client.GetNamespace(ctx, "payments"); err != nil {
		t.Fatalf("GetNamespace() error = %v", err)
	}
	if _, err := client.ListPods(ctx, "payments"); !errors.Is(err, limits.ErrBudgetExceeded) {
		t.Fatalf("ListPods() error = %v, want ErrBudgetExceeded", err)
	}
}
