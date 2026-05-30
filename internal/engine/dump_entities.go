package engine

import (
	"github.com/lucasepe/kctx/internal/model"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

// namespaceDumpEntity normalizes namespace metadata for the dump.
func namespaceDumpEntity(ns *corev1.Namespace) model.DumpEntity {
	return model.DumpEntity{ID: dumpID("Namespace", "", ns.Name), Kind: "Namespace", Name: ns.Name, UID: string(ns.UID), Labels: copyMap(ns.Labels), Status: string(ns.Status.Phase)}
}

// podDumpEntity normalizes Pod identity, readiness, restart count, and node
// placement for the dump.
func podDumpEntity(pod *corev1.Pod) model.DumpEntity {
	ready := isPodReady(pod)
	entity := model.DumpEntity{ID: dumpID("Pod", pod.Namespace, pod.Name), Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, UID: string(pod.UID), Labels: copyMap(pod.Labels), Status: podHealthReason(pod), Ready: &ready, NodeName: pod.Spec.NodeName, RestartCount: podRestarts(pod)}
	entity.LastState, entity.LastReason = podLastState(pod)
	return entity
}

func podLastState(pod *corev1.Pod) (string, string) {
	for _, status := range pod.Status.ContainerStatuses {
		state, reason, _ := containerLastState(status.LastTerminationState)
		if state != "" || reason != "" {
			return state, reason
		}
	}
	return "", ""
}

// serviceDumpEntity normalizes Service metadata and type.
func serviceDumpEntity(service *corev1.Service) model.DumpEntity {
	return model.DumpEntity{ID: dumpID("Service", service.Namespace, service.Name), Kind: "Service", Namespace: service.Namespace, Name: service.Name, UID: string(service.UID), Labels: copyMap(service.Labels), Status: string(service.Spec.Type)}
}

// endpointSliceDumpEntity normalizes EndpointSlice metadata without expanding
// every endpoint inline.
func endpointSliceDumpEntity(slice *discoveryv1.EndpointSlice) model.DumpEntity {
	return model.DumpEntity{ID: dumpID("EndpointSlice", slice.Namespace, slice.Name), Kind: "EndpointSlice", Namespace: slice.Namespace, Name: slice.Name, UID: string(slice.UID), Labels: copyMap(slice.Labels), Status: string(slice.AddressType)}
}

// workloadDumpEntity constructs the common dump entity shape for workload
// resources.
func workloadDumpEntity(kind, namespace, name, uid string, labels map[string]string, status string) model.DumpEntity {
	return model.DumpEntity{ID: dumpID(kind, namespace, name), Kind: kind, Namespace: namespace, Name: name, UID: uid, Labels: copyMap(labels), Status: status}
}

// metadataDumpEntity constructs compact metadata-only entities such as Secrets,
// ConfigMaps, and PVCs.
func metadataDumpEntity(kind, namespace, name, uid string, labels map[string]string, status string) model.DumpEntity {
	return model.DumpEntity{ID: dumpID(kind, namespace, name), Kind: kind, Namespace: namespace, Name: name, UID: uid, Labels: copyMap(labels), Status: status}
}

// cronJobDumpStatus returns the compact CronJob status used in dumps.
func cronJobDumpStatus(cronJob *batchv1.CronJob) string {
	if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
		return "suspended"
	}
	return "active"
}

// jobCompletions applies Kubernetes' implicit single completion default.
func jobCompletions(job *batchv1.Job) int32 {
	if job.Spec.Completions == nil {
		return 1
	}
	return *job.Spec.Completions
}

// countEntities counts dump entities of a specific kind.
func countEntities(entities []model.DumpEntity, kind string) int {
	count := 0
	for _, entity := range entities {
		if entity.Kind == kind {
			count++
		}
	}
	return count
}
