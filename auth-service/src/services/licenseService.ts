import crypto from 'crypto';
import { getDB } from '../config/database';
import { getJwtSecret } from '../config/secrets';
import {
  LicenseClaims,
  LicenseLimits,
  LicenseVerificationError,
  decodeLicenseHeader,
  verifyLicenseToken
} from './licenseTokenVerifier';

export { LicenseVerificationError } from './licenseTokenVerifier';

export class LicenseLimitError extends Error {}

export type LicenseStatus = 'none' | 'active' | 'grace' | 'expired' | 'restricted';

export interface LicenseSummary {
  status: LicenseStatus;
  customer?: string;
  edition?: string;
  accountSub?: string;
  exp?: number;
  graceDays?: number;
  limits?: LicenseLimits;
  features?: string[];
  tenantsUsed: number;
  subtenantsUsed: number;
  installedAt?: string;
  tampered: boolean;
  remoteRevoked: boolean;
}

// Tolerance for NTP jitter / short clock adjustments before treating a
// backward time jump as a rollback attempt.
const CLOCK_SKEW_TOLERANCE_MS = 5 * 60 * 1000;

const TRUSTED_TIME_KEY = 'license.last_trusted_time';
const TRUSTED_TIME_HMAC_KEY = 'license.last_trusted_hmac';

const computeTrustedTimeHmac = (unixMs: number): string =>
  crypto.createHmac('sha256', getJwtSecret()).update(String(unixMs)).digest('hex');

const persistTrustedTime = async (unixMs: number): Promise<void> => {
  const db = getDB();
  const hmac = computeTrustedTimeHmac(unixMs);
  const stmt = await db.prepare('INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)');
  await stmt.run(TRUSTED_TIME_KEY, String(unixMs));
  await stmt.run(TRUSTED_TIME_HMAC_KEY, hmac);
  await stmt.finalize();
};

/**
 * Clock-rollback protection (license design §8 "Saat geri alma"): tracks the
 * last-seen wall-clock time, HMAC-signed with the install secret so an
 * operator can't just edit the settings row to reset it. If `now` moves
 * backwards beyond tolerance, or the stored HMAC doesn't match, we treat it
 * as tampering and report `tampered: true` (callers force a restricted
 * status rather than trusting `exp` comparisons).
 */
const getTrustedNow = async (): Promise<{ now: number; tampered: boolean }> => {
  const db = getDB();
  const now = Date.now();

  const timeRow = await db.get('SELECT value FROM settings WHERE key = ?', TRUSTED_TIME_KEY);
  const hmacRow = await db.get('SELECT value FROM settings WHERE key = ?', TRUSTED_TIME_HMAC_KEY);

  let tampered = false;
  if (timeRow?.value && hmacRow?.value) {
    const lastTrusted = parseInt(timeRow.value, 10);
    const expectedHmac = computeTrustedTimeHmac(lastTrusted);
    if (hmacRow.value !== expectedHmac) {
      tampered = true;
    } else if (now < lastTrusted - CLOCK_SKEW_TOLERANCE_MS) {
      tampered = true;
    }
    await persistTrustedTime(Math.max(now, lastTrusted));
  } else {
    await persistTrustedTime(now);
  }

  return { now, tampered };
};

interface LicenseRow {
  id: number;
  token: string;
  kid: string | null;
  account_sub: string;
  customer: string | null;
  edition: string | null;
  exp: number;
  grace_days: number;
  max_tenants: number;
  max_subtenants: number;
  features: string;
  binding: string | null;
  installed_at: string;
  signal_limits: string;
}

const getActiveLicenseRow = async (): Promise<LicenseRow | undefined> => {
  const db = getDB();
  return db.get('SELECT * FROM license WHERE is_active = 1 ORDER BY id DESC LIMIT 1');
};

const getUsageCounts = async (): Promise<{ tenantsUsed: number; subtenantsUsed: number }> => {
  const db = getDB();
  const tenants = await db.get('SELECT COUNT(*) as c FROM tenants WHERE deleted_at IS NULL');
  const subtenants = await db.get('SELECT COUNT(*) as c FROM subtenants WHERE deleted_at IS NULL');
  return { tenantsUsed: tenants?.c ?? 0, subtenantsUsed: subtenants?.c ?? 0 };
};

