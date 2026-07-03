import { describe, it, expect, beforeEach, vi } from 'vitest';
import express from 'express';
import request from 'supertest';
import bcrypt from 'bcryptjs';
import crypto from 'crypto';
import { createTestLicenseSigner, defaultTestPayload } from '../../test-utils/licenseTestHelper';

const setUpApp = async () => {
  vi.resetModules();
  process.env.DB_PATH = ':memory:';
  process.env.JWT_SECRET = 'test-secret';

  const { initDB, getDB } = await import('../../config/database');
  await initDB();
  const db = getDB();

  // Turn on API-key enforcement for both contexts so checkApiKey() actually runs.
  await db.run("UPDATE settings SET value = 'true' WHERE key = 'security.agent.api_key'");
  await db.run("UPDATE settings SET value = 'true' WHERE key = 'security.external.api_key'");

  const { authenticate } = await import('../auth');
  const authController = await import('../../controllers/authController');
  const licenseService = await import('../../services/licenseService');
  const licenseTokenVerifier = await import('../../services/licenseTokenVerifier');

  const app = express();
  app.use(express.json());
  app.get('/auth/validate', authenticate, authController.validate);

  const makeKey = async (opts: { tenantId?: string; subtenantId?: string; revoked?: boolean } = {}) => {
    const rawKey = 'sk-' + crypto.randomBytes(16).toString('hex');
    const prefix = rawKey.substring(0, 7);
    const hash = await bcrypt.hash(rawKey, 4); // low cost factor — tests only
    const result = await db.run(
      `INSERT INTO api_keys (name, prefix, key_hash, role, tenant_id, subtenant_id, revoked_at)
       VALUES (?, ?, ?, 'agent', ?, ?, ?)`,
      'test-key',
      prefix,
      hash,
      opts.tenantId ?? null,
      opts.subtenantId ?? null,
      opts.revoked ? new Date().toISOString() : null
    );
    return { rawKey, id: result.lastID };
  };

  const makeTenantAndSubtenant = async (opts: { tenantStatus?: string; subtenantStatus?: string } = {}) => {
    const tenantId = crypto.randomUUID();
    await db.run(
      "INSERT INTO tenants (id, account_sub, name, slug, status) VALUES (?, ?, ?, ?, ?)",
      tenantId, 'local', 'T1', 't1', opts.tenantStatus ?? 'active'
    );
    const subtenantId = crypto.randomUUID();
    await db.run(
      "INSERT INTO subtenants (id, tenant_id, name, slug, org_id, status) VALUES (?, ?, ?, ?, ?, ?)",
      subtenantId, tenantId, 'Prod', 'prod', 't1.prod', opts.subtenantStatus ?? 'active'
    );
    return { tenantId, subtenantId };
  };

  const installLicenseWithFeatures = async (features: string[]) => {
    const signer = createTestLicenseSigner();
    licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
    await licenseService.installLicense(signer.sign(defaultTestPayload({ features })));
  };

  const installLicenseWithPayload = async (overrides: Parameters<typeof defaultTestPayload>[0]) => {
    const signer = createTestLicenseSigner();
    licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
    await licenseService.installLicense(signer.sign(defaultTestPayload(overrides)));
  };

  return { app, db, makeKey, makeTenantAndSubtenant, installLicenseWithFeatures, installLicenseWithPayload };
};

