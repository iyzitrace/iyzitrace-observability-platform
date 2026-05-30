# Grafana — iyzitrace Demo

A Grafana instance for demonstrating iyzitrace observability capabilities.

It comes pre-provisioned with:

- **Datasources**: Prometheus, Loki, Tempo, and Infinity (OpAMP Agents API) — all pointing at the iyzitrace platform.

  > ⚠️ **Warning:** The datasource URLs under [`datasources/`](datasources/) default to `host.docker.internal`. Update them to match your actual iyzitrace platform endpoints before running.
- **Folders**: Application Performance Monitoring, Infrastructure Monitoring, Platform Monitoring.
- **Dashboards**: Predefined dashboards organized into the folders above.

## Run

```bash
docker compose up -d
```

Grafana is available at http://localhost:3000 (login: `admin` / `adminadmin`).

## Stop

```bash
docker compose down
```
