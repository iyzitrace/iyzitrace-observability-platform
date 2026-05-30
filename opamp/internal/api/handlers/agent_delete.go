package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// HandleDeleteAgent handles DELETE /api/v1/agents/:id
func (h *AgentHandlers) HandleDeleteAgent(c *gin.Context) {
	agentID := c.Param("id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Agent ID is required"})
		return
	}

	// Parse UUID
	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID format"})
		return
	}

	// 1. Get agent to check status
	agent, err := h.agentService.GetAgent(c.Request.Context(), agentUUID)
	if err != nil {
		h.logger.Error("Failed to get agent for deletion", zap.String("agent_id", agentID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check agent status"})
		return
	}

	if agent == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// 2. Delete agent
	if err := h.agentService.DeleteAgent(c.Request.Context(), agentUUID); err != nil {
		h.logger.Error("Failed to delete agent", zap.String("agent_id", agentID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete agent"})
		return
	}

	h.logger.Info("Deleted agent", zap.String("agent_id", agentID))
	c.Status(http.StatusNoContent)
}
