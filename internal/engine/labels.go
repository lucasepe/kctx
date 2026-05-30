package engine

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

// copyMap returns a defensive copy and preserves nil/empty maps as nil for
// compact JSON.
func copyMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
