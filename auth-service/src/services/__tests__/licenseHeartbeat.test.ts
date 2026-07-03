import { describe, it, expect, beforeEach, vi } from 'vitest';
import { createTestLicenseSigner, defaultTestPayload } from '../../test-utils/licenseTestHelper';

const freshLicenseServiceWithInstalledLicense = async () => {
  vi.resetModules();
  process.env.DB_PATH = ':memory:';
  process.env.JWT_SECRET = 'test-secret';

  const { initDB } = await import('../../config/database');
  await initDB();

  const licenseTokenVerifier = await import('../licenseTokenVerifier');
  const licenseService = await import('../licenseService');

  const signer = createTestLicenseSigner();
  licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
  await licenseService.installLicense(signer.sign(defaultTestPayload()));

  return { licenseService };
};

describe('license heartbeat (SaaS-ready, config-gated)', () => {
  beforeEach(() => {
    vi.resetModules();
    delete process.env.LICENSE_HEARTBEAT_URL;
    vi.unstubAllGlobals();
  });

  it('makes no network call when LICENSE_HEARTBEAT_URL is unset (offline-first default)', async () => {
    const { licenseService } = await freshLicenseServiceWithInstalledLicense();
    const fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);

    await licenseService.checkRevocationHeartbeat();

    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('marks the license expired when the heartbeat endpoint reports revoked:true', async () => {
    const { licenseService } = await freshLicenseServiceWithInstalledLicense();
    process.env.LICENSE_HEARTBEAT_URL = 'https://license.example.com/heartbeat';

    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ revoked: true })
      })
    );

    await licenseService.checkRevocationHeartbeat();

    const summary = await licenseService.getSummary();
    expect(summary.remoteRevoked).toBe(true);
    expect(summary.status).toBe('expired');
  });

  it('leaves the license untouched when the heartbeat request fails (soft cancellation)', async () => {
    const { licenseService } = await freshLicenseServiceWithInstalledLicense();
    process.env.LICENSE_HEARTBEAT_URL = 'https://license.example.com/heartbeat';

    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')));

    await licenseService.checkRevocationHeartbeat();

    const summary = await licenseService.getSummary();
    expect(summary.remoteRevoked).toBe(false);
    expect(summary.status).toBe('active');
  });
});
