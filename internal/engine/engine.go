package engine

import (
	"context"

	"github.com/lucasepe/kctx/internal/kube"
)

// ResourceResolver resolves user-facing resource names into concrete Kubernetes
// GVR/GVK metadata.
type ResourceResolver interface {
	ResolveResource(ctx context.Context, resource string) (*kube.ResolvedResource, error)
}

// Engine is the facade that exposes typed Kubernetes resolvers and the dynamic
// extension path from a single value.
type Engine struct {
	*TypedEngine
	*DynamicEngine

	resolver ResourceResolver
}

// New creates an Engine backed only by typed Kubernetes readers.
func New(kubeReader clusterReader) *Engine {
	eng := &Engine{TypedEngine: NewTypedEngine(kubeReader)}
	if resolver, ok := kubeReader.(ResourceResolver); ok {
		eng.resolver = resolver
	}
	return eng
}

// NewWithDynamic creates an Engine that can use typed readers for built-in
// resources and dynamic readers plus adapters for generic resources.
func NewWithDynamic(kubeReader clusterReader, dynamicReader kube.DynamicReader, adapters ...Adapter) *Engine {
	eng := &Engine{
		TypedEngine:   NewTypedEngine(kubeReader),
		DynamicEngine: NewDynamicEngine(dynamicReader, adapters...),
	}
	if resolver, ok := kubeReader.(ResourceResolver); ok {
		eng.resolver = resolver
	}
	return eng
}

// NewWithResolver creates an Engine with an explicit resource resolver. This is
// useful for tests and embedders that do not use the default kube.Client.
func NewWithResolver(kubeReader clusterReader, resolver ResourceResolver) *Engine {
	eng := New(kubeReader)
	eng.resolver = resolver
	return eng
}

// NewWithDynamicAndResolver creates an Engine with explicit dynamic access and
// resource resolution.
func NewWithDynamicAndResolver(kubeReader clusterReader, dynamicReader kube.DynamicReader, resolver ResourceResolver, adapters ...Adapter) *Engine {
	eng := NewWithDynamic(kubeReader, dynamicReader, adapters...)
	eng.resolver = resolver
	return eng
}

// TypedEngine contains the read-only typed Kubernetes implementation used by
// the CLI and HTTP commands.
type TypedEngine struct {
	kube clusterReader
}

// NewTypedEngine creates an engine for typed Kubernetes resources.
func NewTypedEngine(kubeReader clusterReader) *TypedEngine {
	return &TypedEngine{kube: kubeReader}
}

// DynamicEngine contains the generic unstructured resource path.
type DynamicEngine struct {
	reader   kube.DynamicReader
	adapters []Adapter
}

// NewDynamicEngine creates a dynamic engine with optional adapters for
// resource-specific behavior.
func NewDynamicEngine(reader kube.DynamicReader, adapters ...Adapter) *DynamicEngine {
	return &DynamicEngine{
		reader:   reader,
		adapters: append([]Adapter(nil), adapters...),
	}
}
