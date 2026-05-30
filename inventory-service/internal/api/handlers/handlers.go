// Package handlers provides HTTP API handlers for the inventory service.
package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/iyzitrace/inventory-service/internal/semrev/model"
	"github.com/iyzitrace/inventory-service/internal/storage"
)

// Handlers provides HTTP API handlers.
type Handlers struct {
	store  storage.Store
	logger *zap.Logger
}

// NewHandlers creates new API handlers.
func NewHandlers(store storage.Store, logger *zap.Logger) *Handlers {
	return &Handlers{
		store:  store,
		logger: logger.Named("api"),
	}
}

// RegisterRoutes registers API routes.
func (h *Handlers) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1")
	{
		// Health
		api.GET("/health", h.Health)

		// Entities
		api.GET("/entities", h.ListEntities)
		api.GET("/entities/:id", h.GetEntity)
		api.GET("/entities/:id/relations", h.GetEntityRelations)

		// Relations
		api.GET("/relations", h.ListRelations)

		// Stats
		api.GET("/stats", h.GetStats)

		// Topology (graph view)
		api.GET("/topology", h.GetTopology)
	}
}

// Health returns the health status.
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}

// ListEntities lists entities with optional filtering.
func (h *Handlers) ListEntities(c *gin.Context) {
	filter := storage.EntityFilter{
		Limit:  100,
		Offset: 0,
	}

	// Parse query parameters
	if types := c.QueryArray("type"); len(types) > 0 {
		for _, t := range types {
			filter.Types = append(filter.Types, model.EntityType(t))
		}
	}

	// Parse status filter (e.g., ?status=active&status=stale)
	if statuses := c.QueryArray("status"); len(statuses) > 0 {
		for _, s := range statuses {
			filter.Status = append(filter.Status, model.EntityStatus(s))
		}
	}

	if name := c.Query("name"); name != "" {
		filter.NamePattern = name
	}

	if limit := c.Query("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filter.Limit = l
		}
	}

	if offset := c.Query("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filter.Offset = o
		}
	}

	ctx := c.Request.Context()

	// Get total count for pagination
	total, err := h.store.CountEntities(ctx, filter)
	if err != nil {
		h.logger.Error("Failed to count entities", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count entities"})
		return
	}

	entities, err := h.store.ListEntities(ctx, filter)
	if err != nil {
		h.logger.Error("Failed to list entities", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list entities"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entities": entities,
		"count":    len(entities),
		"total":    total,
		"limit":    filter.Limit,
		"offset":   filter.Offset,
	})
}

// GetEntity retrieves an entity by ID.
func (h *Handlers) GetEntity(c *gin.Context) {
	id := c.Param("id")

	entity, err := h.store.GetEntity(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get entity", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Entity not found"})
		return
	}

	c.JSON(http.StatusOK, entity)
}

// GetEntityRelations retrieves relations for an entity.
func (h *Handlers) GetEntityRelations(c *gin.Context) {
	id := c.Param("id")
	direction := storage.RelationDirectionBoth

	if d := c.Query("direction"); d != "" {
		switch d {
		case "incoming":
			direction = storage.RelationDirectionIncoming
		case "outgoing":
			direction = storage.RelationDirectionOutgoing
		}
	}

	relations, err := h.store.GetEntityRelations(c.Request.Context(), id, direction)
	if err != nil {
		h.logger.Error("Failed to get entity relations", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get relations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"relations": relations,
		"count":     len(relations),
	})
}

// ListRelations lists relations with optional filtering.
func (h *Handlers) ListRelations(c *gin.Context) {
	filter := storage.RelationFilter{
		Limit:  100,
		Offset: 0,
	}

	// Parse query parameters
	if types := c.QueryArray("type"); len(types) > 0 {
		for _, t := range types {
			filter.Types = append(filter.Types, model.RelationType(t))
		}
	}

	if fromID := c.Query("from"); fromID != "" {
		filter.FromID = fromID
	}

	if toID := c.Query("to"); toID != "" {
		filter.ToID = toID
	}

	if limit := c.Query("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filter.Limit = l
		}
	}

	if offset := c.Query("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filter.Offset = o
		}
	}

	ctx := c.Request.Context()

	// Get total count for pagination
	total, err := h.store.CountRelations(ctx, filter)
	if err != nil {
		h.logger.Error("Failed to count relations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count relations"})
		return
	}

	relations, err := h.store.ListRelations(ctx, filter)
	if err != nil {
		h.logger.Error("Failed to list relations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list relations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"relations": relations,
		"count":     len(relations),
		"total":     total,
		"limit":     filter.Limit,
		"offset":    filter.Offset,
	})
}

// GetStats returns storage statistics.
func (h *Handlers) GetStats(c *gin.Context) {
	stats, err := h.store.GetStats(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to get stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// TopologyNode represents a node in the topology graph.
type TopologyNode struct {
	ID    string            `json:"id"`
	Type  model.EntityType  `json:"type"`
	Name  string            `json:"name"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

// TopologyEdge represents an edge in the topology graph.
type TopologyEdge struct {
	From string             `json:"from"`
	To   string             `json:"to"`
	Type model.RelationType `json:"type"`
}

// TopologyResponse represents the topology graph.
type TopologyResponse struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// GetTopology returns the entity topology as a graph.
func (h *Handlers) GetTopology(c *gin.Context) {
	ctx := c.Request.Context()

	// Get all entities
	entities, err := h.store.ListEntities(ctx, storage.EntityFilter{Limit: 1000})
	if err != nil {
		h.logger.Error("Failed to list entities for topology", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get topology"})
		return
	}

	// Get all relations
	relations, err := h.store.ListRelations(ctx, storage.RelationFilter{Limit: 5000})
	if err != nil {
		h.logger.Error("Failed to list relations for topology", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get topology"})
		return
	}

	// Build response
	response := TopologyResponse{
		Nodes: make([]TopologyNode, 0, len(entities)),
		Edges: make([]TopologyEdge, 0, len(relations)),
	}

	for _, entity := range entities {
		response.Nodes = append(response.Nodes, TopologyNode{
			ID:    entity.ID,
			Type:  entity.Type,
			Name:  entity.Name,
			Attrs: entity.Attrs,
		})
	}

	for _, relation := range relations {
		response.Edges = append(response.Edges, TopologyEdge{
			From: relation.FromID,
			To:   relation.ToID,
			Type: relation.Type,
		})
	}

	c.JSON(http.StatusOK, response)
}
