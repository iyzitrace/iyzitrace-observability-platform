# Iyzitrace Observability Platform - Architecture

## Mermaid Diagrams

### High-Level Architecture

```mermaid
flowchart TB
    subgraph Clients["External Clients"]
        OTelAgents["OTel Agents<br/>(Collectors)"]
        WebUI["Web UI<br/>(Browser)"]
        APIClients["API Clients"]
        OpAMPClients["OpAMP Clients<br/>(Collectors)"]
    end

    subgraph Gateway["NGINX Gateway :80/:443"]
        direction TB
        Routes["/v1/traces<br/>/v1/metrics<br/>/v1/logs"]
        OpAMPRoute["/v1/opamp"]
        APIRoutes["/opamp/api/*<br/>/dashboard/*"]
        DSRoutes["/datasource/*"]
        AuthValidate["/auth/validate"]
    end

    subgraph Processing["Signal Processing Layer"]
        direction TB
        TraceOTel["trace-otel-enrichment<br/>━━━━━━━━━━━━━━━━━<br/>• Type Classification<br/>• Span Enrichment<br/>• SpanMetrics Gen<br/>• ServiceGraph Gen"]
        MetricOTel["metric-otel-enrichment<br/>━━━━━━━━━━━━━━━━━<br/>• Memory Limiter<br/>• Batch Processing"]
        LogOTel["log-otel-enrichment<br/>━━━━━━━━━━━━━━━━━<br/>• Memory Limiter<br/>• Batch Processing"]
        AuthService["Auth Service<br/>━━━━━━━━━━━━━━━━━<br/>• JWT Validation<br/>• Token Management"]
    end

    subgraph Storage["Storage Backends"]
        direction TB
        Tempo["Tempo<br/>━━━━━━━━━━━━━━━━━<br/>• 7d Retention<br/>• TraceQL Support"]
        Loki["Loki<br/>━━━━━━━━━━━━━━━━━<br/>• 7d Retention<br/>• OTLP Native"]
        
        subgraph Thanos["Thanos Ecosystem"]
            Prometheus["Prometheus<br/>(2h local)"]
            ThanosSidecar["Sidecar"]
            ThanosStore["Store"]
            ThanosQuery["Query"]
            ThanosCompactor["Compactor"]
        end
        
        SeaweedFS["SeaweedFS (S3)<br/>━━━━━━━━━━━━━━━━━<br/>• tempo-data<br/>• loki-data<br/>• thanos-time-series"]
    end

    subgraph AgentMgmt["Agent Management Layer"]
        direction TB
        subgraph Lawrence["Lawrence (OpAMP Server)"]
            OpAMPServer["OpAMP Server :4320"]
            LawrenceOTLP["OTLP Receiver :4317/4318"]
            LawrenceAPI["REST API :8080"]
            LawrenceUI["Web UI"]
            LawrenceStorage["SQLite + DuckDB"]
        end
        
        subgraph Inventory["Inventory Service"]
            InvOTLP["OTLP Receiver :4317"]
            Detectors["Detector Registry<br/>━━━━━━━━━━━━━━━━━<br/>Infra | K8s | Service<br/>DB | Messaging | Mobile"]
            InvAPI["REST API :8080"]
            InvStorage["SQLite"]
        end
    end

    %% Client connections
    OTelAgents -->|"OTLP"| Routes
    WebUI -->|"HTTP"| APIRoutes
    APIClients -->|"HTTP"| DSRoutes
    OpAMPClients -->|"WebSocket"| OpAMPRoute

    %% Gateway routing
    Routes --> TraceOTel & MetricOTel & LogOTel
    OpAMPRoute --> OpAMPServer
    APIRoutes --> LawrenceAPI & AuthService
    DSRoutes --> ThanosQuery & Tempo & Loki
    Gateway -.->|"auth_request"| AuthValidate
    AuthValidate --> AuthService

    %% Processing to Storage
    TraceOTel --> Tempo
    TraceOTel --> InvOTLP
    TraceOTel -->|"spanmetrics"| MetricOTel
    MetricOTel --> Prometheus
    LogOTel --> Loki

    %% Thanos flow
    Prometheus --> ThanosSidecar
    ThanosSidecar --> SeaweedFS
    ThanosStore --> SeaweedFS
    ThanosSidecar --> ThanosQuery
    ThanosStore --> ThanosQuery
    ThanosCompactor --> SeaweedFS

    %% Storage backends to S3
    Tempo --> SeaweedFS
    Loki --> SeaweedFS

    %% Inventory flow
    InvOTLP --> Detectors
    Detectors --> InvStorage
    InvStorage --> InvAPI

    %% Lawrence internal
    OpAMPServer --> LawrenceStorage
    LawrenceOTLP --> LawrenceStorage
    LawrenceStorage --> LawrenceAPI
    LawrenceAPI --> LawrenceUI
```

### Telemetry Ingestion Flow

