# Infrastructure Monitoring Dashboards

Hosts, containers, language runtimes, protocol-level performance, and logs. Each dashboard focuses on a specific infrastructure layer and auto-discovers the relevant services via OpenTelemetry resource labels.

Grafana folder UID: `infra-monitoring`

---

## Layer overview

```
┌─────────────────────────────────────────────────────────────────┐
│  Application protocols    HTTP Performance · GenAI Observability │
│                          (HTTP server/client + LLM calls)        │
├─────────────────────────────────────────────────────────────────┤
│  Logs                    Log Intelligence (Loki)                 │
├─────────────────────────────────────────────────────────────────┤
│  Language runtimes       .NET · JVM · Node.js · Python           │
├─────────────────────────────────────────────────────────────────┤
│  Process layer           Process Resources (cross-runtime)       │
├─────────────────────────────────────────────────────────────────┤
│  Container layer         Container Resources (Docker stats)      │
├─────────────────────────────────────────────────────────────────┤
│  Host layer              Host System Health (system_*)           │
└─────────────────────────────────────────────────────────────────┘
```

---

## iyzitrace · Host System Health
**File**: `iyzitrace-host-system.json` · **UID**: `iyzitrace-host-system`

Per-host CPU (utilization, by state, load 1m/5m/15m), memory (used/cached/buffered/free stacked + utilization%), paging (major/minor faults, swap, ops), disk (bytes/IOPS/queue/weighted I/O), filesystem inventory table with inodes, network (rx/tx/packets/errors/dropped/connections), processes & threads.

**Source**: `system_*` (OTel hostmetrics receiver) · **Variable**: `host` (multi)

**Best for**: "is the host saturated?" before chasing application-level issues.

---

## iyzitrace · Container Resources
**File**: `iyzitrace-container-resources.json` · **UID**: `iyzitrace-container-resources`

Per-container CPU (top 20, user vs kernel), memory used vs limit (with thresholded coloring — approaches 1.0 = OOMKill), file-backed (page-cache) memory, network rx/tx (top 15), dropped packets, block I/O.

**Source**: `container_*` (OTel docker_stats receiver) · **Variables**: `host`, `container`

**Best for**: identifying which container is starving or hot. The "Memory pressure ranking" table is sorted by used %.

---

## iyzitrace · Process Resources
**File**: `iyzitrace-process-resources.json` · **UID**: `iyzitrace-process-resources`

Cross-runtime view of any service emitting `process_*`. CPU utilization & time, RSS/VMS memory, memory utilization ratio, disk I/O read/write, FDs, threads, voluntary vs involuntary context switches, plus a full inventory table.

**Source**: `process_*` (OTel SDK process metrics) · **Variable**: `service`

**Best for**: leak detection (FD growth, RSS upward drift) regardless of language.

---

## Runtime dashboards

Each runtime dashboard is gated on the relevant `telemetry_sdk_language` so the Service variable only lists services emitting that runtime's metrics.

### iyzitrace · .NET Runtime
**File**: `iyzitrace-runtime-dotnet.json` · **UID**: `iyzitrace-runtime-dotnet`

Heap by generation (gen0/1/2 stacked), GC pause overhead %, GC frequency, allocation rate, heap fragmentation, JIT methods/IL/CPU, **thread-pool threads vs queue length** (the canonical .NET tail-latency cause), lock contention, exceptions, loaded assemblies, timers.

**Source**: `process_runtime_dotnet_*` (legacy naming — modern `dotnet_*` registry empty here) + `process_uptime_seconds`.

### iyzitrace · JVM Runtime
**File**: `iyzitrace-runtime-jvm.json` · **UID**: `iyzitrace-runtime-jvm`

Heap pools stacked (Eden / Survivor / Tenured / G1 variants colored consistently), used vs committed vs limit, heap utilization %, **post-GC heap (sawtooth pattern)** — best leak indicator, non-heap pools (Metaspace + CodeHeap), GC frequency by collector, GC pause overhead %, average pause, GC pause distribution heatmap, threads by state, daemon vs application threads, classloading rate.

