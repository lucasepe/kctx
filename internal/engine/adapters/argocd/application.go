package argocd

import (
	"context"
	"fmt"
	"strings"

	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/model"
	"github.com/lucasepe/kctx/internal/redaction"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	applicationAPIVersion = "argoproj.io/v1alpha1"
	applicationKind       = "Application"
)

// ApplicationAdapter interprets Argo CD Application resources without linking
// against Argo CD Go types.
type ApplicationAdapter struct{}

func (ApplicationAdapter) Supports(obj *unstructured.Unstructured) bool {
	return obj.GetAPIVersion() == applicationAPIVersion && obj.GetKind() == applicationKind
}

func (ApplicationAdapter) Entities(ctx context.Context, obj *unstructured.Unstructured) ([]model.Entity, error) {
	_ = ctx
	entities := []model.Entity{entityFromUnstructured(obj)}
	for _, resource := range applicationResources(obj) {
		entities = append(entities, resource.Entity)
	}
	return entities, nil
}

func (ApplicationAdapter) Relations(ctx context.Context, obj *unstructured.Unstructured) ([]model.Relation, error) {
	_ = ctx
	app := entityFromUnstructured(obj)
	relations := make([]model.Relation, 0)

	if namespace, ok, _ := unstructured.NestedString(obj.Object, "spec", "destination", "namespace"); ok && namespace != "" {
		relations = append(relations, model.Relation{
			Type:   "deploys_to",
			Source: app,
			Target: model.Entity{ID: graph.NodeID("Namespace", "", namespace), APIVersion: "v1", Kind: "Namespace", Name: namespace},
			Reason: "spec.destination.namespace",
		})
	}

	if server, ok, _ := unstructured.NestedString(obj.Object, "spec", "destination", "server"); ok && server != "" {
		relations = append(relations, model.Relation{
			Type:   "targets_cluster",
			Source: app,
			Target: model.Entity{ID: graph.NodeID("Cluster", "", server), Kind: "Cluster", Name: server},
			Reason: "spec.destination.server",
		})
	}

	if repoURL, ok, _ := unstructured.NestedString(obj.Object, "spec", "source", "repoURL"); ok && repoURL != "" {
		repo := gitRepositoryEntity(repoURL)
		relations = append(relations, model.Relation{
			Type:   "syncs_from",
			Source: app,
			Target: repo,
			Reason: "spec.source.repoURL",
		})
	}
	if project, ok, _ := unstructured.NestedString(obj.Object, "spec", "project"); ok && project != "" {
		relations = append(relations, model.Relation{
			Type:   "uses_project",
			Source: app,
			Target: model.Entity{ID: graph.NodeID("AppProject", obj.GetNamespace(), project), APIVersion: applicationAPIVersion, Kind: "AppProject", Namespace: obj.GetNamespace(), Name: project},
			Reason: "spec.project",
		})
	}

	for _, resource := range applicationResources(obj) {
		relations = append(relations, model.Relation{
			Type:       "manages",
			Source:     app,
			Target:     resource.Entity,
			Confidence: "reported",
			Reason:     "status.resources",
		})
	}
	return relations, nil
}

