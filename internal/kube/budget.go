package kube

import (
	"context"

	"github.com/lucasepe/kctx/internal/limits"
)

// takeKubeAPIBudget charges one token for one kube.Client operation. The
// wrapper methods deliberately call this before each Kubernetes Get/List or
// discovery operation so expensive requests fail before issuing more API calls.
func takeKubeAPIBudget(ctx context.Context) error {
	return limits.TakeBudget(ctx)
}
