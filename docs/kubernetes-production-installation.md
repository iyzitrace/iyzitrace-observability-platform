# Deploying IyziTrace Observability Platform on Kubernetes for Production

A step-by-step guide to deploy IyziTrace on a production Kubernetes cluster using the Helm chart and the production override values.

---

## Prerequisites

| Requirement | Notes |
|---|---|
| Kubernetes cluster | v1.28+ recommended |
| `kubectl` | v1.28+ |
| Helm | v3.14+ |
| Ingress controller | `ingress-nginx` or equivalent |
| cert-manager | Required if you want Kubernetes-managed TLS |
| Persistent storage | A default `StorageClass` or explicit storage classes in values |
| DNS | A public hostname pointing to your ingress controller |

> [!IMPORTANT]
> The production profile in [values-prod.yaml](/Users/yahyaozturk/Documents/playground/iyzitrace/iyzitrace-observability-platform/helm/iyzitrace-platform/values-prod.yaml) assumes:
> 1. TLS is provided externally through the secret `iyzitrace-tls`.
> 2. Nginx Ingress is the public entrypoint.
> 3. `lawrence`, `auth-service`, and `inventory-service` remain single replica because they still use local SQLite storage in the current chart design.

---

## 1. Install Cluster Dependencies

Install `cert-manager`:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=180s
```

Install `ingress-nginx` if your cluster does not already have an ingress controller:

```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update

helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --create-namespace
```

Verify ingress is available:

```bash
kubectl get pods -n ingress-nginx
kubectl get svc -n ingress-nginx
```

---

## 2. Create the Namespace

```bash
kubectl create namespace iyzitrace
```

---

## 3. Prepare TLS

The production profile disables bootstrap TLS generation, so the secret `iyzitrace-tls` must exist before deploying the chart.

### Option A: cert-manager Certificate

If you already have a `ClusterIssuer` such as `letsencrypt-prod`, create a `Certificate`:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: iyzitrace
  namespace: iyzitrace
spec:
  secretName: iyzitrace-tls
  issuerRef:
    kind: ClusterIssuer
    name: letsencrypt-prod
  dnsNames:
    - observability.example.com
```

Apply it:

```bash
kubectl apply -f certificate.yaml
kubectl get certificate -n iyzitrace
kubectl get secret iyzitrace-tls -n iyzitrace
```

### Option B: Existing Certificate and Key

```bash
kubectl create secret tls iyzitrace-tls \
  --namespace iyzitrace \
  --cert=/path/to/fullchain.pem \
  --key=/path/to/privkey.pem
```

---

## 4. Review Production Values

Before deploying, review [values-prod.yaml](/Users/yahyaozturk/Documents/playground/iyzitrace/iyzitrace-observability-platform/helm/iyzitrace-platform/values-prod.yaml) and adjust at least:

1. `nginx.ingress.hosts[0].host`
2. `nginx.ingress.tls[0].hosts`
3. custom image tags for `lawrence`, `auth-service`, and `inventory-service` if you publish pinned versions
4. storage sizes and storage classes if your cluster does not use a suitable default `StorageClass`

If you use a non-default storage class, add it explicitly in the relevant sections of the prod values file before install.

---

## 5. Deploy the Platform

### Install from local source

```bash
helm dependency update helm/iyzitrace-platform

helm upgrade --install iyzitrace helm/iyzitrace-platform \
  --namespace iyzitrace \
  --create-namespace \
  --timeout 20m \
  --wait \
  -f helm/iyzitrace-platform/values-prod.yaml
```

### Install from OCI registry

```bash
helm upgrade --install iyzitrace oci://ghcr.io/antreklabs/charts/iyzitrace-platform \
  --version 0.1.0 \
  --namespace iyzitrace \
  --create-namespace \
  --timeout 20m \
  --wait \
  -f helm/iyzitrace-platform/values-prod.yaml
```

> [!NOTE]
> If `iyzitrace-s3-credentials` does not already exist, the chart will generate it automatically. For production, you may still prefer to create and manage it explicitly.

---

## 6. Verify the Deployment

Check pods:

```bash
kubectl get pods -n iyzitrace
```

Check ingress:

```bash
kubectl get ingress -n iyzitrace
```

Check main services:

```bash
kubectl get svc -n iyzitrace
```

You should expect to see:

- `iyzitrace-iyzitrace-platform-nginx`
- `tempo-*`
- `loki-*`
- `thanos-*`
- `iyzitrace-prometheus-server-*`
- `iyzitrace-iyzitrace-platform-auth-service`
- `iyzitrace-iyzitrace-platform-lawrence`
- `iyzitrace-iyzitrace-platform-inventory-service`

---

## 7. Access the Platform

The system has a single public entrypoint through Nginx.

Assuming your ingress hostname is `observability.example.com`, the main routes are:

| Route | Purpose |
|---|---|
| `https://observability.example.com/ingest/otlp/v1/traces` | OTLP trace ingestion |
| `https://observability.example.com/ingest/otlp/v1/metrics` | OTLP metric ingestion |
| `https://observability.example.com/ingest/otlp/v1/logs` | OTLP log ingestion |
| `https://observability.example.com/query/v1/traces/` | Tempo queries |
| `https://observability.example.com/query/v1/logs/` | Loki queries |
| `https://observability.example.com/query/v1/metrics/` | Thanos/Prometheus queries |
| `https://observability.example.com/api/v1/platform/` | Platform API |
| `https://observability.example.com/console/` | Web console |
| `https://observability.example.com/inventory/` | Inventory API |

---

## 8. Verify Ingestion

Send a test trace through the public entrypoint:

```bash
curl -X POST https://observability.example.com/ingest/otlp/v1/traces \
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

Verify the console:

```bash
curl -I https://observability.example.com/console/
```

---

## 9. Troubleshooting

| Issue | Solution |
|---|---|
| Pods stuck in `Pending` | Check PVC binding and node capacity: `kubectl describe pod <pod> -n iyzitrace` and `kubectl get pvc -n iyzitrace`. |
| Ingress is created but not reachable | Verify DNS points to the ingress controller address and confirm the ingress controller service is exposed externally. |
| Nginx pod crashloops | Check `kubectl logs deployment/iyzitrace-iyzitrace-platform-nginx -n iyzitrace`. This usually means invalid Nginx config or missing TLS secret. |
| TLS does not come up | Verify `iyzitrace-tls` exists: `kubectl get secret iyzitrace-tls -n iyzitrace`. If using cert-manager, inspect the `Certificate` and issuer. |
| Helm upgrade reuses old secrets | The generated secrets use `helm.sh/resource-policy: keep`, so uninstall does not remove them. Inspect with `kubectl get secret iyzitrace-s3-credentials iyzitrace-tls -n iyzitrace`. |
| Internal backends are healthy but queries fail | Access the system through the nginx entrypoint. The documented public routes are under `/ingest/otlp/...` and `/query/v1/...`, not direct backend services. |