```mermaid
flowchart LR
    subgraph Sources["Telemetry Sources"]
        App1["Application 1"]
        App2["Application 2"]
        App3["Application N"]
    end

    subgraph Ingress["NGINX Gateway"]
        direction TB
        HTTP80[":80 HTTP"]
        HTTPS443[":443 HTTPS"]
        AuthReq["Auth Subrequest"]
    end

    subgraph Auth["Authentication"]
        AuthSvc["Auth Service"]
    end

    subgraph Collectors["OTel Collectors"]
        direction TB
        TraceColl["trace-otel-enrichment"]
        MetricColl["metric-otel-enrichment"]
        LogColl["log-otel-enrichment"]
    end

    subgraph Backends["Storage Backends"]
        direction TB
        Tempo["Tempo"]
        Prometheus["Prometheus"]
        Loki["Loki"]
        Thanos["Thanos"]
    end

    subgraph ObjectStore["Object Storage"]
        SeaweedFS["SeaweedFS S3"]
    end

    App1 & App2 & App3 -->|"OTLP/HTTP<br/>OTLP/gRPC"| HTTP80 & HTTPS443
    HTTP80 & HTTPS443 -.->|"/auth/validate"| AuthReq
    AuthReq --> AuthSvc
    
    HTTP80 -->|"/v1/traces"| TraceColl
    HTTP80 -->|"/v1/metrics"| MetricColl
    HTTP80 -->|"/v1/logs"| LogColl
    
    HTTPS443 -->|"/v1/traces"| TraceColl
    HTTPS443 -->|"/v1/metrics"| MetricColl
    HTTPS443 -->|"/v1/logs"| LogColl

    TraceColl -->|"traces"| Tempo
    TraceColl -->|"spanmetrics<br/>servicegraph"| MetricColl
    MetricColl -->|"metrics"| Prometheus
    LogColl -->|"logs"| Loki
    
    Prometheus --> Thanos
    Tempo --> SeaweedFS
    Loki --> SeaweedFS
    Thanos --> SeaweedFS
```

### OpAMP Agent Management Flow

```mermaid
flowchart TB
    subgraph Agents["OTel Collector Agents"]
        Agent1["Agent 1"]
        Agent2["Agent 2"]
        AgentN["Agent N"]
    end

    subgraph Gateway["NGINX Gateway"]
        OpAMPEndpoint["/v1/opamp<br/>(WebSocket)"]
        TelemetryEndpoint["/opamp/telemetry/*<br/>(OTLP)"]
        APIEndpoint["/opamp/api/v1/*"]
    end

    subgraph Lawrence["Lawrence Server"]
        direction TB
        OpAMPSrv["OpAMP Server<br/>:4320"]
        OTLPRecv["OTLP Receiver<br/>:4317/:4318"]
        RESTAPI["REST API<br/>:8080"]
        WebUI["Web UI"]
        
        subgraph Storage["Storage"]
            SQLite["SQLite<br/>━━━━━━━━━━━━<br/>• Agents<br/>• Groups<br/>• Configs"]
            DuckDB["DuckDB<br/>━━━━━━━━━━━━<br/>• Telemetry<br/>• Rollups"]
        end
    end

    subgraph Operations["Operations"]
        ConfigPush["Config Distribution"]
        StatusTrack["Status Tracking"]
        Capabilities["Capability Negotiation"]
    end

    Agent1 & Agent2 & AgentN <-->|"WebSocket<br/>OpAMP Protocol"| OpAMPEndpoint
    Agent1 & Agent2 & AgentN -->|"OTLP<br/>Agent Metrics/Logs"| TelemetryEndpoint
    
    OpAMPEndpoint --> OpAMPSrv
    TelemetryEndpoint --> OTLPRecv
    APIEndpoint --> RESTAPI
    
    OpAMPSrv <--> SQLite
    OpAMPSrv --> ConfigPush & StatusTrack & Capabilities
    OTLPRecv --> DuckDB
    SQLite & DuckDB --> RESTAPI
    RESTAPI --> WebUI

    ConfigPush -->|"Push configs"| Agent1 & Agent2 & AgentN
```

### Inventory Service - Entity Detection Flow

