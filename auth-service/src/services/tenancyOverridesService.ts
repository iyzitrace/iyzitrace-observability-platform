import fs from 'fs';
import path from 'path';
import { getDB } from '../config/database';
import { getSummary } from './licenseService';

/**
 * Generates the per-tenant override files Tempo and Loki poll from disk
 * (`overrides.per_tenant_override_config` / `runtime_config.file`), driven
 * by the active license's optional `limits.traces` / `limits.logs` caps.
 * Schemas verified against the pinned image versions with
 * `tempo -config.verify=true` and `loki -verify-config` before writing this
 * — see docs/architecture/multitenancy-licensing.md §16.
 *
 * Only subtenants get an entry (they're the hard isolation unit — see §3);
 * a subtenant with no matching license limits configured is simply absent
 * from the file, so Tempo/Loki fall back to their own global defaults.
 * Nothing here throws — a write failure is logged and skipped, since a
 * stale-but-valid overrides file is far safer than crashing request
 * handling over what is fundamentally a background sync job.
 */

const TEMPO_OVERRIDES_PATH =
  process.env.TEMPO_OVERRIDES_PATH || path.join(__dirname, '../../tenancy/tempo-overrides.yaml');
const LOKI_RUNTIME_CONFIG_PATH =
  process.env.LOKI_RUNTIME_CONFIG_PATH || path.join(__dirname, '../../tenancy/loki-runtime-config.yaml');

interface SubtenantRow {
  org_id: string;
}

const listActiveOrgIds = async (): Promise<string[]> => {
  const db = getDB();
  // A subtenant only gets an override entry (and, by extension, is only
  // reachable at all — see the auth.ts suspension check) while BOTH it and
  // its parent tenant are active.
  const rows: SubtenantRow[] = await db.all(`
    SELECT s.org_id FROM subtenants s
    JOIN tenants t ON t.id = s.tenant_id
    WHERE s.deleted_at IS NULL AND s.status = 'active'
      AND t.deleted_at IS NULL AND t.status = 'active'
  `);
  return rows.map((r) => r.org_id);
};

// Minimal, dependency-free YAML emitter for the flat key/value shapes these
// two override files need — avoids pulling in a YAML library for what is
// always a `overrides: { <quoted-org-id>: { <scalar keys> } }` structure.
const yamlScalar = (value: number | string): string =>
  typeof value === 'number' ? String(value) : value;

const writeAtomic = (filePath: string, content: string): void => {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  const tmpPath = `${filePath}.tmp-${process.pid}`;
  fs.writeFileSync(tmpPath, content, { mode: 0o644 });
  fs.renameSync(tmpPath, filePath); // atomic on the same filesystem — Tempo/Loki never see a partial file
};

const regenerateTempoOverrides = async (orgIds: string[]): Promise<void> => {
  const summary = await getSummary();
  const traces = summary.limits?.traces;

  const lines = ['overrides:'];
  if (traces && orgIds.length > 0) {
    for (const orgId of orgIds) {
      lines.push(`  "${orgId}":`);
      if (traces.ingestion_rate_limit_bytes || traces.ingestion_burst_size_bytes || traces.max_traces_per_user) {
        lines.push('    ingestion:');
        if (traces.ingestion_rate_limit_bytes) lines.push(`      rate_limit_bytes: ${yamlScalar(traces.ingestion_rate_limit_bytes)}`);
        if (traces.ingestion_burst_size_bytes) lines.push(`      burst_size_bytes: ${yamlScalar(traces.ingestion_burst_size_bytes)}`);
        if (traces.max_traces_per_user) lines.push(`      max_traces_per_user: ${yamlScalar(traces.max_traces_per_user)}`);
      }
      if (traces.retention_days) {
        lines.push('    compaction:');
        lines.push(`      block_retention: ${traces.retention_days * 24}h`);
      }
    }
  } else {
    lines.push('  {}');
  }

  writeAtomic(TEMPO_OVERRIDES_PATH, lines.join('\n') + '\n');
};

const regenerateLokiRuntimeConfig = async (orgIds: string[]): Promise<void> => {
  const summary = await getSummary();
  const logs = summary.limits?.logs;

  const lines = ['overrides:'];
  if (logs && orgIds.length > 0) {
    for (const orgId of orgIds) {
      lines.push(`  "${orgId}":`);
      if (logs.ingestion_rate_mb) lines.push(`    ingestion_rate_mb: ${yamlScalar(logs.ingestion_rate_mb)}`);
      if (logs.ingestion_burst_size_mb) lines.push(`    ingestion_burst_size_mb: ${yamlScalar(logs.ingestion_burst_size_mb)}`);
      if (logs.max_streams_per_user) lines.push(`    max_streams_per_user: ${yamlScalar(logs.max_streams_per_user)}`);
      if (logs.retention_days) lines.push(`    retention_period: ${logs.retention_days * 24}h`);
    }
  } else {
    lines.push('  {}');
  }

  writeAtomic(LOKI_RUNTIME_CONFIG_PATH, lines.join('\n') + '\n');
};

export const regenerateOverrides = async (): Promise<void> => {
  try {
    const orgIds = await listActiveOrgIds();
    await Promise.all([regenerateTempoOverrides(orgIds), regenerateLokiRuntimeConfig(orgIds)]);
  } catch (err) {
    console.error('Failed to regenerate tenancy override files', err);
  }
};

export const _paths = { TEMPO_OVERRIDES_PATH, LOKI_RUNTIME_CONFIG_PATH };
