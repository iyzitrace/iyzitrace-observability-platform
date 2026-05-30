package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/getlawrence/lawrence-oss/internal/config"
)

// TelemetryQueryServiceImpl implements the TelemetryQueryService interface
type TelemetryQueryServiceImpl struct {
	prometheusURL string
	lokiURL       string
	tempoURL      string
	agentService  AgentService
	logger        *zap.Logger
}

// NewTelemetryQueryService creates a new telemetry query service
func NewTelemetryQueryService(appConfig *config.Config, agentService AgentService, logger *zap.Logger) TelemetryQueryService {
	return &TelemetryQueryServiceImpl{
		prometheusURL: appConfig.ExternalPlatform.PrometheusURL,
		lokiURL:       appConfig.ExternalPlatform.LokiURL,
		tempoURL:      appConfig.ExternalPlatform.TempoURL,
		agentService:  agentService,
		logger:        logger,
	}
}

// GetPrometheusURL returns the configured Prometheus URL
func (s *TelemetryQueryServiceImpl) GetPrometheusURL() string {
	return s.prometheusURL
}

// QueryMetrics queries metrics data from Prometheus
func (s *TelemetryQueryServiceImpl) QueryMetrics(ctx context.Context, query MetricQuery) ([]Metric, error) {
	s.logger.Debug("Received metric query in service", zap.Any("query", query))
	// Construct PromQL query
	promQL := ""
	if query.MetricName != nil {
		promQL = *query.MetricName
	}

	// Add filters
	filters := ""
	if query.AgentID != nil {
		filters += fmt.Sprintf("agent_id=\"%s\"", query.AgentID.String())
	}
	if query.GroupID != nil {
		if filters != "" {
			filters += ","
		}
		filters += fmt.Sprintf("agent_group_id=\"%s\"", *query.GroupID)
	}

	if filters != "" {
		promQL = fmt.Sprintf("%s{%s}", promQL, filters)
	} else if promQL == "" {
		return nil, fmt.Errorf("either metric name or filter (agent/group) is required for Prometheus query")
	}

	// Prometheus range query endpoint
	u, err := url.Parse(fmt.Sprintf("%s/api/v1/query_range", s.prometheusURL))
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("query", promQL)
	q.Set("start", query.StartTime.Format(time.RFC3339))
	q.Set("end", query.EndTime.Format(time.RFC3339))
	q.Set("step", "15s")
	u.RawQuery = q.Encode()

	s.logger.Debug("Executing Prometheus query", zap.String("query", promQL), zap.String("url", u.String()))

	resp, err := http.Get(u.String())
	if err != nil {
		s.logger.Error("Failed to execute Prometheus query", zap.Error(err), zap.String("url", u.String()))
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		s.logger.Error("Prometheus returned error status",
			zap.String("status", resp.Status),
			zap.String("body", string(b)),
			zap.String("query", promQL))
		return nil, fmt.Errorf("prometheus returned status %s: %s", resp.Status, string(b))
	}

	var promResp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Values [][]interface{}   `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		return nil, err
	}

	var metrics []Metric
	for _, res := range promResp.Data.Result {
		for _, val := range res.Values {
			if len(val) < 2 {
				continue
			}

			// Safe timestamp extraction
			var ts int64
			switch t := val[0].(type) {
			case float64:
				ts = int64(t)
			case string:
				f, _ := strconv.ParseFloat(t, 64)
				ts = int64(f)
			default:
				s.logger.Warn("Unexpected timestamp type from Prometheus", zap.Any("type", fmt.Sprintf("%T", val[0])))
				continue
			}
			timestamp := time.Unix(ts, 0)

			// Safe value extraction
			valueStr, ok := val[1].(string)
			if !ok {
				s.logger.Warn("Unexpected value type from Prometheus", zap.Any("type", fmt.Sprintf("%T", val[1])))
				continue
			}

			var value float64
			fmt.Sscanf(valueStr, "%f", &value)

			// Convert labels to MetricAttributes (interface{} map for JSON)
			metricAttrs := make(map[string]interface{})
			for k, v := range res.Metric {
				metricAttrs[k] = v
			}

			// Extract agent_id from labels
			var agentID uuid.UUID
			if aid, ok := res.Metric["agent_id"]; ok {
				if parsed, err := uuid.Parse(aid); err == nil {
					agentID = parsed
				}
			}

			// Extract group_id from labels
			var groupID *string
			if gid, ok := res.Metric["agent_group_id"]; ok && gid != "" {
				groupID = &gid
			}

			metrics = append(metrics, Metric{
				Timestamp:        timestamp,
				AgentID:          agentID,
				GroupID:          groupID,
				Name:             res.Metric["__name__"],
				Value:            value,
				Labels:           res.Metric,
				MetricAttributes: metricAttrs,
				ServiceName:      res.Metric["service_name"],
			})
		}
	}

	return metrics, nil
}

// QueryLogs queries logs data from Loki
func (s *TelemetryQueryServiceImpl) QueryLogs(ctx context.Context, query LogQuery) ([]Log, error) {
	// Base selector
	logQL := `{service_name!=""}`

	// Use JSON pipeline and filter by service.instance.id (which equals agent_id in our enriched logs)
	if query.AgentID != nil {
		// Filter using service.instance.id which is set to agent_id during enrichment
		logQL += fmt.Sprintf(` | json | service_instance_id="%s"`, query.AgentID.String())
	} else if query.GroupID != nil && *query.GroupID != "" {
		logQL += fmt.Sprintf(` | json | agent_group_id="%s"`, *query.GroupID)
	} else {
		logQL += ` | json`
	}

	// Add severity filter using regex line filter (more reliable than label filter)
	if query.Severity != nil && *query.Severity != "" {
		// Filter by severity_text JSON field (case-insensitive regex)
		logQL += fmt.Sprintf(` | severity_text=~"(?i)%s"`, *query.Severity)
	}

	if query.Search != nil && *query.Search != "" {
		logQL += fmt.Sprintf(` |= "%s"`, *query.Search)
	}

	// Loki query range endpoint
	u, err := url.Parse(fmt.Sprintf("%s/loki/api/v1/query_range", s.lokiURL))
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("query", logQL)
	q.Set("start", fmt.Sprintf("%d", query.StartTime.UnixNano()))
	q.Set("end", fmt.Sprintf("%d", query.EndTime.UnixNano()))
	if query.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", query.Limit))
	}
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("loki returned status %s: %s", resp.Status, string(b))
	}

	var lokiResp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&lokiResp); err != nil {
		return nil, err
	}

	var logs []Log
	for _, res := range lokiResp.Data.Result {
		for _, val := range res.Values {
			if len(val) < 2 {
				continue
			}
			tsNano := 0
			fmt.Sscanf(val[0], "%d", &tsNano)
			timestamp := time.Unix(0, int64(tsNano))
			body := val[1]

			log := Log{
				Timestamp: timestamp,
				Body:      body,
			}

			// Try to parse body as JSON for structured metadata
			var bodyMap map[string]interface{}
			if err := json.Unmarshal([]byte(body), &bodyMap); err == nil {
				// Extract from JSON body
				if svc, ok := bodyMap["service_name"].(string); ok {
					log.ServiceName = svc
				}
				if sev, ok := bodyMap["severity_text"].(string); ok {
					log.SeverityText = sev
				} else if sev, ok := bodyMap["severity"].(string); ok {
					log.SeverityText = sev
				} else if sev, ok := bodyMap["detected_level"].(string); ok {
					log.SeverityText = sev
				}

				if sn, ok := bodyMap["severity_number"]; ok {
					switch v := sn.(type) {
					case string:
						n, _ := strconv.Atoi(v)
						log.SeverityNumber = n
					case float64:
						log.SeverityNumber = int(v)
					}
				}

				if aid, ok := bodyMap["agent_id"].(string); ok {
					if u, err := uuid.Parse(aid); err == nil {
						log.AgentID = u
					}
				} else if aid, ok := bodyMap["service_instance_id"].(string); ok {
					if u, err := uuid.Parse(aid); err == nil {
						log.AgentID = u
					}
				}

				if gid, ok := bodyMap["agent_group_id"].(string); ok {
					log.GroupID = &gid
				} else if gid, ok := bodyMap["agent_group_name"].(string); ok {
					// Fallback to name if ID not present
					log.GroupID = &gid
				}

				if tid, ok := bodyMap["trace_id"].(string); ok {
					log.TraceID = &tid
				}
				if sid, ok := bodyMap["span_id"].(string); ok {
					log.SpanID = &sid
				}

				// Move other fields to attributes
				log.LogAttributes = bodyMap
			}

			// Fallback to labels (Stream) if still missing
			if log.ServiceName == "" {
				log.ServiceName = res.Stream["service_name"]
			}
			if log.SeverityText == "" {
				if sev, ok := res.Stream["severity_text"]; ok {
					log.SeverityText = sev
				} else if sev, ok := res.Stream["severity"]; ok {
					log.SeverityText = sev
				} else if sev, ok := res.Stream["level"]; ok {
					log.SeverityText = sev
				}
			}
			if log.AgentID == (uuid.UUID{}) {
				if aid, ok := res.Stream["agent_id"]; ok {
					if u, err := uuid.Parse(aid); err == nil {
						log.AgentID = u
					}
				} else if aid, ok := res.Stream["agent.id"]; ok {
					if u, err := uuid.Parse(aid); err == nil {
						log.AgentID = u
					}
				}
			}
			if log.GroupID == nil {
				if gid, ok := res.Stream["agent_group_id"]; ok {
					log.GroupID = &gid
				} else if gid, ok := res.Stream["agent.group_id"]; ok {
					log.GroupID = &gid
				}
			}

			logs = append(logs, log)
		}
	}

	return logs, nil
}

// QueryTraces queries trace data from Tempo
func (s *TelemetryQueryServiceImpl) QueryTraces(ctx context.Context, query TraceQuery) ([]Trace, error) {
	if s.tempoURL == "" {
		return nil, fmt.Errorf("tempo URL not configured")
	}

	// Tempo search API endpoint - no filter for now since traces don't have agent IDs
	u, err := url.Parse(fmt.Sprintf("%s/api/search", s.tempoURL))
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("start", fmt.Sprintf("%d", query.StartTime.Unix()))
	q.Set("end", fmt.Sprintf("%d", query.EndTime.Unix()))
	if query.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", query.Limit))
	} else {
		q.Set("limit", "100")
	}
	u.RawQuery = q.Encode()

	s.logger.Debug("Executing Tempo search", zap.String("url", u.String()))

	resp, err := http.Get(u.String())
	if err != nil {
		s.logger.Error("Failed to execute Tempo query", zap.Error(err), zap.String("url", u.String()))
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		s.logger.Error("Tempo returned error status",
			zap.String("status", resp.Status),
			zap.String("body", string(b)))
		return nil, fmt.Errorf("tempo returned status %s: %s", resp.Status, string(b))
	}

	var tempoResp struct {
		Traces []struct {
			TraceID           string `json:"traceID"`
			RootServiceName   string `json:"rootServiceName"`
			RootTraceName     string `json:"rootTraceName"`
			StartTimeUnixNano string `json:"startTimeUnixNano"`
			DurationMs        int64  `json:"durationMs"`
		} `json:"traces"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tempoResp); err != nil {
		return nil, err
	}

	var traces []Trace
	for _, t := range tempoResp.Traces {
		startNano, _ := strconv.ParseInt(t.StartTimeUnixNano, 10, 64)
		trace := Trace{
			TraceID:   t.TraceID,
			Name:      t.RootTraceName,
			Timestamp: time.Unix(0, startNano),
			Duration:  t.DurationMs, // Duration in milliseconds as int64
		}
		traces = append(traces, trace)
	}

	return traces, nil
}