func (ApplicationAdapter) Signals(ctx context.Context, obj *unstructured.Unstructured) ([]model.Signal, error) {
	_ = ctx
	var signals []model.Signal

	if health, ok, _ := unstructured.NestedString(obj.Object, "status", "health", "status"); ok && health != "" && health != "Healthy" {
		signals = append(signals, model.Signal{
			Severity: severityForHealth(health),
			Reason:   "ApplicationHealth" + health,
			Message:  fmt.Sprintf("Argo CD reports application health as %s", health),
			Source:   "status.health.status",
		})
	}

	if sync, ok, _ := unstructured.NestedString(obj.Object, "status", "sync", "status"); ok && sync != "" && sync != "Synced" {
		signals = append(signals, model.Signal{
			Severity: "warning",
			Reason:   "ApplicationSync" + sync,
			Message:  fmt.Sprintf("Argo CD reports application sync status as %s", sync),
			Source:   "status.sync.status",
		})
	}

	if phase, ok, _ := unstructured.NestedString(obj.Object, "status", "operationState", "phase"); ok && operationPhaseIsProblem(phase) {
		message, _, _ := unstructured.NestedString(obj.Object, "status", "operationState", "message")
		if message == "" {
			message = fmt.Sprintf("Argo CD operation phase is %s", phase)
		}
		signals = append(signals, model.Signal{
			Severity: "error",
			Reason:   "ApplicationOperation" + phase,
			Message:  redaction.Text(message),
			Source:   "status.operationState",
		})
	}

	for _, resource := range applicationResources(obj) {
		if resource.Health == "" || resource.Health == "Healthy" {
			continue
		}
		signals = append(signals, model.Signal{
			Severity: severityForHealth(resource.Health),
			Reason:   "ManagedResourceHealth" + resource.Health,
			Message:  fmt.Sprintf("%s %s/%s health is %s", resource.Entity.Kind, resource.Entity.Namespace, resource.Entity.Name, resource.Health),
			Source:   "status.resources",
		})
	}

	return signals, nil
}

func (ApplicationAdapter) Status(ctx context.Context, obj *unstructured.Unstructured) (map[string]string, error) {
	_ = ctx
	status := map[string]string{}
	addNestedString(status, "health", obj, "status", "health", "status")
	addNestedString(status, "sync", obj, "status", "sync", "status")
	addNestedString(status, "revision", obj, "status", "sync", "revision")
	addNestedString(status, "operationPhase", obj, "status", "operationState", "phase")
	addNestedString(status, "operationMessage", obj, "status", "operationState", "message")
	addNestedString(status, "reconciledAt", obj, "status", "reconciledAt")
	addNestedString(status, "sourceRepoURL", obj, "spec", "source", "repoURL")
	addNestedString(status, "sourcePath", obj, "spec", "source", "path")
	addNestedString(status, "targetRevision", obj, "spec", "source", "targetRevision")
	addNestedString(status, "destinationNamespace", obj, "spec", "destination", "namespace")
	addNestedString(status, "destinationServer", obj, "spec", "destination", "server")
	addNestedString(status, "project", obj, "spec", "project")
	return redaction.StringMap(status), nil
}

func (ApplicationAdapter) Nodes(ctx context.Context, obj *unstructured.Unstructured) ([]graph.Node, error) {
	_ = ctx
	app := entityFromUnstructured(obj)
	nodes := []graph.Node{nodeFromEntity(app)}

	if namespace, ok, _ := unstructured.NestedString(obj.Object, "spec", "destination", "namespace"); ok && namespace != "" {
		nodes = append(nodes, graph.Node{ID: graph.NodeID("Namespace", "", namespace), Kind: "Namespace", Name: namespace})
	}
	if server, ok, _ := unstructured.NestedString(obj.Object, "spec", "destination", "server"); ok && server != "" {
		nodes = append(nodes, graph.Node{ID: graph.NodeID("Cluster", "", server), Kind: "Cluster", Name: server})
	}
	if repoURL, ok, _ := unstructured.NestedString(obj.Object, "spec", "source", "repoURL"); ok && repoURL != "" {
		nodes = append(nodes, nodeFromEntity(gitRepositoryEntity(repoURL)))
	}
	if project, ok, _ := unstructured.NestedString(obj.Object, "spec", "project"); ok && project != "" {
		nodes = append(nodes, graph.Node{ID: graph.NodeID("AppProject", obj.GetNamespace(), project), Kind: "AppProject", Namespace: obj.GetNamespace(), Name: project})
	}
	for _, resource := range applicationResources(obj) {
		nodes = append(nodes, nodeFromEntity(resource.Entity))
	}
	return nodes, nil
}

