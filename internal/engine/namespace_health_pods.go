package engine

import (
	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
)

// namespacePodHealth converts Pods into compact readiness and restart summaries.
func namespacePodHealth(pods []corev1.Pod) []model.PodHealth {
	out := make([]model.PodHealth, 0, len(pods))
	for _, pod := range pods {
		out = append(out, model.PodHealth{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			Phase:     string(pod.Status.Phase),
			Ready:     isPodReady(&pod),
			Restarts:  podRestarts(&pod),
			Reason:    podHealthReason(&pod),
			Node:      pod.Spec.NodeName,
		})
	}
	return out
}

// podHealthReason chooses the most operator-relevant Pod reason from container
// state, falling back to phase.
func podHealthReason(pod *corev1.Pod) string {
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			return status.State.Waiting.Reason
		}
		if status.State.Terminated != nil && status.State.Terminated.Reason != "" {
			return status.State.Terminated.Reason
		}
	}
	if pod.Status.Phase != "" {
		return string(pod.Status.Phase)
	}
	return ""
}