```mermaid
flowchart LR
    subgraph Input["Trace Input"]
        TraceOTel["trace-otel-enrichment"]
    end

    subgraph Receiver["OTLP Receiver"]
        GRPC[":4317 gRPC"]
    end

    subgraph Pipeline["Classification Pipeline"]
        direction TB
        subgraph Detectors["Detector Registry"]
            InfraD["Infra Detector<br/>━━━━━━━━━━━━━━<br/>host, container,<br/>process, cloud"]
            K8sD["K8s Detector<br/>━━━━━━━━━━━━━━<br/>cluster, node,<br/>namespace, pod"]
            ServiceD["Service Detector<br/>━━━━━━━━━━━━━━<br/>service"]
            DBD["Database Detector<br/>━━━━━━━━━━━━━━<br/>db.instance,<br/>cache.instance"]
            MsgD["Messaging Detector<br/>━━━━━━━━━━━━━━<br/>messaging.system,<br/>messaging.destination"]
            MobileD["Mobile Detector<br/>━━━━━━━━━━━━━━<br/>mobile.app.ios,<br/>mobile.app.android"]
        end
    end

    subgraph Output["Entity & Relation Store"]
        SQLite["SQLite"]
    end

    subgraph API["REST API"]
        Entities["/api/v1/entities"]
        Relations["/api/v1/relations"]
        Topology["/api/v1/topology"]
        Stats["/api/v1/stats"]
    end

    subgraph UI["Inventory UI"]
        Dashboard["Dashboard"]
        TreeView["Tree View"]
        TableView["Table View"]
        TopoView["Topology View"]
    end

    TraceOTel -->|"OTLP spans"| GRPC
    GRPC --> InfraD & K8sD & ServiceD & DBD & MsgD & MobileD
    InfraD & K8sD & ServiceD & DBD & MsgD & MobileD -->|"Entities<br/>Relations"| SQLite
    SQLite --> Entities & Relations & Topology & Stats
    Entities & Relations & Topology & Stats --> Dashboard & TreeView & TableView & TopoView
```

### Storage Architecture

```mermaid
flowchart TB
    subgraph Ingest["Data Ingestion"]
        Traces["Traces"]
        Metrics["Metrics"]
        Logs["Logs"]
    end

    subgraph ShortTerm["Short-Term Storage"]
        Tempo["Tempo<br/>━━━━━━━━━━━━━━━━━<br/>WAL + Local Blocks"]
        Prometheus["Prometheus<br/>━━━━━━━━━━━━━━━━━<br/>TSDB (2h retention)"]
        Loki["Loki<br/>━━━━━━━━━━━━━━━━━<br/>Ingester + Chunks"]
    end

    subgraph ThanosLayer["Thanos Layer"]
        direction TB
        Sidecar["Thanos Sidecar<br/>━━━━━━━━━━━━━━━━━<br/>Uploads TSDB blocks"]
        Store["Thanos Store<br/>━━━━━━━━━━━━━━━━━<br/>S3 Gateway"]
        Query["Thanos Query<br/>━━━━━━━━━━━━━━━━━<br/>Unified queries"]
        Compactor["Thanos Compactor<br/>━━━━━━━━━━━━━━━━━<br/>Downsampling:<br/>• 14d raw<br/>• 90d @ 5m<br/>• 1y @ 1h"]
    end

    subgraph ObjectStorage["SeaweedFS (S3 Compatible)"]
        direction TB
        TempoBucket["tempo-data bucket<br/>━━━━━━━━━━━━━━━━━<br/>Trace blocks"]
        LokiBucket["loki-data bucket<br/>━━━━━━━━━━━━━━━━━<br/>Log chunks + index"]
        ThanosBucket["thanos-time-series<br/>━━━━━━━━━━━━━━━━━<br/>Metric blocks"]
    end

    subgraph LocalVolumes["Local Volumes"]
        direction TB
        TempoVol["./data/tempo"]
        PrometheusVol["./data/prometheus"]
        LokiVol["./data/loki"]
        SeaweedVol["./data/seaweedfs"]
    end

    Traces --> Tempo
    Metrics --> Prometheus
    Logs --> Loki

    Tempo -->|"S3 API"| TempoBucket
    Prometheus --> Sidecar
    Sidecar -->|"Upload blocks"| ThanosBucket
    Store -->|"Read blocks"| ThanosBucket
    Sidecar --> Query
    Store --> Query
    Compactor -->|"Compact & Downsample"| ThanosBucket
    Loki -->|"S3 API"| LokiBucket

    TempoBucket --> SeaweedVol
    LokiBucket --> SeaweedVol
    ThanosBucket --> SeaweedVol

    Tempo -.->|"WAL"| TempoVol
    Prometheus -.->|"TSDB"| PrometheusVol
    Loki -.->|"Cache"| LokiVol
```

### Network & Port Mapping

```mermaid
flowchart TB
    subgraph External["External Access (Host Ports)"]
        P80["80 → nginx"]
        P443["443 → nginx"]
        P8081["8081 → auth-service"]
        P8082["8082 → inventory-service"]
        P4319["4319 → inventory-service OTLP"]
        P9091["9091 → thanos-query"]
        P8333["8333 → seaweedfs S3"]
        P9333["9333 → seaweedfs Master"]
    end

    subgraph Internal["Internal Network (iyzitrace-network)"]
        direction TB
        
        subgraph GW["Gateway"]
            nginx["nginx-platform<br/>:80, :443"]
        end
        
        subgraph OTelColl["OTel Collectors"]
            trace["trace-otel-enrichment<br/>:4317, :4318"]
            metric["metric-otel-enrichment<br/>:4317, :4318, :14317"]
            log["log-otel-enrichment<br/>:4317, :4318"]
        end
        
        subgraph Backends["Storage"]
            tempo["tempo<br/>:3200, :4317"]
            loki["loki<br/>:3100"]
            prometheus["prometheus<br/>:9090"]
            seaweed["seaweedfs<br/>:8333, :9333"]
        end
        
        subgraph ThanosC["Thanos"]
            sidecar["thanos-sidecar<br/>:10901, :10902"]
            store["thanos-store<br/>:10901, :10902"]
            query["thanos-query<br/>:9091"]
            compactor["thanos-compactor"]
        end
        
        subgraph Services["Services"]
            lawrence["lawrence<br/>:8080, :4320, :4317, :4318"]
            auth["auth-service<br/>:8080"]
            inv["inventory-service<br/>:8080, :4317"]
        end
    end

    P80 --> nginx
    P443 --> nginx
    P8081 --> auth
    P8082 --> inv
    P4319 --> inv
    P9091 --> query
    P8333 --> seaweed
    P9333 --> seaweed

    nginx --> trace & metric & log & lawrence & auth
    trace --> tempo & inv & metric
    metric --> prometheus
    log --> loki
    prometheus --> sidecar
    sidecar --> seaweed
    store --> seaweed
    tempo --> seaweed
    loki --> seaweed
```

