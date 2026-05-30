# iyzitrace CLI

`iyzitrace` is the lifecycle manager for the iyzitrace observability platform.
On a fresh Linux host, `iyzitrace init && iyzitrace apply && iyzitrace up`
brings up the data plane (Tempo, Loki, Prometheus, Thanos, OTel collectors,
NGINX, SeaweedFS).

> **Full reference:** [docs/cli.md](../docs/cli.md) — Linux install from
> scratch, every command and flag, configuration model, common operations,
> troubleshooting.

## Build

    make build         # ./bin/iyzitrace
    make test
    cd .. && make bundle    # produces dist/iyzitrace-bundle-<ver>.tar.gz

## How it fits together

```
                      ┌───────────────────┐
                      │  bundle tarball   │   versioned templates +
                      │  (GitHub release) │   default iyzitrace.yaml
                      └────────┬──────────┘
                               │ init
                               ▼
   ~/.iyzitrace (or /etc/iyzitrace if root)
   ├── iyzitrace.yaml    ← the source of truth (edit by hand or `config set`)
   ├── bundle/           ← extracted templates
   ├── secrets/vault.enc ← generated S3 keys + JWT secret (mode 600)
   └── rendered/         ← `apply` outputs here
       ├── docker-compose.yaml
       ├── config/...     (prometheus, loki, tempo, ...)
       └── .secrets.env   (transient, mode 600)
                               │ apply + up
                               ▼
                         docker compose
```

## Day-1 commands

| Command | What it does |
|---|---|
| `iyzitrace init` | Fetch the bundle, generate secrets, write `iyzitrace.yaml` |
| `iyzitrace config show / get / set / edit / validate` | Inspect and edit the config |
| `iyzitrace apply` | Render templates into `rendered/`. Idempotent. |
| `iyzitrace up` | `docker compose up -d` against the rendered file |
| `iyzitrace status` | `docker compose ps` in a tabular form |
| `iyzitrace logs <service> [-f]` | `docker compose logs` |
| `iyzitrace down [--purge]` | Stop the stack; `--purge` also removes data |
| `iyzitrace secrets list / show / rotate / set` | Manage the vault |

## Install location

`iyzitrace` chooses its install layout by uid:

| Run as | Install root | Data root | Secrets |
|---|---|---|---|
| root  | `/etc/iyzitrace` | `/var/lib/iyzitrace/data` | `/var/lib/iyzitrace/secrets` |
| user  | `~/.iyzitrace`   | `~/.iyzitrace/data`       | `~/.iyzitrace/secrets`       |

Override either with `--install-dir` or `IYZITRACE_INSTALL_DIR`.

## Per-service storage

The default data root is `storage.data_dir` from `iyzitrace.yaml`. Any service
can override it:

    iyzitrace config set storage.data_dir=/mnt/data
    iyzitrace config set services.tempo.data_dir=/mnt/fast-ssd/tempo
    iyzitrace apply

`apply` creates the directories if they don't exist.

## Bundle distribution

`iyzitrace init` downloads `iyzitrace-bundle-<version>.tar.gz` from a GitHub
release on this repo by default. Override with `--bundle-url` (any
`https://...` or `file://...` URL); pair with a sibling `<url>.sha256` for
integrity. `--require-checksum` makes the checksum mandatory.

## Scope of v1

Data plane only: Tempo, Loki, Prometheus, Thanos, SeaweedFS, OTel collectors,
NGINX. The custom services (`auth`, `lawrence`, `inventory`) ship `enabled:
false` until pre-built images are published — they currently require building
from source which doesn't fit the CLI-only install model.

v2 hooks deliberately preserved in the schema:
`services.<name>.host` for multi-host, `deployment.mode` for k8s/systemd,
`secrets.backend: vault` for HashiCorp Vault.
