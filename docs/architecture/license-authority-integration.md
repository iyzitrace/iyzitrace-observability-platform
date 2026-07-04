# License Authority Integration — iyzitrace.com Website Brief

> Hand this document to whoever (person or agent) builds the license-generation
> feature on the iyzitrace.com website. It specifies the exact token contract
> the platform (`auth-service`) verifies — see
> [multitenancy-licensing.md](multitenancy-licensing.md) for the full platform-side
> design this integrates with.

## 1. Your role

You are the **License Authority** — the party holding the Ed25519 **private**
key, signing tokens. The platform side (this repo) only embeds the
corresponding **public** key and verifies tokens **entirely offline**; it
never calls out to the website at validation time. Every token you produce
must be verifiable on a customer machine with no network access at all.

## 2. Token format — exact and non-negotiable

A compact, three-part, dot-separated string (looks like a JWT, but **most
mainstream JWT libraries don't support Ed25519/EdDSA verification** — our
side verifies with Node's native `crypto` module directly, not `jsonwebtoken`.
Raw Ed25519 signing on your side is the safe choice):

```
base64url(header) + "." + base64url(payload) + "." + base64url(signature)
```

**Header (JSON, base64url-encoded):**
```json
{ "alg": "EdDSA", "typ": "JWT", "kid": "iyzi-lic-2026-01" }
```
- `alg` must **always** be `"EdDSA"` — anything else is rejected.
- `kid` identifies which signing key was used (for rotation). You must send
  us the **public** key for this `kid` separately so we can add it to
  `config/license/public-keys.json` on the platform side (§5).

**Signature computation:**
```
signingInput = base64url(header) + "." + base64url(payload)
signature = Ed25519_Sign(privateKey, UTF8Bytes(signingInput))   // raw Ed25519, not HMAC/RSA
token = signingInput + "." + base64url(signature)
```

base64url = standard base64 with `+`→`-`, `/`→`_`, and trailing `=` padding
**stripped entirely**.

**Reference implementation** (tested, confirmed to interop with our
verifier): `scripts/license/sign.mjs` in this repo — shows exactly how
signing works with Node's `crypto` module, byte-for-byte. Whatever
stack the website uses, the resulting token must be byte-identical in
format to what this script produces. If you're on Node, use `jose`
(`alg: 'EdDSA'`) or raw `crypto`; on Python, `cryptography`'s
`Ed25519PrivateKey.sign()`; on Go, `crypto/ed25519`. Whatever you pick,
**test a real token against our verifier before shipping** (§7).

## 3. Payload — field by field

```json
{
  "sub": "acc_globex_corp",
  "customer": "Globex Corporation",
  "edition": "enterprise",
  "iat": 1751500800,
  "exp": 1783036800,
  "grace_days": 14,

  "limits": {
    "tenants": { "max": 5 },
    "subtenants": { "max_total": 20 },
    "traces": {
      "retention_days": 7,
      "ingestion_rate_limit_bytes": 25000000,
      "ingestion_burst_size_bytes": 50000000,
      "max_traces_per_user": 20000
    },
    "logs": {
      "retention_days": 14,
      "ingestion_rate_mb": 10,
      "ingestion_burst_size_mb": 20,
      "max_streams_per_user": 5000
    }
  },

  "features": ["inventory", "agent_management", "external_query", "alerting"],
  "binding": null
}
```

| Field | Required | Type | Notes |
|---|---|---|---|
| `sub` | **Yes** | string | Account/customer ID — must be unique. |
| `customer` | No | string | Display name shown in the Console. |
| `edition` | No | string | Free-text label (`enterprise`/`pro`/...) — no logic on it in code. |
| `iat` | No | number (unix seconds) | Issued-at, informational only. |
| `exp` | **Yes** | number (unix seconds) | Expiry timestamp. |
| `grace_days` | No (default 0) | number | Days of continued operation in "grace" status after `exp`. |
| `limits.tenants.max` | **Yes** | number | Max tenants this account can create. |
| `limits.subtenants.max_total` | **Yes** | number | Max subtenants across ALL tenants combined. |
| `limits.traces.*` | No | — | Omit for no per-tenant trace ingestion limit (Tempo global default applies). |
| `limits.logs.*` | No | — | Same, for logs (Loki). |
| `features` | No (default `[]`) | string[] | Only these four values matter: `"inventory"`, `"agent_management"`, `"external_query"`, `"alerting"`. Unknown strings are silently ignored. |
| `binding` | No | string \| null | Hardware node-lock fingerprint — **not yet enforced** on the platform side, reserved for a future phase. Send `null` for now. |

Portal UX suggestion: `sub`/`customer` likely come from the customer record,
`exp` from a duration picker (30 days / 1 year / 3 years), `limits.tenants.max`
/ `subtenants.max_total` from the edition's package definition, `features`
from a pre-defined checkbox list per edition.

## 4. How the platform actually uses these fields (read before assuming behavior)

- **Missing a required field rejects the token outright**: `sub`, `exp`,
  `limits.tenants.max`, `limits.subtenants.max_total`.
- **`now < exp`** → license status `active`. **`exp ≤ now < exp + grace_days*86400`**
  → `grace` (still fully functional, Console shows a warning). After that →
  `expired`.
- **Telemetry ingestion (traces/logs/metrics)** is gated purely by license
  status (`active`/`grace` = allowed) — **independent of `features`**. With
  no license installed at all, or an expired one, the platform accepts zero
  telemetry, regardless of what `features` would otherwise say.
- **The four `features` modules** follow a different rule: with no license
  installed at all, all four are open (open-source-friendly default). Once
  a license exists, only the modules explicitly listed pass — an empty
  `features: []` locks out all four (but ingestion keeps working, since
  that's the separate rule above).
- Exceeding `limits.tenants.max` / `subtenants.max_total` returns `409` on
  new tenant/subtenant creation; existing ones are unaffected.

## 5. Key exchange (the one thing you must coordinate with us)

Generate and store the private key in your KMS/HSM — never send it to us.
But you must send us the corresponding **public** key (SPKI PEM format,
e.g. `-----BEGIN PUBLIC KEY-----...`) and the `kid` you're using with it —
we add it to `config/license/public-keys.json` on the platform side
(`{ "<kid>": "<public-key-pem>" }`). Without this, none of your tokens can
ever verify — you'll get `"Unknown license signing key"` (we hit this exact
error ourselves from a deployment gap before this doc was written). For key
rotation, generate a new key under a new `kid` — we keep old entries around
so previously issued licenses keep validating.

## 6. Delivery format

The customer downloads a plain-text file named **`license.lic`** from the
portal — its content is the raw three-part token string above, with no
wrapping, no JSON envelope, no extra formatting. They upload this file (or
paste its contents) into our Console UI.

## 7. Testing before shipping

Once your signer is built, verify a sample token round-trips correctly —
either compare it against a token signed with `scripts/license/sign.mjs`
for the exact same payload, or hand us one to check against our verifier.
A single differing bit in the signature format causes outright rejection.
We can also temporarily add a test `kid` to our dev environment's
`config/license/public-keys.json` for a live end-to-end check if useful.