/**
 * Verifies signature + shape, then persists the token as the single active
 * license (previous license rows are kept for audit, deactivated).
 */
export const installLicense = async (rawToken: string): Promise<LicenseSummary> => {
  const claims: LicenseClaims = verifyLicenseToken(rawToken);
  const header = decodeLicenseHeader(rawToken);

  const db = getDB();
  await db.exec('BEGIN');
  try {
    await db.run('UPDATE license SET is_active = 0 WHERE is_active = 1');
    await db.run(
      `INSERT INTO license
        (token, kid, account_sub, customer, edition, exp, grace_days,
         max_tenants, max_subtenants, features, binding, signal_limits, signature_valid, status, is_active)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'active', 1)`,
      rawToken,
      header.kid ?? null,
      claims.sub,
      claims.customer ?? null,
      claims.edition ?? null,
      claims.exp,
      claims.grace_days ?? 0,
      claims.limits.tenants.max,
      claims.limits.subtenants.max_total,
      JSON.stringify(claims.features ?? []),
      claims.binding ?? null,
      JSON.stringify({ traces: claims.limits.traces, logs: claims.limits.logs })
    );
    await db.exec('COMMIT');
  } catch (err) {
    await db.exec('ROLLBACK');
    throw err;
  }

  return getSummary();
};

export const getSummary = async (): Promise<LicenseSummary> => {
  const row = await getActiveLicenseRow();
  const { tenantsUsed, subtenantsUsed } = await getUsageCounts();

  if (!row) {
    return { status: 'none', tenantsUsed, subtenantsUsed, tampered: false, remoteRevoked: false };
  }

  const { now, tampered } = await getTrustedNow();
  const nowSec = Math.floor(now / 1000);
  const graceEndSec = row.exp + (row.grace_days || 0) * 86400;
  const remoteRevoked = await isRemoteRevoked();

  // Priority: clock tampering > remote (SaaS heartbeat) revocation > the
  // token's own exp/grace window. Both restricted and remote-revoked are
  // reported distinctly so the Console can explain *why* creation is
  // blocked, but both fall outside CREATABLE_STATUSES below.
  let status: LicenseStatus;
  if (tampered) {
    status = 'restricted';
  } else if (remoteRevoked) {
    status = 'expired';
  } else if (nowSec < row.exp) {
    status = 'active';
  } else if (nowSec < graceEndSec) {
    status = 'grace';
  } else {
    status = 'expired';
  }

  const signalLimits = JSON.parse(row.signal_limits || '{}') as Pick<LicenseLimits, 'traces' | 'logs'>;

  return {
    status,
    customer: row.customer ?? undefined,
    edition: row.edition ?? undefined,
    accountSub: row.account_sub,
    exp: row.exp,
    graceDays: row.grace_days,
    limits: {
      tenants: { max: row.max_tenants },
      subtenants: { max_total: row.max_subtenants },
      traces: signalLimits.traces,
      logs: signalLimits.logs
    },
    features: JSON.parse(row.features || '[]'),
    tenantsUsed,
    subtenantsUsed,
    installedAt: row.installed_at,
    tampered,
    remoteRevoked
  };
};

// --- SaaS-ready online heartbeat (license design §9-11 "Phase 3: SaaS") ---
//
// Fully optional and off by default: the platform is offline-first, and an
// operator who never sets LICENSE_HEARTBEAT_URL gets zero network calls out
// of this module. When set, we periodically check in with that endpoint;
// only an explicit `{ revoked: true }` response flips local status —
// network errors or a slow/unreachable endpoint never restrict the install
// (matching the "soft cancellation" principle: absence of a heartbeat is
// not treated as revocation).

const REMOTE_REVOKED_KEY = 'license.remote_revoked';

const isRemoteRevoked = async (): Promise<boolean> => {
  const db = getDB();
  const row = await db.get('SELECT value FROM settings WHERE key = ?', REMOTE_REVOKED_KEY);
  return row?.value === 'true';
};

const setRemoteRevoked = async (revoked: boolean): Promise<void> => {
  const db = getDB();
  await db.run(
    'INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)',
    REMOTE_REVOKED_KEY,
    String(revoked)
  );
};

interface HeartbeatResponse {
  revoked?: boolean;
}

/**
 * Best-effort online check-in. Safe to call on a timer; every failure mode
 * (no URL configured, no license installed, network error, non-2xx, bad
 * JSON) is swallowed and simply skips the update — this must never throw
 * or block request handling.
 */
