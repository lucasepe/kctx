package engine

import (
	"context"
	"fmt"

	"github.com/lucasepe/kctx/internal/graph"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildPodGraphRequest identifies the Pod used as the root of a dependency and
// ownership graph.
type BuildPodGraphRequest struct {
	Namespace string
	Name      string
}

// BuildPodGraph constructs a deterministic graph of resources directly related
// to a Pod.
func (e *TypedEngine) BuildPodGraph(ctx context.Context, req BuildPodGraphRequest) (*graph.Graph, error) {
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	pod, err := e.kube.GetPod(ctx, req.Namespace, req.Name)
	if err != nil {
		return nil, err
	}

	builder := graph.NewBuilder()
	podNode := builder.AddNode(nodeFromPod(pod))

	if err := e.addOwnerChain(ctx, builder, pod.Namespace, pod.OwnerReferences, podNode.ID, map[string]bool{}); err != nil {
		return nil, err
	}
	e.addPodNode(ctx, builder, pod, podNode.ID)

	if err := e.addSelectingServices(ctx, builder, pod, podNode.ID); err != nil {
		return nil, err
	}
	addPodDependencies(builder, pod, podNode.ID)

	out := builder.Graph()
	return &out, nil
}

// addOwnerChain adds owner nodes and owns edges by recursively following owner
// references.
func (e *TypedEngine) addOwnerChain(ctx context.Context, builder *graph.Builder, namespace string, refs []metav1.OwnerReference, childID string, seen map[string]bool) error {
	for _, ref := range refs {
		owner, ownerRefs, err := e.ownerNodeAndRefs(ctx, namespace, ref)
		if err != nil {
			return err
		}
		owner = builder.AddNode(owner)
		builder.AddEdge(graph.Edge{Type: "owns", Source: owner.ID, Target: childID})

		if seen[owner.ID] {
			continue
		}
		seen[owner.ID] = true
		if err := e.addOwnerChain(ctx, builder, namespace, ownerRefs, owner.ID, seen); err != nil {
			return err
		}
	}
	return nil
}

// ownerNodeAndRefs fetches a typed owner when possible and falls back to the
// owner reference metadata when the object is unavailable.
func (e *TypedEngine) ownerNodeAndRefs(ctx context.Context, namespace string, ref metav1.OwnerReference) (graph.Node, []metav1.OwnerReference, error) {
	generic := graphNodeFromOwnerRef(namespace, ref)

	switch ref.Kind {
	case "ReplicaSet":
		rs, err := e.kube.GetReplicaSet(ctx, namespace, ref.Name)
		if err != nil {
			return fallbackOwner(generic, err)
		}
		return nodeFromReplicaSet(rs), rs.OwnerReferences, nil
	case "Deployment":
		deploy, err := e.kube.GetDeployment(ctx, namespace, ref.Name)
		if err != nil {
			return fallbackOwner(generic, err)
		}
		return nodeFromDeployment(deploy), deploy.OwnerReferences, nil
	case "StatefulSet":
		statefulSet, err := e.kube.GetStatefulSet(ctx, namespace, ref.Name)
		if err != nil {
			return fallbackOwner(generic, err)
		}
		return nodeFromStatefulSet(statefulSet), statefulSet.OwnerReferences, nil
	case "DaemonSet":
		daemonSet, err := e.kube.GetDaemonSet(ctx, namespace, ref.Name)
		if err != nil {
			return fallbackOwner(generic, err)
		}
		return nodeFromDaemonSet(daemonSet), daemonSet.OwnerReferences, nil
	case "Job":
		job, err := e.kube.GetJob(ctx, namespace, ref.Name)
		if err != nil {
			return fallbackOwner(generic, err)
		}
		return nodeFromJob(job), job.OwnerReferences, nil
	case "CronJob":
		cronJob, err := e.kube.GetCronJob(ctx, namespace, ref.Name)
		if err != nil {
			return fallbackOwner(generic, err)
		}
		return nodeFromCronJob(cronJob), cronJob.OwnerReferences, nil
	default:
		return generic, nil, nil
	}
}

// fallbackOwner preserves graph shape for missing or forbidden owners while
// surfacing unexpected API errors.
func fallbackOwner(node graph.Node, err error) (graph.Node, []metav1.OwnerReference, error) {
	if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
		return node, nil, nil
	}
	return graph.Node{}, nil, err
}

// addPodNode adds the scheduled Node and the scheduled_on edge when the Pod has
// a node assignment.
func (e *TypedEngine) addPodNode(ctx context.Context, builder *graph.Builder, pod *corev1.Pod, podID string) {
	if pod.Spec.NodeName == "" {
		return
	}

	node := graph.Node{ID: graph.NodeID("Node", "", pod.Spec.NodeName), Kind: "Node", Name: pod.Spec.NodeName}
	if kubeNode, err := e.kube.GetNode(ctx, pod.Spec.NodeName); err == nil {
		node = nodeFromNode(kubeNode)
	}
	node = builder.AddNode(node)
	builder.AddEdge(graph.Edge{Type: "scheduled_on", Source: node.ID, Target: podID})
}

