package kube

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/restmapper"
)

// ResolvedResource is the normalized Kubernetes resource identity produced from
// user input such as "pod", "pods", "po", or "applications.argoproj.io".
type ResolvedResource struct {
	Input      string                      `json:"input"`
	GVR        schema.GroupVersionResource `json:"gvr"`
	GVK        schema.GroupVersionKind     `json:"gvk"`
	Namespaced bool                        `json:"namespaced"`
}

// Ref returns the dynamic-client resource reference for this resolved resource.
func (r ResolvedResource) Ref() ResourceRef {
	return ResourceRef{
		Group:    r.GVR.Group,
		Version:  r.GVR.Version,
		Resource: r.GVR.Resource,
	}
}

// ResolveResource resolves human resource input through Kubernetes discovery and
// RESTMapper, following the same broad idea kubectl uses for resources and
// shortcuts.
func (c *Client) ResolveResource(ctx context.Context, resource string) (*ResolvedResource, error) {
	_ = ctx
	if c.discovery == nil {
		return nil, fmt.Errorf("resource discovery is not configured")
	}
	return ResolveResource(c.discovery, resource)
}

// ResolveResource resolves a resource name using the provided discovery client.
// It is a package-level function to keep tests and future resolvers lightweight.
func ResolveResource(discoveryClient discovery.DiscoveryInterface, resource string) (*ResolvedResource, error) {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return nil, fmt.Errorf("resource is required")
	}

	mapper, err := resourceMapper(discoveryClient)
	if err != nil {
		return nil, fmt.Errorf("cannot load Kubernetes resource discovery: %w", err)
	}
	partial := parseResourceInput(resource)
	gvr, err := mapper.ResourceFor(partial)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve resource %q: %w", resource, err)
	}
	gvk, err := mapper.KindFor(gvr)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve kind for resource %q: %w", resource, err)
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve REST mapping for resource %q: %w", resource, err)
	}

	return &ResolvedResource{
		Input:      resource,
		GVR:        gvr,
		GVK:        gvk,
		Namespaced: mapping.Scope.Name() == meta.RESTScopeNameNamespace,
	}, nil
}

// resourceMapper builds a shortcut-aware mapper backed by a deterministic
// discovery snapshot.
func resourceMapper(discoveryClient discovery.DiscoveryInterface) (meta.RESTMapper, error) {
	resources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDiscoveryRESTMapper(resources)
	return restmapper.NewShortcutExpander(mapper, discoveryClient, nil), nil
}

// parseResourceInput accepts kubectl-like resource input and turns it into a
// partial GVR for RESTMapper to complete.
func parseResourceInput(input string) schema.GroupVersionResource {
	if slash := strings.Split(input, "/"); len(slash) == 3 {
		return schema.GroupVersionResource{Group: slash[0], Version: slash[1], Resource: slash[2]}
	} else if len(slash) == 2 {
		return schema.GroupVersionResource{Version: slash[0], Resource: slash[1]}
	}

	parts := strings.SplitN(input, ".", 2)
	if len(parts) == 2 {
		return schema.GroupVersionResource{Group: parts[1], Resource: parts[0]}
	}
	return schema.GroupVersionResource{Resource: input}
}
