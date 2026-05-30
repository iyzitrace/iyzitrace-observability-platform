# Roadmap

This roadmap is directional and maintained by the core maintainers.

## Near Term

- Stabilize open-source contribution workflow and CI gates.
- Publish signed Docker images, Helm charts, CLI binaries, and bundle releases.
- Improve first-run documentation for Docker Compose and Kubernetes users.
- Expand service tests for auth, inventory, OpAMP, and gateway behavior.
- Improve license and third-party dependency reporting.

## Contribution Areas

Community contributions are especially welcome in:

- Documentation and examples.
- Grafana dashboard improvements.
- OpenTelemetry ingestion examples.
- Helm chart validation and production hardening.
- CLI usability and troubleshooting.
- Test coverage for services and deployment templates.

## Non-Goals

- Replacing Tempo, Loki, Prometheus, Thanos, or OpenTelemetry Collector with
  custom storage engines.
- Supporting deployment platforms that maintainers cannot test.
- Accepting changes that require committing generated secrets, private keys, or
  local runtime state.

