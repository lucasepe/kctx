package engine

import (
	"sort"

	"github.com/lucasepe/kctx/internal/model"
)

// sortDump applies deterministic ordering to every collection in a namespace
// dump.
func sortDump(resp *DumpNamespaceResponse) {
	sort.Slice(resp.Entities, func(i, j int) bool {
		if resp.Entities[i].Kind != resp.Entities[j].Kind {
			return resp.Entities[i].Kind < resp.Entities[j].Kind
		}
		return resp.Entities[i].Name < resp.Entities[j].Name
	})
	sort.Slice(resp.Relations, func(i, j int) bool {
		if resp.Relations[i].Source != resp.Relations[j].Source {
			return resp.Relations[i].Source < resp.Relations[j].Source
		}
		if resp.Relations[i].Type != resp.Relations[j].Type {
			return resp.Relations[i].Type < resp.Relations[j].Type
		}
		return resp.Relations[i].Target < resp.Relations[j].Target
	})
	sort.Slice(resp.Events, func(i, j int) bool {
		if !resp.Events[i].Timestamp.Equal(resp.Events[j].Timestamp) {
			return resp.Events[i].Timestamp.After(resp.Events[j].Timestamp)
		}
		return resp.Events[i].ObjectName < resp.Events[j].ObjectName
	})
	sortDumpSignals(resp.Signals)
}

// sortDumpSignals orders signals by severity, reason, entity, and message.
func sortDumpSignals(signals []model.DumpSignal) {
	sort.Slice(signals, func(i, j int) bool {
		if severityRank(signals[i].Severity) != severityRank(signals[j].Severity) {
			return severityRank(signals[i].Severity) > severityRank(signals[j].Severity)
		}
		if signals[i].Reason != signals[j].Reason {
			return signals[i].Reason < signals[j].Reason
		}
		if signals[i].EntityID != signals[j].EntityID {
			return signals[i].EntityID < signals[j].EntityID
		}
		return signals[i].Message < signals[j].Message
	})
}