### Component Dependencies

```mermaid
flowchart BT
    subgraph Layer1["Layer 1: Object Storage"]
        SeaweedFS["SeaweedFS"]
    end

    subgraph Layer2["Layer 2: Storage Backends"]
        Tempo["Tempo"]
        Loki["Loki"]
        Prometheus["Prometheus"]
    end

    subgraph Layer3["Layer 3: Query & Long-term"]
        ThanosStore["Thanos Store"]
        ThanosSidecar["Thanos Sidecar"]
        ThanosQuery["Thanos Query"]
        ThanosCompactor["Thanos Compactor"]
    end

    subgraph Layer4["Layer 4: Signal Processing"]
        TraceOTel["trace-otel-enrichment"]
        MetricOTel["metric-otel-enrichment"]
        LogOTel["log-otel-enrichment"]
    end

    subgraph Layer5["Layer 5: Gateway & Services"]
        NGINX["NGINX"]
        Lawrence["Lawrence"]
        AuthService["Auth Service"]
        InventoryService["Inventory Service"]
    end

    %% Dependencies
    SeaweedFS --> Tempo & Loki
    SeaweedFS --> ThanosStore & ThanosSidecar & ThanosCompactor
    
    Tempo --> TraceOTel
    Prometheus --> MetricOTel & ThanosSidecar
    Loki --> LogOTel
    
    ThanosStore & ThanosSidecar --> ThanosQuery
    
    TraceOTel & MetricOTel & LogOTel --> NGINX
    Lawrence --> NGINX
    AuthService --> NGINX
    ThanosQuery & Tempo & Loki --> NGINX
    
    TraceOTel --> InventoryService
```

---

## ASCII Architecture Diagrams

### High-Level Architecture Diagram

