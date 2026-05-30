package engine

import (
	"sort"

	"github.com/lucasepe/kctx/internal/model"
)

// sortEntities orders entities by kind and name for deterministic output.
func sortEntities(entities []model.Entity) {
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].Kind == entities[j].Kind {
			return entities[i].Name < entities[j].Name
		}
		return entities[i].Kind < entities[j].Kind
	})
}
