# APM (Application Performance Monitoring)

End-user / service performance dashboards. Driven primarily by OpenTelemetry span metrics (`iyzitrace_span_metrics_*`), service graph metrics (`iyzitrace_service_graph_*`), and Tempo trace search.

Grafana folder UID: `ffky1xm7z1hxcb`

---

## iyzitrace · APM Overview
**File**: `iyzitrace-apm-overview.json` · **UID**: `iyzitrace-apm-v1`

Top-level RED-metrics view across every service in the system. Use this as the entry point — it surfaces global request rate, error rate, and latency, then breaks them down per service.

**Source**: `iyzitrace_span_metrics_*` (Prometheus)

**Best for**: morning health check, first dashboard during an incident.

---

## iyzitrace · Service Insights (SRE)
**File**: `iyzitrace-service-insights.json` · **UID**: `iyzitrace-service-insights`

Per-service deep-dive on RED metrics. Pick a `service_name` and see request rate by route, error rate over time, latency percentiles, and breakdown by `span_kind` and `status_code`.

**Variables**: `service` (single, required)

**Best for**: drilling into one service when APM Overview shows it's anomalous.

---

## Service Dependency Map
**File**: `iyzitrace-service-map.json` · **UID**: `iyzitrace-service-map`

Visual service topology rendered as a Grafana nodeGraph from Tempo trace data, plus a sortable RED table per edge. Built on `iyzitrace_service_graph_*` recording-rule aliases that mirror the metrics into the names Tempo expects (`traces_service_graph_*`).

**Best for**: "what does this service call?" / "who calls this service?".

**Note**: requires the recording rules in `config/prometheus/rules/service-graph-aliases.yaml`.

---

## iyzitrace · Traffic Flow Atlas
**File**: `iyzitrace-traffic-flow-atlas.json` · **UID**: `iyzitrace-traffic-flow-atlas`

Five complementary visualizations of the same `iyzitrace_service_graph_*` data — built using the **`ae3e-plotly-panel`** plugin:

1. **Sankey · client → server** — service-to-service flow, ribbon width = req/s, **red ribbons = failed**, DBs colored orange
2. **Sankey · client → client_op → server → server_op** — 4-stage flow showing exactly which client method calls which server operation
3. **Adjacency matrix** (heatmap) — client × server traffic intensity
4. **Sunburst** — connection_type → client → server → server_op drillable hierarchy
5. **Parallel categories** — client → client_type → server polylines weighted by rps

**Variables**: `Client` (focus service — BFS-filters all panels to the reachable subgraph downstream of the picked service), `min_rps` (declutter threshold)

**Plugin requirement**: `ae3e-plotly-panel` (set via `GF_INSTALL_PLUGINS` in docker-compose).

**Best for**: spotting fan-out patterns, dependency depth, or unexpected paths.

---

## iyzitrace · Operation Dependencies
**File**: `iyzitrace-operation-dependencies.json` · **UID**: `iyzitrace-operation-dependencies`

The granular sibling of Service Dependency Map. While the service map shows `serviceA → serviceB`, this dashboard shows **`serviceA.method1 → serviceB.method2`** — i.e. which exact RPC call / HTTP route / DB operation triggers which downstream operation.

Built from `iyzitrace_service_graph_request_total` grouped by `(client, server, client_operation_name, server_operation_name)`.

**Panels:**

1. **Granularity pulse** — side-by-side stat tiles showing distinct *service pairs* vs distinct *operation pairs*. The latter is typically 30–50% larger and that's the point — operations reveal the real call shape.
2. **Operation Sankey** (Plotly) — `client.op → server.op` flow ribbons. Width = req/s, **edge color tints from green to red as error rate rises**. Hover for req/s + error % + failed/s.
3. **Pipeline DAG** (Plotly, custom layout) — layered DAG rooted at the focus service. BFS levels go left → right; each box is a `service.op` node, each curved edge shows a downstream call with width hint by req/s and color by error %. Closest thing to a "call-graph" view from raw metrics.
4. **Adjacency matrix** (Plotly heatmap) — `client.op` × `server.op` cells colored by req/s. Empty cells = no traffic. Hover for error %.
5. **Top error pairs** (bar chart) — top 10 operation pairs sorted by error rate.
6. **Full RED table** — every operation pair with rps (gauge), error % (color-banded), client p95 latency (color-banded), server p95 latency (color-banded). The full operation-level dependency catalog, sortable.

**Variables**: `Focus service` (single, drives the Pipeline DAG root and BFS-filters the Sankey to its reachable downstream subgraph), `min_rps` (declutter threshold)

**Plugin requirement**: `ae3e-plotly-panel`

**Best for**: "When user calls `frontend.GET /cart`, what's the full chain of operations that fires downstream?" — the pipeline DAG answers exactly that.

---

## iyzitrace · Trace Explorer
**File**: `iyzitrace-trace-explorer.json` · **UID**: `iyzitrace-trace-explorer`

Hybrid trace investigation workspace. Stat tiles and time series come from Prometheus span metrics; the actual trace lists and service map come from Tempo via TraceQL.

**Tempo TraceQL queries used**:
- Recent traces: `{resource.service.name =~ "${service:regex}"}`
- Error traces: `{... && status = error}`
- Slow traces: `{... && duration > $slow_threshold}`
- DB call traces: `{... && span.db.system != ""}`
- HTTP 4xx/5xx traces: `{... && span.http.status_code >= 400}`
- Slow DB calls: `{... && span.db.system != "" && duration > 200ms}`
- **Root-cause errors**: `{... && status = error && parent.status != error}` — origin of fault chains, not cascading noise

**Variables**: `service` (multi), `slow_threshold` (200ms / 500ms / 1s / 2s / 5s)

**Best for**: incident investigation. Pivot from "I see errors" → "show me the actual error traces" → "open the flame graph".

**Why hybrid**: in this environment Tempo's `metrics-generator` isn't running (`empty ring`), so `traceqlMetrics` queries fail. Once enabled, the stats/time series can be swapped back to TraceQL.
