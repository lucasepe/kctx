package engine

import (
	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/model"
)

// dumpBuilder accumulates dump entities and relations while deduplicating by
// deterministic IDs.
type dumpBuilder struct {
	entities  map[string]model.DumpEntity
	relations map[string]model.DumpRelation
}

// newDumpBuilder creates an empty namespace dump accumulator.
func newDumpBuilder() *dumpBuilder {
	return &dumpBuilder{entities: map[string]model.DumpEntity{}, relations: map[string]model.DumpRelation{}}
}

// addEntity stores a normalized entity, filling its deterministic ID when
// needed.
func (b *dumpBuilder) addEntity(entity model.DumpEntity) {
	if entity.ID == "" {
		entity.ID = dumpID(entity.Kind, entity.Namespace, entity.Name)
	}
	if _, exists := b.entities[entity.ID]; !exists {
		entity.Labels = copyMap(entity.Labels)
		b.entities[entity.ID] = entity
	}
}

// addRelation stores a normalized relation and ignores incomplete edges.
func (b *dumpBuilder) addRelation(relation model.DumpRelation) {
	if relation.Type == "" || relation.Source == "" || relation.Target == "" {
		return
	}
	key := relation.Source + "\x00" + relation.Type + "\x00" + relation.Target + "\x00" + relation.Reason
	b.relations[key] = relation
}

// entityList returns the accumulated entities; sorting is handled later.
func (b *dumpBuilder) entityList() []model.DumpEntity {
	out := make([]model.DumpEntity, 0, len(b.entities))
	for _, entity := range b.entities {
		out = append(out, entity)
	}
	return out
}

// relationList returns the accumulated relations; sorting is handled later.
func (b *dumpBuilder) relationList() []model.DumpRelation {
	out := make([]model.DumpRelation, 0, len(b.relations))
	for _, relation := range b.relations {
		out = append(out, relation)
	}
	return out
}

// dumpID centralizes deterministic IDs for dump entities and relations.
func dumpID(kind, namespace, name string) string {
	return graph.NodeID(kind, namespace, name)
}
