# Service Recording Rules

Documentation for the service-level Prometheus recording rules defined in [config/prometheus/rules/service-rules.yaml](/Users/yahyaozturk/Documents/playground/iyzitrace/iyzitrace-observability-platform/config/prometheus/rules/service-rules.yaml).

---

## Overview

The `service_observability_v1` rule group creates pre-aggregated service metrics from OpenTelemetry spanmetrics. These rules are intended to make service inventory and dashboard queries cheaper and simpler for common service-level latency and error views.

### Rule Group

- **Name**: `service_observability_v1`
- **Evaluation interval**: `30s`
- **Lookback window**: `5m`
- **Primary aggregation label**: `service_name`

### Source Metrics

The rules read from these spanmetrics series:

- `iyzitrace_span_metrics_calls_total`
- `iyzitrace_span_metrics_duration_milliseconds_sum`
- `iyzitrace_span_metrics_duration_milliseconds_count`
- `iyzitrace_span_metrics_duration_milliseconds_bucket`

These are generated from traces by the OTel `spanmetrics` connector and renamed by the metric enrichment collector.

---

## Recorded Metrics

### Traffic and Errors

| Metric | Meaning | Unit |
|---|---|---|
| `iyzitrace_service_calls_total_5m` | Total calls observed per service over the last 5 minutes | calls |
| `iyzitrace_service_requests_per_second_5m` | Average request rate per service over the last 5 minutes | requests/sec |
| `iyzitrace_service_error_calls_total_5m` | Total error calls observed per service over the last 5 minutes | calls |
| `iyzitrace_service_errors_per_second_5m` | Average error request rate per service over the last 5 minutes | requests/sec |
| `iyzitrace_service_success_calls_total_5m` | Total non-error calls observed per service over the last 5 minutes | calls |
| `iyzitrace_service_error_percentage_5m` | Error percentage over the last 5 minutes | percent |

### Duration

| Metric | Meaning | Unit |
|---|---|---|
| `iyzitrace_service_duration_milliseconds_sum_per_second_5m` | Rate of total observed duration over the last 5 minutes | ms/sec |
| `iyzitrace_service_duration_milliseconds_count_per_second_5m` | Rate of span count contributing to duration over the last 5 minutes | spans/sec |
| `iyzitrace_service_duration_milliseconds_avg_5m` | Average service response time over the last 5 minutes | ms |
| `iyzitrace_service_duration_milliseconds_p90_5m` | 90th percentile service response time over the last 5 minutes | ms |
| `iyzitrace_service_duration_milliseconds_p95_5m` | 95th percentile service response time over the last 5 minutes | ms |
| `iyzitrace_service_duration_milliseconds_p99_5m` | 99th percentile service response time over the last 5 minutes | ms |

### Apdex

| Metric | Meaning | Unit |
|---|---|---|
| `iyzitrace_service_apdex_100ms_400ms_5m` | Apdex-like score using `100ms` as satisfied and `400ms` as tolerated | ratio |
| `iyzitrace_service_is_active_5m` | Boolean-like helper indicating whether the service saw calls in the last 5 minutes | 0 or 1 |

### Internal Helper Metric

| Metric | Meaning |
|---|---|
| `__iyzitrace_service_duration_milliseconds_bucket_per_second_5m` | Internal bucket-rate series used to compute percentile and Apdex metrics |

The helper metric is prefixed with `__` because it is intended for rule composition rather than direct UI use.

---

## PromQL Definitions

### Request Rate

```promql
sum by (service_name) (
  rate(iyzitrace_span_metrics_calls_total[5m])
)
```

### Call Count

```promql
sum by (service_name) (
  increase(iyzitrace_span_metrics_calls_total[5m])
)
```

### Error Rate

```promql
sum by (service_name) (
  rate(iyzitrace_span_metrics_calls_total{status_code="STATUS_CODE_ERROR"}[5m])
)
```

### Error Call Count

```promql
sum by (service_name) (
  increase(iyzitrace_span_metrics_calls_total{status_code="STATUS_CODE_ERROR"}[5m])
)
```

