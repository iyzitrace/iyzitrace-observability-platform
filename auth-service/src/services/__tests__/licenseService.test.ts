import { describe, it, expect, beforeEach, vi } from 'vitest';
import { createTestLicenseSigner, defaultTestPayload } from '../../test-utils/licenseTestHelper';

// Each test gets a fully fresh module graph (fresh `db` singleton in
// config/database, fresh in-memory sqlite) so tests never leak state into
// each other via module-level caches.
const freshLicenseService = async () => {
  vi.resetModules();
  process.env.DB_PATH = ':memory:';
  process.env.JWT_SECRET = 'test-secret';

  const { initDB } = await import('../../config/database');
  await initDB();

  const licenseTokenVerifier = await import('../licenseTokenVerifier');
  const licenseService = await import('../licenseService');
  return { licenseService, licenseTokenVerifier };
};

describe('licenseService', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it('reports status "none" when no license has been installed', async () => {
    const { licenseService } = await freshLicenseService();
    const summary = await licenseService.getSummary();
    expect(summary.status).toBe('none');
    expect(summary.tenantsUsed).toBe(0);
  });

  it('installs a valid signed token and reports it as active', async () => {
    const { licenseService, licenseTokenVerifier } = await freshLicenseService();
    const signer = createTestLicenseSigner();
    licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);

    const token = signer.sign(defaultTestPayload({ customer: 'Acme' }));
    const summary = await licenseService.installLicense(token);

    expect(summary.status).toBe('active');
    expect(summary.customer).toBe('Acme');
    expect(summary.limits?.tenants.max).toBe(2);
    expect(summary.limits?.subtenants.max_total).toBe(3);
  });

  it('rejects installing a token with an invalid signature', async () => {
    const { licenseService, licenseTokenVerifier } = await freshLicenseService();
    const signer = createTestLicenseSigner();
    licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);

    const otherSigner = createTestLicenseSigner('other-kid');
    const foreignToken = otherSigner.sign(defaultTestPayload());

    await expect(licenseService.installLicense(foreignToken)).rejects.toThrow();
  });

  it('reports "expired" once exp has passed, and "grace" during the grace window', async () => {
    const { licenseService, licenseTokenVerifier } = await freshLicenseService();
    const signer = createTestLicenseSigner();
    licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);

    const nowSec = Math.floor(Date.now() / 1000);
    const token = signer.sign(
      defaultTestPayload({ exp: nowSec - 10, grace_days: 1 }) // expired 10s ago, 1 day grace
    );
    const summary = await licenseService.installLicense(token);
    expect(summary.status).toBe('grace');

    const expiredToken = signer.sign(defaultTestPayload({ exp: nowSec - 1000, grace_days: 0 }));
    const expiredSummary = await licenseService.installLicense(expiredToken);
    expect(expiredSummary.status).toBe('expired');
  });

  it('assertCanCreateTenant throws once the license is expired', async () => {
    const { licenseService, licenseTokenVerifier } = await freshLicenseService();
    const signer = createTestLicenseSigner();
    licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);

    const nowSec = Math.floor(Date.now() / 1000);
    const token = signer.sign(defaultTestPayload({ exp: nowSec - 1000, grace_days: 0 }));
    await licenseService.installLicense(token);

    await expect(licenseService.assertCanCreateTenant()).rejects.toThrow(licenseService.LicenseLimitError);
  });

  it('assertCanCreateTenant throws with no license installed', async () => {
    const { licenseService } = await freshLicenseService();
    await expect(licenseService.assertCanCreateTenant()).rejects.toThrow(licenseService.LicenseLimitError);
  });

  describe('feature gating (assertFeatureAllowed)', () => {
    it('allows any feature when no license is installed (open-source default)', async () => {
      const { licenseService } = await freshLicenseService();
      await expect(licenseService.assertFeatureAllowed('inventory')).resolves.toBeUndefined();
      await expect(licenseService.assertFeatureAllowed('anything_at_all')).resolves.toBeUndefined();
    });

    it('allows a feature explicitly listed in the installed license', async () => {
      const { licenseService, licenseTokenVerifier } = await freshLicenseService();
      const signer = createTestLicenseSigner();
      licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
      await licenseService.installLicense(signer.sign(defaultTestPayload({ features: ['inventory', 'alerting'] })));

      await expect(licenseService.assertFeatureAllowed('inventory')).resolves.toBeUndefined();
      await expect(licenseService.assertFeatureAllowed('alerting')).resolves.toBeUndefined();
    });

    it('blocks a feature not listed once a license is installed', async () => {
      const { licenseService, licenseTokenVerifier } = await freshLicenseService();
      const signer = createTestLicenseSigner();
      licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
      await licenseService.installLicense(signer.sign(defaultTestPayload({ features: ['inventory'] })));

      await expect(licenseService.assertFeatureAllowed('agent_management')).rejects.toThrow(
        licenseService.LicenseFeatureError
      );
    });

    it('blocks all features once the license is expired', async () => {
      const { licenseService, licenseTokenVerifier } = await freshLicenseService();
      const signer = createTestLicenseSigner();
      licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
      const nowSec = Math.floor(Date.now() / 1000);
      await licenseService.installLicense(
        signer.sign(defaultTestPayload({ exp: nowSec - 1000, grace_days: 0, features: ['inventory'] }))
      );

      await expect(licenseService.assertFeatureAllowed('inventory')).rejects.toThrow(
        licenseService.LicenseFeatureError
      );
    });
  });

  it('parses optional traces/logs signal limits from the license payload', async () => {
    const { licenseService, licenseTokenVerifier } = await freshLicenseService();
    const signer = createTestLicenseSigner();
    licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);

    const token = signer.sign(
      defaultTestPayload({
        limits: {
          tenants: { max: 2 },
          subtenants: { max_total: 3 },
          traces: { retention_days: 7, ingestion_rate_limit_bytes: 25000000 },
          logs: { retention_days: 14, ingestion_rate_mb: 10 }
        }
      })
    );
    const summary = await licenseService.installLicense(token);

    expect(summary.limits?.traces?.retention_days).toBe(7);
    expect(summary.limits?.traces?.ingestion_rate_limit_bytes).toBe(25000000);
    expect(summary.limits?.logs?.retention_days).toBe(14);
    expect(summary.limits?.logs?.ingestion_rate_mb).toBe(10);
  });

  describe('telemetry ingestion gating (assertIngestionAllowed) — confirmed 2026-07-03', () => {
    it('blocks ingestion outright when no license is installed (opposite default from assertFeatureAllowed)', async () => {
      const { licenseService } = await freshLicenseService();
      await expect(licenseService.assertIngestionAllowed()).rejects.toThrow(licenseService.LicenseFeatureError);
    });

    it('allows ingestion while the license is active', async () => {
      const { licenseService, licenseTokenVerifier } = await freshLicenseService();
      const signer = createTestLicenseSigner();
      licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
      await licenseService.installLicense(signer.sign(defaultTestPayload()));

      await expect(licenseService.assertIngestionAllowed()).resolves.toBeUndefined();
    });

    it('allows ingestion during the grace window', async () => {
      const { licenseService, licenseTokenVerifier } = await freshLicenseService();
      const signer = createTestLicenseSigner();
      licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
      const nowSec = Math.floor(Date.now() / 1000);
      await licenseService.installLicense(signer.sign(defaultTestPayload({ exp: nowSec - 10, grace_days: 1 })));

      await expect(licenseService.assertIngestionAllowed()).resolves.toBeUndefined();
    });

    it('blocks ingestion once the license (and its grace window) has expired', async () => {
      const { licenseService, licenseTokenVerifier } = await freshLicenseService();
      const signer = createTestLicenseSigner();
      licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
      const nowSec = Math.floor(Date.now() / 1000);
      await licenseService.installLicense(signer.sign(defaultTestPayload({ exp: nowSec - 1000, grace_days: 0 })));

      await expect(licenseService.assertIngestionAllowed()).rejects.toThrow(licenseService.LicenseFeatureError);
    });

    it('ingestion gating is independent of the features array — no features needed', async () => {
      const { licenseService, licenseTokenVerifier } = await freshLicenseService();
      const signer = createTestLicenseSigner();
      licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
      await licenseService.installLicense(signer.sign(defaultTestPayload({ features: [] })));

      await expect(licenseService.assertIngestionAllowed()).resolves.toBeUndefined();
    });
  });
});
