package argocd

import (
	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/model"
	"github.com/lucasepe/kctx/internal/redaction"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type condition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

func gitRepositoryEntity(repo string) model.Entity {
	name := redaction.Text(repo)
	return model.Entity{ID: graph.NodeID("GitRepository", "", name), Kind: "GitRepository", Name: name}
}

func conditions(obj *unstructured.Unstructured) []condition {
	items, ok, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !ok {
		return nil
	}
	out := make([]condition, 0, len(items))
	for _, item := range items {
		cond, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, condition{
			Type:    stringValue(cond["type"]),
			Status:  stringValue(cond["status"]),
			Reason:  stringValue(cond["reason"]),
			Message: stringValue(cond["message"]),
		})
	}
	return out
}

func stringSlice(obj *unstructured.Unstructured, fields ...string) []string {
	items, ok, _ := unstructured.NestedStringSlice(obj.Object, fields...)
	if ok {
		return items
	}
	raw, ok, _ := unstructured.NestedSlice(obj.Object, fields...)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if value := stringValue(item); value != "" {
			out = append(out, value)
		}
	}
	return out
}
