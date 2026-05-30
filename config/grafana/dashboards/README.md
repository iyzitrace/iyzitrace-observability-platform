# iyzitrace · Grafana Dashboards

This folder contains all Grafana dashboards for the iyzitrace observability platform, organized by Grafana folder. Every dashboard is a standalone JSON file and is provisioned into Grafana via `config/grafana/dashboards.yml`.

## Folder layout

```
config/grafana/dashboards/
├── apm/                       # Application Performance Monitoring (folder UID: ffky1xm7z1hxcb)
├── infra-monitoring/          # Infrastructure Monitoring Dashboards (folder UID: infra-monitoring)
└── platform-monitoring/       # Platform Monitoring (folder UID: platform-monitoring)
```

A dedicated `README.md` lives in each subfolder with per-dashboard details. This top-level file is the index.

## Conventions

- **`iyzitrace-*.json`** — first-party dashboards built for this platform. They use `${DS_PROMETHEUS}`, `${DS_TEMPO}`, `${DS_LOKI}` template-variable datasources for portability on import.
- **`<vendor>-dashboard-mcp.json`** / **`<VENDOR>-metrics-mcp.json`** — vendor dashboards generated via the `mcp` exporter (Kafka, NGINX, Postgres, Redis, Linux). Kept as-is.
- All dashboards in a folder are picked up automatically by the file provider; just drop a JSON file in.

## Index

### Application Performance Monitoring (`apm/`)
End-user / service performance: traces, service map, RED metrics, dependency flows.

| UID | Title | What it answers |
|---|---|---|
| `iyzitrace-apm-v1` | iyzitrace · APM Overview | "How is the application doing right now?" — top-level RED metrics for every service |
| `iyzitrace-service-insights` | iyzitrace · Service Insights (SRE) | "Drill into a single service" — RED + breakdowns per `service_name` from span metrics |
| `iyzitrace-service-map` | Service Dependency Map | "Who calls whom?" — Tempo-driven nodeGraph + edge RED table |
| `iyzitrace-traffic-flow-atlas` | iyzitrace · Traffic Flow Atlas | "Where does traffic actually flow?" — Sankey, sunburst, parcats, adjacency matrix from `iyzitrace_service_graph_*` |
| `iyzitrace-operation-dependencies` | iyzitrace · Operation Dependencies | "Which client.op calls which server.op?" — operation-level dependencies with req/sec, error rate, latency. Pipeline DAG view + Sankey + adjacency matrix + RED table |
| `iyzitrace-trace-explorer` | iyzitrace · Trace Explorer | "Find me the trace" — recent / slow / error / DB-call / HTTP-error / root-cause traces from Tempo, plus rate/latency timeseries from span metrics, plus the live service map |

### Infrastructure Monitoring Dashboards (`infra-monitoring/`)
Hosts, containers, runtimes, protocol-level performance, and logs.

| UID | Title | What it answers |
|---|---|---|
| `iyzitrace-host-system` | iyzitrace · Host System Health | "Are the underlying hosts OK?" — CPU/load/memory/disk/network/processes from `system_*` |
| `iyzitrace-container-resources` | iyzitrace · Container Resources | "Is any container starving?" — per-container CPU, memory vs limit, network, blockio from `container_*` |
| `iyzitrace-process-resources` | iyzitrace · Process Resources | "What is each process consuming?" — cross-runtime `process_*` (CPU, RSS/VMS, FDs, threads, disk I/O) |
| `iyzitrace-runtime-dotnet` | iyzitrace · .NET Runtime | "Is .NET healthy?" — GC by gen, JIT, thread pool queue, lock contention, exceptions |
| `iyzitrace-runtime-jvm` | iyzitrace · JVM Runtime | "Is the JVM healthy?" — heap pools, GC by collector, classloading, threads by state |
| `iyzitrace-runtime-nodejs` | iyzitrace · Node.js Runtime | "Is the event loop saturated?" — `nodejs_eventloop_*` deep-dive (utilization, p50/p90/p99 delay, jitter) |
| `iyzitrace-runtime-python` | iyzitrace · Python (CPython) Runtime | "Is Python leaking / GIL-bound?" — generational GC, RSS/VMS, GIL CPU, context switches, **uncollectable circular refs** |
| `iyzitrace-http-performance` | iyzitrace · HTTP Performance | "How fast is HTTP?" — server & client RED, status mix, payloads, latency heatmap, DNS lookup p95 |
| `iyzitrace-genai-observability` | iyzitrace · GenAI Observability | "How much is AI costing us?" — LLM calls/sec, tokens (input/output), p95 latency, **configurable $/hour cost estimate** |
| `iyzitrace-log-intelligence` | iyzitrace · Log Intelligence | "What are the logs telling us?" — Loki-driven volume, severity mix, **per-service health timeline**, error feed, Envoy access logs |
| `kafka-dashboard-mcp` (vendor) | Kafka | Broker/consumer metrics |
| `linux-dashboard-mcp` (vendor) | Linux | Host basics |
| `postgresql-dashboard-mcp` (vendor) | PostgreSQL | DB internals |
| `redis-dashboard-mcp` (vendor) | Redis | Cache internals |
| `NGINX-metrics-mcp` (vendor) | NGINX | Web server metrics |

