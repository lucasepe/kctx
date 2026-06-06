package engine

import (
	"context"
	"fmt"

	"github.com/lucasepe/kctx/internal/kube"
	"github.com/lucasepe/kctx/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResolveResourceContextRequest identifies one arbitrary Kubernetes resource
// addressable through the dynamic client.
type ResolveResourceContextRequest struct {
	Resource       kube.ResourceRef
	Namespace      string
	Name           string
	RequireAdapter bool
}

// ResolveResourceContextResponse is the generic context shape returned for
// dynamic resources.
type ResolveResourceContextResponse struct {
	SchemaVersion string            `json:"schemaVersion"`
	Kind          string            `json:"kind"`
	Resource      model.Entity      `json:"resource"`
	Status        map[string]string `json:"status,omitempty"`
	Owners        []model.Entity    `json:"owners"`
	Related       []model.Entity    `json:"related"`
	Relations     []model.Relation  `json:"relations"`
	Signals       []model.Signal    `json:"signals"`
}

// UnsupportedResourceError reports that Kubernetes knows the resource but kctx
// does not have semantic knowledge for it yet.
type UnsupportedResourceError struct {
	APIVersion string
	Kind       string
	Resource   kube.ResourceRef
}

func (e UnsupportedResourceError) Error() string {
	return fmt.Sprintf("resource %s/%s is known to Kubernetes, but kctx has no semantic adapter for %s yet", e.APIVersion, e.Kind, e.Resource.GVR().String())
}

// ResolveResourceContext builds normalized context for an unstructured
// resource, using an adapter when one is registered and a generic owner-based
// fallback otherwise.
func (e *DynamicEngine) ResolveResourceContext(ctx context.Context, req ResolveResourceContextRequest) (*ResolveResourceContextResponse, error) {
	if e == nil || e.reader == nil {
		return nil, fmt.Errorf("dynamic engine is not configured")
	}

	obj, err := e.reader.Get(ctx, req.Resource, req.Namespace, req.Name)
	if err != nil {
		return nil, err
	}

	resource := entityFromObject(obj.GetAPIVersion(), obj.GetKind(), obj)
	resp := &ResolveResourceContextResponse{
		SchemaVersion: model.SchemaVersion,
		Kind:          "DynamicResourceContext",
		Resource:      resource,
		Owners:        ownerEntities(obj.GetNamespace(), ownerReferences(obj)),
		Related:       []model.Entity{},
		Relations:     ownerRelations(resource, obj.GetNamespace(), ownerReferences(obj)),
		Signals:       []model.Signal{},
	}

	adapter := e.AdapterFor(obj)
	if adapter == nil && req.RequireAdapter {
		return nil, UnsupportedResourceError{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Resource:   req.Resource,
		}
	}
	if adapter != nil {
		entities, err := adapter.Entities(ctx, obj)
		if err != nil {
			return nil, err
		}
		relations, err := adapter.Relations(ctx, obj)
		if err != nil {
			return nil, err
		}
		signals, err := adapter.Signals(ctx, obj)
		if err != nil {
			return nil, err
		}
		if statusAdapter, ok := adapter.(StatusAdapter); ok {
			status, err := statusAdapter.Status(ctx, obj)
			if err != nil {
				return nil, err
			}
			resp.Status = status
		}
		if len(entities) > 0 {
			resp.Resource = entities[0]
			if len(entities) > 1 {
				resp.Related = append(resp.Related, entities[1:]...)
			}
		}
		resp.Relations = append(resp.Relations, relations...)
		resp.Signals = append(resp.Signals, signals...)
	}

	sortEntities(resp.Owners)
	return resp, nil
}

// ownerEntities converts Kubernetes owner references into namespace-scoped
// normalized entities.
func ownerEntities(namespace string, refs []metav1.OwnerReference) []model.Entity {
	owners := make([]model.Entity, 0, len(refs))
	for _, ref := range refs {
		owners = append(owners, entityFromOwnerRef(namespace, ref))
	}
	return owners
}

// ownerRelations converts Kubernetes owner references into normalized ownership
// relations from child to owner.
func ownerRelations(child model.Entity, namespace string, refs []metav1.OwnerReference) []model.Relation {
	relations := make([]model.Relation, 0, len(refs))
	for _, ref := range refs {
		relations = append(relations, ownerRelation(child, namespace, ref))
	}
	return relations
}
