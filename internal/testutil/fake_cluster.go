package testutil

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/lucasepe/kctx/internal/kube"
)

func NewFakeReader(objects ...runtime.Object) *kube.Client {
	return kube.NewClient(fake.NewSimpleClientset(objects...))
}
