package engine

import (
	"context"
	"sort"
	"time"

	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
)

// resolveEvents selects recent warning Events involving the Pod.
func (e *TypedEngine) resolveEvents(ctx context.Context, pod *corev1.Pod, limit int) ([]model.Event, error) {
	events, err := e.kube.ListEvents(ctx, pod.Namespace)
	if err != nil {
		return nil, err
	}

	var matched []corev1.Event
	for _, event := range events {
		if event.Type == "Warning" && eventForPod(event, pod) {
			matched = append(matched, event)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return eventTime(matched[i]).After(eventTime(matched[j]))
	})
	matched = dedupePodEvents(matched)
	if len(matched) > limit {
		matched = matched[:limit]
	}

	var out []model.Event
	for _, event := range matched {
		out = append(out, eventModel(event))
	}
	return out, nil
}

func dedupePodEvents(events []corev1.Event) []corev1.Event {
	seen := make(map[string]struct{}, len(events))
	out := make([]corev1.Event, 0, len(events))
	for _, event := range events {
		key := event.Type + "|" + event.Reason + "|" + event.Message
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, event)
	}
	return out
}

// eventForPod checks both UID-based and name/kind-based Event references.
func eventForPod(event corev1.Event, pod *corev1.Pod) bool {
	involved := event.InvolvedObject
	if involved.UID != "" && involved.UID == pod.UID {
		return true
	}
	return involved.Kind == "Pod" && involved.Namespace == pod.Namespace && involved.Name == pod.Name
}

// eventModel strips a Kubernetes Event down to the stable fields kctx exposes.
func eventModel(event corev1.Event) model.Event {
	return model.Event{
		Type:           event.Type,
		Reason:         event.Reason,
		Message:        event.Message,
		Source:         event.Source.Component,
		FirstTimestamp: event.FirstTimestamp.Time,
		LastTimestamp:  event.LastTimestamp.Time,
		Count:          event.Count,
	}
}

// eventTime chooses the best timestamp for deterministic newest-first ordering.
func eventTime(event corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	return event.CreationTimestamp.Time
}
