# Multitenancy & License Management — Architecture

> Status: **Phases 1-3 implemented** · Branch: `multitenancy` · Owner: platform maintainers
>
> This document is the source of truth for how IyziTrace becomes multi-tenant and
> how tenancy is gated by an offline-first, signed-token license (per the
> *Offline-First Lisans Yönetimi* design, Singleton Software House).
>
> Control-plane (Phase 1) is live and enforced by default. Data-plane tenancy
> (Phase 2: Loki/Tempo; Phase 3: metrics) is fully wired but **opt-in** — see
> §14 for the exact operator cutover. This avoids a hard breaking change for
> existing zero-config `make up` installs where API-key enforcement is off.

---

## 1. Goals & non-goals

**Goals**
- Isolate telemetry (traces / logs / metrics) per tenant using OpenTelemetry-native
  multi-tenancy (`X-Scope-OrgID`).
- Introduce a federation hierarchy: **Account → Tenant → Subtenant**.
- Gate tenant/subtenant creation by a **cryptographically signed license** that
  the operator downloads from the IyziTrace website and uploads into the platform.
- Enforce license limits (`tenants.max`, `subtenants.max_total`), expiry (`exp` +
  `grace_days`), and feature flags **offline** — no call to the licensing server at
  validation time.

**Non-goals (for now)**
- Building the License Authority (portal + signing service + KMS). That lives on
  the IyziTrace website; this repo only *consumes and verifies* its tokens.
- Per-tenant physical stacks. We use one shared stack with logical isolation.
- Replacing Tempo/Loki/Prometheus/Thanos with tenant-aware alternatives (e.g.
  Mimir) — but see §8 for the metrics caveat.

---

## 2. Role mapping (license design → this platform)

| License-design component | Where it lives | Responsibility |
|---|---|---|
| **License Authority** | External (iyzitrace.com + KMS/HSM) | Holds the Ed25519 **private** key. Portal issues, records, and signs `license.lic` tokens. Out of scope for this repo. |
| **Token** (`license.lic`) | Uploaded by operator into Console | Signed JWT (EdDSA/Ed25519). Self-verifying; carries limits + expiry + features. |
| **Client SDK** | **`auth-service`** | Embeds the Ed25519 **public** key(s). Verifies signature, reads claims, enforces all limits at a single point (`LicenseService`). |

One platform install = one **Account** = one active license.

---

## 3. Federation hierarchy & OrgID scheme

```
Account            acc_CCI_Holding              (= license "sub"; the whole customer)
  └─ Tenant        cci-tr, cci-kz, …            (≤ limits.tenants.max)         RBAC/grouping unit
       └─ Subtenant   prod, staging, …          (≤ limits.subtenants.max_total across ALL tenants)   telemetry scope
```

**Decision (locked): the subtenant is the hard isolation unit.**
Each subtenant gets a unique `X-Scope-OrgID`; the tenant is an organizational /
RBAC / quota grouping. This matches the license counting subtenants as the real
billable unit (`max_total = 12`), and the fact that an actual telemetry-emitting
app maps to a subtenant (e.g. `cci-tr/prod`).

**OrgID format:** `org_id = "<tenant_slug>.<subtenant_slug>"`, e.g. `cci-tr.prod`.

- Slugs are validated `^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$` (lowercase, DNS-ish).
- `.` separator is safe for Loki/Tempo tenant IDs; slugs may not contain `.`.
- `org_id` is stored explicitly on the subtenant row (never recomputed from names,
  so renames never move data).
- A **platform/admin** credential (Console, no tenant) carries no OrgID.

---

## 4. Token contract

