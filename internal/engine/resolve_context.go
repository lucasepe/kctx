package engine

import (
	"context"
	"fmt"

	"github.com/lucasepe/kctx/internal/kube"
	corev1 "k8s.io/api/core/v1"
)

// ResolveContextRequest is the generic "explain this Kubernetes resource"
// request. Resource is the user-facing token, such as "pod", "po",
// "ingresses.networking.k8s.io", or a CRD resource name.
type ResolveContextRequest struct {
	Resource   string
	Namespace  string
	Name       string
	EventLimit int
}

// ResolveContextResponse wraps either a rich typed response or a generic
// dynamic response after resource discovery has normalized the input.
type ResolveContextResponse struct {
	Resolved *kube.ResolvedResource          `json:"resolved"`
	Pod      *ResolvePodContextResponse      `json:"pod,omitempty"`
	Resource *ResolveResourceContextResponse `json:"resource,omitempty"`
}

// ResolveContext resolves the resource input first, then dispatches to the
// richest available resolver for that GVK. Built-in typed resolvers are used as
// semantic accelerators; everything else falls back to dynamic/adapters.
func (e *Engine) ResolveContext(ctx context.Context, req ResolveContextRequest) (*ResolveContextResponse, error) {
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
		pod, err := e.ResolvePodContext(ctx, ResolvePodContextRequest{
			Namespace:  namespace,
			Name:       req.Name,
			EventLimit: req.EventLimit,
		})
		if err != nil {
			return nil, err
		}
		return &ResolveContextResponse{Resolved: resolved, Pod: pod}, nil
	}

	if e.DynamicEngine == nil || e.DynamicEngine.reader == nil {
		return nil, fmt.Errorf("no typed resolver or dynamic reader configured for %s", resolved.GVK.String())
	}
	resource, err := e.DynamicEngine.ResolveResourceContext(ctx, ResolveResourceContextRequest{
		Resource:  resolved.Ref(),
		Namespace: namespace,
		Name:      req.Name,
	})
	if err != nil {
		return nil, err
	}
	return &ResolveContextResponse{Resolved: resolved, Resource: resource}, nil
}

// resolveResource isolates the discovery dependency and provides a narrow typed
// fallback for tests and old embedders that construct Engine without discovery.
func (e *Engine) resolveResource(ctx context.Context, resource string) (*kube.ResolvedResource, error) {
	if e.resolver != nil {
		return e.resolver.ResolveResource(ctx, resource)
	}
	if resource == "pod" || resource == "pods" || resource == "po" {
		return &kube.ResolvedResource{
			Input:      resource,
			GVR:        corev1.SchemeGroupVersion.WithResource("pods"),
			GVK:        corev1.SchemeGroupVersion.WithKind("Pod"),
			Namespaced: true,
		}, nil
	}
	return nil, fmt.Errorf("cannot resolve resource %q: resource discovery is not configured", resource)
}
