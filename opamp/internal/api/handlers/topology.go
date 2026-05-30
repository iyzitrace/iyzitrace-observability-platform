package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/getlawrence/lawrence-oss/internal/services"
)

// TopologyHandlers handles topology-related API endpoints
type TopologyHandlers struct {
	agentService     services.AgentService
	telemetryService services.TelemetryQueryService
	logger           *zap.Logger
}

// NewTopologyHandlers creates a new topology handlers instance
func NewTopologyHandlers(agentService services.AgentService, telemetryService services.TelemetryQueryService, logger *zap.Logger) *TopologyHandlers {
	return &TopologyHandlers{
		agentService:     agentService,
		telemetryService: telemetryService,
		logger:           logger,
	}
}

// TopologyNode represents a node in the topology graph
type TopologyNode struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"` // "agent", "group", "service"
	Name      string                 `json:"name"`
	Status    string                 `json:"status"`
	GroupID   *string                `json:"group_id,omitempty"`
	GroupName *string                `json:"group_name,omitempty"`
	Labels    map[string]string      `json:"labels"`
	Metrics   *NodeMetrics           `json:"metrics,omitempty"`
	LastSeen  *time.Time             `json:"last_seen,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TopologyEdge represents a connection between nodes
type TopologyEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // "belongs_to", "sends_to", etc.
	Label  string `json:"label,omitempty"`
}

// NodeMetrics represents metrics for a topology node
type NodeMetrics struct {
	MetricCount   int64   `json:"metric_count"`
	LogCount      int64   `json:"log_count"`
	TraceCount    int64   `json:"trace_count"`
	ErrorRate     float64 `json:"error_rate"`
	Latency       float64 `json:"latency"`
	ThroughputRPS float64 `json:"throughput_rps"`
}

// TopologyResponse represents the complete topology graph
type TopologyResponse struct {
	Nodes     []TopologyNode `json:"nodes"`
	Edges     []TopologyEdge `json:"edges"`
	Groups    []GroupSummary `json:"groups"`
	Services  []string       `json:"services"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// GroupSummary represents a group with agent count
type GroupSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AgentCount int    `json:"agent_count"`
	Status     string `json:"status"`
}

