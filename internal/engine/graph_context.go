package engine

import (
	"context"
	"fmt"

	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/kube"
	corev1 "k8s.io/api/core/v1"
)

// BuildGraphRequest identifies one supported resource to turn into a graph.
type BuildGraphRequest struct {
	Resource  string
	Namespace string
	Name      string
}

// BuildGraph resolves the resource input and dispatches to the richest graph
// implementation available for that GVK.
func (e *Engine) BuildGraph(ctx context.Context, req BuildGraphRequest) (*graph.Graph, error) {
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Resource == "" {
		return nil, fmt.Errorf("resource is required")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("resource name is required")
	}
	if e == nil {
		return nil, fmt.Errorf("engine is not configured")
	}

	resolved, err := e.resolveResource(ctx, req.Resource)
	if err != nil {
		return nil, err
	}

	namespace := req.Namespace
	if !resolved.Namespaced {
		namespace = ""
	}

	if resolved.GVK == corev1.SchemeGroupVersion.WithKind("Pod") {
		return e.BuildPodGraph(ctx, BuildPodGraphRequest{Namespace: namespace, Name: req.Name})
	}

	if e.DynamicEngine == nil || e.DynamicEngine.reader == nil {
		return nil, fmt.Errorf("no typed graph resolver or dynamic reader configured for %s", resolved.GVK.String())
	}
	return e.DynamicEngine.BuildResourceGraph(ctx, BuildResourceGraphRequest{
		Resource:  resolved.Ref(),
		Namespace: namespace,
		Name:      req.Name,
	})
}

// BuildResourceGraphRequest identifies one arbitrary Kubernetes resource for
// dynamic graph construction.
type BuildResourceGraphRequest struct {
	Resource  kube.ResourceRef
	Namespace string
	Name      string
}

// BuildResourceGraph builds a graph for an unstructured resource when a
// GraphAdapter is registered for it.
func (e *DynamicEngine) BuildResourceGraph(ctx context.Context, req BuildResourceGraphRequest) (*graph.Graph, error) {
	if e == nil || e.reader == nil {
		return nil, fmt.Errorf("dynamic engine is not configured")
	}

	obj, err := e.reader.Get(ctx, req.Resource, req.Namespace, req.Name)
	if err != nil {
		return nil, err
	}

	adapter := e.AdapterFor(obj)
	graphAdapter, ok := adapter.(GraphAdapter)
	if !ok {
		return nil, UnsupportedResourceError{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Resource:   req.Resource,
		}
	}

	nodes, err := graphAdapter.Nodes(ctx, obj)
	if err != nil {
		return nil, err
	}
	edges, err := graphAdapter.Edges(ctx, obj)
	if err != nil {
		return nil, err
	}

	builder := graph.NewBuilder()
	for _, node := range nodes {
		builder.AddNode(node)
	}
	for _, edge := range edges {
		builder.AddEdge(edge)
	}
	out := builder.Graph()
	out.Kind = "ResourceGraph"
	return &out, nil
}