```
                                    ┌─────────────────────────────────────────────────────────────────────────────────┐
                                    │                              EXTERNAL CLIENTS                                    │
                                    │   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
                                    │   │ OTel Agents  │  │   Web UI     │  │  API Clients │  │OpAMP Clients │       │
                                    │   │ (Collectors) │  │  (Browser)   │  │              │  │ (Collectors) │       │
                                    │   └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘       │
                                    └──────────┼─────────────────┼─────────────────┼─────────────────┼───────────────┘
                                               │                 │                 │                 │
                                               │ OTLP            │ HTTP            │ HTTP            │ WebSocket
                                               │ (4317/4318)     │                 │                 │ (OpAMP)
                                               ▼                 ▼                 ▼                 ▼
┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                           NGINX GATEWAY (Port 80/443)                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐ │
│  │                                              ROUTING LAYER                                                           │ │
│  │  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  │ │
│  │  │  /v1/traces   │  │  /v1/metrics  │  │   /v1/logs    │  │  /v1/opamp    │  │  /opamp/api/  │  │  /dashboard/  │  │ │
│  │  └───────┬───────┘  └───────┬───────┘  └───────┬───────┘  └───────┬───────┘  └───────┬───────┘  └───────┬───────┘  │ │
│  │          │                  │                  │                  │                  │                  │          │ │
│  │  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐                                                           │ │
│  │  │/datasource/   │  │/datasource/   │  │/datasource/   │    Auth Request (/auth/validate) ─────────────────────┐   │ │
│  │  │   traces/     │  │   metrics/    │  │    logs/      │                                                       │   │ │
│  │  └───────┬───────┘  └───────┬───────┘  └───────┬───────┘                                                       │   │ │
│  └──────────┼──────────────────┼──────────────────┼──────────────────────────────────────────────────────────────┼───┘ │
│             │                  │                  │                                                               │     │
│             │     API Key Auth                    │                                                               ▼     │
└─────────────┼──────────────────┼──────────────────┼─────────────────────────────────────────────────────────────────────┘
              │                  │                  │                  │                  │                  │
              ▼                  ▼                  ▼                  ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                        SIGNAL PROCESSING LAYER                                                          │
│                                                                                                                          │
│  ┌──────────────────────────┐  ┌──────────────────────────┐  ┌──────────────────────────┐  ┌───────────────────────────┐│
│  │  trace-otel-enrichment   │  │  metric-otel-enrichment  │  │   log-otel-enrichment    │  │      AUTH SERVICE         ││
│  │  ═══════════════════════ │  │  ════════════════════════│  │  ═══════════════════════ │  │  ═════════════════════    ││
│  │  • OTLP Receiver         │  │  • OTLP Receiver         │  │  • OTLP Receiver         │  │  • JWT Validation         ││
│  │  • Type Classification   │  │  • Memory Limiter        │  │  • Memory Limiter        │  │  • JWT Validation             ││
│  │  • Span Enrichment       │  │  • Batch Processing      │  │  • Batch Processing      │  │  • Token Management           ││
│  │  • Span Metrics Gen      │  │                          │  │                          │  │  • UI Dashboard               ││
│  │  • Service Graph Gen     │  │                          │  │                          │  │                               ││
│  └────────┬──────┬──────────┘  └────────────┬─────────────┘  └─────────────┬────────────┘  └───────────────────────────┘│
│           │      │                          │                              │                                             │
│           │      │  ┌───────────────────────┘                              │                                             │
│           │      │  │                                                      │                                             │
│           │      │  │   ┌──────────────────────────────────────────────────┼─────────────────────────────────────────┐  │
│           │      │  │   │                                                  │                                         │  │
│           ▼      │  │   ▼                                                  ▼                                         │  │
│  ┌────────────┐  │  │  ┌────────────────────────────┐             ┌─────────────────────┐                            │  │
│  │ Inventory  │  │  │  │       PROMETHEUS           │             │        LOKI         │                            │  │
│  │  Service   │  │  │  │  ═════════════════════════ │             │  ═══════════════════│                            │  │
│  │ ══════════ │  │  │  │  • 2h Local Retention      │             │  • 7d Retention     │                            │  │
│  │ • Semrev   │  │  │  │  • OTLP Receiver Enabled   │             │  • S3 via SeaweedFS │                            │  │
│  │ • Entity   │  │  │  │  • Recording Rules         │             │  • OTLP Native      │                            │  │
│  │   Detection│  │  │  │  • Exemplar Storage        │             │  • Stream Limits    │                            │  │
│  │ • Topology │  │  │  └─────────────┬──────────────┘             └──────────┬──────────┘                            │  │
│  │ • API      │  │  │                │                                       │                                       │  │
│  └────────────┘  │  │                │                                       │                                       │  │
│                  │  │                ▼                                       │                                       │  │
│                  │  │   ┌─────────────────────────────────────────┐          │                                       │  │
│                  │  │   │         THANOS ECOSYSTEM                │          │                                       │  │
│                  │  │   │  ═══════════════════════════════════════│          │                                       │  │
│                  │  │   │  ┌───────────────┐  ┌───────────────┐   │          │                                       │  │
│                  │  │   │  │Thanos Sidecar │  │ Thanos Store  │   │          │                                       │  │
│                  │  │   │  │ (Uploads      │  │ (S3 Gateway)  │   │          │                                       │  │
│                  │  │   │  │  TSDB blocks) │  │               │   │          │                                       │  │
│                  │  │   │  └───────┬───────┘  └───────┬───────┘   │          │                                       │  │
│                  │  │   │          │                  │           │          │                                       │  │
│                  │  │   │          ▼                  ▼           │          │                                       │  │
│                  │  │   │  ┌─────────────────────────────────┐   │          │                                       │  │
│                  │  │   │  │        Thanos Query             │   │          │                                       │  │
│                  │  │   │  │   (Unified Metric Queries)      │   │          │                                       │  │
│                  │  │   │  └─────────────────────────────────┘   │          │                                       │  │
│                  │  │   │  ┌─────────────────────────────────┐   │          │                                       │  │
│                  │  │   │  │      Thanos Compactor           │   │          │                                       │  │
│                  │  │   │  │  (Downsampling & Retention)     │   │          │                                       │  │
│                  │  │   │  │  • 14d raw, 90d 5m, 1y 1h       │   │          │                                       │  │
│                  │  │   │  └─────────────────────────────────┘   │          │                                       │  │
│                  │  │   └─────────────────────────────────────────┘          │                                       │  │
│                  │  │                                                        │                                       │  │
│                  ▼  ▼                                                        ▼                                       │  │
│           ┌──────────────────────┐                              ┌───────────────────────┐                            │  │
│           │        TEMPO         │                              │      SeaweedFS        │◄─────────────────────────────┘  │
│           │  ════════════════════│                              │   (S3 Compatible)     │                               │
│           │  • Distributed       │                              │  ═════════════════════│                               │
│           │    Tracing           │─────────────────────────────►│  • tempo-data bucket  │                               │
│           │  • 7d Retention      │                              │  • loki-data bucket   │                               │
│           │  • S3 via SeaweedFS  │                              │  • thanos-time-series │                               │
│           │  • TraceQL Support   │                              │    bucket             │                               │
│           └──────────────────────┘                              └───────────────────────┘                               │
│                                                                                                                          │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                          AGENT MANAGEMENT LAYER                                                          │
│                                                                                                                          │
│  ┌───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │                                           LAWRENCE (OpAMP Server)                                                  │  │
│  │  ═════════════════════════════════════════════════════════════════════════════════════════════════════════════════│  │
│  │                                                                                                                    │  │
│  │  ┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────────────────┐   │  │
│  │  │    OpAMP Server     │  │    OTLP Receiver    │  │      REST API       │  │          Web UI                 │   │  │
│  │  │    (Port 4320)      │  │   (4317/4318)       │  │     (Port 8080)     │  │     (Embedded React)            │   │  │
│  │  │  ─────────────────  │  │  ─────────────────  │  │  ─────────────────  │  │  ─────────────────────────────  │   │  │
│  │  │  • WebSocket conn   │  │  • Agent telemetry  │  │  • Agent CRUD       │  │  • Agent dashboard              │   │  │
│  │  │  • Config push      │  │  • Internal metrics │  │  • Group management │  │  • Config editor                │   │  │
│  │  │  • Status tracking  │  │  • Store in DuckDB  │  │  • Telemetry query  │  │  • Topology view                │   │  │
│  │  │  • Capabilities     │  │                     │  │  • Lawrence QL      │  │  • Health monitoring            │   │  │
│  │  └─────────────────────┘  └─────────────────────┘  └─────────────────────┘  └─────────────────────────────────┘   │  │
│  │                                                                                                                    │  │
│  │  Storage:  SQLite (agents, groups, configs)  +  DuckDB (telemetry, rollups)                                       │  │
│  └───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                                                          │
│  ┌───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │                                    INVENTORY SERVICE (Semantic Reverse Classification)                             │  │
│  │  ═════════════════════════════════════════════════════════════════════════════════════════════════════════════════│  │
│  │                                                                                                                    │  │
│  │  ┌─────────────────────┐  ┌─────────────────────────────────────────────────────────────┐  ┌────────────────────┐ │  │
│  │  │   OTLP Receiver     │  │                    Detector Registry                        │  │     REST API       │ │  │
│  │  │   (Port 4317)       │──►  ┌──────┐ ┌──────┐ ┌────────┐ ┌──────┐ ┌─────────┐ ┌──────┐│──►   (Port 8080)     │ │  │
│  │  │  ─────────────────  │  │  │Infra │ │ K8s  │ │Service │ │  DB  │ │Messaging│ │Mobile││  │  ────────────────  │ │  │
│  │  │  • Traces from      │  │  └──────┘ └──────┘ └────────┘ └──────┘ └─────────┘ └──────┘│  │  • /api/v1/entities│ │  │
│  │  │    trace-otel       │  └─────────────────────────────────────────────────────────────┘  │  • /api/v1/relations│ │  │
│  │  │  • Entity detection │                              │                                    │  • /api/v1/topology│ │  │
│  │  └─────────────────────┘                              ▼                                    └────────────────────┘ │  │
│  │                                              ┌────────────────┐                                                    │  │
│  │                                              │ SQLite Storage │                                                    │  │
│  │                                              │ (Entities,     │                                                    │  │
│  │                                              │  Relations)    │                                                    │  │
│  │                                              └────────────────┘                                                    │  │
│  └───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                                                          │
│  ┌───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │                                              INVENTORY UI                                                          │  │
│  │  ═════════════════════════════════════════════════════════════════════════════════════════════════════════════════│  │
│  │  • React + TypeScript + Vite                                                                                      │  │
│  │  • Dashboard, Tree View, Table View, Topology View                                                                │  │
│  │  • Proxies API calls to inventory-service via embedded Nginx                                                      │  │
│  └───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                                                          │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Component Breakdown

### 1. NGINX Gateway (Entry Point)

The NGINX gateway serves as the unified entry point for all traffic, handling:

| Route | Destination | Purpose |
|-------|-------------|---------|
| `/v1/traces` | trace-otel-enrichment:4318 | OTLP trace ingestion |
| `/v1/metrics` | metric-otel-enrichment:4318 | OTLP metric ingestion |
| `/v1/logs` | log-otel-enrichment:4318 | OTLP log ingestion |
| `/v1/opamp` | lawrence:4320 | OpAMP WebSocket connections |
| `/opamp/api/v1/*` | lawrence:8080 | Lawrence REST API |
| `/opamp/telemetry/*` | lawrence:4318 | Agent-specific telemetry |
| `/dashboard/*` | auth-service:8080 | Auth dashboard UI |
| `/datasource/metrics/` | thanos-query:9091 | Prometheus/Thanos queries |
| `/datasource/traces/` | tempo:3200 | Tempo trace queries |
| `/datasource/logs/` | loki:3100 | Loki log queries |

**Security Features:**
- HTTP (port 80) and HTTPS (port 443)
- Auth subrequests to `/auth/validate` for protected endpoints
- Keepalive connections (64 per upstream)

---

### 2. Signal Processing Layer (OTel Collectors)

Three dedicated OpenTelemetry Collector instances handle signal-specific enrichment:

#### trace-otel-enrichment
```yaml
Pipeline: OTLP → Processors → [Tempo, Inventory, SpanMetrics, ServiceGraph]
```
- **Type Classification**: Categorizes spans (http, database, messaging, cache, rpc)
- **Span Enrichment**: Adds `operation.name`, `host.name` attributes
- **Span Metrics Generation**: Creates RED metrics from traces (via spanmetrics connector)
- **Service Graph Generation**: Builds service dependency topology

#### metric-otel-enrichment
```yaml
Pipeline: OTLP → Processors → Prometheus (via OTLP write)
```
- Receives span-derived metrics from trace-otel-enrichment
- Processes and forwards to Prometheus OTLP endpoint

#### log-otel-enrichment
```yaml
Pipeline: OTLP → Processors → Loki
```
- Processes logs with batch and memory limiting
- Exports to Loki via OTLP/HTTP

---

### 3. Storage Backends

#### Tempo (Distributed Tracing)
- **Retention**: 7 days
- **Storage**: SeaweedFS S3 (`tempo-data` bucket)
- **Features**: TraceQL support, exemplar links

#### Loki (Log Aggregation)
- **Retention**: 7 days (168h)
- **Storage**: SeaweedFS S3 (`loki-data` bucket)
- **Index Labels**: level, service_name, service_namespace, agent.id, agent.name, agent.group_id
- **Limits**: 5000 max global streams, 10MB/s ingestion rate

#### Prometheus + Thanos (Metrics)
- **Prometheus Local Retention**: 2 hours (ephemeral)
- **Thanos Long-term**: S3 via SeaweedFS (`thanos-time-series` bucket)
- **Downsampling**:
  - Raw: 14 days
  - 5-minute resolution: 90 days
  - 1-hour resolution: 1 year

**Thanos Components:**
| Component | Role |
|-----------|------|
| Sidecar | Uploads TSDB blocks from Prometheus to S3 |
| Store | Gateway for querying historical data from S3 |
| Query | Unified query layer across sidecar + store |
| Compactor | Downsamples and applies retention policies |

#### SeaweedFS (Object Storage)
- S3-compatible storage for all backends
- **Buckets**: `tempo-data`, `loki-data`, `thanos-time-series`
- Single-node deployment (development mode)

---

### 4. Agent Management (Lawrence)

Lawrence is the OpAMP-based agent management server:

| Port | Protocol | Purpose |
|------|----------|---------|
| 8080 | HTTP | REST API + Web UI |
| 4320 | WebSocket | OpAMP agent connections |
| 4317 | gRPC | OTLP (agent telemetry) |
| 4318 | HTTP | OTLP (agent telemetry) |

**Capabilities:**
- Remote configuration management for OTel collectors
- Agent grouping and bulk operations
- Agent telemetry ingestion and storage
- Lawrence QL query language
- Topology visualization

**Storage:**
- SQLite: Agent metadata, groups, configurations
- DuckDB: Raw telemetry and aggregated rollups

---

### 5. Inventory Service (Semantic Reverse Classification)

Analyzes incoming telemetry to automatically detect and classify entities:

**Entity Types Detected:**
- **Infrastructure**: cloud.region, host, container, process
- **Kubernetes**: cluster, node, namespace, pod
- **Services**: service
- **Databases**: db.instance, db.database, cache.instance
- **Messaging**: messaging.system, messaging.destination
- **Mobile**: mobile.app, mobile.app.ios, mobile.app.android

**Relationship Types:**
- `runs_on`, `runs_in`, `located_in`, `uses`
- `publishes_to`, `consumes_from`, `belongs_to`, `part_of`

**Data Flow:**
```
trace-otel-enrichment → (OTLP) → Inventory Service → SQLite → REST API → Inventory UI
```

---

### 6. Authentication Service

Handles authentication and authorization:

- **JWT Token Validation**: API key authentication
- **Dashboard UI**: Token management interface

---

## Data Flow Diagrams

### Telemetry Ingestion Flow

```
┌─────────────┐    OTLP/HTTP     ┌───────────┐    Routes    ┌────────────────────────┐
│ Application │───────────────►  │   NGINX   │ ───────────► │  OTel Enrichment       │
│  (w/ OTel)  │    :80/:443     │  Gateway  │              │  Collectors            │
└─────────────┘                  └───────────┘              └───────────┬────────────┘
                                       │                               │
                                       │ /auth/validate                │
                                       ▼                               │
                                 ┌───────────┐                         │
                                 │   Auth    │                         │
                                 │  Service  │                         │
                                 └───────────┘                         │
                                                                       │
                    ┌──────────────────────────────────────────────────┤
                    │                      │                           │
                    ▼                      ▼                           ▼
             ┌──────────┐           ┌──────────┐               ┌──────────────┐
             │  Tempo   │           │   Loki   │               │ Prometheus   │
             │ (Traces) │           │  (Logs)  │               │  (Metrics)   │
             └────┬─────┘           └────┬─────┘               └──────┬───────┘
                  │                      │                            │
                  │                      │                            ▼
                  │                      │                     ┌──────────────┐
                  │                      │                     │   Thanos     │
                  │                      │                     │  (Long-term) │
                  │                      │                     └──────┬───────┘
                  │                      │                            │
                  └──────────────────────┴────────────────────────────┘
                                         │
                                         ▼
                                  ┌──────────────┐
                                  │  SeaweedFS   │
                                  │ (S3 Storage) │
                                  └──────────────┘
```

### OpAMP Agent Management Flow

```
┌─────────────────┐     WebSocket      ┌───────────┐                ┌───────────────┐
│  OTel Collector │────────────────►   │   NGINX   │ ─────────────► │   Lawrence    │
│  (with OpAMP)   │   /v1/opamp        │  Gateway  │   :4320        │  OpAMP Server │
└────────┬────────┘                    └───────────┘                └───────┬───────┘
         │                                                                  │
         │                                                                  │
         │  Agent Telemetry                                                 │
         │  (OTLP)                                                          │
         │                                                          ┌───────┴───────┐
         │                                                          │               │
         ▼                                                          ▼               ▼
┌─────────────────┐                                          ┌──────────┐   ┌──────────┐
│     NGINX       │                                          │  SQLite  │   │  DuckDB  │
│ /opamp/telemetry│──────────────► Lawrence:4318 ──────────► │ (Config) │   │(Telemetry)│
└─────────────────┘                                          └──────────┘   └──────────┘
```

---

## Network Topology

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          iyzitrace-network (bridge)                          │
│                                                                              │
│  External Ports:                                                             │
│  ───────────────                                                             │
│  80, 443    → nginx-platform                                                 │
│  8081       → auth-service                                                   │
│  8082       → inventory-service (HTTP API)                                   │
│  4319       → inventory-service (OTLP gRPC)                                  │
│  9091       → thanos-query                                                   │
│  8333, 9333 → seaweedfs (S3 + Master)                                        │
│                                                                              │
│  Internal Services (no external port exposure):                              │
│  ──────────────────────────────────────────────                              │
│  tempo:3200, tempo:4317                                                      │
│  loki:3100                                                                   │
│  prometheus:9090                                                             │
│  lawrence:8080, lawrence:4320, lawrence:4317, lawrence:4318                  │
│  trace-otel-enrichment:4317, :4318                                           │
│  metric-otel-enrichment:4317, :4318, :14317                                  │
│  log-otel-enrichment:4317, :4318                                             │
│  thanos-sidecar:10901, :10902                                                │
│  thanos-store:10901, :10902                                                  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Persistent Volumes

| Volume | Path | Service | Purpose |
|--------|------|---------|---------|
| `tempo-data` | `./data/tempo` | Tempo | Trace blocks (WAL) |
| `loki-data` | `./data/loki` | Loki | Log chunks |
| `prometheus-data` | `./data/prometheus` | Prometheus + Sidecar | TSDB blocks |
| `seaweedfs-data` | `./data/seaweedfs` | SeaweedFS | Object storage |
| `thanos-store-data` | `./data/thanos-store` | Thanos Store | Cache |
| `thanos-compactor-data` | `./data/thanos-compactor` | Thanos Compactor | Work directory |
| `auth-data` | `./data/auth` | Auth Service | SQLite DB |
| `lawrence-data` | `./data/lawrence` | Lawrence | SQLite + DuckDB |
| `inventory-data` | `./data/inventory` | Inventory Service | SQLite DB |

---

## Technology Stack Summary

| Layer | Technology | Purpose |
|-------|------------|---------|
| Gateway | NGINX 1.27 | Routing, TLS, auth subrequests |
| Collectors | OTel Collector Contrib | Signal processing & enrichment |
| Traces | Grafana Tempo | Distributed tracing storage |
| Logs | Grafana Loki | Log aggregation |
| Metrics | Prometheus + Thanos | Short + long-term metrics |
| Object Storage | SeaweedFS | S3-compatible storage |
| Agent Mgmt | Lawrence (Go) | OpAMP server |
| Inventory | Inventory Service (Go) | Entity classification |
| Auth | Auth Service (Node.js) | Authentication |
| UIs | React + TypeScript | Dashboard interfaces |

---

## Key Architectural Decisions

1. **Unified Ingestion Point**: All telemetry flows through NGINX, enabling centralized auth and routing
2. **Signal Separation**: Dedicated OTel collectors per signal type for independent scaling
3. **Span-to-Metrics**: Automatic RED metrics generation from traces via spanmetrics connector
4. **Long-term Metrics**: Thanos provides unlimited retention with downsampling
5. **S3 Backend**: SeaweedFS provides local S3-compatible storage for all backends
6. **OpAMP for Agent Management**: Lawrence enables remote configuration of collectors
7. **Semantic Classification**: Inventory service auto-discovers infrastructure topology from telemetry
