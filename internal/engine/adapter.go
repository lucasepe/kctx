package engine

import (
	"context"

	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Adapter teaches DynamicEngine how to interpret one unstructured Kubernetes
// resource kind as normalized entities, relations, and signals.
type Adapter interface {
	// Supports reports whether this adapter knows how to interpret obj.
	Supports(obj *unstructured.Unstructured) bool
	// Entities returns normalized entities derived from obj.
	Entities(ctx context.Context, obj *unstructured.Unstructured) ([]model.Entity, error)
	// Relations returns normalized relations derived from obj.
	Relations(ctx context.Context, obj *unstructured.Unstructured) ([]model.Relation, error)
	// Signals returns factual operational signals derived from obj.
	Signals(ctx context.Context, obj *unstructured.Unstructured) ([]model.Signal, error)
}

// GraphAdapter extends Adapter with graph-specific normalized nodes and edges.
type GraphAdapter interface {
	Adapter
	// Nodes returns graph nodes derived from obj.
	Nodes(ctx context.Context, obj *unstructured.Unstructured) ([]graph.Node, error)
	// Edges returns graph edges derived from obj.
	Edges(ctx context.Context, obj *unstructured.Unstructured) ([]graph.Edge, error)
}

// StatusAdapter extends Adapter with a compact resource-specific status
// summary. It is meant for stable facts, not raw manifests.
type StatusAdapter interface {
	Adapter
	Status(ctx context.Context, obj *unstructured.Unstructured) (map[string]string, error)
}

// AdapterFor returns the first registered adapter that supports obj.
func (e *DynamicEngine) AdapterFor(obj *unstructured.Unstructured) Adapter {
	if e == nil {
		return nil
	}
	for _, adapter := range e.adapters {
		if adapter.Supports(obj) {
			return adapter
		}
	}
	return nil
}
