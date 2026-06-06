package engine

import "github.com/lucasepe/kctx/internal/redaction"

// selectorMatches implements exact Service-style label selector matching for
// map selectors.
func selectorMatches(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, want := range selector {
		if labels[key] != want {
			return false
		}
	}
	return true
}

// copyMap returns a redacted defensive copy and preserves nil/empty maps as nil
// for compact JSON.
func copyMap(in map[string]string) map[string]string {
	return redaction.StringMap(in)
}