### Platform Monitoring (`platform-monitoring/`)
The observability platform's own pulse — telemetry pipelines and OpAMP agents.

| UID | Title | What it answers |
|---|---|---|
| `iyzitrace-opamp-agents` | OpAMP — Agent Fleet | "Which agents are connected, what's their status?" — OpAMP agents API via Infinity datasource |
| `iyzitrace-otelcol-health` | iyzitrace · OTel Collector Health | "Is my telemetry pipeline delivering?" — receivers/exporters/queues/drops, **accepted-vs-sent gap detector**, batch processor stats |
| `iyzitrace-signal-correlator` | iyzitrace · Telemetry Signal Correlator | "Are metrics, traces, and logs aligned?" — three signals side-by-side per service, spot silent telemetry loss |

## When to use which

| Scenario | Open this |
|---|---|
| User reports slowness | APM Overview → Service Insights → Trace Explorer (slow traces) |
| Error spike | Service Insights → Trace Explorer (error traces) → Log Intelligence (error feed) |
| "Service A talks to whom?" | Traffic Flow Atlas (Sankey) or Service Dependency Map |
| Memory leak suspected | Runtime dashboard for that language → Process Resources (FDs, RSS trend) |
| Host running hot | Host System Health → Container Resources (which container is the culprit) |
| ".NET pool stalled" | .NET Runtime (thread-pool queue length panel) |
| "Is my OTel pipeline broken?" | OTel Collector Health (accepted-vs-sent diff) → Telemetry Signal Correlator |
| "Is data missing somewhere?" | Telemetry Signal Correlator |
| "What's our LLM bill running at?" | GenAI Observability (set token prices via variables) |
| HTTP latency hunting | HTTP Performance (heatmap + endpoint RED table) |

## Variables convention

Most iyzitrace dashboards expose:

- **`DS_PROMETHEUS` / `DS_TEMPO` / `DS_LOKI`** — datasource selectors. Default to the platform's own UIDs (`prometheus-platform`, `tempo-platform`, `loki-platform`). Re-bind on import.
- **`service`** — multi-select, scoped via `label_values()` of a metric specific to that dashboard's runtime/concern. `allValue` is `.+` (Prometheus) or `.*` for label-optional cases.
- **`instance`** — when present, cascades from `service` and uses `.*` so series without `service_instance_id` still render.
- Always reference the regex form: `service_name=~"${service:regex}"`. This produces `cart|frontend` for multi-select and `.+` for "All", which is valid in PromQL/LogQL `=~`.

## How dashboards are provisioned

The Grafana container mounts `config/grafana/` as `/etc/grafana/provisioning/`. The relevant pieces:

- `config/grafana/dashboards.yml` — file provider, points at `/etc/grafana/provisioning/dashboards/{apm,infra-monitoring,platform-monitoring}`
- `config/grafana/folders.yml` — folder definitions (Grafana 12 needs the folders created via API; provisioning sync isn't first-class for folders)
- `config/grafana/datasources/*.yml` — datasource definitions (Prometheus, Tempo, Loki, Infinity)

Dropping a new JSON file into the right subfolder picks it up on the next Grafana reload.
