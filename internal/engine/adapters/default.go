package adapters

import (
	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/engine/adapters/argocd"
	"github.com/lucasepe/kctx/internal/engine/adapters/certmanager"
)

// Default returns semantic adapters for non-native Kubernetes resources that
// kctx understands through unstructured objects.
func Default() []engine.Adapter {
	return []engine.Adapter{
		argocd.ApplicationAdapter{},
		argocd.AppProjectAdapter{},
		certmanager.CertificateAdapter{},
	}
}