describe('authenticate middleware', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it('rejects requests with no credentials when enforcement is on', async () => {
    const { app } = await setUpApp();
    const res = await request(app).get('/auth/validate').set('X-Auth-Context', 'external');
    expect(res.status).toBe(401);
  });

  it('accepts a valid, non-revoked API key', async () => {
    const { app, makeKey } = await setUpApp();
    const { rawKey } = await makeKey();

    const res = await request(app)
      .get('/auth/validate')
      .set('X-Auth-Context', 'external')
      .set('iyzitrace-api-key', rawKey);

    expect(res.status).toBe(200);
  });

  it('rejects a revoked API key even though the hash still matches (regression: prior bug)', async () => {
    const { app, makeKey } = await setUpApp();
    const { rawKey } = await makeKey({ revoked: true });

    const res = await request(app)
      .get('/auth/validate')
      .set('X-Auth-Context', 'external')
      .set('iyzitrace-api-key', rawKey);

    expect(res.status).toBe(401);
  });

  it('sets X-Scope-OrgID from the key`s bound subtenant', async () => {
    const { app, db, makeKey } = await setUpApp();

    const tenantId = crypto.randomUUID();
    await db.run('INSERT INTO tenants (id, account_sub, name, slug) VALUES (?, ?, ?, ?)', tenantId, 'local', 'T1', 't1');
    const subtenantId = crypto.randomUUID();
    await db.run(
      'INSERT INTO subtenants (id, tenant_id, name, slug, org_id) VALUES (?, ?, ?, ?, ?)',
      subtenantId,
      tenantId,
      'Prod',
      'prod',
      't1.prod'
    );

    const { rawKey } = await makeKey({ tenantId, subtenantId });

    const res = await request(app)
      .get('/auth/validate')
      .set('X-Auth-Context', 'agent')
      .set('iyzitrace-api-key', rawKey);

    expect(res.status).toBe(200);
    expect(res.headers['x-scope-orgid']).toBe('t1.prod');
    expect(res.headers['x-tenant-id']).toBe(tenantId);
    expect(res.headers['x-subtenant-id']).toBe(subtenantId);
  });

  it('does not set X-Scope-OrgID for an unscoped (platform) key', async () => {
    const { app, makeKey } = await setUpApp();
    const { rawKey } = await makeKey();

    const res = await request(app)
      .get('/auth/validate')
      .set('X-Auth-Context', 'agent')
      .set('iyzitrace-api-key', rawKey);

    expect(res.status).toBe(200);
    expect(res.headers['x-scope-orgid']).toBeUndefined();
  });

  describe('tenant/subtenant suspension (regression: was stored but never enforced)', () => {
    it('rejects a key bound to a suspended subtenant', async () => {
      const { app, makeKey, makeTenantAndSubtenant } = await setUpApp();
      const { tenantId, subtenantId } = await makeTenantAndSubtenant({ subtenantStatus: 'suspended' });
      const { rawKey } = await makeKey({ tenantId, subtenantId });

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'agent')
        .set('iyzitrace-api-key', rawKey);

      expect(res.status).toBe(401);
    });

    it('rejects a key bound to an active subtenant whose parent tenant is suspended', async () => {
      const { app, makeKey, makeTenantAndSubtenant } = await setUpApp();
      const { tenantId, subtenantId } = await makeTenantAndSubtenant({ tenantStatus: 'suspended' });
      const { rawKey } = await makeKey({ tenantId, subtenantId });

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'agent')
        .set('iyzitrace-api-key', rawKey);

      expect(res.status).toBe(401);
    });

    it('accepts a key bound to an active subtenant under an active tenant', async () => {
      const { app, makeKey, makeTenantAndSubtenant } = await setUpApp();
      const { tenantId, subtenantId } = await makeTenantAndSubtenant();
      const { rawKey } = await makeKey({ tenantId, subtenantId });

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'agent')
        .set('iyzitrace-api-key', rawKey);

      expect(res.status).toBe(200);
    });
  });

  describe('X-License-Capability feature gating', () => {
    it('allows a gated capability when no license is installed (open-source default)', async () => {
      const { app, makeKey } = await setUpApp();
      const { rawKey } = await makeKey();

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'external')
        .set('iyzitrace-api-key', rawKey)
        .set('X-License-Capability', 'inventory');

      expect(res.status).toBe(200);
    });

    it('blocks a capability not included in the installed license', async () => {
      const { app, makeKey, installLicenseWithFeatures } = await setUpApp();
      await installLicenseWithFeatures(['alerting']);
      const { rawKey } = await makeKey();

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'external')
        .set('iyzitrace-api-key', rawKey)
        .set('X-License-Capability', 'inventory');

      expect(res.status).toBe(403);
      expect(res.body.reason).toBe('license_feature');
    });

    it('allows a capability included in the installed license', async () => {
      const { app, makeKey, installLicenseWithFeatures } = await setUpApp();
      await installLicenseWithFeatures(['inventory']);
      const { rawKey } = await makeKey();

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'external')
        .set('iyzitrace-api-key', rawKey)
        .set('X-License-Capability', 'inventory');

      expect(res.status).toBe(200);
    });

    it('blocks even before credential checks — a locked-out capability rejects unauthenticated requests too', async () => {
      const { app, installLicenseWithFeatures } = await setUpApp();
      await installLicenseWithFeatures(['alerting']);

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'external')
        .set('X-License-Capability', 'inventory');

      expect(res.status).toBe(403);
      expect(res.body.reason).toBe('license_feature');
    });
  });

  describe('X-License-Capability: "ingest" (telemetry ingestion) — opposite no-license default, confirmed 2026-07-03', () => {
    it('blocks telemetry ingestion outright with no license installed, even for a valid API key', async () => {
      const { app, makeKey } = await setUpApp();
      const { rawKey } = await makeKey();

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'agent')
        .set('iyzitrace-api-key', rawKey)
        .set('X-License-Capability', 'ingest');

      expect(res.status).toBe(403);
      expect(res.body.reason).toBe('license_required');
    });

    it('allows telemetry ingestion once an active license is installed', async () => {
      const { app, makeKey, installLicenseWithPayload } = await setUpApp();
      await installLicenseWithPayload({});
      const { rawKey } = await makeKey();

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'agent')
        .set('iyzitrace-api-key', rawKey)
        .set('X-License-Capability', 'ingest');

      expect(res.status).toBe(200);
    });

    it('blocks telemetry ingestion again once the license has expired', async () => {
      const { app, makeKey, installLicenseWithPayload } = await setUpApp();
      const nowSec = Math.floor(Date.now() / 1000);
      await installLicenseWithPayload({ exp: nowSec - 1000, grace_days: 0 });
      const { rawKey } = await makeKey();

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'agent')
        .set('iyzitrace-api-key', rawKey)
        .set('X-License-Capability', 'ingest');

      expect(res.status).toBe(403);
      expect(res.body.reason).toBe('license_required');
    });

    it('an empty features array still allows ingestion — ingestion is not feature-gated', async () => {
      const { app, makeKey, installLicenseWithPayload } = await setUpApp();
      await installLicenseWithPayload({ features: [] });
      const { rawKey } = await makeKey();

      const res = await request(app)
        .get('/auth/validate')
        .set('X-Auth-Context', 'agent')
        .set('iyzitrace-api-key', rawKey)
        .set('X-License-Capability', 'ingest');

      expect(res.status).toBe(200);
    });
  });
});