**Source**: `jvm_*` (OTel Java auto-instrumentation 2.x). Currently kafka and fraud-detection.

### iyzitrace · Node.js Runtime
**File**: `iyzitrace-runtime-nodejs.json` · **UID**: `iyzitrace-runtime-nodejs`

Event loop is the canonical Node health signal — drives p99 more than CPU. Event-loop utilization, event-loop time/sec, p50/p90/p99 delay, min/mean/max variance, jitter (stddev).

**Source**: `nodejs_eventloop_*`

### iyzitrace · Python (CPython) Runtime
**File**: `iyzitrace-runtime-python.json` · **UID**: `iyzitrace-runtime-python`

RSS leak detector, RSS vs VMS, generational GC frequency by gen0/gen1/gen2 (stacked), objects collected per generation, **uncollectable circular refs** (pure leaks Python's GC can't reclaim — should be 0), GIL CPU utilization, threads, voluntary vs involuntary context switches.

**Source**: `cpython_*`, `process_runtime_cpython_*`

---

## iyzitrace · HTTP Performance
**File**: `iyzitrace-http-performance.json` · **UID**: `iyzitrace-http-performance`

Server- and client-side HTTP across all services. Handles **both legacy (`http_*_duration_milliseconds_*`) and new (`http_*_request_duration_seconds_*`) metric naming** by querying both and unit-normalizing.

Server: rate, p50/p95/p99, status code class mix (2xx green, 3xx blue, 4xx orange, 5xx red), in-flight requests, latency heatmap, payload sizes, endpoint RED table.

Client: rate by destination host, latency, **connection-establishment duration p95** (separates network latency from app latency), **DNS lookup p95** (with thresholds: 50ms = slow, 500ms = critical).

**Source**: `http_server_*`, `http_client_*`, `dns_lookup_duration_seconds_*`

---

## iyzitrace · GenAI Observability
**File**: `iyzitrace-genai-observability.json` · **UID**: `iyzitrace-genai-observability`

OpenTelemetry GenAI semantic-convention metrics — for LLM calls. LLM calls/sec, input vs output tokens/sec, p50/p95/p99 call duration, call-duration heatmap, **token-usage distribution heatmap** (long-tail spotter for bloated prompts/runaway generations), avg tokens per call by model, calls by provider (donut), calls by operation (donut), per-model summary table.

**Cost estimation**: variables `Input $/1M tokens` and `Output $/1M tokens` plug into a $/hour stat tile and timeseries. Set them to your actual OpenAI/Anthropic/whoever rates. Default 0 disables cost panels.

**Source**: `gen_ai_client_*`. Currently product-reviews calling `astronomy-llm` via `openai`-compatible API.

---

## iyzitrace · Log Intelligence
**File**: `iyzitrace-log-intelligence.json` · **UID**: `iyzitrace-log-intelligence`

100% Loki-driven. Active streams, lines/sec, error rate, warn rate, log volume by service & by severity (stacked), **service-health timeline (status-history panel)** — every service as a row with cells colored by error rate at each bucket, live error log feed with trace_id chips, **Envoy access-log analytics** (HTTP request rate by upstream cluster, top URL paths, top user-agents, top source IPs), trace-correlation hygiene (% of logs carrying a trace_id).

**Source**: Loki LogQL with structured-metadata filters (`detected_level`, `trace_id`, `service_name`, etc.)

**Best for**: catching log-only incidents (auth failures, weird user agents) and verifying that traces and logs are aligned.

---

## Vendor dashboards (kept as-is)

- `iyzitrace-kafka.json` — Kafka broker & consumer metrics
- `iyzitrace-linux.json` — Linux host basics
- `iyzitrace-postgresql.json` — PostgreSQL internals
- `iyzitrace-redis.json` — Redis stats
- `iyzitrace-nginx.json` — NGINX metrics

Some of these overlap with the iyzitrace dashboards (e.g. host metrics) — keep both for now; consolidate later if helpful.
