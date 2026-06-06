package argocd

import (
	"context"
	"fmt"

	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/model"
	"github.com/lucasepe/kctx/internal/redaction"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	appProjectAPIVersion = "argoproj.io/v1alpha1"
	appProjectKind       = "AppProject"
)

// AppProjectAdapter interprets Argo CD AppProject resources as policy context.
type AppProjectAdapter struct{}

func (AppProjectAdapter) Supports(obj *unstructured.Unstructured) bool {
	return obj.GetAPIVersion() == appProjectAPIVersion && obj.GetKind() == appProjectKind
}

func (AppProjectAdapter) Entities(ctx context.Context, obj *unstructured.Unstructured) ([]model.Entity, error) {
	_ = ctx
	entities := []model.Entity{entityFromUnstructured(obj)}
	for _, repo := range stringSlice(obj, "spec", "sourceRepos") {
		entities = append(entities, gitRepositoryEntity(repo))
	}
	for _, dest := range appProjectDestinations(obj) {
		if dest.Namespace != "" {
			entities = append(entities, model.Entity{ID: graph.NodeID("Namespace", "", dest.Namespace), APIVersion: "v1", Kind: "Namespace", Name: dest.Namespace})
		}
		if dest.Server != "" {
			entities = append(entities, model.Entity{ID: graph.NodeID("Cluster", "", dest.Server), Kind: "Cluster", Name: dest.Server})
		}
	}
	return entities, nil
}

func (AppProjectAdapter) Relations(ctx context.Context, obj *unstructured.Unstructured) ([]model.Relation, error) {
	_ = ctx
	project := entityFromUnstructured(obj)
	var relations []model.Relation
	for _, repo := range stringSlice(obj, "spec", "sourceRepos") {
		relations = append(relations, model.Relation{Type: "allows_source", Source: project, Target: gitRepositoryEntity(repo), Reason: "spec.sourceRepos"})
	}
	for _, dest := range appProjectDestinations(obj) {
		if dest.Namespace != "" {
			relations = append(relations, model.Relation{Type: "allows_namespace", Source: project, Target: model.Entity{ID: graph.NodeID("Namespace", "", dest.Namespace), APIVersion: "v1", Kind: "Namespace", Name: dest.Namespace}, Reason: "spec.destinations"})
		}
		if dest.Server != "" {
			relations = append(relations, model.Relation{Type: "allows_cluster", Source: project, Target: model.Entity{ID: graph.NodeID("Cluster", "", dest.Server), Kind: "Cluster", Name: dest.Server}, Reason: "spec.destinations"})
		}
	}
	return relations, nil
}

func (AppProjectAdapter) Signals(ctx context.Context, obj *unstructured.Unstructured) ([]model.Signal, error) {
	_ = ctx
	var signals []model.Signal
	for _, condition := range conditions(obj) {
		if condition.Type == "" || condition.Status == "True" {
			continue
		}
		message := condition.Message
		if message == "" {
			message = fmt.Sprintf("Argo CD AppProject condition %s is %s", condition.Type, condition.Status)
		}
		signals = append(signals, model.Signal{Severity: "warning", Reason: "AppProjectCondition" + condition.Type, Message: redaction.Text(message), Source: "status.conditions"})
	}
	return signals, nil
}

func (AppProjectAdapter) Status(ctx context.Context, obj *unstructured.Unstructured) (map[string]string, error) {
	_ = ctx
	status := map[string]string{}
	status["sourceRepos"] = fmt.Sprintf("%d", len(stringSlice(obj, "spec", "sourceRepos")))
	status["destinations"] = fmt.Sprintf("%d", len(appProjectDestinations(obj)))
	return redaction.StringMap(status), nil
}

func (AppProjectAdapter) Nodes(ctx context.Context, obj *unstructured.Unstructured) ([]graph.Node, error) {
	entities, err := AppProjectAdapter{}.Entities(ctx, obj)
	if err != nil {
		return nil, err
	}
	nodes := make([]graph.Node, 0, len(entities))
	for _, entity := range entities {
		nodes = append(nodes, nodeFromEntity(entity))
	}
	return nodes, nil
}

func (AppProjectAdapter) Edges(ctx context.Context, obj *unstructured.Unstructured) ([]graph.Edge, error) {
	relations, err := AppProjectAdapter{}.Relations(ctx, obj)
	if err != nil {
		return nil, err
	}
	edges := make([]graph.Edge, 0, len(relations))
	for _, relation := range relations {
		edges = append(edges, graph.Edge{Type: relation.Type, Source: relation.Source.ID, Target: relation.Target.ID, Reason: relation.Reason})
	}
	return edges, nil
}

type appProjectDestination struct {
	Namespace string
	Server    string
}

func appProjectDestinations(obj *unstructured.Unstructured) []appProjectDestination {
	items, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "destinations")
	if !ok {
		return nil
	}
	out := make([]appProjectDestination, 0, len(items))
	for _, item := range items {
		dest, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, appProjectDestination{Namespace: stringValue(dest["namespace"]), Server: stringValue(dest["server"])})
	}
	return out
}
