// Package otlp provides OTLP receiver handlers for the inventory service.
package otlp

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip" // Register gzip compressor
	"google.golang.org/grpc/reflection"

	"github.com/iyzitrace/inventory-service/internal/semrev/model"
	"github.com/iyzitrace/inventory-service/internal/semrev/pipeline"
)

// Receiver manages OTLP receivers for all signal types.
type Receiver struct {
	traceReceiver   *TraceReceiver
	logsReceiver    *LogsReceiver
	metricsReceiver *MetricsReceiver
	server          *grpc.Server
	logger          *zap.Logger
}

// NewReceiver creates a new OTLP receiver.
func NewReceiver(pipeline *pipeline.Pipeline, logger *zap.Logger) *Receiver {
	return &Receiver{
		traceReceiver:   NewTraceReceiver(pipeline, logger),
		logsReceiver:    NewLogsReceiver(pipeline, logger),
		metricsReceiver: NewMetricsReceiver(pipeline, logger),
		logger:          logger.Named("otlp-receiver"),
	}
}

// Start starts the OTLP gRPC server.
func (r *Receiver) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	r.server = grpc.NewServer()
	tracev1.RegisterTraceServiceServer(r.server, r.traceReceiver)
	logsv1.RegisterLogsServiceServer(r.server, r.logsReceiver)
	metricsv1.RegisterMetricsServiceServer(r.server, r.metricsReceiver)
	reflection.Register(r.server)

	r.logger.Info("Starting OTLP gRPC receiver", zap.String("address", address))

	go func() {
		if err := r.server.Serve(listener); err != nil {
			r.logger.Error("OTLP server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop stops the OTLP receiver.
func (r *Receiver) Stop() {
	if r.server != nil {
		r.server.GracefulStop()
	}
}

// TraceReceiver handles trace exports.
type TraceReceiver struct {
	tracev1.UnimplementedTraceServiceServer
	pipeline *pipeline.Pipeline
	logger   *zap.Logger
}

// NewTraceReceiver creates a new trace receiver.
func NewTraceReceiver(pipeline *pipeline.Pipeline, logger *zap.Logger) *TraceReceiver {
	return &TraceReceiver{
		pipeline: pipeline,
		logger:   logger.Named("trace"),
	}
}

// Export implements tracev1.TraceServiceServer.
func (r *TraceReceiver) Export(ctx context.Context, req *tracev1.ExportTraceServiceRequest) (*tracev1.ExportTraceServiceResponse, error) {
	for _, rs := range req.GetResourceSpans() {
		resourceAttrs := extractAttributes(rs.GetResource().GetAttributes())

		// Extract service info from resource for source context
		serviceName := resourceAttrs["service.name"]
		serviceNamespace := resourceAttrs["service.namespace"]

		for _, ss := range rs.GetScopeSpans() {
			scopeAttrs := extractAttributes(ss.GetScope().GetAttributes())

			for _, span := range ss.GetSpans() {
				spanAttrs := extractAttributes(span.GetAttributes())

				// Merge attributes: span > scope > resource
				merged := mergeAttributes(resourceAttrs, scopeAttrs, spanAttrs)

				// Build source info for provenance tracking
				source := &model.SignalSource{
					SignalType:       model.SignalTypeSpan,
					Timestamp:        time.Now(),
					TraceID:          hex.EncodeToString(span.GetTraceId()),
					SpanID:           hex.EncodeToString(span.GetSpanId()),
					SpanName:         span.GetName(),
					ServiceName:      serviceName,
					ServiceNamespace: serviceNamespace,
				}

				item := &model.TelemetryItem{
					SignalType: model.SignalTypeSpan,
					Attrs:      merged,
					Timestamp:  time.Now(),
					Source:     source,
				}

				if !r.pipeline.Submit(item) {
					r.logger.Warn("Pipeline backpressure - dropping trace item")
				}
			}
		}
	}

	return &tracev1.ExportTraceServiceResponse{}, nil
}

// LogsReceiver handles logs exports.
type LogsReceiver struct {
	logsv1.UnimplementedLogsServiceServer
	pipeline *pipeline.Pipeline
	logger   *zap.Logger
}

// NewLogsReceiver creates a new logs receiver.
func NewLogsReceiver(pipeline *pipeline.Pipeline, logger *zap.Logger) *LogsReceiver {
	return &LogsReceiver{
		pipeline: pipeline,
		logger:   logger.Named("logs"),
	}
}

// Export implements logsv1.LogsServiceServer.
func (r *LogsReceiver) Export(ctx context.Context, req *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	for _, rl := range req.GetResourceLogs() {
		resourceAttrs := extractAttributes(rl.GetResource().GetAttributes())

		// Extract service info from resource for source context
		serviceName := resourceAttrs["service.name"]
		serviceNamespace := resourceAttrs["service.namespace"]

		for _, sl := range rl.GetScopeLogs() {
			scopeAttrs := extractAttributes(sl.GetScope().GetAttributes())

			for _, log := range sl.GetLogRecords() {
				logAttrs := extractAttributes(log.GetAttributes())

				// Merge attributes: log > scope > resource
				merged := mergeAttributes(resourceAttrs, scopeAttrs, logAttrs)

				// Extract log body (truncate to 100 chars for reference)
				logBody := extractValue(log.GetBody())
				if len(logBody) > 100 {
					logBody = logBody[:100] + "..."
				}

				// Map severity number to string
				severityText := log.GetSeverityText()
				if severityText == "" {
					severityText = severityNumberToString(int32(log.GetSeverityNumber()))
				}

				// Build source info for provenance tracking
				source := &model.SignalSource{
					SignalType:       model.SignalTypeLog,
					Timestamp:        time.Now(),
					TraceID:          hex.EncodeToString(log.GetTraceId()),
					SpanID:           hex.EncodeToString(log.GetSpanId()),
					LogSeverity:      severityText,
					LogBody:          logBody,
					ServiceName:      serviceName,
					ServiceNamespace: serviceNamespace,
				}

				item := &model.TelemetryItem{
					SignalType: model.SignalTypeLog,
					Attrs:      merged,
					Timestamp:  time.Now(),
					Source:     source,
				}

				if !r.pipeline.Submit(item) {
					r.logger.Warn("Pipeline backpressure - dropping log item")
				}
			}
		}
	}

	return &logsv1.ExportLogsServiceResponse{}, nil
}

// MetricsReceiver handles metrics exports.
type MetricsReceiver struct {
	metricsv1.UnimplementedMetricsServiceServer
	pipeline *pipeline.Pipeline
	logger   *zap.Logger
}

// NewMetricsReceiver creates a new metrics receiver.
func NewMetricsReceiver(pipeline *pipeline.Pipeline, logger *zap.Logger) *MetricsReceiver {
	return &MetricsReceiver{
		pipeline: pipeline,
		logger:   logger.Named("metrics"),
	}
}

// Export implements metricsv1.MetricsServiceServer.
func (r *MetricsReceiver) Export(ctx context.Context, req *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	for _, rm := range req.GetResourceMetrics() {
		resourceAttrs := extractAttributes(rm.GetResource().GetAttributes())

		// Extract service info from resource for source context
		serviceName := resourceAttrs["service.name"]
		serviceNamespace := resourceAttrs["service.namespace"]

		for _, sm := range rm.GetScopeMetrics() {
			scopeAttrs := extractAttributes(sm.GetScope().GetAttributes())

			for _, metric := range sm.GetMetrics() {
				metricName := metric.GetName()
				metricType := getMetricType(metric)

				// Process each data point
				var dataPointAttrs []model.NormalizedAttrs

				switch data := metric.GetData().(type) {
				case *metricspb.Metric_Gauge:
					for _, dp := range data.Gauge.GetDataPoints() {
						dataPointAttrs = append(dataPointAttrs, extractAttributes(dp.GetAttributes()))
					}
				case *metricspb.Metric_Sum:
					for _, dp := range data.Sum.GetDataPoints() {
						dataPointAttrs = append(dataPointAttrs, extractAttributes(dp.GetAttributes()))
					}
				case *metricspb.Metric_Histogram:
					for _, dp := range data.Histogram.GetDataPoints() {
						dataPointAttrs = append(dataPointAttrs, extractAttributes(dp.GetAttributes()))
					}
				case *metricspb.Metric_Summary:
					for _, dp := range data.Summary.GetDataPoints() {
						dataPointAttrs = append(dataPointAttrs, extractAttributes(dp.GetAttributes()))
					}
				case *metricspb.Metric_ExponentialHistogram:
					for _, dp := range data.ExponentialHistogram.GetDataPoints() {
						dataPointAttrs = append(dataPointAttrs, extractAttributes(dp.GetAttributes()))
					}
				}

				// Build source info for provenance tracking
				source := &model.SignalSource{
					SignalType:       model.SignalTypeMetric,
					Timestamp:        time.Now(),
					MetricName:       metricName,
					MetricType:       metricType,
					ServiceName:      serviceName,
					ServiceNamespace: serviceNamespace,
				}

				// For metrics, we often care more about resource attributes
				if len(dataPointAttrs) == 0 {
					// No data points, just process resource attributes
					merged := mergeAttributes(resourceAttrs, scopeAttrs, nil)
					item := &model.TelemetryItem{
						SignalType: model.SignalTypeMetric,
						Attrs:      merged,
						Timestamp:  time.Now(),
						Source:     source,
					}
					if !r.pipeline.Submit(item) {
						r.logger.Warn("Pipeline backpressure - dropping metric item")
					}
				} else {
					for _, dpAttrs := range dataPointAttrs {
						merged := mergeAttributes(resourceAttrs, scopeAttrs, dpAttrs)
						item := &model.TelemetryItem{
							SignalType: model.SignalTypeMetric,
							Attrs:      merged,
							Timestamp:  time.Now(),
							Source:     source,
						}
						if !r.pipeline.Submit(item) {
							r.logger.Warn("Pipeline backpressure - dropping metric item")
						}
					}
				}
			}
		}
	}

	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

// extractAttributes converts OTLP attributes to a normalized map.
func extractAttributes(attrs []*commonpb.KeyValue) model.NormalizedAttrs {
	result := make(model.NormalizedAttrs, len(attrs))
	for _, kv := range attrs {
		key := kv.GetKey()
		value := extractValue(kv.GetValue())
		if key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

// extractValue extracts a string value from an AnyValue.
func extractValue(v *commonpb.AnyValue) string {
	if v == nil {
		return ""
	}

	switch val := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return val.StringValue
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", val.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%f", val.DoubleValue)
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", val.BoolValue)
	default:
		return ""
	}
}

// mergeAttributes merges multiple attribute maps with precedence.
// Later maps take precedence over earlier maps for conflicting keys.
func mergeAttributes(maps ...model.NormalizedAttrs) model.NormalizedAttrs {
	result := make(model.NormalizedAttrs)

	for _, m := range maps {
		for k, v := range m {
			if v != "" {
				if _, exists := result[k]; !exists {
					result[k] = v
				}
			}
		}
	}

	return result
}

// severityNumberToString converts OTLP severity number to string.
func severityNumberToString(sev int32) string {
	switch {
	case sev <= 0:
		return "UNSPECIFIED"
	case sev <= 4:
		return "TRACE"
	case sev <= 8:
		return "DEBUG"
	case sev <= 12:
		return "INFO"
	case sev <= 16:
		return "WARN"
	case sev <= 20:
		return "ERROR"
	default:
		return "FATAL"
	}
}

// getMetricType returns the metric type as a string.
func getMetricType(metric *metricspb.Metric) string {
	switch metric.GetData().(type) {
	case *metricspb.Metric_Gauge:
		return "gauge"
	case *metricspb.Metric_Sum:
		return "sum"
	case *metricspb.Metric_Histogram:
		return "histogram"
	case *metricspb.Metric_Summary:
		return "summary"
	case *metricspb.Metric_ExponentialHistogram:
		return "exponential_histogram"
	default:
		return "unknown"
	}
}
