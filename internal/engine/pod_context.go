package engine

import (
	"context"

	"github.com/lucasepe/kctx/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const defaultEventLimit = 8

// ResolvePodContextRequest identifies the Pod to explain and controls how many
// recent related events are included.
type ResolvePodContextRequest struct {
	Namespace  string
	Name       string
	EventLimit int
}

// ResolvePodContextResponse is the normalized context envelope for one Pod.
type ResolvePodContextResponse struct {
	Pod       model.Entity      `json:"pod"`
	Status    model.PodStatus   `json:"status"`
	Owners    []model.Entity    `json:"owners"`
	Node      *model.Entity     `json:"node,omitempty"`
	Services  []model.Entity    `json:"services"`
	Volumes   []model.VolumeRef `json:"volumes"`
	Events    []model.Event     `json:"events"`
	Signals   []model.Signal    `json:"signals"`
	Relations []model.Relation  `json:"relations"`
}

// ResolvePodContext resolves a Pod into identity, status, dependencies,
// selectors, ownership, events, and factual signals.
func (e *TypedEngine) ResolvePodContext(ctx context.Context, req ResolvePodContextRequest) (*ResolvePodContextResponse, error) {
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.EventLimit <= 0 {
		req.EventLimit = defaultEventLimit
	}

	pod, err := e.kube.GetPod(ctx, req.Namespace, req.Name)
	if err != nil {
		return nil, err
	}

	podEntity := podEntity(pod)
	owners, relations := e.resolveOwners(ctx, pod, podEntity)
	node := resolveNode(pod)
	if node != nil {
		relations = append(relations, model.Relation{Type: "scheduled_on", Source: podEntity, Target: *node})
	}

	volumes := resolveVolumes(pod)
	for _, volume := range volumes {
		relations = append(relations, volumeRelation(podEntity, pod.Namespace, volume))
	}

	services, serviceRelations, err := e.resolveServices(ctx, pod, podEntity)
	if err != nil {
		return nil, err
	}
	relations = append(relations, serviceRelations...)

	events, err := e.resolveEvents(ctx, pod, req.EventLimit)
	if err != nil {
		return nil, err
	}

	status := podStatus(pod)
	signals := buildSignals(status, events)

	return &ResolvePodContextResponse{
		Pod:       podEntity,
		Status:    status,
		Owners:    owners,
		Node:      node,
		Services:  services,
		Volumes:   volumes,
		Events:    events,
		Signals:   signals,
		Relations: relations,
	}, nil
}

// resolveOwners follows the Pod owner references and expands ReplicaSet to
// Deployment when that relationship is available.
func (e *TypedEngine) resolveOwners(ctx context.Context, pod *corev1.Pod, podEntity model.Entity) ([]model.Entity, []model.Relation) {
	var owners []model.Entity
	var relations []model.Relation

	for _, ref := range pod.OwnerReferences {
		owner := entityFromOwnerRef(pod.Namespace, ref)
		owners = append(owners, owner)
		relations = append(relations, ownerRelation(podEntity, pod.Namespace, ref))

		if ref.Kind == "ReplicaSet" {
			rsOwners := e.resolveReplicaSetOwners(ctx, pod.Namespace, ref.Name, owner)
			owners = append(owners, rsOwners.owners...)
			relations = append(relations, rsOwners.relations...)
		}
	}

	return owners, relations
}

// ownerResolution groups owner entities with the relations that connect them.
type ownerResolution struct {
	owners    []model.Entity
	relations []model.Relation
}

// resolveReplicaSetOwners expands a ReplicaSet owner chain one level, most
// commonly from ReplicaSet to Deployment.
func (e *TypedEngine) resolveReplicaSetOwners(ctx context.Context, namespace, name string, source model.Entity) ownerResolution {
	rs, err := e.kube.GetReplicaSet(ctx, namespace, name)
	if err != nil {
		return ownerResolution{}
	}

	var result ownerResolution
	for _, ref := range rs.OwnerReferences {
		owner := entityFromOwnerRef(namespace, ref)
		if ref.Kind == "Deployment" {
			if deploy, err := e.kube.GetDeployment(ctx, namespace, ref.Name); err == nil {
				owner = deploymentEntity(deploy)
			}
		}
		result.owners = append(result.owners, owner)
		result.relations = append(result.relations, model.Relation{Type: "owned_by", Source: source, Target: owner})
	}
	return result
}

// resolveServices finds Services whose selectors match the Pod labels.
func (e *TypedEngine) resolveServices(ctx context.Context, pod *corev1.Pod, podEntity model.Entity) ([]model.Entity, []model.Relation, error) {
	services, err := e.kube.ListServices(ctx, pod.Namespace)
	if err != nil {
		return nil, nil, err
	}

	var matches []model.Entity
	var relations []model.Relation
	for _, svc := range services {
		if selectorMatches(svc.Spec.Selector, pod.Labels) {
			entity := serviceEntity(&svc)
			matches = append(matches, entity)
			relations = append(relations, model.Relation{
				Type:       "selected_by",
				Source:     podEntity,
				Target:     entity,
				Confidence: "high",
				Reason:     "service selector matches pod labels",
			})
		}
	}
	sortEntities(matches)
	return matches, relations, nil
}

// resolveNode returns a Node entity when the Pod has been scheduled.
func resolveNode(pod *corev1.Pod) *model.Entity {
	if pod.Spec.NodeName == "" {
		return nil
	}
	return &model.Entity{Kind: "Node", Name: pod.Spec.NodeName}
}

// podEntity normalizes a corev1 Pod into the shared entity model.
func podEntity(pod *corev1.Pod) model.Entity {
	return entityFromObject("v1", "Pod", pod)
}

// serviceEntity normalizes a corev1 Service into the shared entity model.
func serviceEntity(service *corev1.Service) model.Entity {
	return entityFromObject("v1", "Service", service)
}

// deploymentEntity normalizes an apps/v1 Deployment into the shared entity model.
func deploymentEntity(deploy *appsv1.Deployment) model.Entity {
	return entityFromObject("apps/v1", "Deployment", deploy)
}