export const checkRevocationHeartbeat = async (): Promise<void> => {
  const heartbeatUrl = process.env.LICENSE_HEARTBEAT_URL;
  if (!heartbeatUrl) return;

  const row = await getActiveLicenseRow();
  if (!row) return;

  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 5000);
    const response = await fetch(heartbeatUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        account_sub: row.account_sub,
        kid: row.kid,
        tenants_used: (await getUsageCounts()).tenantsUsed,
        subtenants_used: (await getUsageCounts()).subtenantsUsed
      }),
      signal: controller.signal
    });
    clearTimeout(timeout);

    if (!response.ok) return;
    const body = (await response.json()) as HeartbeatResponse;
    await setRemoteRevoked(body.revoked === true);
  } catch {
    // Network failure, timeout, or malformed response — offline-first means
    // we simply skip this check-in and try again next interval.
  }
};

export const hasFeature = async (name: string): Promise<boolean> => {
  const summary = await getSummary();
  return summary.features?.includes(name) ?? false;
};

export class LicenseFeatureError extends Error {}

// Shared threshold: a license is "usable" (grants anything at all) only
// while active or within its grace window. `'none'`/`expired`/`restricted`
// all fail this check — used both for tenant/subtenant creation below and
// for telemetry ingestion (assertIngestionAllowed).
const CREATABLE_STATUSES: ReadonlySet<LicenseStatus> = new Set(['active', 'grace']);

/**
 * Optional-module feature gating (Console → inventory, agent_management,
 * external_query, alerting — see docs/architecture/
 * multitenancy-licensing.md §16.1).
 *
 * IMPORTANT: this platform is Apache-2.0 open source. With NO license
 * installed (`status === 'none'`), every feature check passes — licensing
 * only ever *restricts further* once an operator has actually installed
 * one; it never locks self-hosters out of functionality they had before
 * touching licensing at all. Once a license is installed (active, grace,
 * expired, or restricted), only the features it explicitly lists pass —
 * confirmed intentional, not a bug (§16.1).
 *
 * Distinct from assertIngestionAllowed() below, which uses the OPPOSITE
 * no-license default — confirmed with the operator on 2026-07-03.
 */
export const assertFeatureAllowed = async (feature: string): Promise<void> => {
  const summary = await getSummary();
  if (summary.status === 'none') return;
  if (!CREATABLE_STATUSES.has(summary.status) || !summary.features?.includes(feature)) {
    throw new LicenseFeatureError(`Feature "${feature}" is not available (license status: ${summary.status})`);
  }
};

/**
 * Telemetry ingestion gate (traces/logs/metrics — see docs/architecture/
 * multitenancy-licensing.md §16.0). Unlike assertFeatureAllowed, this uses
 * the SAME no-license default as tenant/subtenant creation: with no
 * license installed at all, ingestion is blocked outright. Confirmed with
 * the operator on 2026-07-03 — the platform must not accept any telemetry
 * without an active or grace-period license, deliberately overriding the
 * "open by default" stance used for the optional feature modules above.
 */
export const assertIngestionAllowed = async (): Promise<void> => {
  const summary = await getSummary();
  if (!CREATABLE_STATUSES.has(summary.status)) {
    throw new LicenseFeatureError(`Telemetry ingestion requires a valid license (license status: ${summary.status})`);
  }
};

export const assertCanCreateTenant = async (): Promise<void> => {
  const summary = await getSummary();
  if (!CREATABLE_STATUSES.has(summary.status)) {
    throw new LicenseLimitError(`License is ${summary.status}; cannot create new tenants`);
  }
  const max = summary.limits?.tenants.max ?? 0;
  if (summary.tenantsUsed >= max) {
    throw new LicenseLimitError(`Tenant limit reached (${max})`);
  }
};

export const assertCanCreateSubtenant = async (): Promise<void> => {
  const summary = await getSummary();
  if (!CREATABLE_STATUSES.has(summary.status)) {
    throw new LicenseLimitError(`License is ${summary.status}; cannot create new subtenants`);
  }
  const max = summary.limits?.subtenants.max_total ?? 0;
  if (summary.subtenantsUsed >= max) {
    throw new LicenseLimitError(`Subtenant limit reached (${max})`);
  }
};
