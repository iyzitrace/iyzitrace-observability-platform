# Platform Monitoring

The observability platform's own pulse. If something here goes red, every other dashboard becomes a lie — telemetry is being dropped, agents are disconnected, or signals don't agree.

Grafana folder UID: `platform-monitoring`

---

## OpAMP — Agent Fleet
**File**: `iyzitrace-opamp-agents.json` · **UID**: `iyzitrace-opamp-agents`

Live view of OpAMP agents reporting in. Each agent's name, status, group, version, capabilities, and labels (host.name, host.arch, os.type).

**Datasource**: Infinity (REST/JSON), querying `http://host.docker.internal/api/v1/platform/opamp/agents`. The agents are returned as a UUID-keyed object map; the panel uses UQL with JSONata `$each()` to flatten it into a table.

**Plugin requirement**: `yesoreyeram-infinity-datasource`

**Best for**: "is everything reporting?" before trusting anything else.

---

## iyzitrace · OTel Collector Health
**File**: `iyzitrace-otelcol-health.json` · **UID**: `iyzitrace-otelcol-health`

The telemetry pipeline's own internal metrics. Built on `otelcol_*` from the collector self-instrumentation.

Key sections:
- **Pipeline pulse** — accepted spans/metrics/logs/sec, refused/sec, send-failures/sec
- **Receivers** — incoming throughput by receiver type, refused/failed split
- **Exporters** — outgoing throughput by destination, send-failures
- **Queue health** — exporter queue size vs capacity (line goes flat at capacity = data loss imminent), queue utilization %, batch size
- **Pipeline integrity** — **accepted-vs-sent diff per signal** (the drop detector — if the gap grows, something between receivers and exporters is silently dropping)
- **Processor pipeline** — incoming vs outgoing items (filter detection), batch processor timeout-triggered sends, processor duration heatmap
- **Collector runtime** — RSS / CPU / Go heap allocation rate per agent

**Variables**: `agent_group`, `agent` (cascading)

**Best for**: "is my telemetry pipeline delivering?". This is the dashboard SREs look at first when application dashboards go suspicious — to rule out / confirm a telemetry-side issue.

---

## iyzitrace · Telemetry Signal Correlator
**File**: `iyzitrace-signal-correlator.json` · **UID**: `iyzitrace-signal-correlator`

Three telemetry signals — metric, trace, log — for the same services side-by-side. The novel observation: **when one signal drops while the others stay steady, you know that pipeline is broken** (collector queue, exporter, instrumentation crashed, etc).

Layout:
- Stat strip: span_metrics rate (Prometheus) · spans/sec (also from span_metrics — Tempo metrics-generator isn't running here, see note below) · log lines/sec (Loki)
- Three time-aligned timeseries with the same per-service breakdown
- Per-service signal coverage table: services that emit each signal, RED metrics, p95 latency
- Recent log feed scoped to selected services, with trace_id chips for direct trace navigation

**Variables**: `service` (multi)

**Best for**: detecting silent telemetry loss. If service X has metrics but suddenly no logs, you instantly see which collector is broken.

**Tempo note**: when Tempo's metrics-generator is enabled (config `metrics_generator.processor.span_metrics`), the middle column can be swapped from Prometheus span_metrics back to native Tempo `traceqlMetrics` queries. Until then it uses the same span_metrics that feed APM.
