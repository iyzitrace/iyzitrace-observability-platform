package worker

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/getlawrence/lawrence-oss/internal/otlp"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// OTLPExporter handles forwarding telemetry to an external OTLP backend
type OTLPExporter struct {
	url    string
	client *http.Client
	logger *zap.Logger
}

// NewOTLPExporter creates a new OTLP exporter
func NewOTLPExporter(url string, logger *zap.Logger) *OTLPExporter {
	return &OTLPExporter{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// WriteTraces implements TelemetryWriter and exports traces via OTLP/HTTP
func (e *OTLPExporter) WriteTraces(ctx context.Context, traces []otlp.TraceData) error {
	if len(traces) == 0 {
		return nil
	}

	// Enrich resource attributes with agent information
	enrichedAttrs := e.enrichAttributes(traces[0].ResourceAttributes, traces[0].AgentID, traces[0].GroupID, traces[0].GroupName)

	// Group traces by agent/service (simplified: one request per call)
	resourceSpans := &tracepb.ResourceSpans{
		Resource: &resourcepb.Resource{
			Attributes: mapToAttributes(enrichedAttrs),
		},
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: make([]*tracepb.Span, len(traces)),
			},
		},
	}

	for i, t := range traces {
		resourceSpans.ScopeSpans[0].Spans[i] = &tracepb.Span{
			TraceId:           unformatID(t.TraceId, 16),
			SpanId:            unformatID(t.SpanId, 8),
			ParentSpanId:      unformatID(t.ParentSpanId, 8),
			Name:              t.SpanName,
			Kind:              tracepb.Span_SpanKind(t.SpanKind),
			StartTimeUnixNano: uint64(t.Timestamp.UnixNano()),
			EndTimeUnixNano:   uint64(t.Timestamp.Add(time.Duration(t.Duration)).UnixNano()),
			Attributes:        mapToAttributes(t.SpanAttributes),
			TraceState:        t.TraceState,
			Status: &tracepb.Status{
				Code:    tracepb.Status_StatusCode(tracepb.Status_StatusCode_value[t.StatusCode]),
				Message: t.StatusMessage,
			},
		}
	}

	request := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{resourceSpans},
	}

	return e.send(ctx, "traces", request)
}

// WriteMetrics implements TelemetryWriter and exports metrics via OTLP/HTTP
func (e *OTLPExporter) WriteMetrics(ctx context.Context, sums []otlp.MetricSumData, gauges []otlp.MetricGaugeData, histograms []otlp.MetricHistogramData) error {
	if len(sums) == 0 && len(gauges) == 0 && len(histograms) == 0 {
		return nil
	}

	// Enrich resource attributes with agent information
	agentID, groupID, groupName := firstMetricAgentInfo(sums, gauges, histograms)
	enrichedAttrs := e.enrichAttributes(firstResourceAttrs(sums, gauges, histograms), agentID, groupID, groupName)

	resourceMetrics := &metricspb.ResourceMetrics{
		Resource: &resourcepb.Resource{
			// Take resource attributes from the first available metric
			Attributes: mapToAttributes(enrichedAttrs),
		},
		ScopeMetrics: []*metricspb.ScopeMetrics{
			{
				Metrics: make([]*metricspb.Metric, 0),
			},
		},
	}

	for _, s := range sums {
		resourceMetrics.ScopeMetrics[0].Metrics = append(resourceMetrics.ScopeMetrics[0].Metrics, &metricspb.Metric{
			Name:        s.MetricName,
			Description: s.MetricDescription,
			Unit:        s.MetricUnit,
			Data: &metricspb.Metric_Sum{
				Sum: &metricspb.Sum{
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes:        mapToAttributes(s.Attributes),
							StartTimeUnixNano: uint64(s.StartTimeUnix.UnixNano()),
							TimeUnixNano:      uint64(s.TimeUnix.UnixNano()),
							Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: s.Value},
							Flags:             s.Flags,
						},
					},
					AggregationTemporality: metricspb.AggregationTemporality(s.AggregationTemporality),
					IsMonotonic:            s.IsMonotonic,
				},
			},
		})
	}

	for _, g := range gauges {
		resourceMetrics.ScopeMetrics[0].Metrics = append(resourceMetrics.ScopeMetrics[0].Metrics, &metricspb.Metric{
			Name:        g.MetricName,
			Description: g.MetricDescription,
			Unit:        g.MetricUnit,
			Data: &metricspb.Metric_Gauge{
				Gauge: &metricspb.Gauge{
					DataPoints: []*metricspb.NumberDataPoint{
						{
							Attributes:        mapToAttributes(g.Attributes),
							StartTimeUnixNano: uint64(g.StartTimeUnix.UnixNano()),
							TimeUnixNano:      uint64(g.TimeUnix.UnixNano()),
							Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: g.Value},
							Flags:             g.Flags,
						},
					},
				},
			},
		})
	}

	for _, h := range histograms {
		resourceMetrics.ScopeMetrics[0].Metrics = append(resourceMetrics.ScopeMetrics[0].Metrics, &metricspb.Metric{
			Name:        h.MetricName,
			Description: h.MetricDescription,
			Unit:        h.MetricUnit,
			Data: &metricspb.Metric_Histogram{
				Histogram: &metricspb.Histogram{
					DataPoints: []*metricspb.HistogramDataPoint{
						{
							Attributes:        mapToAttributes(h.Attributes),
							StartTimeUnixNano: uint64(h.StartTimeUnix.UnixNano()),
							TimeUnixNano:      uint64(h.TimeUnix.UnixNano()),
							Count:             h.Count,
							Sum:               &h.Sum,
							ExplicitBounds:    h.ExplicitBounds,
							BucketCounts:      h.BucketCounts,
							Min:               &h.Min,
							Max:               &h.Max,
						},
					},
					AggregationTemporality: metricspb.AggregationTemporality(h.AggregationTemporality),
				},
			},
		})
	}

	request := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{resourceMetrics},
	}

	return e.send(ctx, "metrics", request)
}

