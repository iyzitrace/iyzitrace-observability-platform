# Third-Party Notices

This project depends on third-party software from the Go, npm, Docker, Helm,
Grafana, OpenTelemetry, Prometheus, Loki, Tempo, Thanos, SeaweedFS, and related
cloud-native ecosystems.

## Source Attribution

- `opamp/` contains code derived from Lawrence OSS. Existing upstream
  copyright notices and `SPDX-License-Identifier: Apache-2.0` headers must be
  preserved.
- `opamp/internal/storage/telemetrystore/reader.go` contains code attributed to
  The Jaeger Authors and must preserve the upstream Apache-2.0 notice.

## Dependency Notices

Dependency notices should be regenerated before major releases and whenever a
new dependency family is introduced.

Recommended commands:

```bash
go install github.com/google/go-licenses@latest
go-licenses report ./... > THIRD_PARTY_NOTICES.go.txt
npx license-checker --production --summary
```

Generated notice artifacts may be attached to releases or committed when they
are stable and reviewable. Do not remove upstream copyright or SPDX headers.