// AgentTopologyRequest represents request for agent topology
type AgentTopologyRequest struct {
	AgentID   *string    `json:"agent_id"`
	GroupID   *string    `json:"group_id"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
}

// HandleGetTopology handles GET /api/v1/topology
func (h *TopologyHandlers) HandleGetTopology(c *gin.Context) {
	ctx := c.Request.Context()

	// Get all agents
	agents, err := h.agentService.ListAgents(ctx)
	if err != nil {
		h.logger.Error("Failed to get agents for topology", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch topology"})
		return
	}

	// Get all groups
	groups, err := h.agentService.ListGroups(ctx)
	if err != nil {
		h.logger.Error("Failed to get groups for topology", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch topology"})
		return
	}

	// Build topology
	var nodes []TopologyNode
	var edges []TopologyEdge
	var groupSummaries []GroupSummary

	// Create group nodes and summaries
	groupAgentCount := make(map[string]int)
	for _, group := range groups {
		nodes = append(nodes, TopologyNode{
			ID:     "group-" + group.ID,
			Type:   "group",
			Name:   group.Name,
			Status: "active",
			Labels: group.Labels,
			Metadata: map[string]interface{}{
				"created_at": group.CreatedAt,
			},
		})
		groupAgentCount[group.ID] = 0
	}

	// Create agent nodes
	for _, agent := range agents {
		agentID := agent.ID.String()

		// Get metrics for this agent (default 1h for topology overview)
		metrics := h.getAgentMetrics(ctx, agent.ID, "1h")

		node := TopologyNode{
			ID:        "agent-" + agentID,
			Type:      "agent",
			Name:      agent.Name,
			Status:    string(agent.Status),
			GroupID:   agent.GroupID,
			GroupName: agent.GroupName,
			Labels:    agent.Labels,
			Metrics:   metrics,
			LastSeen:  &agent.LastSeen,
			Metadata: map[string]interface{}{
				"version":      agent.Version,
				"capabilities": agent.Capabilities,
			},
		}
		nodes = append(nodes, node)

		// Create edge from agent to group if assigned
		if agent.GroupID != nil && *agent.GroupID != "" {
			edges = append(edges, TopologyEdge{
				Source: "agent-" + agentID,
				Target: "group-" + *agent.GroupID,
				Type:   "belongs_to",
				Label:  "member of",
			})
			groupAgentCount[*agent.GroupID]++
		}
	}

	// Build group summaries
	for _, group := range groups {
		summary := GroupSummary{
			ID:         group.ID,
			Name:       group.Name,
			AgentCount: groupAgentCount[group.ID],
			Status:     "active",
		}
		if summary.AgentCount == 0 {
			summary.Status = "empty"
		}
		groupSummaries = append(groupSummaries, summary)
	}

	// Get unique services
	servicesQuery := `
		SELECT DISTINCT service_name FROM (
			SELECT service_name FROM metrics_sum
			UNION
			SELECT service_name FROM metrics_gauge
			UNION
			SELECT service_name FROM logs
			UNION
			SELECT service_name FROM traces
		) AS all_services
		WHERE service_name IS NOT NULL AND service_name != ''
		ORDER BY service_name
	`
	var services []string
	if rows, err := h.telemetryService.QueryRaw(ctx, servicesQuery); err == nil {
		for _, row := range rows {
			if svc, ok := row["service_name"].(string); ok && svc != "" {
				services = append(services, svc)
			}
		}
	}

	response := TopologyResponse{
		Nodes:     nodes,
		Edges:     edges,
		Groups:    groupSummaries,
		Services:  services,
		UpdatedAt: time.Now(),
	}

	c.JSON(http.StatusOK, response)
}

// HandleGetAgentTopology handles GET /api/v1/topology/agent/:id
func (h *TopologyHandlers) HandleGetAgentTopology(c *gin.Context) {
	agentIDStr := c.Param("id")
	if agentIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Agent ID is required"})
		return
	}

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID format"})
		return
	}

	// Parse time range (default 1h)
	timeRange := c.DefaultQuery("time_range", "1h")

	ctx := c.Request.Context()

	// Get agent
	agent, err := h.agentService.GetAgent(ctx, agentID)
	if err != nil {
		h.logger.Error("Failed to get agent", zap.String("agent_id", agentIDStr), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch agent"})
		return
	}

	if agent == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Get metrics for this agent
	metrics := h.getAgentMetrics(ctx, agentID, timeRange)

	// Get pipeline info (from config if available)
	config, _ := h.agentService.GetLatestConfigForAgent(ctx, agentID)
	var pipelineInfo map[string]interface{}
	if config != nil {
		pipelineInfo = map[string]interface{}{
			"config_id":      config.ID,
			"config_version": config.Version,
			"config_hash":    config.ConfigHash,
		}
	}

	response := gin.H{
		"agent":    agent,
		"metrics":  metrics,
		"pipeline": pipelineInfo,
	}

	c.JSON(http.StatusOK, response)
}

// HandleGetGroupTopology handles GET /api/v1/topology/group/:id
func (h *TopologyHandlers) HandleGetGroupTopology(c *gin.Context) {
	groupID := c.Param("id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Group ID is required"})
		return
	}

	ctx := c.Request.Context()

	// Get group
	group, err := h.agentService.GetGroup(ctx, groupID)
	if err != nil {
		h.logger.Error("Failed to get group", zap.String("group_id", groupID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch group"})
		return
	}

	if group == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Group not found"})
		return
	}

	// Get all agents in this group
	allAgents, err := h.agentService.ListAgents(ctx)
	if err != nil {
		h.logger.Error("Failed to get agents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch agents"})
		return
	}

	var groupAgents []*services.Agent
	for _, agent := range allAgents {
		if agent.GroupID != nil && *agent.GroupID == groupID {
			groupAgents = append(groupAgents, agent)
		}
	}

	// Get aggregated metrics for the group (last 5 minutes)
	endTime := time.Now()
	startTime := endTime.Add(-5 * time.Minute)

	metricsQuery := `
		SELECT COUNT(*) as count FROM (
			SELECT 1 FROM metrics_sum WHERE group_id = ? AND timestamp >= ? AND timestamp <= ?
			UNION ALL
			SELECT 1 FROM metrics_gauge WHERE group_id = ? AND timestamp >= ? AND timestamp <= ?
		) AS all_metrics
	`
	var metricCount int64
	if rows, err := h.telemetryService.QueryRaw(ctx, metricsQuery, groupID, startTime, endTime, groupID, startTime, endTime); err == nil && len(rows) > 0 {
		if count, ok := rows[0]["count"].(int64); ok {
			metricCount = count
		}
	}

	response := gin.H{
		"group":        group,
		"agents":       groupAgents,
		"agent_count":  len(groupAgents),
		"metric_count": metricCount,
	}

	c.JSON(http.StatusOK, response)
}

// getAgentMetrics retrieves metrics for an agent using Prometheus OTel collector self-telemetry.
// Instead of counting raw data rows (which has limits and no agent filtering for traces),
// we use the sum of receiver_accepted_* counters which are accurate and agent-specific.
func (h *TopologyHandlers) getAgentMetrics(ctx context.Context, agentID uuid.UUID, timeRange string) *NodeMetrics {
	agentIDStr := agentID.String()

	// Parse time range for rate window
	rateWindow := "1h"
	switch timeRange {
	case "6h":
		rateWindow = "6h"
	case "24h":
		rateWindow = "24h"
	}

	// Query Prometheus for OTel collector self-telemetry metrics (agent-specific)
	// These are cumulative counters (with _total suffix), so we use increase() to get the count within the time window
	spanCount := h.queryPrometheusSum(ctx,
		fmt.Sprintf(`sum(increase(otelcol_receiver_accepted_spans_total{agent_id="%s"}[%s]))`, agentIDStr, rateWindow))
	metricCount := h.queryPrometheusSum(ctx,
		fmt.Sprintf(`sum(increase(otelcol_receiver_accepted_metric_points_total{agent_id="%s"}[%s]))`, agentIDStr, rateWindow))
	logCount := h.queryPrometheusSum(ctx,
		fmt.Sprintf(`sum(increase(otelcol_receiver_accepted_log_records_total{agent_id="%s"}[%s]))`, agentIDStr, rateWindow))

	// Calculate throughput from rate (spans per second over the time window)
	var durationSeconds float64
	switch timeRange {
	case "6h":
		durationSeconds = 6 * 3600
	case "24h":
		durationSeconds = 24 * 3600
	default:
		durationSeconds = 3600
	}
	totalCount := spanCount + metricCount + logCount
	throughput := float64(totalCount) / durationSeconds

	return &NodeMetrics{
		MetricCount:   metricCount,
		LogCount:      logCount,
		TraceCount:    spanCount,
		ErrorRate:     0,
		Latency:       0,
		ThroughputRPS: throughput,
	}
}

// queryPrometheusSum executes a Prometheus instant query and returns the scalar result.
// Used for sum/increase queries on OTel collector self-telemetry counters.
func (h *TopologyHandlers) queryPrometheusSum(ctx context.Context, promQL string) int64 {
	prometheusURL := h.telemetryService.GetPrometheusURL()
	if prometheusURL == "" {
		return 0
	}

	u, err := url.Parse(fmt.Sprintf("%s/api/v1/query", prometheusURL))
	if err != nil {
		h.logger.Debug("Failed to parse Prometheus URL", zap.Error(err))
		return 0
	}

	q := u.Query()
	q.Set("query", promQL)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		h.logger.Debug("Failed to create request", zap.Error(err))
		return 0
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.logger.Debug("Prometheus query failed", zap.Error(err), zap.String("query", promQL))
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var promResp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Value []interface{} `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		h.logger.Debug("Failed to decode Prometheus response", zap.Error(err))
		return 0
	}

	if promResp.Status != "success" || len(promResp.Data.Result) == 0 {
		return 0
	}

	// Extract scalar value from instant query result
	result := promResp.Data.Result[0]
	if len(result.Value) < 2 {
		return 0
	}

	valueStr, ok := result.Value[1].(string)
	if !ok {
		return 0
	}

	var value float64
	fmt.Sscanf(valueStr, "%f", &value)
	return int64(value)
}