func (ApplicationAdapter) Edges(ctx context.Context, obj *unstructured.Unstructured) ([]graph.Edge, error) {
	_ = ctx
	appID := graph.NodeID(obj.GetKind(), obj.GetNamespace(), obj.GetName())
	var edges []graph.Edge

	if namespace, ok, _ := unstructured.NestedString(obj.Object, "spec", "destination", "namespace"); ok && namespace != "" {
		edges = append(edges, graph.Edge{Type: "deploys_to", Source: appID, Target: graph.NodeID("Namespace", "", namespace), Reason: "spec.destination.namespace"})
	}
	if server, ok, _ := unstructured.NestedString(obj.Object, "spec", "destination", "server"); ok && server != "" {
		edges = append(edges, graph.Edge{Type: "targets_cluster", Source: appID, Target: graph.NodeID("Cluster", "", server), Reason: "spec.destination.server"})
	}
	if repoURL, ok, _ := unstructured.NestedString(obj.Object, "spec", "source", "repoURL"); ok && repoURL != "" {
		edges = append(edges, graph.Edge{Type: "syncs_from", Source: appID, Target: gitRepositoryEntity(repoURL).ID, Reason: "spec.source.repoURL"})
	}
	if project, ok, _ := unstructured.NestedString(obj.Object, "spec", "project"); ok && project != "" {
		edges = append(edges, graph.Edge{Type: "uses_project", Source: appID, Target: graph.NodeID("AppProject", obj.GetNamespace(), project), Reason: "spec.project"})
	}
	for _, resource := range applicationResources(obj) {
		edges = append(edges, graph.Edge{Type: "manages", Source: appID, Target: resource.Entity.ID, Reason: "status.resources"})
	}
	return edges, nil
}

type applicationResource struct {
	Entity model.Entity
	Health string
}

func applicationResources(obj *unstructured.Unstructured) []applicationResource {
	items, ok, _ := unstructured.NestedSlice(obj.Object, "status", "resources")
	if !ok {
		return nil
	}

	out := make([]applicationResource, 0, len(items))
	for _, item := range items {
		resource, ok := item.(map[string]any)
		if !ok {
			continue
		}
		group := stringValue(resource["group"])
		version := stringValue(resource["version"])
		kind := stringValue(resource["kind"])
		name := stringValue(resource["name"])
		if kind == "" || name == "" {
			continue
		}
		out = append(out, applicationResource{
			Entity: model.Entity{
				ID:         graph.NodeID(kind, stringValue(resource["namespace"]), name),
				APIVersion: apiVersion(group, version),
				Kind:       kind,
				Namespace:  stringValue(resource["namespace"]),
				Name:       name,
				Status:     resourceStatus(resource),
			},
			Health: stringValue(resource["health"]),
		})
	}
	return out
}

func entityFromUnstructured(obj *unstructured.Unstructured) model.Entity {
	status := ""
	if health, ok, _ := unstructured.NestedString(obj.Object, "status", "health", "status"); ok {
		status = health
	}
	return model.Entity{
		ID:         graph.NodeID(obj.GetKind(), obj.GetNamespace(), obj.GetName()),
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
		UID:        string(obj.GetUID()),
		Labels:     copyStringMap(obj.GetLabels()),
		Status:     status,
	}
}

func resourceStatus(resource map[string]any) string {
	if health := stringValue(resource["health"]); health != "" {
		return health
	}
	return stringValue(resource["status"])
}

func addNestedString(out map[string]string, key string, obj *unstructured.Unstructured, fields ...string) {
	value, ok, _ := unstructured.NestedString(obj.Object, fields...)
	if ok && value != "" {
		out[key] = value
	}
}

func nodeFromEntity(entity model.Entity) graph.Node {
	return graph.Node{
		ID:        entity.ID,
		Kind:      entity.Kind,
		Namespace: entity.Namespace,
		Name:      entity.Name,
		Labels:    copyStringMap(entity.Labels),
		Status:    entity.Status,
	}
}

func apiVersion(group, version string) string {
	if group == "" {
		return version
	}
	if version == "" {
		return group
	}
	return group + "/" + version
}

func severityForHealth(status string) string {
	switch status {
	case "Degraded", "Missing":
		return "error"
	case "Suspended", "Progressing", "Unknown":
		return "warning"
	default:
		return "info"
	}
}

func operationPhaseIsProblem(phase string) bool {
	switch phase {
	case "Error", "Failed":
		return true
	default:
		return false
	}
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func copyStringMap(in map[string]string) map[string]string {
	return redaction.StringMap(in)
}
