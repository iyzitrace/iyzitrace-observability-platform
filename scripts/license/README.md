# License dev tooling

Local Ed25519 signer for testing IyziTrace's offline license verification
end-to-end. **The production private key never lives here** — it stays in the
License Authority's KMS/HSM on the iyzitrace.com side. This directory only
lets you generate a throwaway dev keypair and sign test tokens against it.

See [docs/architecture/multitenancy-licensing.md](../../docs/architecture/multitenancy-licensing.md)
for the full token contract and verification rules.

## Usage

```bash
# 1. Generate a dev keypair (once). Embeds the public key into
#    config/license/public-keys.json so auth-service can verify tokens
#    signed with it.
node scripts/license/keygen.mjs iyzi-lic-dev-2026-01

# 2. Edit sample-payload.json (or point at your own payload file), then sign:
node scripts/license/sign.mjs iyzi-lic-dev-2026-01

# -> writes scripts/license/license.lic

# 3. Upload license.lic via Console (Auth > License) or:
curl -X POST http://localhost/api/v1/platform/auth/license/install \
  -H "Authorization: Bearer <admin JWT>" \
  -H "Content-Type: application/json" \
  -d "{\"token\": \"$(cat scripts/license/license.lic)\"}"
```

## Files

| File | Committed? | Purpose |
|---|---|---|
| `keygen.mjs` | yes | Generates an Ed25519 keypair, writes it to `config/license/public-keys.json` |
| `sign.mjs` | yes | Signs a payload JSON into a compact EdDSA JWT (`license.lic`) |
| `sample-payload.json` | yes | Example license payload matching the token contract |
| `dev-private.pem` | **no** (gitignored) | Generated private key — local only |
| `dev-public.pem` | **no** (gitignored) | Generated public key (also embedded in `config/license/public-keys.json`) |
| `license.lic` | **no** (gitignored) | Signed test token |

Rotating keys: run `keygen.mjs` again with a new `kid` — old entries in
`config/license/public-keys.json` are preserved, so previously issued dev
tokens keep verifying.

## Available `features` keys

`sample-payload.json` lists all four currently-wired optional modules (see
docs/architecture/multitenancy-licensing.md §16.1) — remove any you want to
test as *locked*:

| Key | Gates |
|---|---|
| `inventory` | Inventory Service REST API (`/inventory/*`) |
| `agent_management` | Lawrence's mutating OpAMP calls (config push, restart, group CRUD) — read-only GETs stay open regardless |
| `external_query` | `/query/v1/{metrics,logs,traces}/` |
| `alerting` | `/query/v1/alerts/` (Alertmanager) |

Telemetry ingestion (traces/logs/metrics) is **not** a feature — it's gated
purely by license status (§16.0) and works as soon as any active/grace
license is installed, regardless of `features`.

`limits.traces`/`limits.logs` drive Tempo/Loki per-subtenant retention and
rate-limit overrides (§16.2) — inert until Loki `auth_enabled`/Tempo
`multitenancy_enabled` are turned on (§14).

Note: `docker-compose.yml` bind-mounts `config/license/public-keys.json`
directly into the running `auth-service` container, so re-running
`keygen.mjs` (adding a new `kid`) or editing `sample-payload.json` and
re-signing takes effect immediately — no container restart needed. A
restart is only required the first time the mount itself is added (already
done).
