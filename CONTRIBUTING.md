# Contributing

Thank you for contributing to IyziTrace Observability Platform.

## Contribution Workflow

1. Open or find an issue before starting substantial work.
2. Fork the repository and create a focused branch.
3. Add tests or validation for behavior changes.
4. Run the relevant local checks.
5. Open a pull request using the PR template.
6. Address maintainer feedback until the PR is ready to merge.

Small documentation fixes may go directly to a pull request.

## Local Validation

Run the broad validation target before larger PRs:

```bash
make validate
```

Useful component checks:

```bash
make -C cli test
go test ./...                  # inside opamp/ or inventory-service/
go test -tags=integration ./integration/...  # optional OpAMP integration suite
npm ci && npm run build        # inside auth-service/
docker compose config
helm lint helm/iyzitrace-platform
sh scripts/check-binary-assets.sh
```

## Good First Issues

Begin with issues labeled:

- `good first issue`
- `documentation`
- `help wanted`

Avoid starting large architecture, security, or release workflow changes
without maintainer discussion.

## Architectural Boundaries

- Public API changes belong in `docs/platform-api.openapi.yaml` and
  `auth-service/platform-api.openapi.yaml`.
- Runtime stack changes must keep Docker Compose and Helm deployment paths in
  sync.
- Generated secrets, local databases, rendered configs, and TLS private keys
  must not be committed.
- Upstream copyright and SPDX notices must be preserved.

## Non-Goals For Initial Community Contributions

- Replacing the full observability backend stack.
- Adding unsupported deployment targets without maintainers agreeing to own
  testing and documentation.
- Committing real production credentials, sample private keys, or generated
  local state.

## Developer Certificate Of Origin

By contributing, you certify that you have the right to submit your
contribution under the Apache License, Version 2.0. Add a `Signed-off-by`
trailer when required by maintainers or automation.