func firstResourceAttrs(sums []otlp.MetricSumData, gauges []otlp.MetricGaugeData, histograms []otlp.MetricHistogramData) map[string]string {
	if len(sums) > 0 {
		return sums[0].ResourceAttributes
	}
	if len(gauges) > 0 {
		return gauges[0].ResourceAttributes
	}
	if len(histograms) > 0 {
		return histograms[0].ResourceAttributes
	}
	return nil
}

// WriteLogs implements TelemetryWriter and exports logs via OTLP/HTTP
func (e *OTLPExporter) WriteLogs(ctx context.Context, logs []otlp.LogData) error {
	if len(logs) == 0 {
		return nil
	}

	// Enrich resource attributes with agent information
	enrichedAttrs := e.enrichAttributes(logs[0].ResourceAttributes, logs[0].AgentID, logs[0].GroupID, logs[0].GroupName)

	request := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{
			{
				Resource: &resourcepb.Resource{
					Attributes: mapToAttributes(enrichedAttrs),
				},
				ScopeLogs: []*logspb.ScopeLogs{
					{
						LogRecords: make([]*logspb.LogRecord, len(logs)),
					},
				},
			},
		},
	}

	e.logger.Debug("Exporting logs",
		zap.Int("count", len(logs)),
		zap.String("firstAgentID", logs[0].AgentID),
		zap.String("firstGroupID", logs[0].GroupID),
		zap.String("firstGroupName", logs[0].GroupName),
		zap.Any("enrichedResourceAttrs", enrichedAttrs))

	for i, l := range logs {
		request.ResourceLogs[0].ScopeLogs[0].LogRecords[i] = &logspb.LogRecord{
			TimeUnixNano:   uint64(l.Timestamp.UnixNano()),
			SeverityText:   l.SeverityText,
			SeverityNumber: logspb.SeverityNumber(l.SeverityNumber),
			Body: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_StringValue{StringValue: l.Body},
			},
			Attributes: mapToAttributes(l.LogAttributes),
			TraceId:    unformatID(l.TraceId, 16),
			SpanId:     unformatID(l.SpanId, 8),
		}
	}

	return e.send(ctx, "logs", request)
}

func (e *OTLPExporter) send(ctx context.Context, signal string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", signal, err)
	}

	baseURL := strings.TrimSuffix(e.url, "/")
	url := fmt.Sprintf("%s/v1/%s", baseURL, signal)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send %s: %w", signal, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("failed to send %s: status %s", signal, resp.Status)
	}

	return nil
}

func mapToAttributes(m map[string]string) []*commonpb.KeyValue {
	attrs := make([]*commonpb.KeyValue, 0, len(m))
	for k, v := range m {
		attrs = append(attrs, &commonpb.KeyValue{
			Key: k,
			Value: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_StringValue{StringValue: v},
			},
		})
	}
	return attrs
}

func unformatID(id string, expectedLen int) []byte {
	if id == "" {
		return nil
	}
	data, err := hex.DecodeString(id)
	if err != nil {
		return nil
	}
	return data
}

func (e *OTLPExporter) enrichAttributes(attrs map[string]string, agentID, groupID, groupName string) map[string]string {
	newAttrs := make(map[string]string)
	for k, v := range attrs {
		newAttrs[k] = v
	}
	if agentID != "" && agentID != "default" {
		newAttrs["agent.id"] = agentID
	}
	if groupID != "" {
		newAttrs["agent.group_id"] = groupID
	}
	if groupName != "" {
		newAttrs["agent.group_name"] = groupName
	}
	return newAttrs
}

func firstMetricAgentInfo(sums []otlp.MetricSumData, gauges []otlp.MetricGaugeData, histograms []otlp.MetricHistogramData) (agentID, groupID, groupName string) {
	if len(sums) > 0 {
		return sums[0].AgentID, sums[0].GroupID, sums[0].GroupName
	}
	if len(gauges) > 0 {
		return gauges[0].AgentID, gauges[0].GroupID, gauges[0].GroupName
	}
	if len(histograms) > 0 {
		return histograms[0].AgentID, histograms[0].GroupID, histograms[0].GroupName
	}
	return "", "", ""
}
