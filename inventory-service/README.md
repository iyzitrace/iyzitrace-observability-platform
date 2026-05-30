# Inventory Service

The Inventory Service implements a **Semantic Reverse Classification** pipeline that receives OTLP telemetry data (metrics, logs, traces) and infers:

- **Technology/System types** (e.g., Redis, Kafka, PostgreSQL, HTTP, iOS, Android)
- **Domain entities** (e.g., Database, Messaging System, Queue/Topic, Cache, Mobile App, Host, Container, Process, Service)
- **Relationships** between entities (e.g., service → uses → redis instance; producer → writes → kafka topic)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      OTLP Receiver                               │
│              (gRPC: 4317, HTTP: 4318)                           │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Classification Pipeline                        │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Detector Registry                       │   │
│  │  ┌─────────┐ ┌─────────┐ ┌──────────┐ ┌────────────┐    │   │
│  │  │ Infra   │ │   K8s   │ │ Service  │ │  Database  │    │   │
│  │  └─────────┘ └─────────┘ └──────────┘ └────────────┘    │   │
│  │  ┌──────────┐ ┌─────────┐ ┌──────────┐                  │   │
│  │  │Messaging │ │  HTTP   │ │  Mobile  │                  │   │
│  │  └──────────┘ └─────────┘ └──────────┘                  │   │
│  └──────────────────────────────────────────────────────────┘   │
│                          │                                       │
│                          ▼                                       │
│               Entity & Relation Emission                         │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                     SQLite Storage                               │
│              (Entities, Relations, Evidence)                     │
└─────────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                       HTTP API                                   │
│                   (GET /api/v1/...)                             │
└─────────────────────────────────────────────────────────────────┘
```

## Entity Types

### Infrastructure
- `cloud.region` - Cloud region
- `host` - Physical or virtual host
- `container` - Container instance
- `process` - Running process

### Kubernetes
- `k8s.cluster` - Kubernetes cluster
- `k8s.node` - Kubernetes node
- `k8s.namespace` - Kubernetes namespace
- `k8s.pod` - Kubernetes pod

### Services
- `service` - Application service

### Databases
- `db.instance` - Database instance
- `db.database` - Database within an instance
- `cache.instance` - Cache instance (Redis, Memcached)

### Messaging
- `messaging.system` - Messaging system (Kafka, RabbitMQ)
- `messaging.destination` - Topic or queue

### Mobile
- `mobile.app` - Mobile application
- `mobile.app.ios` - iOS application
- `mobile.app.android` - Android application

## Relation Types

- `runs_on` - Service runs on host
- `runs_in` - Service runs in container/pod
- `located_in` - Host located in region
- `uses` - Service uses database/cache/messaging
- `publishes_to` - Producer publishes to destination
- `consumes_from` - Consumer consumes from destination
- `belongs_to` - Database belongs to instance
- `part_of` - K8s resource part of parent

## API Endpoints

### Health
- `GET /health` - Service health check

### Entities
- `GET /api/v1/entities` - List entities (with filtering)
- `GET /api/v1/entities/:id` - Get entity by ID
- `GET /api/v1/entities/:id/relations` - Get entity relations

### Relations
- `GET /api/v1/relations` - List relations (with filtering)

### Statistics
- `GET /api/v1/stats` - Get inventory statistics

### Topology
- `GET /api/v1/topology` - Get entity graph for visualization

## Configuration

Environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP API port | `8080` |
| `DB_PATH` | SQLite database path | `/app/data/inventory.db` |
| `SEMCONV_VERSION` | Semantic conventions version | `1.26.0` |
| `SEMREV_ENABLED` | Enable classification pipeline | `true` |
| `WORKERS` | Number of classification workers | `4` |
| `BATCH_SIZE` | Batch size for DB writes | `1000` |
| `LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |
| `LOG_FORMAT` | Log format (json, console) | `json` |

## Development

### Build
```bash
go build ./cmd/inventory-service
```

### Test
```bash
go test ./...
```

### Run locally
```bash
./inventory-service
```

### Docker
```bash
docker build -t inventory-service .
docker run -p 8080:8080 -p 4317:4317 inventory-service
```

## Semantic Conventions

The service uses OpenTelemetry semantic conventions v1.26.0 for attribute detection. The adapter pattern allows easy upgrades to newer versions.

Supported semconv keys include:
- Cloud: `cloud.provider`, `cloud.region`, `cloud.availability_zone`
- Host: `host.id`, `host.name`, `host.type`
- Container: `container.id`, `container.name`, `container.runtime`
- Kubernetes: `k8s.cluster.name`, `k8s.namespace.name`, `k8s.pod.name`
- Service: `service.name`, `service.namespace`, `service.version`
- Database: `db.system`, `db.name`, `server.address`
- Messaging: `messaging.system`, `messaging.destination.name`, `messaging.operation`
- HTTP: `http.request.method`, `http.route`, `url.scheme`
- Mobile: `os.type`, `device.id`, `device.model.name`

## License

Apache-2.0