Signed **JWT**, `alg: "EdDSA"` (Ed25519). Header carries `kid` for key rotation.
`jsonwebtoken@9.0.3`'s `verify()` rejects ed25519 keys outright (`Unknown key
type "ed25519"` — confirmed while implementing this), so verification is done
manually with Node's built-in `crypto.verify()` in
`src/services/licenseTokenVerifier.ts`, matching the same primitive
`scripts/license/sign.mjs` uses to sign — no new runtime dependency either way.

**Header**
```json
{ "alg": "EdDSA", "typ": "JWT", "kid": "iyzi-lic-2026-01" }
```

**Payload** (aligned with the license design, slide 5)
```json
{
  "sub": "acc_CCI_Holding",
  "customer": "CCI Holding A.Ş.",
  "edition": "enterprise",
  "iat": 1718800000,
  "exp": 1726576000,
  "grace_days": 14,
  "limits": {
    "tenants":    { "max": 3 },
    "subtenants": { "max_total": 12 }
  },
  "features": ["crm", "sap", "online"],
  "binding": "fp_3b1c…"        // optional node-lock fingerprint (Phase 2)
}
```

**Verification rules (`LicenseService.verify`)**
1. Parse header → look up embedded public key by `kid`. Unknown `kid` ⇒ invalid.
2. Verify EdDSA signature. One flipped character ⇒ reject (whole license invalid).
3. Compute status against a **trusted clock** (see §6):
   - `now < exp` → `active`
   - `exp ≤ now < exp + grace_days` → `grace` (functional, warns)
   - `now ≥ exp + grace_days` → `expired` (blocks new tenants/subtenants; existing keep working read-only per policy)
4. Cache parsed claims in memory + persist to the `license` table (audit + offline restart).

**Key rotation:** `auth-service` embeds a map `kid → publicKeyPem`. New keys are added
ahead of rotation so old licenses keep validating (`config/license/public-keys.json`,
baked into the image / mounted).

---

## 5. Data model (auth-service SQLite)

New tables + one `api_keys` extension. `src/config/database.ts` was upgraded
from the old blind `try/catch`-wrapped `ALTER TABLE` calls to a real versioned
migration runner (`schema_migrations` table, ordered `{version, up()}` list,
each migration wrapped in a transaction) — migration 1 is the pre-existing
baseline schema, migration 2 adds everything below.

```sql
-- Single active license (history retained for audit)
CREATE TABLE IF NOT EXISTS license (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  token           TEXT NOT NULL,           -- raw signed JWT
  kid             TEXT,
  account_sub     TEXT NOT NULL,           -- "acc_CCI_Holding"
  customer        TEXT,
  edition         TEXT,
  exp             INTEGER NOT NULL,        -- unix seconds
  grace_days      INTEGER NOT NULL DEFAULT 0,
  max_tenants     INTEGER NOT NULL,
  max_subtenants  INTEGER NOT NULL,        -- max_total
  features        TEXT NOT NULL DEFAULT '[]',  -- json array
  binding         TEXT,
  signature_valid INTEGER NOT NULL DEFAULT 0,  -- bool
  status          TEXT NOT NULL DEFAULT 'active', -- active|grace|expired|invalid
  installed_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  is_active       INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS tenants (
  id          TEXT PRIMARY KEY,            -- uuid
  account_sub TEXT NOT NULL,
  name        TEXT NOT NULL,
  slug        TEXT NOT NULL UNIQUE,        -- [a-z0-9-]
  status      TEXT NOT NULL DEFAULT 'active', -- active|suspended
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  deleted_at  DATETIME
);

CREATE TABLE IF NOT EXISTS subtenants (
  id          TEXT PRIMARY KEY,            -- uuid
  tenant_id   TEXT NOT NULL REFERENCES tenants(id),
  name        TEXT NOT NULL,
  slug        TEXT NOT NULL,               -- unique within tenant
  org_id      TEXT NOT NULL UNIQUE,        -- "<tenant_slug>.<subtenant_slug>" (X-Scope-OrgID)
  status      TEXT NOT NULL DEFAULT 'active',
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  deleted_at  DATETIME,
  UNIQUE (tenant_id, slug)
);

-- api_keys gains a scope
ALTER TABLE api_keys ADD COLUMN tenant_id    TEXT;   -- nullable (platform/admin keys: null)
ALTER TABLE api_keys ADD COLUMN subtenant_id TEXT;   -- nullable
```

**Usage counters are derived, not stored** — the source of truth is
`COUNT(*) … WHERE deleted_at IS NULL`. A cached `usage` view may be added later
purely for the dashboard.

---

## 6. LicenseService — the single enforcement point

Implemented as `auth-service/src/services/licenseService.ts` (verification
itself lives in `licenseTokenVerifier.ts`) per license-design “tek noktadan
uygula”:

```
installLicense(rawToken)             -> verify → persist → deactivate prior → return summary
getSummary()                         -> { status, customer, edition, exp, graceDays,
                                           limits, features, tenantsUsed, subtenantsUsed,
                                           tampered, remoteRevoked }
hasFeature(name)                     -> bool
assertCanCreateTenant()              -> throws LicenseLimitError (409) if tenantsUsed >= max
                                         or status ∉ {active, grace}
assertCanCreateSubtenant()           -> throws LicenseLimitError (409) if subtenantsUsed >= max_total
                                         or status ∉ {active, grace}
checkRevocationHeartbeat()           -> optional SaaS check-in, see §11 Phase 3
```

`status` is one of `none | active | grace | expired | restricted`.

**Clock-rollback protection (license-design “saat geri alma”)**
- Persists `license.last_trusted_time` alongside an HMAC
  (`HMAC(JWT_SECRET, time)`) in the `settings` table.
- On each `getSummary()`, if `now < last_trusted_time − 5min skew` or the HMAC
  doesn't match ⇒ `tampered: true` ⇒ status forced to `restricted` regardless
  of `exp`, until the clock recovers.

**Grace window:** during `grace`, tenant/subtenant creation still succeeds and
`getSummary().status` reports `grace` so the Console can show a banner. Once
past `exp + grace_days`, status becomes `expired` and creation is blocked
(existing tenants/subtenants and their API keys keep authenticating — only
*new* scope creation is gated).

---

## 7. Auth flow & NGINX OrgID injection

### 7.1 API-key → scope resolution
`api_keys` rows carry `tenant_id` + `subtenant_id`. On `/auth/validate`,
`middleware/auth.ts` resolves the key to its subtenant’s `org_id`.

> Bundled fixes shipped with this change (see known-issues): filter
> `WHERE revoked_at IS NULL` (revoked keys must fail), and look up by `prefix`
> before `bcrypt.compare` (kills the O(n) scan).

### 7.2 auth-service returns the scope as response headers
`/auth/validate` (the nginx `auth_request` target) responds `200` and sets:
```
X-Scope-OrgID: cci-tr.prod
X-Tenant-Id:   <tenant uuid>
X-Subtenant-Id:<subtenant uuid>
```
Platform/admin credentials return `200` with no OrgID header.

### 7.3 NGINX captures and forwards
For every protected ingest/query location:
```nginx
auth_request /auth/validate;
auth_request_set $scope_orgid $upstream_http_x_scope_orgid;
proxy_set_header X-Scope-OrgID $scope_orgid;
```
So the OrgID the operator’s key is bound to is injected server-side — the client
cannot spoof it (any client-sent `X-Scope-OrgID` is overwritten).

### 7.4 Signal path
- **Ingest:** collectors receive `X-Scope-OrgID`; the OTLP pipeline stamps a
  `tenant.org_id` resource attribute and forwards the header to Tempo/Loki.
- **Query:** `/query/v1/*` forwards `X-Scope-OrgID`; Tempo/Loki filter by tenant
  natively. (Metrics: see §8.)

---

## 8. Storage-backend multi-tenancy (implemented, opt-in)

The full ingest→collector→backend chain is wired and verified (see §15). What
each backend needs to actually *enforce* isolation:

| Backend | Mechanism | Status |
|---|---|---|
| **Loki** | `auth_enabled: true` (`config/loki/config.yaml.template`) → requires `X-Scope-OrgID` on every request; per-tenant streams/index. Collector (`log-otel-enrichment`) forwards it via the `headers_setter` extension. | Wired, defaults to `false` (opt-in — see §14) |
| **Tempo** | `multitenancy_enabled: true` (`config/tempo/config.yaml.template`) → per-tenant blocks; TraceQL scoped. Collector (`trace-otel-enrichment`) forwards it the same way. | Wired, defaults to `false` (opt-in — see §14) |
| **Prometheus + Thanos** | No native org isolation (this stack uses the sidecar, not Receive/Mimir). `metric-otel-enrichment` already stamps `tenant.org_id` as a resource attribute (from `X-Scope-OrgID`), which its `prometheusremotewrite` exporter's existing `resource_to_telemetry_conversion: true` turns into a `tenant_org_id` metric label automatically — no exporter change needed. Query-side enforcement is a new opt-in `metrics-tenancy-proxy` service (`quay.io/prometheuscommunity/prom-label-proxy:v0.11.1`, flags confirmed via `--help`) sitting in front of `thanos-query`, enforcing the `tenant_org_id` label from the `X-Scope-OrgID` header (`-header-name`). | Wired (compose service added, inert until nginx's `datasource_metrics` upstream is switched — see §14) |

Collector wiring detail: `otlp` receivers set `include_metadata: true` so the
inbound `X-Scope-OrgID` (forwarded by nginx via `auth_request_set`) is
reachable in-context as `metadata.x-scope-orgid`; a `resource/tenant`
processor (`action: upsert, from_context: metadata.x-scope-orgid`) stamps it
as the `tenant.org_id` resource attribute on every span/log/metric; a
`headers_setter` extension re-emits the same value as an outgoing
`X-Scope-OrgID` header on the exporters that talk to Tempo/Loki over
HTTP/gRPC (`auth.authenticator: headers_setter` on the exporter). All three
collector configs (`trace.yaml`, `log.yaml`, `metric.yaml`) were validated
with `otelcol-contrib validate --config=...` (exit 0) against the exact
image pinned in `docker-compose.yml` (`0.145.0`).

---

## 9. Dev license signer (`scripts/license/`)

For end-to-end testing without the real KMS. **The real private key never lives
here** — this is a local dev keypair only.

```
scripts/license/
  keygen.mjs        # generates ed25519 keypair → dev-private.pem (gitignored), dev-public.pem
  sign.mjs          # signs a payload json → license.lic (compact JWT), sets kid
  README.md
config/license/
  public-keys.json  # { "kid": "<pem>" } embedded/mounted into auth-service (dev key added here)
```

- `dev-private.pem` and `*.lic` go into `.gitignore`.
- The dev public key is added to `config/license/public-keys.json` so auth-service
  verifies dev-signed tokens.
- Uses only Node's built-in `crypto` (ed25519) — no dependency, mirroring
  `licenseTokenVerifier.ts` exactly (see §4 for why `jsonwebtoken` isn't used here).

---

## 10. Console UI (auth-service SPA)

Additive screens in the existing vanilla-JS SPA:
- **License** card: install via a `license.lic` file picker (reads the file
  client-side with `FileReader`, mirroring the existing SSL-cert upload
  pattern) with a paste-token textarea as a fallback for dev/scripted
  workflows; shows customer/edition/exp/grace, limits vs. usage
  (`3 tenants · 7/12 subtenants`), the license status badge
  (active/grace/expired/restricted), and a separate "Telemetry Ingestion"
  status badge (§16.0) since that's gated independently of the license's
  own status label.
- **Tenants** view: list/create/suspend tenants; nested subtenant create — each
  create call surfaces the license-limit `409` inline.
- **API keys**: bind a new key to a tenant/subtenant on generation.

---

## 11. Phased roadmap

**Phase 1 — Control plane — ✅ Implemented**
1. ✅ Versioned migration runner + `license`, `tenants`, `subtenants` tables; `api_keys` scope columns (`auth-service/src/config/database.ts`).
2. ✅ `LicenseService` (verify EdDSA, status/limits, clock-rollback) + `config/license/public-keys.json`.
3. ✅ `scripts/license/` dev signer (`keygen.mjs`, `sign.mjs`).
4. ✅ Platform license install/status endpoints (`/auth/license/install`, `/auth/license/status`) as a *new* controller/router — the pre-existing GCP-backed `controllers/licenseController.ts` / `/api/v1/license` (external plugin licensing) is untouched; they're distinct concerns sharing the word "license".
5. ✅ Tenant/subtenant CRUD with limit enforcement (`services/tenantService.ts`, `/auth/tenants`, `/auth/subtenants`).
6. ✅ `api_keys` bind-to-scope on create; `/auth/validate` returns `X-Scope-OrgID`/`X-Tenant-Id`/`X-Subtenant-Id`; **bundled auth fixes** (revoked-key filter, prefix lookup, JWT-secret fail-fast via `config/secrets.ts`).
7. ✅ Console UI: License card, Tenants/subtenants management, scoped key creation (`auth-service/ui/dist/assets/app.js`).
8. ✅ Test harness (vitest + supertest, none existed before): 43 tests across `licenseTokenVerifier`, `licenseService`, `tenantService`, `tenancyOverridesService`, the `authenticate` middleware (incl. regression tests for the revoked-key bug and the suspension-not-enforced bug — §16.3), feature gating, and the heartbeat. Wired into `Makefile`'s `test` target.
9. ✅ Feature gating (`inventory`, `agent_management`, `external_query`, `alerting`, `metrics_ingest`) and per-signal Tempo/Loki retention & rate-limit overrides — see §16 (added after initial Phase 1-3 delivery, in response to a follow-up scoping request).

**Phase 2 — Ingest/query wiring — ✅ Implemented (opt-in cutover)**
- ✅ nginx `auth_request_set` OrgID injection on all `/ingest/*` and `/query/*` locations (both :80 and :443), validated with `nginx -t`.
- ✅ OTLP collectors (`trace.yaml`, `log.yaml`) stamp `tenant.org_id` and forward `X-Scope-OrgID` to Tempo/Loki via the `headers_setter` extension, validated with `otelcol-contrib validate`.
- ✅ Loki `auth_enabled` / Tempo `multitenancy_enabled` toggles are in place in the templates, defaulted `false` with an explicit cutover note (§14) rather than force-enabled, to avoid breaking existing zero-config installs mid-upgrade.
- ⏳ Node-lock binding + short-lived token renewal (license-design Phase 2) — **not implemented**; `binding` is accepted/stored as a claim but not enforced against host fingerprint. Tracked as follow-up.

**Phase 3 — Metrics tenancy & SaaS — ✅ Implemented (opt-in cutover)**
- ✅ `metric-otel-enrichment` stamps `tenant.org_id`, which its existing `resource_to_telemetry_conversion` turns into a `tenant_org_id` Prometheus label automatically.
- ✅ `metrics-tenancy-proxy` (prom-label-proxy) added to `docker-compose.yml`, enforcing `tenant_org_id` from `X-Scope-OrgID`; opt-in via nginx upstream swap (§14).
- ✅ Online license heartbeat (`checkRevocationHeartbeat`, config-gated via `LICENSE_HEARTBEAT_URL`) — off by default, soft-fails on any network error, only an explicit `{revoked:true}` response restricts.
- ⏳ Usage telemetry beyond the heartbeat payload (tenant/subtenant counts) and a full hybrid-mode UX — **not implemented**; the heartbeat is the minimal SaaS-ready hook, not a full telemetry pipeline.
- ⏳ Helm chart parity for `metrics-tenancy-proxy` — **not implemented** (docker-compose only); no `helm` binary was available in this environment to validate a template addition.

---

## 12. Security considerations
- Verification is offline & signature-based; content is not secret (readable JWT) —
  integrity, not confidentiality, is what matters.
- Embed the **public** key only; private key stays in KMS on the website side.
- `kid` + multi-key embed enables rotation without breaking issued licenses.
- Client-supplied `X-Scope-OrgID` is always overwritten by nginx from the
  auth-resolved value — no cross-tenant spoofing.
- Clock-rollback and (Phase 2) node-lock harden against offline abuse; the license
  design is explicit that the ultimate boundary is contractual, not technical.

## 13. Open questions
- Post-grace policy for **existing** subtenants: hard-stop ingestion, or read-only?
  (default proposed: keep ingesting existing scopes, block creation of new ones.)
- Should a tenant also get its own roll-up OrgID for cross-subtenant dashboards, or
  is that purely a query-time union? (leaning: query-time union, no extra OrgID.)
- Metrics: label-enforcement proxy vs. Mimir migration — chosen: label-enforcement
  proxy (prom-label-proxy), see §8. Revisit if metrics volume ever justifies Mimir.

---

## 14. Operator runbook: graduating to enforced multitenancy

**Step 0 is no longer optional.** As of §16.0, `make up` with no license
installed rejects every OTLP request (traces/logs/metrics) with `403`. A
license must be installed before the platform ingests anything at all —
this isn't part of the opt-in multitenancy cutover below, it's a hard
prerequisite for basic operation.

0. **Install a license** (Console → License → Install License, or sign a
   dev token with `scripts/license/` for testing — see its README). Nothing
   flows until this is done. `getSummary().status` must be `active` or
   `grace`.

Beyond that baseline, full **multi-tenant data-plane isolation**
(Loki/Tempo/metrics per-tenant enforcement) stays inert until you flip four
more things — in this order:

1. Create at least one tenant + subtenant (Console → Tenants).
2. **Bind every API key you use to a subtenant.** Any *unscoped* key will
   resolve to an empty `X-Scope-OrgID`, and once step 4 is done, Loki/Tempo
   will reject those requests outright.
3. **Turn on API-key enforcement** (Console → Security → "Enforce API Key
   (Agents)" and/or "(External Consumers)"). Without this, `authenticate()`
   short-circuits with `next()` before ever resolving a scope — see
   `middleware/auth.ts`. (Note: this toggle only affects credential
   checking/scope resolution, not the step-0 license gate above, which
   always runs regardless.)
4. **Flip the backend toggles:**
   - `config/loki/config.yaml.template`: `auth_enabled: true`
   - `config/tempo/config.yaml.template`: `multitenancy_enabled: true`
   - `config/nginx/nginx.conf`: change the `datasource_metrics` upstream from
     `server thanos-query:9091;` to `server metrics-tenancy-proxy:8080;` (both
     the :80 and :443 server blocks use the same `upstream` block, so this is
     a single edit)
   - Re-run `apply`/`gen-creds` as appropriate for your deploy path, then
     restart `loki`, `tempo`, and `nginx`.

Doing steps 1-3 without step 4 is safe and reversible (nothing enforces
per-tenant isolation yet — you're just populating the control plane and
validating that requests resolve the right `X-Scope-OrgID`, e.g. by curling
`/api/v1/platform/auth/validate` with a scoped key and checking the response
headers). Step 4 is the actual hard cutover for *isolation*; step 0 is the
hard cutover for *operation at all*.

**Telemetry ingestion (§16.0), optional-module feature gating (§16.1), and
tenant/subtenant suspension are NOT behind the steps-1-4 opt-in cutover** —
ingestion and suspension are enforced from step 0 onward; the four optional
modules are enforced the instant a license exists, independent of steps
1-4. This is intentional: those are pure control-plane decisions (is there a
usable license at all? does it include this module? is this tenant
currently suspended?) with no risk of breaking per-tenant isolation,
unlike flipping backend auth on across every ingest/query request.
Per-signal retention/rate limits (§16.2) *do* share step 4's Loki/Tempo
coupling — they're inert until `auth_enabled`/`multitenancy_enabled` are on.

---

## 15. What was verified in this session

Nothing below was assumed — each was independently checked against a real
binary/tool, not just read for plausibility:

- `auth-service`: `tsc --noEmit` clean, `npm run build` produces a `dist/`
  with **no** test files (tsconfig excludes `__tests__`/`*.test.ts`/`test-utils`),
  and `npx vitest run` → **52/52 passing**, including regression tests for
  the pre-existing revoked-API-key bug, the pre-existing
  stored-but-never-enforced tenant/subtenant suspension bug, and the §16.0
  ingestion-gating behavior (blocks with no license, allows during
  active/grace, blocks again once expired, independent of `features`).
- `tempo -config.verify=true` (2.10.1) and `loki -verify-config` (3.6.6) both
  passed against the actual repo templates (`config/tempo/config.yaml.template`,
  `config/loki/config.yaml.template`) with `per_tenant_override_config` /
  `runtime_config` wired in and a real per-org_id overrides file mounted
  alongside — confirming the §16 retention/rate-limit schema before writing
  `tenancyOverridesService.ts` against it.
- `scripts/license/keygen.mjs` + `sign.mjs` were actually run end-to-end, and
  the resulting token was verified by `jsonwebtoken.verify()` failing with
  `Unknown key type "ed25519"` — the exact finding that redirected §4 away
  from `jsonwebtoken` to native `crypto`.
- `config/nginx/nginx.conf` passed `nginx -t` inside the real
  `nginx:1.27-alpine` image (with `--add-host` entries standing in for the
  compose network's DNS and a throwaway self-signed cert for the `:443`
  block).
- `config/otel/{trace,log,metric}.yaml` each passed
  `otelcol-contrib validate --config=...` (exit 0) against
  `otel/opentelemetry-collector-contrib:0.145.0` — the exact image pinned in
  `docker-compose.yml` — confirming the `from_context`, `include_metadata`,
  and `headers_setter`/`auth.authenticator` syntax is valid for this version.
  `quay.io/prometheuscommunity/prom-label-proxy:v0.11.1 --help` was run to
  confirm `-header-name`, `-label`, and `-upstream` are real, current flags
  before committing to them in `docker-compose.yml`.
- `docker compose config --quiet` (after `python3 setup/generate_credentials.py`
  to satisfy the `.secrets.env` requirement) parses the full compose file
  including the new `metrics-tenancy-proxy` service without error.
- All three `config/otel/*.yaml` files and both `.template` configs
  round-tripped through `yaml.safe_load` in a `python:3.12-alpine` container.

Not verified (couldn't be, in this environment): an actual live end-to-end
run of `make up` with a real signed license, live traffic through the
enforced path (step 4 of §14), or the Console UI in a browser. Before
relying on this in production, run that live smoke test once.

---

## 16. Feature & limit gating

Beyond the tenant/subtenant count limits in §5, the license gates telemetry
ingestion itself, specific optional modules, and per-signal
ingestion/retention caps. This section covers all three — and importantly,
**they use two genuinely different no-license defaults, both confirmed with
the operator. Do not "unify" them without discussing first.**

### 16.0 Telemetry ingestion gating — license required, no exceptions

**Confirmed with the operator on 2026-07-03: with no license installed at
all, the platform must not accept any telemetry — no traces, no logs, no
metrics.** This was a deliberate correction after the initial Phase 1-3
delivery shipped ingestion as open-by-default; the operator's reasoning:
the license check IS the platform's entry gate, so requiring one to operate
doesn't conflict with the Apache-2.0 license on the *code* (which grants
rights to read/modify/redistribute the source — it says nothing about
requiring the software to run unrestricted).

- `licenseService.assertIngestionAllowed()` uses the exact same
  `CREATABLE_STATUSES` threshold as `assertCanCreateTenant()`
  (`active`/`grace` pass; `none`/`expired`/`restricted` all fail) — and,
  unlike §16.1's feature gate, does **not** consult the `features` array at
  all. An installed license with `features: []` still permits ingestion;
  ingestion is the base right-to-operate, not an optional module.
- Wired via the same `X-License-Capability` mechanism as §16.1, using the
  reserved value `"ingest"` (`middleware/auth.ts`'s `INGEST_CAPABILITY`
  constant) so `authenticate()` branches to `assertIngestionAllowed()`
  instead of `assertFeatureAllowed()`.
- nginx sets `$license_capability "ingest"` on all three OTLP locations:
  `/ingest/otlp/v1/traces`, `/ingest/otlp/v1/logs`, `/ingest/otlp/v1/metrics`
  (both :80 and :443). This **replaced** the earlier `metrics_ingest`
  feature flag — metrics used to be feature-gated while traces/logs were
  fully open; now all three share the identical license-required gate.
  `/ingest/opamp/v1/ws` and `/ingest/opamp/v1/telemetry/` are deliberately
  **not** included — agents must always be able to connect and report their
  own health/status regardless of licensing; only the three *telemetry*
  signals are gated.
- Rejections return `403 { error, reason: "license_required", feature: "ingest" }`
  — distinct from `reason: "license_feature"` used by §16.1, so API
  consumers/UI can tell "you need a license at all" apart from "your
  license doesn't include this module."

**Practical consequence: the platform no longer ingests anything out of the
box.** A fresh `make up` with no license installed will reject every OTLP
request with 403. Operators must install a license (Console → License →
Install License, or `scripts/license/` for a dev-signed test token) before
any traces/logs/metrics flow — including before running the smoke-test curl
in `docs/cli.md` or the README's quick-start example. This is the single
biggest behavior change in this document; update onboarding docs
accordingly (not yet done in this session — see the open item at the end of
§16).

### 16.1 Licensable optional modules

`limits.features` (already part of the token contract, §4) drives real
enforcement, not just display, for four capability names:

| Capability | Gates | Where |
|---|---|---|
| `inventory` | Inventory Service REST API (`/inventory/*`) | nginx `location /inventory/` — previously had **no** auth_request at all; adding one to gate the feature is a deliberate, disclosed behavior change. |
| `agent_management` | Lawrence's *mutating* OpAMP REST calls (config push, restart, group CRUD, agent delete) | nginx `location /api/v1/platform/opamp/`, gated only for non-`GET` methods — read-only status/listing stays open, matching "sadece agent telemetrisi almak temel katman" from the original ask. |
| `external_query` | `/query/v1/{metrics,logs,traces}/` — third-party tools (e.g. your own Grafana) querying Tempo/Loki/Thanos through the gateway | nginx, all three query locations. |
| `alerting` | `/query/v1/alerts/` (new route — Alertmanager was previously unreachable through the gateway) | nginx, new `datasource_alerts` upstream → `alertmanager:9093`. |

(`metrics_ingest` no longer exists as a separate feature — folded into
§16.0's blanket ingestion gate, since the operator wanted all three signals
to share one rule rather than singling out metrics.)

**Mechanism:** each gated nginx `location` sets `$license_capability` to the
capability name (or `""` for ungated locations, or `"ingest"` for the three
OTLP locations per §16.0) before calling `auth_request /auth/validate`; the
shared internal `/auth/validate` location forwards it as
`X-License-Capability`. `middleware/auth.ts`'s `authenticate()` checks it
**before** the agent/external API-key-enforcement branches — so a
locked-out capability is blocked even when API-key enforcement is off
entirely, and even for a request with no credentials at all (tested
explicitly — see `auth.test.ts`).

**Critical design decision — no license installed = unrestricted for these
four modules only.** `assertFeatureAllowed()` returns immediately (no-op)
when `getSummary().status === 'none'`. This is the OPPOSITE of §16.0's
ingestion gate, and that's intentional, not an oversight: federation
(Account→Tenant→Subtenant, §3) and telemetry ingestion (§16.0) are the core,
always-licensed capabilities this scheme was built around from the original
ask; these four modules were added later as *additional*, separately
licensable enhancements layered on top, so they default open until a
license actively restricts them.

**Once a license exists, `features` is a strict allow-list for these four —
confirmed intentional, not a bug.** Only status `active`/`grace` **and** an
explicit listing of the capability pass. A license with an empty or missing
`features` array locks out **all four** — there is no "unless restricted"
fallback once a license is present. Anyone issuing licenses must remember to
list every module the edition grants. (Confirmed with the operator on
2026-07-03 — this is the desired behavior, not scheduled to change.)

**Summary table of the two rules:**

| | No license (`status === 'none'`) | Licensed but expired/restricted |
|---|---|---|
| Tenant/subtenant creation (§5) | ❌ Blocked | ❌ Blocked |
| Telemetry ingestion (§16.0) | ❌ Blocked | ❌ Blocked |
| `inventory`/`agent_management`/`external_query`/`alerting` (§16.1) | ✅ Open | ❌ Blocked |

### 16.2 Per-signal ingestion/retention limits (Tempo & Loki)

`limits.traces` / `limits.logs` (both optional in the token — §4) drive
per-**subtenant** overrides, generated by a new
`auth-service/src/services/tenancyOverridesService.ts`:

```
limits.traces.retention_days              -> Tempo overrides[org_id].compaction.block_retention (days*24 + "h")
limits.traces.ingestion_rate_limit_bytes  -> Tempo overrides[org_id].ingestion.rate_limit_bytes
limits.traces.ingestion_burst_size_bytes  -> Tempo overrides[org_id].ingestion.burst_size_bytes
limits.traces.max_traces_per_user         -> Tempo overrides[org_id].ingestion.max_traces_per_user

limits.logs.retention_days                -> Loki overrides[org_id].retention_period (days*24 + "h")
limits.logs.ingestion_rate_mb             -> Loki overrides[org_id].ingestion_rate_mb
limits.logs.ingestion_burst_size_mb       -> Loki overrides[org_id].ingestion_burst_size_mb
limits.logs.max_streams_per_user          -> Loki overrides[org_id].max_streams_per_user
```

Only *active* subtenants under an *active* tenant get an entry (deleted or
suspended ones are omitted entirely, so their limits stop applying the
instant they're suspended — see the JOIN in `listActiveOrgIds()`). A
subtenant absent from the file simply inherits Tempo's `overrides.defaults`
/ Loki's `limits_config` global defaults — i.e. "not specified in the
license" means "unrestricted," exactly like feature gating in §16.1.

**Files & wiring:**
- `auth-service` writes `data/tenancy/tempo-overrides.yaml` and
  `data/tenancy/loki-runtime-config.yaml` atomically (temp file + rename)
  on: license install, and every tenant/subtenant create/suspend/reactivate/
  delete, plus once at startup.
- `config/tempo/config.yaml.template` adds `overrides.per_tenant_override_config`
  + `per_tenant_override_period: 10s` (Tempo polls the file itself — no
  restart needed to pick up a change).
- `config/loki/config.yaml.template` adds `runtime_config.file` +
  `period: 10s` (same auto-reload behavior). **Only takes effect once
  `auth_enabled: true`** — with it `false`, every request is bucketed under
  Loki's single implicit "fake" tenant and per-org_id entries never match
  (same coupling as §14's Loki cutover step).
- `docker-compose.yml`: the two files are bind-mounted read-only into
  `tempo`/`loki` and read-write into `auth-service` (`/app/tenancy`); `make
  dirs` pre-creates both as empty `overrides: {}` files so Docker never
  substitutes a directory for a missing bind-mount source on first boot.

### 16.3 Bonus fix found while wiring this: tenant/subtenant suspension was never enforced

While extending `checkApiKey()` to add the capability check, we found that
**suspending** a tenant or subtenant (`status = 'suspended'`, a feature
Phase 1 already shipped in the Console) never actually blocked its API keys
— the column was written but never read back during authentication. Fixed
in the same change: `checkApiKey()` now requires `status = 'active'` on
both the subtenant and its parent tenant before authenticating a scoped key.
Covered by three new regression tests in `auth.test.ts`.
