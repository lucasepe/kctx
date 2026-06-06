package engine

import "github.com/lucasepe/kctx/internal/model"

// dedupeEntities removes duplicate entities by kind/namespace/name.
func dedupeEntities(in []model.Entity) []model.Entity {
	seen := map[string]model.Entity{}
	for _, entity := range in {
		seen[entityKey(entity)] = entity
	}
	out := make([]model.Entity, 0, len(seen))
	for _, entity := range seen {
		out = append(out, entity)
	}
	return out
}

// dedupeRelations removes duplicate relations by type, endpoints, and reason.
func dedupeRelations(in []model.Relation) []model.Relation {
	seen := map[string]model.Relation{}
	for _, relation := range in {
		seen[relationKey(relation)] = relation
	}
	out := make([]model.Relation, 0, len(seen))
	for _, relation := range seen {
		out = append(out, relation)
	}
	return out
}

// entityKey returns the stable identity key for a normalized entity.
func entityKey(entity model.Entity) string {
	return entity.Kind + "/" + entity.Namespace + "/" + entity.Name
}

// relationKey returns the stable identity key for a normalized relation.
func relationKey(relation model.Relation) string {
	return relation.Type + "|" + entityKey(relation.Source) + "|" + entityKey(relation.Target) + "|" + relation.Reason
}

// severityRank assigns an ordering weight to supported signal severities.
func severityRank(severity string) int {
	switch severity {
	case "error":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}
