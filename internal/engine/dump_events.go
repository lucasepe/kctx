package engine

import (
	"sort"

	"github.com/lucasepe/kctx/internal/model"
	"github.com/lucasepe/kctx/internal/redaction"
	corev1 "k8s.io/api/core/v1"
)

// dumpWarningEvents returns the newest Warning events in dump-safe normalized
// form.
func dumpWarningEvents(events []corev1.Event, limit int) []model.DumpEventSummary {
	var warnings []corev1.Event
	for _, event := range events {
		if event.Type == corev1.EventTypeWarning {
			warnings = append(warnings, event)
		}
	}
	sort.SliceStable(warnings, func(i, j int) bool { return eventTime(warnings[i]).After(eventTime(warnings[j])) })
	if len(warnings) > limit {
		warnings = warnings[:limit]
	}
	out := make([]model.DumpEventSummary, 0, len(warnings))
	for _, event := range warnings {
		out = append(out, model.DumpEventSummary{Type: event.Type, Reason: event.Reason, Message: redaction.Text(event.Message), ObjectKind: event.InvolvedObject.Kind, ObjectName: event.InvolvedObject.Name, Timestamp: eventTime(event)})
	}
	return out
}
