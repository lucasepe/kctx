package engine

import (
	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// entityFromObject normalizes any Kubernetes object that exposes metav1.Object.
func entityFromObject(apiVersion, kind string, obj metav1.Object) model.Entity {
	return model.Entity{
		APIVersion: apiVersion,
		Kind:       kind,
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
		UID:        string(obj.GetUID()),
		Labels:     copyMap(obj.GetLabels()),
	}
}

// entityFromOwnerRef converts an owner reference into an entity in the child's
// namespace.
func entityFromOwnerRef(namespace string, ref metav1.OwnerReference) model.Entity {
	return model.Entity{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Namespace:  namespace,
		Name:       ref.Name,
		UID:        string(ref.UID),
	}
}

// ownerReferences returns a defensive copy of an object's owner references.
func ownerReferences(obj metav1.Object) []metav1.OwnerReference {
	return append([]metav1.OwnerReference(nil), obj.GetOwnerReferences()...)
}

// ownerRelation connects a child entity to one of its owners.
func ownerRelation(child model.Entity, namespace string, ref metav1.OwnerReference) model.Relation {
	return model.Relation{
		Type:   "owned_by",
		Source: child,
		Target: entityFromOwnerRef(namespace, ref),
	}
}

// graphNodeFromObject normalizes any metav1 object into a graph node.
func graphNodeFromObject(kind, status string, obj metav1.Object) graph.Node {
	return graph.Node{
		ID:        graph.NodeID(kind, obj.GetNamespace(), obj.GetName()),
		Kind:      kind,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Labels:    copyMap(obj.GetLabels()),
		Status:    status,
	}
}

// graphNodeFromOwnerRef converts an owner reference into a graph node identity.
func graphNodeFromOwnerRef(namespace string, ref metav1.OwnerReference) graph.Node {
	return graph.Node{
		ID:        graph.NodeID(ref.Kind, namespace, ref.Name),
		Kind:      ref.Kind,
		Namespace: namespace,
		Name:      ref.Name,
	}
}
