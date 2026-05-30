// Package storage provides the storage layer for the inventory service.
package storage

import (
	"context"
	"time"

	"github.com/iyzitrace/inventory-service/internal/semrev/model"
)

// Store defines the interface for entity and relation storage.
type Store interface {
	// UpsertEntities inserts or updates entities.
	UpsertEntities(ctx context.Context, entities []*model.Entity) error

	// UpsertRelations inserts or updates relations.
	UpsertRelations(ctx context.Context, relations []*model.Relation) error

	// GetEntity retrieves an entity by ID.
	GetEntity(ctx context.Context, id string) (*model.Entity, error)

	// GetRelation retrieves a relation by from/to/type.
	GetRelation(ctx context.Context, fromID, toID string, relType model.RelationType) (*model.Relation, error)

	// ListEntities lists entities with optional filtering.
	ListEntities(ctx context.Context, filter EntityFilter) ([]*model.Entity, error)

	// CountEntities counts entities matching the filter (ignoring limit/offset).
	CountEntities(ctx context.Context, filter EntityFilter) (int64, error)

	// ListRelations lists relations with optional filtering.
	ListRelations(ctx context.Context, filter RelationFilter) ([]*model.Relation, error)

	// CountRelations counts relations matching the filter (ignoring limit/offset).
	CountRelations(ctx context.Context, filter RelationFilter) (int64, error)

	// GetEntityRelations gets all relations for an entity.
	GetEntityRelations(ctx context.Context, entityID string, direction RelationDirection) ([]*model.Relation, error)

	// GetStats returns storage statistics.
	GetStats(ctx context.Context) (*Stats, error)

	// MarkStaleEntities marks entities as stale if not seen since the given time.
	MarkStaleEntities(ctx context.Context, staleThreshold time.Time) (int64, error)

	// MarkInstancesStale marks instances as stale within entities.
	MarkInstancesStale(ctx context.Context, staleThreshold time.Time) (int64, error)

	// Close closes the store.
	Close() error
}

// RelationDirection specifies the direction for relation queries.
type RelationDirection string

const (
	RelationDirectionIncoming RelationDirection = "incoming"
	RelationDirectionOutgoing RelationDirection = "outgoing"
	RelationDirectionBoth     RelationDirection = "both"
)

// EntityFilter defines filtering options for entity queries.
type EntityFilter struct {
	Types       []model.EntityType
	Status      []model.EntityStatus // Filter by status (active, stale, stopped)
	NamePattern string
	Limit       int
	Offset      int
}

// RelationFilter defines filtering options for relation queries.
type RelationFilter struct {
	Types    []model.RelationType
	FromID   string
	ToID     string
	Limit    int
	Offset   int
}

// Stats holds storage statistics.
type Stats struct {
	EntityCount   int64
	RelationCount int64
	EntityTypes   map[model.EntityType]int64
	RelationTypes map[model.RelationType]int64
}