// QueryRaw is not supported in proxy mode
func (s *TelemetryQueryServiceImpl) QueryRaw(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error) {
	return nil, fmt.Errorf("raw SQL queries not supported in proxy mode")
}

// CreateRollups is not supported in proxy mode
func (s *TelemetryQueryServiceImpl) CreateRollups(ctx context.Context, window time.Time, interval RollupInterval) error {
	return nil
}

// QueryRollups is not supported in proxy mode
func (s *TelemetryQueryServiceImpl) QueryRollups(ctx context.Context, query RollupQuery) ([]Rollup, error) {
	return nil, fmt.Errorf("rollup queries not supported in proxy mode")
}

// CleanupOldData is not supported in proxy mode
func (s *TelemetryQueryServiceImpl) CleanupOldData(ctx context.Context, retention time.Duration) error {
	return nil
}

// GetTelemetryOverview gets the telemetry overview (simplified for proxy mode)
func (s *TelemetryQueryServiceImpl) GetTelemetryOverview(ctx context.Context) (*TelemetryOverview, error) {
	agents, _ := s.agentService.ListAgents(ctx)
	activeAgents := 0
	for _, agent := range agents {
		if agent.Status == "online" {
			activeAgents++
		}
	}

	return &TelemetryOverview{
		ActiveAgents: activeAgents,
		LastUpdated:  time.Now(),
	}, nil
}

// GetServices gets the list of unique services (placeholder)
func (s *TelemetryQueryServiceImpl) GetServices(ctx context.Context) ([]string, error) {
	return []string{}, nil
}