// addSelectingServices adds Services whose selectors match the Pod labels.
func (e *TypedEngine) addSelectingServices(ctx context.Context, builder *graph.Builder, pod *corev1.Pod, podID string) error {
	services, err := e.kube.ListServices(ctx, pod.Namespace)
	if err != nil {
		return err
	}
	for _, service := range services {
		if !selectorMatches(service.Spec.Selector, pod.Labels) {
			continue
		}
		serviceNode := builder.AddNode(nodeFromService(&service))
		builder.AddEdge(graph.Edge{
			Type:   "selects",
			Source: serviceNode.ID,
			Target: podID,
			Reason: "service selector matches pod labels",
		})
	}
	return nil
}

// addPodDependencies adds ConfigMap, Secret, and PVC dependency nodes and edges.
func addPodDependencies(builder *graph.Builder, pod *corev1.Pod, podID string) {
	for _, volume := range resolveVolumes(pod) {
		node := dependencyNode(pod.Namespace, volume.Type, volume.Name)
		node = builder.AddNode(node)

		edgeType := "uses_configmap"
		switch volume.Type {
		case "PVC":
			edgeType = "mounts_pvc"
		case "Secret":
			edgeType = "uses_secret"
		}
		builder.AddEdge(graph.Edge{Type: edgeType, Source: podID, Target: node.ID})
	}
}

// dependencyNode converts a dependency reference into the graph node identity
// used by pod graphs.
func dependencyNode(namespace, kind, name string) graph.Node {
	if kind == "PVC" {
		kind = "PersistentVolumeClaim"
	}
	return graph.Node{
		ID:        graph.NodeID(kind, namespace, name),
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
	}
}

// nodeFromPod normalizes a Pod into a graph node.
func nodeFromPod(pod *corev1.Pod) graph.Node {
	return graphNodeFromObject("Pod", string(pod.Status.Phase), pod)
}

// nodeFromService normalizes a Service into a graph node.
func nodeFromService(service *corev1.Service) graph.Node {
	return graphNodeFromObject("Service", "", service)
}

// nodeFromNode normalizes a Kubernetes Node into a graph node.
func nodeFromNode(node *corev1.Node) graph.Node {
	return graph.Node{
		ID:     graph.NodeID("Node", "", node.Name),
		Kind:   "Node",
		Name:   node.Name,
		Labels: copyMap(node.Labels),
		Status: nodeReadyStatus(node),
	}
}

// nodeFromReplicaSet normalizes a ReplicaSet into a workload graph node.
func nodeFromReplicaSet(rs *appsv1.ReplicaSet) graph.Node {
	return workloadNode("ReplicaSet", rs.Namespace, rs.Name, rs.Labels, fmt.Sprintf("%d/%d ready", rs.Status.ReadyReplicas, rs.Status.Replicas))
}

// nodeFromDeployment normalizes a Deployment into a workload graph node.
func nodeFromDeployment(deploy *appsv1.Deployment) graph.Node {
	return workloadNode("Deployment", deploy.Namespace, deploy.Name, deploy.Labels, fmt.Sprintf("%d/%d ready", deploy.Status.ReadyReplicas, deploy.Status.Replicas))
}

// nodeFromStatefulSet normalizes a StatefulSet into a workload graph node.
func nodeFromStatefulSet(statefulSet *appsv1.StatefulSet) graph.Node {
	return workloadNode("StatefulSet", statefulSet.Namespace, statefulSet.Name, statefulSet.Labels, fmt.Sprintf("%d/%d ready", statefulSet.Status.ReadyReplicas, statefulSet.Status.Replicas))
}

// nodeFromDaemonSet normalizes a DaemonSet into a workload graph node.
func nodeFromDaemonSet(daemonSet *appsv1.DaemonSet) graph.Node {
	return workloadNode("DaemonSet", daemonSet.Namespace, daemonSet.Name, daemonSet.Labels, fmt.Sprintf("%d/%d ready", daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled))
}

// nodeFromJob normalizes a Job into a workload graph node.
func nodeFromJob(job *batchv1.Job) graph.Node {
	return workloadNode("Job", job.Namespace, job.Name, job.Labels, fmt.Sprintf("%d succeeded, %d failed", job.Status.Succeeded, job.Status.Failed))
}

// nodeFromCronJob normalizes a CronJob into a workload graph node.
func nodeFromCronJob(cronJob *batchv1.CronJob) graph.Node {
	status := "active"
	if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
		status = "suspended"
	}
	return workloadNode("CronJob", cronJob.Namespace, cronJob.Name, cronJob.Labels, status)
}

// workloadNode constructs the common graph node shape for workload resources.
func workloadNode(kind, namespace, name string, labels map[string]string, status string) graph.Node {
	return graph.Node{
		ID:        graph.NodeID(kind, namespace, name),
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		Labels:    copyMap(labels),
		Status:    status,
	}
}

// nodeReadyStatus returns the NodeReady condition status when present.
func nodeReadyStatus(node *corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return string(condition.Status)
		}
	}
	return ""
}
