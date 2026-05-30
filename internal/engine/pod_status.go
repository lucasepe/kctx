package engine

import (
	"sort"

	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
)

// podStatus extracts the stable status fields that are useful for humans and
// tools without exposing the full Kubernetes status object.
func podStatus(pod *corev1.Pod) model.PodStatus {
	status := model.PodStatus{
		Phase: string(pod.Status.Phase),
		Ready: isPodReady(pod),
	}
	for _, condition := range pod.Status.Conditions {
		status.Conditions = append(status.Conditions, model.PodCondition{
			Type:    string(condition.Type),
			Status:  string(condition.Status),
			Reason:  condition.Reason,
			Message: condition.Message,
		})
	}
	for _, container := range pod.Status.ContainerStatuses {
		item := model.ContainerStatus{
			Name:         container.Name,
			Ready:        container.Ready,
			RestartCount: container.RestartCount,
		}
		item.State, item.Reason, item.Message = containerState(container.State)
		item.LastState, item.LastStateReason, item.LastStateMessage = containerLastState(container.LastTerminationState)
		status.Restarts += container.RestartCount
		status.Containers = append(status.Containers, item)
	}
	sort.Slice(status.Containers, func(i, j int) bool {
		return status.Containers[i].Name < status.Containers[j].Name
	})
	return status
}

// isPodReady returns true only when the PodReady condition is explicitly true.
func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// containerState converts Kubernetes' one-of container state into a compact
// state/reason/message tuple.
func containerState(state corev1.ContainerState) (string, string, string) {
	switch {
	case state.Waiting != nil:
		return "waiting", state.Waiting.Reason, state.Waiting.Message
	case state.Running != nil:
		return "running", "", ""
	case state.Terminated != nil:
		return "terminated", state.Terminated.Reason, state.Terminated.Message
	default:
		return "", "", ""
	}
}

// containerLastState converts the previous container state into the same compact
// tuple used for current state.
func containerLastState(state corev1.ContainerState) (string, string, string) {
	switch {
	case state.Waiting != nil:
		return "waiting", state.Waiting.Reason, state.Waiting.Message
	case state.Running != nil:
		return "running", "", ""
	case state.Terminated != nil:
		return "terminated", state.Terminated.Reason, state.Terminated.Message
	default:
		return "", "", ""
	}
}
