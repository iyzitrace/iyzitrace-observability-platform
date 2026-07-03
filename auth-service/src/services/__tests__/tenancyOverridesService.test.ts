import { describe, it, expect, beforeEach, vi } from 'vitest';
import fs from 'fs';
import os from 'os';
import path from 'path';
import { createTestLicenseSigner, defaultTestPayload } from '../../test-utils/licenseTestHelper';

const freshOverridesService = async (tenancyDir: string) => {
  vi.resetModules();
  process.env.DB_PATH = ':memory:';
  process.env.JWT_SECRET = 'test-secret';
  process.env.TEMPO_OVERRIDES_PATH = path.join(tenancyDir, 'tempo-overrides.yaml');
  process.env.LOKI_RUNTIME_CONFIG_PATH = path.join(tenancyDir, 'loki-runtime-config.yaml');

  const { initDB } = await import('../../config/database');
  await initDB();

  const licenseTokenVerifier = await import('../licenseTokenVerifier');
  const licenseService = await import('../licenseService');
  const tenantService = await import('../tenantService');
  const tenancyOverridesService = await import('../tenancyOverridesService');

  return { licenseTokenVerifier, licenseService, tenantService, tenancyOverridesService };
};

describe('tenancyOverridesService', () => {
  let tenancyDir: string;

  beforeEach(() => {
    vi.resetModules();
    tenancyDir = fs.mkdtempSync(path.join(os.tmpdir(), 'iyzi-tenancy-test-'));
  });

  it('writes empty overrides files when there are no subtenants', async () => {
    const { tenancyOverridesService } = await freshOverridesService(tenancyDir);
    await tenancyOverridesService.regenerateOverrides();

    const tempoContent = fs.readFileSync(path.join(tenancyDir, 'tempo-overrides.yaml'), 'utf-8');
    const lokiContent = fs.readFileSync(path.join(tenancyDir, 'loki-runtime-config.yaml'), 'utf-8');

    expect(tempoContent).toContain('overrides:');
    expect(tempoContent).toContain('{}');
    expect(lokiContent).toContain('overrides:');
    expect(lokiContent).toContain('{}');
  });

  it('writes a per-org_id entry for each active subtenant when the license specifies traces/logs limits', async () => {
    const { licenseTokenVerifier, licenseService, tenantService, tenancyOverridesService } =
      await freshOverridesService(tenancyDir);

    const signer = createTestLicenseSigner();
    licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
    await licenseService.installLicense(
      signer.sign(
        defaultTestPayload({
          limits: {
            tenants: { max: 2 },
            subtenants: { max_total: 3 },
            traces: { retention_days: 7, ingestion_rate_limit_bytes: 25000000 },
            logs: { retention_days: 14, ingestion_rate_mb: 10 }
          }
        })
      )
    );

    const tenant = await tenantService.createTenant('Acme Holding Turkey', 'acme-tr');
    await tenantService.createSubtenant(tenant.id, 'Production', 'prod');

    await tenancyOverridesService.regenerateOverrides();

    const tempoContent = fs.readFileSync(path.join(tenancyDir, 'tempo-overrides.yaml'), 'utf-8');
    const lokiContent = fs.readFileSync(path.join(tenancyDir, 'loki-runtime-config.yaml'), 'utf-8');

    expect(tempoContent).toContain('"acme-tr.prod"');
    expect(tempoContent).toContain('rate_limit_bytes: 25000000');
    expect(tempoContent).toContain('block_retention: 168h'); // 7 days * 24h

    expect(lokiContent).toContain('"acme-tr.prod"');
    expect(lokiContent).toContain('ingestion_rate_mb: 10');
    expect(lokiContent).toContain('retention_period: 336h'); // 14 days * 24h
  });

  it('omits a suspended subtenant from the generated overrides', async () => {
    const { licenseTokenVerifier, licenseService, tenantService, tenancyOverridesService } =
      await freshOverridesService(tenancyDir);

    const signer = createTestLicenseSigner();
    licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
    await licenseService.installLicense(
      signer.sign(defaultTestPayload({ limits: { tenants: { max: 2 }, subtenants: { max_total: 3 }, traces: { retention_days: 7 } } }))
    );

    const tenant = await tenantService.createTenant('T1', 't1');
    const subtenant = await tenantService.createSubtenant(tenant.id, 'Prod', 'prod');
    await tenantService.suspendSubtenant(subtenant.id);

    await tenancyOverridesService.regenerateOverrides();

    const tempoContent = fs.readFileSync(path.join(tenancyDir, 'tempo-overrides.yaml'), 'utf-8');
    expect(tempoContent).not.toContain('t1.prod');
  });

  it('does not throw when the target directory does not exist yet', async () => {
    const missingDir = path.join(tenancyDir, 'nested', 'does-not-exist-yet');
    const { tenancyOverridesService } = await freshOverridesService(missingDir);
    await expect(tenancyOverridesService.regenerateOverrides()).resolves.toBeUndefined();
    expect(fs.existsSync(path.join(missingDir, 'tempo-overrides.yaml'))).toBe(true);
  });
});