### Success Call Count

```promql
iyzitrace_service_calls_total_5m
  - iyzitrace_service_error_calls_total_5m
```

### Error Percentage

```promql
100 * iyzitrace_service_errors_per_second_5m
  / clamp_min(iyzitrace_service_requests_per_second_5m, 1e-9)
```

### Average Response Time

```promql
iyzitrace_service_duration_milliseconds_sum_per_second_5m
  / clamp_min(iyzitrace_service_duration_milliseconds_count_per_second_5m, 1e-9)
```

### Percentiles

```promql
histogram_quantile(0.90, __iyzitrace_service_duration_milliseconds_bucket_per_second_5m)
histogram_quantile(0.95, __iyzitrace_service_duration_milliseconds_bucket_per_second_5m)
histogram_quantile(0.99, __iyzitrace_service_duration_milliseconds_bucket_per_second_5m)
```

### Apdex

```promql
(
  sum by (service_name) (__iyzitrace_service_duration_milliseconds_bucket_per_second_5m{le="100"})
  + sum by (service_name) (__iyzitrace_service_duration_milliseconds_bucket_per_second_5m{le="400"})
) / 2
/ clamp_min(iyzitrace_service_duration_milliseconds_count_per_second_5m, 1e-9)
```

### Service Active Helper

```promql
iyzitrace_service_calls_total_5m > bool 0
```

---

## Usage

Use these metrics when the query only needs service-level aggregation and a fixed 5-minute smoothing window is acceptable.

Good fits:

- Service inventory pages
- Overview dashboards
- Service latency leaderboards
- Error-rate and Apdex summary panels
- Top services by total calls or error calls
- Active versus inactive service filters

Prefer raw spanmetrics instead of these recorded metrics when:

- You need a different time window than `5m`
- You need additional dimensions such as `span_name`, `type`, `http_method`, or `http_url`
- You need ad hoc filtering on status, tags, or duration

---

## Example Queries

### Top 10 Services by Average Response Time

```promql
topk(10, iyzitrace_service_duration_milliseconds_avg_5m)
```

### Top 10 Services by Total Calls

```promql
topk(10, iyzitrace_service_calls_total_5m)
```

### Top 10 Services by P95

```promql
topk(10, iyzitrace_service_duration_milliseconds_p95_5m)
```

### Services with Highest Error Percentage

```promql
topk(10, iyzitrace_service_error_percentage_5m)
```

### Top 10 Services by Error Calls

```promql
topk(10, iyzitrace_service_error_calls_total_5m)
```

### Single Service View

```promql
iyzitrace_service_duration_milliseconds_p95_5m{service_name="frontend"}
```

---

## Notes and Caveats

- These rules aggregate by `service_name` only. They do not preserve `service_namespace`, `type`, `span_name`, `http_method`, or `http_url`.
- Because the duration histogram is in milliseconds, the percentile and average metrics are also in milliseconds.
- The Apdex thresholds are interpreted as milliseconds even though some legacy config names still mention `_seconds`.
- Ratio-based metrics use `clamp_min(..., 1e-9)` to avoid `NaN` and `+Inf` when traffic is zero or extremely low.
- Latency-budget percentage recordings such as `iyzitrace_service_under_100ms_percentage_5m` are intentionally not included in this iteration.
- The internal helper metric is expected to have higher cardinality than the final service-level outputs because it keeps the `le` label.

---

## File Location

- Rule file: [config/prometheus/rules/service-rules.yaml](/Users/yahyaozturk/Documents/playground/iyzitrace/iyzitrace-observability-platform/config/prometheus/rules/service-rules.yaml)
- Alert file: [config/prometheus/rules/service-alerts.yaml](/Users/yahyaozturk/Documents/playground/iyzitrace/iyzitrace-observability-platform/config/prometheus/rules/service-alerts.yaml)
- Documentation: [docs/service-rules.md](/Users/yahyaozturk/Documents/playground/iyzitrace/iyzitrace-observability-platform/docs/service-rules.md)
