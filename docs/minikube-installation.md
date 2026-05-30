# Deploying IyziTrace Observability Platform on Minikube

A step-by-step guide to deploy the full IyziTrace observability stack on a local Minikube cluster using the Helm chart.

---

## Prerequisites

| Tool | Minimum Version | Install |
|---|---|---|
| [minikube](https://minikube.sigs.k8s.io/) | v1.33+ | `brew install minikube` |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | v1.28+ | `brew install kubectl` |
| [Helm](https://helm.sh/) | v3.14+ | `brew install helm` |
| Docker Desktop | — | [docker.com](https://www.docker.com/products/docker-desktop/) |

---

## 1. Start Minikube

```bash
minikube start \
  --cpus=4 \
  --memory=8192 \
  --disk-size=40g \
  --driver=docker \
  --kubernetes-version=v1.30.0
```

For Mac Users -- can use alternative driver for ingress

```bash
minikube start \
  --cpus=4 \
  --memory=8192 \
  --disk-size=40g \
  --driver=vfkit \
  --kubernetes-version=v1.30.0
```
OPTIONAL - Enable the addons for ingress controller

```bash
minikube addons enable ingress
minikube addons enable ingress-dns
```

> [!TIP]
> **Why these resources?** The platform runs Tempo, Loki, Prometheus, Thanos, SeaweedFS, and OTel collectors simultaneously. 4 CPU / 8 GB RAM is the comfortable minimum.

Verify the cluster is running:

```bash
kubectl cluster-info
kubectl get nodes
```

---

## 2. Install the OTel Operator CRDs

The chart uses `OpenTelemetryCollector` custom resources managed by the OpenTelemetry Operator. Install the CRDs before deploying the chart:

```bash
# Install cert-manager (required by OTel Operator)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s
```

---

## 3. Create a Namespace

```bash
kubectl create namespace iyzitrace
```

---

## 4. Deploy the Platform

### Install from local source

If you have cloned the repository, use the local chart:

```bash
# Update dependencies (first time or if Chart.lock is missing)
helm dependency update helm/iyzitrace-platform

# Install the chart
helm install iyzitrace helm/iyzitrace-platform \
  --namespace iyzitrace \
  --timeout 15m \
  --wait
```

### Install from OCI registry (Alternative)

```bash
helm install iyzitrace oci://ghcr.io/antreklabs/charts/iyzitrace-platform \
  --version 0.1.0 \
  --namespace iyzitrace \
  --timeout 15m \
  --wait
```

> [!NOTE]
> **What happens automatically:**
> 1. **Helm-managed secrets** — the chart creates bootstrap S3/JWT credentials and a bootstrap TLS secret if they do not already exist.
> 2. **Main release** — all components deploy (SeaweedFS, Tempo, Loki, Prometheus, Thanos, Nginx, OTel Operator, custom services).
> 3. **SeaweedFS bucket creation** — the SeaweedFS dependency chart creates the required S3 buckets (`tempo-data`, `loki-data`, `thanos-time-series`).

---

## 5. Wait for All Pods

```bash
# Watch until pods stabilize
kubectl get pods -n iyzitrace -w
```

You should expect to see the core workloads come up in this namespace, including:

- `iyzitrace-iyzitrace-platform-nginx`
- `tempo-*`
- `loki-*`
- `thanos-*`
- `iyzitrace-prometheus-server-*`
- `iyzitrace-iyzitrace-platform-auth-service`
- `iyzitrace-iyzitrace-platform-lawrence`
- `iyzitrace-iyzitrace-platform-inventory-service`

---

## 6. Access the Platform

The platform has a single entrypoint: the Nginx service.

Use `kubectl port-forward` to expose that entrypoint locally:

| Component | Port Forward Command | Local URL |
|---|---|---|
| **Platform Entrypoint (Nginx)** | `kubectl port-forward -n iyzitrace svc/iyzitrace-iyzitrace-platform-nginx 8080:80` | `http://localhost:8080` |

Main routes behind the Nginx entrypoint:

| Route | Purpose | Example |
|---|---|---|
| `/ingest/otlp/v1/traces` | OTLP trace ingestion | `http://localhost:8080/ingest/otlp/v1/traces` |
| `/ingest/otlp/v1/metrics` | OTLP metric ingestion | `http://localhost:8080/ingest/otlp/v1/metrics` |
| `/ingest/otlp/v1/logs` | OTLP log ingestion | `http://localhost:8080/ingest/otlp/v1/logs` |
| `/query/v1/traces/` | Tempo queries | `http://localhost:8080/query/v1/traces/` |
| `/query/v1/logs/` | Loki queries | `http://localhost:8080/query/v1/logs/` |
| `/query/v1/metrics/` | Thanos/Prometheus queries | `http://localhost:8080/query/v1/metrics/` |
| `/api/v1/platform/` | Platform API | `http://localhost:8080/api/v1/platform/` |
| `/console/` | Web console | `http://localhost:8080/console/` |
| `/inventory/` | Inventory API | `http://localhost:8080/inventory/` |

If you need direct access to an internal service for debugging, you can still port-forward it manually, but that is not the normal entrypoint model anymore.

---

## 7. Verify Ingestion

Send a test trace through the Nginx entrypoint:

```bash
curl -X POST http://localhost:8080/ingest/otlp/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [{"key": "service.name", "value": {"stringValue": "test-service"}}]
      },
      "scopeSpans": [{
        "spans": [{
          "traceId": "5b8efff798038103d269b633813fc60c",
          "spanId": "eee19b7ec3c1b174",
          "name": "test-span",
          "kind": 1,
          "startTimeUnixNano": "1544712660000000000",
          "endTimeUnixNano": "1544712661000000000"
        }]
      }]
    }]
  }'
```

You can verify the console route as well:

```bash
curl -I http://localhost:8080/console/
```

---

## 8. Troubleshooting

| Issue | Solution |
|---|---|
| Pods stuck in `Pending` | Check resources: `kubectl describe pod <pod> -n iyzitrace`. Increase Minikube CPU/memory. |
| Nginx pod crashloops on startup | Check logs: `kubectl logs deployment/iyzitrace-iyzitrace-platform-nginx -n iyzitrace`. This usually means one of the routed services is not healthy yet or Nginx config is invalid. |
| OTel collectors not appearing | Ensure cert-manager is running and OTel Operator is healthy. |
| Helm timeout | First install takes longer due to image pulls. Increase `--timeout` or pre-pull images. |
| Access after reinstall behaves unexpectedly | The generated secrets use `helm.sh/resource-policy: keep`, so uninstall does not remove them. Inspect with `kubectl get secret iyzitrace-s3-credentials iyzitrace-tls -n iyzitrace`. |
