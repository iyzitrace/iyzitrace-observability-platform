package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/getlawrence/lawrence-oss/internal/services"
)

// HealthHandlers handles health check endpoints
type HealthHandlers struct {
	agentService services.AgentService
	logger       *zap.Logger
}

// NewHealthHandlers creates a new health handlers instance
func NewHealthHandlers(agentService services.AgentService, logger *zap.Logger) *HealthHandlers {
	return &HealthHandlers{
		agentService: agentService,
		logger:       logger,
	}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Version   string            `json:"version"`
	Services  map[string]string `json:"services"`
}

// handleHealth handles GET /health
func (h *HealthHandlers) HandleHealth(c *gin.Context) {
	sqliteHealthy := h.checkSQLiteHealth(c)

	status := "healthy"
	if !sqliteHealthy {
		status = "unhealthy"
	}

	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Version:   "0.1.0",
		Services: map[string]string{
			"sqlite": h.getHealthStatus(sqliteHealthy),
		},
	}

	httpStatus := http.StatusOK
	if status == "unhealthy" {
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, response)
}

// checkSQLiteHealth checks if SQLite is healthy
func (h *HealthHandlers) checkSQLiteHealth(c *gin.Context) bool {
	_, err := h.agentService.ListAgents(c.Request.Context())
	return err == nil
}

// getHealthStatus converts boolean to status string
func (h *HealthHandlers) getHealthStatus(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "unhealthy"
}
