import { describe, it, expect, beforeEach, vi } from 'vitest';
import { createTestLicenseSigner, defaultTestPayload } from '../../test-utils/licenseTestHelper';

const freshTenantService = async (limits?: { tenants: number; subtenants: number }) => {
  vi.resetModules();
  process.env.DB_PATH = ':memory:';
  process.env.JWT_SECRET = 'test-secret';

  const { initDB } = await import('../../config/database');
  await initDB();

  const licenseTokenVerifier = await import('../licenseTokenVerifier');
  const licenseService = await import('../licenseService');
  const tenantService = await import('../tenantService');

  const signer = createTestLicenseSigner();
  licenseTokenVerifier.setPublicKeysForTest(signer.publicKeyMap);
  const token = signer.sign(
    defaultTestPayload({
      limits: {
        tenants: { max: limits?.tenants ?? 2 },
        subtenants: { max_total: limits?.subtenants ?? 3 }
      }
    })
  );
  await licenseService.installLicense(token);

  return { tenantService, licenseService };
};

describe('tenantService', () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it('creates a tenant and derives the correct org_id for its subtenants', async () => {
    const { tenantService } = await freshTenantService();

    const tenant = await tenantService.createTenant('CCI Holding Turkey', 'cci-tr');
    expect(tenant.slug).toBe('cci-tr');

    const subtenant = await tenantService.createSubtenant(tenant.id, 'Production', 'prod');
    expect(subtenant.org_id).toBe('cci-tr.prod');
  });

  it('rejects a duplicate tenant slug', async () => {
    const { tenantService } = await freshTenantService();
    await tenantService.createTenant('Tenant A', 'dup');
    await expect(tenantService.createTenant('Tenant B', 'dup')).rejects.toThrow(tenantService.ConflictError);
  });

  it('rejects an invalid slug', async () => {
    const { tenantService } = await freshTenantService();
    await expect(tenantService.createTenant('Bad Slug', 'Not_Valid!')).rejects.toThrow(tenantService.ValidationError);
  });

  it('enforces the license tenant limit', async () => {
    const { tenantService } = await freshTenantService({ tenants: 1, subtenants: 3 });
    await tenantService.createTenant('First', 'first');
    await expect(tenantService.createTenant('Second', 'second')).rejects.toThrow(/Tenant limit reached/);
  });

  it('enforces the license subtenant limit across all tenants combined', async () => {
    const { tenantService } = await freshTenantService({ tenants: 2, subtenants: 1 });
    const tenant = await tenantService.createTenant('T1', 't1');
    await tenantService.createSubtenant(tenant.id, 'Sub 1', 'sub1');

    await expect(tenantService.createSubtenant(tenant.id, 'Sub 2', 'sub2')).rejects.toThrow(/Subtenant limit reached/);
  });

  it('deleting a tenant soft-deletes its subtenants and frees license capacity', async () => {
    const { tenantService } = await freshTenantService({ tenants: 1, subtenants: 1 });
    const tenant = await tenantService.createTenant('T1', 't1');
    await tenantService.createSubtenant(tenant.id, 'Sub 1', 'sub1');

    await tenantService.deleteTenant(tenant.id);

    const tenants = await tenantService.listTenants();
    expect(tenants).toHaveLength(0);
    const subtenants = await tenantService.listSubtenants();
    expect(subtenants).toHaveLength(0);

    // Capacity freed — creating a new tenant/subtenant should succeed again.
    const tenant2 = await tenantService.createTenant('T2', 't2');
    await expect(tenantService.createSubtenant(tenant2.id, 'Sub', 'sub')).resolves.toBeDefined();
  });

  it('resolveSubtenantByOrgId finds the subtenant by its X-Scope-OrgID value', async () => {
    const { tenantService } = await freshTenantService();
    const tenant = await tenantService.createTenant('T1', 't1');
    await tenantService.createSubtenant(tenant.id, 'Prod', 'prod');

    const resolved = await tenantService.resolveSubtenantByOrgId('t1.prod');
    expect(resolved?.name).toBe('Prod');

    const notFound = await tenantService.resolveSubtenantByOrgId('nope.nope');
    expect(notFound).toBeUndefined();
  });
});
