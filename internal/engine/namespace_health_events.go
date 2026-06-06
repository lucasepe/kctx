package engine

import (
	"sort"

	"github.com/lucasepe/kctx/internal/model"
	"github.com/lucasepe/kctx/internal/redaction"
	corev1 "k8s.io/api/core/v1"
)

// namespaceWarningEvents extracts recent Warning events and returns both the
// limited summaries and the total warning count.
func namespaceWarningEvents(events []corev1.Event, limit int) ([]model.EventSummary, int) {
	var warnings []corev1.Event
	for _, event := range events {
		if event.Type == corev1.EventTypeWarning {
			warnings = append(warnings, event)
		}
	}
	sort.Slice(warnings, func(i, j int) bool {
		return eventTime(warnings[i]).After(eventTime(warnings[j]))
	})
	total := len(warnings)
	if limit > 0 && len(warnings) > limit {
		warnings = warnings[:limit]
	}

	out := make([]model.EventSummary, 0, len(warnings))
	for _, event := range warnings {
		out = append(out, model.EventSummary{
			Type:      event.Type,
			Reason:    event.Reason,
			Message:   redaction.Text(event.Message),
			Kind:      event.InvolvedObject.Kind,
			Name:      event.InvolvedObject.Name,
			Timestamp: eventTime(event),
		})
	}
	return out, total
}
