# iyzitrace CLI — Installation & Operations Guide

`iyzitrace` is the lifecycle manager for the iyzitrace observability platform.
A single Go binary turns a fresh Linux or macOS host into a running platform
in four commands:

```bash
iyzitrace init
iyzitrace config edit       # set data_dir + build paths
iyzitrace apply
iyzitrace up
```

The platform is a Docker Compose stack: Tempo, Loki, Prometheus, Thanos
(sidecar/query/store/compactor), SeaweedFS S3, OpenTelemetry collectors
(trace/metric/log), NGINX gateway, and three custom services (auth, lawrence
opamp server, inventory).

---

## Contents

- [Prerequisites](#prerequisites)
- [What you build, what you ship](#what-you-build-what-you-ship)
- [Step-by-step install](#step-by-step-install)
- [Install layout](#install-layout)
- [Configuration model](#configuration-model)
- [Command reference](#command-reference)
- [Day-2 operations](#day-2-operations)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

| | Minimum | Notes |
|---|---|---|
| Docker Engine | 24+ | `docker compose version` must print v2.x |
| Compose | v2 (plugin) | The legacy `docker-compose` binary is not supported |
| OS | Linux (Ubuntu 22.04+, Debian 12, RHEL 9) or macOS | |
| Disk | ~20 GB | Bundle is ~100 KB; the rest is data + images |
| Ports | 80, 443 | Configurable (`HTTP_PORT`, `HTTPS_PORT`) |
| Source repo checkout | yes (today) | The three custom services build from source until v1.1 ships pre-built images |

> **v0.x status.** The CLI binary and the bundle tarball are not yet
> published to GitHub Releases. Both are built from the
> `iyzitrace-observability-platform` repository today. See
> [What you build, what you ship](#what-you-build-what-you-ship).

---

## What you build, what you ship

Two artifacts come out of the maintainer repo:

```
iyzitrace-observability-platform/
├─ cli/bin/iyzitrace                          ← built by `make cli`
└─ dist/iyzitrace-bundle-<version>.tar.gz     ← built by `make bundle`
   dist/iyzitrace-bundle-<version>.tar.gz.sha256
```

- The **binary** is statically built Go; copy it to any host with the right
  architecture.
- The **bundle** is a tarball mirroring the install dir layout: it contains
  `docker-compose.yml`, `config/*` (with `.template` files for the
  secret-bearing configs), `iyzitrace.yaml.default`, and `BUNDLE_VERSION`.

The bundle is the *only* shipping unit that touches the operator's host.
It contains zero secrets — only `.template` files with `${VAR}` placeholders
get baked in. Secrets are generated locally by `iyzitrace init` and stay on
the operator's box.

### Building both (one-time)

In the repo:

```bash
make cli       # → cli/bin/iyzitrace
make bundle    # → dist/iyzitrace-bundle-<version>.tar.gz (+ .sha256)
```

---

## Step-by-step install

This walks through a fresh setup in a directory you control. Paths assume:

- repo: `~/code/iyzitrace-observability-platform`
- install/setup dir: `~/iyzitrace-setup`

Adjust to your layout.

### 1. Prepare the setup directory

```bash
mkdir -p ~/iyzitrace-setup
cd ~/iyzitrace-setup
cp ~/code/iyzitrace-observability-platform/cli/bin/iyzitrace ./iyzitrace
# Make every iyzitrace command use this directory as the install root.
export IYZITRACE_INSTALL_DIR=$PWD
```

> The CLI also accepts `--install-dir <path>` on each invocation if you'd
> rather not use the env var. Without either, it picks `/etc/iyzitrace` (if
> run as root) or `~/.iyzitrace` (otherwise).

### 2. `init` — fetch the bundle, generate secrets

```bash
./iyzitrace init \
  --bundle-url "file://$HOME/code/iyzitrace-observability-platform/dist/iyzitrace-bundle-0.0.1.tar.gz" \
  --require-checksum
```

`init` does four things:
1. Downloads `iyzitrace-bundle-<ver>.tar.gz`, verifies its sha256.
2. Extracts the tarball **directly into the install dir** (no intermediate
   `bundle/` subdir). After this you'll see `docker-compose.yml`, `config/`,
   `iyzitrace.yaml.default`, `BUNDLE_VERSION` at the top level.
3. Copies `iyzitrace.yaml.default` → `iyzitrace.yaml` if it doesn't already
   exist (use `--force` to overwrite).
4. Generates a random secrets vault at `secrets/vault.json` (mode 0600) with
   the S3 access keys and JWT secret.

### 3. Configure knobs

```bash
./iyzitrace config edit
```

Set, at minimum:

```yaml
deployment:
  data_dir: /Users/you/iyzitrace-setup/data        # absolute path

services:
  auth:      { build: /Users/you/code/iyzitrace-observability-platform/auth-service }
  lawrence:  { build: /Users/you/code/iyzitrace-observability-platform/opamp }
  inventory: { build: /Users/you/code/iyzitrace-observability-platform/inventory-service }
```

> **Why absolute paths?** The defaults `./auth-service`, `./opamp`, etc., are
> resolved against the cwd at apply time. If you run `apply` from outside
> the source repo, the relative paths won't find anything. Once binaries
> are published in v1.1, set `image:` instead of `build:` and the paths
> become irrelevant.

Verify the file is well-formed:

```bash
./iyzitrace config validate         # → ok
```

### 4. `apply` — generate `.env`, `.secrets.env`, expand templates

```bash
./iyzitrace apply
```

`apply` writes four kinds of files into the install dir:

- **`.env`** — non-secret variables for `docker compose` substitution:
  `DATA_DIR`, `HTTP_PORT`, `HTTPS_PORT`, `DOMAIN`, `CERTS_DIR`,
  `TEMPO_IMAGE`, `LOKI_IMAGE`, …, `AUTH_BUILD`, `LAWRENCE_BUILD`,
  `INVENTORY_BUILD` (resolved to absolute paths).
- **`.secrets.env`** (mode 0600) — vault contents in env-file format:
  `LOKI_ACCESS_KEY`, `LOKI_SECRET_KEY`, `TEMPO_*`, `THANOS_*`, `JWT_SECRET`.
  Services that need these mount this file via `env_file:`.
- **Expanded `*.yaml` / `*.json`** — for every `*.template` under `config/`,
  `apply` runs `${VAR}` substitution against the vault and writes the
  rendered file (mode 0600) next to the template.
- **`data/<service>/` directories** and **`certs/`** under the install dir.

### 5. `up` — start the platform

```bash
./iyzitrace up
```

First-time start builds the three local images (`auth-service`, `lawrence`,
`inventory-service`) — usually 30–60 seconds. Subsequent ups are instant.

`iyzitrace up` invokes:

```
docker compose -f $IYZITRACE_INSTALL_DIR/docker-compose.yml \
               --env-file $IYZITRACE_INSTALL_DIR/.env \
               up -d
```

cwd is set to the install dir so relative volume paths in `docker-compose.yml`
resolve correctly.

### 6. Verify

```bash
./iyzitrace status                              # all 14 services should be running

# Ingress smoke tests
curl -s -o /dev/null -w "%{http_code}\n" http://localhost/console/
curl -s -o /dev/null -w "%{http_code}\n" http://localhost/inventory/health
curl -s -o /dev/null -w "%{http_code}\n" http://localhost/api/v1/platform/health

# Send a test OTLP trace
curl -sS -X POST http://localhost/ingest/otlp/v1/traces \
  -H 'Content-Type: application/json' \
  --data '{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"smoke"}}]},"scopeSpans":[{"spans":[{"traceId":"5b8aa5a2d2c872e8321cf37308d69df2","spanId":"051581bf3cb55c13","name":"smoke","startTimeUnixNano":"1779660000000000000","endTimeUnixNano":"1779660001000000000","kind":1}]}]}]}'
# Expected: {"partialSuccess":{}}
```

---

## Install layout

```
<install>/
├── iyzitrace.yaml          ← your knobs (the source of truth)
├── iyzitrace.yaml.default  ← reference copy from the bundle
├── docker-compose.yml      ← from the bundle; references ${VARS}
├── BUNDLE_VERSION
├── state.json              ← bundle version + last-apply hash
├── .env                    ← generated by apply (compose variable substitution)
├── .secrets.env            ← generated by apply, mode 0600 (env_file: for containers)
├── config/                 ← from the bundle, edit in place
│   ├── loki/config.yaml.template        ← source for loki S3 creds
│   ├── loki/config.yaml                 ← generated by apply, mode 0600
│   ├── tempo/config.yaml.template       ← source for tempo S3 creds
│   ├── tempo/config.yaml                ← generated by apply, mode 0600
│   ├── thanos/bucket.yaml.template      ← source for thanos S3 creds
│   ├── thanos/bucket.yaml               ← generated by apply, mode 0600
│   ├── seaweedfs/s3.json.template       ← source for IAM identities
│   ├── seaweedfs/s3.json                ← generated by apply, mode 0600
│   ├── prometheus/config.yaml           ← edit in place; no template
│   ├── prometheus/rules/*.yaml          ← edit in place
│   ├── otel/{log,metric,trace}.yaml     ← edit in place; collector reads ${VAR} natively
│   ├── inventory/config.yaml            ← edit in place
│   ├── lawrence/config.yaml             ← edit in place
│   └── nginx/nginx.conf                 ← edit in place
├── secrets/
│   └── vault.json          ← mode 0600
├── certs/                  ← bootstrapped TLS certs (init-certs container)
└── data/                   ← per-service bind-mount root
    ├── tempo/
    ├── loki/
    ├── prometheus/
    ├── seaweedfs/
    ├── thanos-store/
    ├── thanos-compactor/
    ├── auth/
    ├── lawrence/
    └── inventory/
```

### Editing config

| If you want to change… | Edit… | What gets re-generated by `apply` |
|---|---|---|
| Image versions, ports, data_dir, domain | `iyzitrace.yaml` | `.env`, then `up` picks it up |
| Secrets (rotate keys) | `iyzitrace secrets rotate` | `.secrets.env` + every `*.template` sibling |
| Loki / Tempo / Thanos / Seaweedfs config | the `.template` file | the substituted sibling gets overwritten |
| Prometheus / OTel / nginx / lawrence / inventory config | the `.yaml`/`.conf`/`.json` directly | nothing — `apply` doesn't touch these |

---

## Configuration model

### `iyzitrace.yaml`

```yaml
deployment:
  network: iyzitrace-network              # docker network name
  data_dir: /var/lib/iyzitrace/data       # absolute path; per-service dirs are created under here
  http_port: 80                           # nginx ingress (HTTP)
  https_port: 443                         # nginx ingress (HTTPS, bootstrap self-signed by default)
  domain: localhost                       # used by the cert bootstrap subjectAltName

secrets:
  backend: file                           # only "file" supported in v1
  rotate_on_upgrade: false

services:
  tempo:      { image: grafana/tempo:2.10.1 }
  loki:       { image: grafana/loki:3.6.6 }
  prometheus: { image: prom/prometheus:v3.5.0 }
  thanos:     { image: quay.io/thanos/thanos:v0.32.5 }
  seaweedfs:  { image: chrislusf/seaweedfs:4.13 }
  otel:       { image: otel/opentelemetry-collector-contrib:0.145.0 }
  nginx:      { image: nginx:1.27-alpine }
  auth:       { build: /abs/path/to/auth-service }
  lawrence:   { build: /abs/path/to/opamp }
  inventory:  { build: /abs/path/to/inventory-service }
```

Each `services.<name>` entry needs **exactly one** of `image:` or `build:`.
`image:` takes precedence if both are set. `apply` resolves relative
`build:` paths to absolute using the cwd of the invocation.

### `.env` (generated)

```
DATA_DIR=/Users/you/iyzitrace-setup/data
CERTS_DIR=/Users/you/iyzitrace-setup/certs
HTTP_PORT=80
HTTPS_PORT=443
DOMAIN=localhost
NETWORK=iyzitrace-network
TEMPO_IMAGE=grafana/tempo:2.10.1
LOKI_IMAGE=grafana/loki:3.6.6
…
AUTH_BUILD=/Users/you/code/iyzitrace-observability-platform/auth-service
…
```

### `.secrets.env` (generated, mode 0600)

```
LOKI_ACCESS_KEY=…
LOKI_SECRET_KEY=…
TEMPO_ACCESS_KEY=…
TEMPO_SECRET_KEY=…
THANOS_ACCESS_KEY=…
THANOS_SECRET_KEY=…
JWT_SECRET=…
```

Only the services that connect to S3 mount this file via `env_file:`.

---

## Command reference

### Lifecycle

| Command | What it does |
|---|---|
| `iyzitrace init` | Fetch bundle, extract to install dir, generate secrets vault, write `iyzitrace.yaml` from default |
| `iyzitrace apply` | Write `.env`, `.secrets.env`, expand `*.template` files, mkdir data/certs, stamp state |
| `iyzitrace up [svc…]` | `docker compose up -d` |
| `iyzitrace down [--purge]` | `docker compose down` (`--purge` also wipes `data/`) |
| `iyzitrace restart [svc…]` | `docker compose restart` |
| `iyzitrace status` | Tabular `docker compose ps` |
| `iyzitrace logs [svc] [-f] [--tail N]` | `docker compose logs` |
| `iyzitrace version` | Print binary version/commit/date |

### Configuration

| Command | What it does |
|---|---|
| `iyzitrace config show` | Print `iyzitrace.yaml` |
| `iyzitrace config edit` | Open in `$EDITOR` (defaults to `vi`) |
| `iyzitrace config validate` | Schema + sanity check; prints `ok` on success |

### Secrets

| Command | What it does |
|---|---|
| `iyzitrace secrets list` | List secret names + sizes (never values) |
| `iyzitrace secrets show <name> --yes-i-really-want-this` | Print one secret value (gated by the flag) |
| `iyzitrace secrets rotate [name…]` | Generate fresh values; remember to `apply` + `up` |
| `iyzitrace secrets set <name>=<value>` | Set a known value (e.g. import an existing JWT key) |

### Common flags

| Flag | Effect |
|---|---|
| `--install-dir <path>` | Use a non-default install dir |
| `IYZITRACE_INSTALL_DIR=<path>` (env var) | Same as above, applied to every command |
| `--bundle-url <url>` (on `init`) | `file://` or `https://` location of the bundle |
| `--require-checksum` (on `init`) | Fail if `<url>.sha256` is missing |
| `--force` (on `init`) | Overwrite an existing `iyzitrace.yaml` |

---

## Day-2 operations

### Change a service's config

```bash
$EDITOR ~/iyzitrace-setup/config/prometheus/config.yaml
iyzitrace restart prometheus
```

Or for secret-bearing services:

```bash
$EDITOR ~/iyzitrace-setup/config/loki/config.yaml.template
iyzitrace apply               # re-expands the template
iyzitrace restart loki
```

### Bump an image version

```bash
iyzitrace config edit         # change services.tempo.image
iyzitrace apply               # rewrites .env
iyzitrace up                  # compose picks up the new image tag
```

### Rotate secrets

```bash
iyzitrace secrets rotate
iyzitrace apply               # re-renders the .template files with new keys
iyzitrace down && iyzitrace up
```

> A rolling restart isn't enough — the S3 IAM identities live in
> `seaweedfs/s3.json`, which only reloads on container start, so seaweedfs
> needs to come down and back up. After it does, every client (loki,
> tempo, thanos-store, thanos-compactor) sees the new keys via env_file.

### Move data to a different disk

```bash
iyzitrace down
mv ~/iyzitrace-setup/data /mnt/big-disk/iyzitrace-data
iyzitrace config edit         # set deployment.data_dir to the new path
iyzitrace apply
iyzitrace up
```

### Reset to a clean state

```bash
iyzitrace down --purge        # stops containers AND removes data/
rm -rf ~/iyzitrace-setup
```

### Upgrade to a newer bundle

There's no automated upgrade yet. Manual procedure:

```bash
iyzitrace down
mv ~/iyzitrace-setup ~/iyzitrace-setup.bak
mkdir -p ~/iyzitrace-setup && cd $_
cp ~/iyzitrace-setup.bak/iyzitrace ./iyzitrace
export IYZITRACE_INSTALL_DIR=$PWD
./iyzitrace init --bundle-url file:///path/to/new-bundle.tar.gz --require-checksum
cp ~/iyzitrace-setup.bak/iyzitrace.yaml ./iyzitrace.yaml
cp ~/iyzitrace-setup.bak/secrets/vault.json ./secrets/vault.json
./iyzitrace apply
./iyzitrace up
```

---

## Cutting a release

The release pipeline lives in `.github/workflows/release.yml` and publishes the bundle tarball plus multi-platform CLI binaries to a GitHub Release.

### 1. Bump the version

Edit `bundle/BUNDLE_VERSION` to the next semver:

```bash
echo "0.0.2" > bundle/BUNDLE_VERSION
```

### 2. Commit and tag

```bash
git add bundle/BUNDLE_VERSION
git commit -m "release: bundle v0.0.2"
git tag bundle-v0.0.2
git push origin HEAD --tags
```

Pushing a `bundle-v*` tag triggers the workflow automatically. The workflow validates that the tag version matches `bundle/BUNDLE_VERSION` exactly before building anything.

### 3. (Optional) Trigger manually without tagging

Useful for testing a release from a feature branch without merging:

```bash
gh workflow run release.yml \
  --ref feature/my-branch \
  -f version=0.0.2 \
  -f draft=true \
  -f prerelease=true
```

`draft=true` keeps the release hidden until you publish it in the GitHub UI. `prerelease=true` marks it as pre-release once published.

### 4. Monitor the run

```bash
gh run list --workflow=release.yml --limit 5
gh run watch   # streams the most recent run
```

### What gets published

| Artifact | Description |
|---|---|
| `iyzitrace-bundle-<v>.tar.gz` | Bundle tarball — config, templates, docker-compose |
| `iyzitrace-bundle-<v>.tar.gz.sha256` | SHA-256 checksum for the bundle |
| `iyzitrace-<v>-linux-amd64` | CLI binary + `.sha256` |
| `iyzitrace-<v>-linux-arm64` | CLI binary + `.sha256` |
| `iyzitrace-<v>-darwin-amd64` | CLI binary + `.sha256` |
| `iyzitrace-<v>-darwin-arm64` | CLI binary + `.sha256` |

### Using a specific release

```bash
# Download CLI for your platform
gh release download bundle-v0.0.2 \
  --pattern "iyzitrace-0.0.2-darwin-arm64" \
  --dir /tmp

chmod +x /tmp/iyzitrace-0.0.2-darwin-arm64
sudo mv /tmp/iyzitrace-0.0.2-darwin-arm64 /usr/local/bin/iyzitrace

# Init with the matching bundle
iyzitrace init --bundle-version 0.0.2 --require-checksum
```

Or point directly at the bundle URL:

```bash
iyzitrace init \
  --bundle-url https://github.com/<owner>/iyzitrace-observability-platform/releases/download/bundle-v0.0.2/iyzitrace-bundle-0.0.2.tar.gz \
  --require-checksum
```

---

## Troubleshooting

### `iyzitrace apply` says "load secrets: …no such file or directory"

You ran `apply` before `init`. Run `init` first.

### `iyzitrace up` says "compose file missing at …"

`init` hasn't been run for this install dir. Run `iyzitrace init …` first.

### A service container is stuck restarting

```bash
iyzitrace status                       # which one
iyzitrace logs <service> --tail 50     # what's it saying
```

Common causes:
- `data_dir` not writable by the container's user. Check perms on
  `~/iyzitrace-setup/data/<service>/`. Some services (thanos-store,
  thanos-compactor) run as root inside the container.
- A `*.template` file references a `${VAR}` not in the vault. Re-run
  `apply` and confirm `.secrets.env` has the expected keys.

### "host not found in upstream X" from nginx

Nginx is trying to proxy to a service that isn't in compose. This happened
in older versions where services could be selectively disabled. The current
default enables all services. If you see this after editing
`docker-compose.yml`, restore the missing service block.

### Seaweedfs health check fails

The healthcheck uses `curl` against `http://127.0.0.1:8333`. If you see
`Connection refused`, give seaweedfs more time to bind (`start_period: 15s`
should usually be enough; bump to 30s for slow disks).

### Lawrence reports "lookup thanos-query: no such host" at startup

Race condition between lawrence's first health check and DNS registration
of `thanos-query` in the docker network. Lawrence retries; this clears
within a few seconds and reports `(healthy)`.

### "thanos.shipper.json: no such file" warning at startup

Harmless. First-boot only. The thanos sidecar creates the file on its
first sync attempt.

### How do I see what `apply` actually wrote?

```bash
ls -la ~/iyzitrace-setup/.env ~/iyzitrace-setup/.secrets.env
ls -la ~/iyzitrace-setup/config/loki/config.yaml ~/iyzitrace-setup/config/seaweedfs/s3.json
cat ~/iyzitrace-setup/state.json
```

### How do I see what compose will actually run?

```bash
docker compose -f ~/iyzitrace-setup/docker-compose.yml \
               --env-file ~/iyzitrace-setup/.env \
               config
```

This renders the final compose YAML with all `${VAR}` substitutions
applied. Useful for verifying that build paths, image tags, and data
mounts resolved the way you expected.
