package engine

import (
	"context"

	"github.com/lucasepe/kctx/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

// addDumpOwnership converts ownerReferences into owns relations for supported
// namespace resources.
func addDumpOwnership(builder *dumpBuilder, pods []corev1.Pod, replicaSets []appsv1.ReplicaSet, jobs []batchv1.Job) {
	for _, pod := range pods {
		child := dumpID("Pod", pod.Namespace, pod.Name)
		for _, ref := range pod.OwnerReferences {
			builder.addRelation(model.DumpRelation{Type: "owns", Source: dumpID(ref.Kind, pod.Namespace, ref.Name), Target: child})
		}
	}
	for _, rs := range replicaSets {
		child := dumpID("ReplicaSet", rs.Namespace, rs.Name)
		for _, ref := range rs.OwnerReferences {
			builder.addRelation(model.DumpRelation{Type: "owns", Source: dumpID(ref.Kind, rs.Namespace, ref.Name), Target: child})
		}
	}
	for _, job := range jobs {
		child := dumpID("Job", job.Namespace, job.Name)
		for _, ref := range job.OwnerReferences {
			builder.addRelation(model.DumpRelation{Type: "owns", Source: dumpID(ref.Kind, job.Namespace, ref.Name), Target: child})
		}
	}
}

// addDumpServiceRelations links Services to Pods selected by their label
// selectors.
func addDumpServiceRelations(builder *dumpBuilder, services []corev1.Service, pods []corev1.Pod) {
	for _, service := range services {
		if len(service.Spec.Selector) == 0 {
			continue
		}
		serviceID := dumpID("Service", service.Namespace, service.Name)
		for _, pod := range pods {
			if selectorMatches(service.Spec.Selector, pod.Labels) {
				builder.addRelation(model.DumpRelation{Type: "selects", Source: serviceID, Target: dumpID("Pod", pod.Namespace, pod.Name), Reason: "service selector matches pod labels"})
			}
		}
	}
}

// addDumpEndpointSliceRelations links Services to EndpointSlices and
// EndpointSlices to target Pods when correlation is possible.
func addDumpEndpointSliceRelations(builder *dumpBuilder, slices []discoveryv1.EndpointSlice, podByName, podByIP map[string]corev1.Pod) {
	for _, slice := range slices {
		sliceID := dumpID("EndpointSlice", slice.Namespace, slice.Name)
		if serviceName := slice.Labels[serviceNameLabel]; serviceName != "" {
			builder.addRelation(model.DumpRelation{Type: "has_endpoint", Source: dumpID("Service", slice.Namespace, serviceName), Target: sliceID})
		}
		for _, endpoint := range slice.Endpoints {
			podName := targetPodName(endpoint.TargetRef)
			for _, address := range endpoint.Addresses {
				if podName == "" {
					if pod, ok := podByIP[address]; ok {
						podName = pod.Name
					}
				}
				if pod, ok := podByName[podName]; ok {
					builder.addRelation(model.DumpRelation{Type: "endpoint_targets", Source: sliceID, Target: dumpID("Pod", pod.Namespace, pod.Name)})
				}
			}
		}
	}
}

// addDumpPodRelations adds scheduling, node, storage, and configuration
// relations for Pods.
func addDumpPodRelations(ctx context.Context, e *TypedEngine, builder *dumpBuilder, pods []corev1.Pod) {
	for _, pod := range pods {
		podID := dumpID("Pod", pod.Namespace, pod.Name)
		if pod.Spec.NodeName != "" {
			nodeEntity := model.DumpEntity{ID: dumpID("Node", "", pod.Spec.NodeName), Kind: "Node", Name: pod.Spec.NodeName}
			if node, err := e.kube.GetNode(ctx, pod.Spec.NodeName); err == nil {
				nodeEntity.UID = string(node.UID)
				nodeEntity.Labels = copyMap(node.Labels)
				nodeEntity.Status = nodeReadyStatus(node)
			}
			builder.addEntity(nodeEntity)
			builder.addRelation(model.DumpRelation{Type: "scheduled_on", Source: podID, Target: nodeEntity.ID})
		}
		for _, volume := range resolveVolumes(&pod) {
			switch volume.Type {
			case "PVC":
				builder.addRelation(model.DumpRelation{Type: "mounts_pvc", Source: podID, Target: dumpID("PersistentVolumeClaim", pod.Namespace, volume.Name)})
			case "Secret":
				builder.addRelation(model.DumpRelation{Type: "uses_secret", Source: podID, Target: dumpID("Secret", pod.Namespace, volume.Name)})
			case "ConfigMap":
				builder.addRelation(model.DumpRelation{Type: "uses_configmap", Source: podID, Target: dumpID("ConfigMap", pod.Namespace, volume.Name)})
			}
		}
	}
}
