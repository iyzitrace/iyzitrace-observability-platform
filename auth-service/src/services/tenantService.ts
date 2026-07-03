import crypto from 'crypto';
import { getDB } from '../config/database';
import { assertCanCreateSubtenant, assertCanCreateTenant } from './licenseService';

export class ValidationError extends Error {}
export class NotFoundError extends Error {}
export class ConflictError extends Error {}

// Lowercase alphanumeric + dashes, DNS-label-ish, matches the org_id scheme
// in docs/architecture/multitenancy-licensing.md §3.
const SLUG_RE = /^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/;

const assertSlug = (slug: unknown, field: string): void => {
  if (typeof slug !== 'string' || !SLUG_RE.test(slug)) {
    throw new ValidationError(
      `${field} must be lowercase alphanumeric with dashes, 1-40 chars, not starting/ending with a dash`
    );
  }
};

export interface Tenant {
  id: string;
  account_sub: string;
  name: string;
  slug: string;
  status: string;
  created_at: string;
  deleted_at: string | null;
}

export interface Subtenant {
  id: string;
  tenant_id: string;
  name: string;
  slug: string;
  org_id: string;
  status: string;
  created_at: string;
  deleted_at: string | null;
}

const ACCOUNT_SUB_PLACEHOLDER = 'local';

export const listTenants = async (): Promise<Tenant[]> => {
  const db = getDB();
  return db.all('SELECT * FROM tenants WHERE deleted_at IS NULL ORDER BY created_at DESC');
};

export const getTenant = async (id: string): Promise<Tenant | undefined> => {
  const db = getDB();
  return db.get('SELECT * FROM tenants WHERE id = ? AND deleted_at IS NULL', id);
};

export const createTenant = async (name: string, slug: string): Promise<Tenant> => {
  if (!name || typeof name !== 'string') throw new ValidationError('name is required');
  assertSlug(slug, 'slug');

  const db = getDB();
  const existing = await db.get('SELECT id FROM tenants WHERE slug = ? AND deleted_at IS NULL', slug);
  if (existing) throw new ConflictError(`Tenant slug '${slug}' already exists`);

  // License check happens after uniqueness validation so a doomed-anyway
  // duplicate-slug request doesn't consume a limit-check round trip, and
  // after obvious 400s so callers get the more specific error first.
  await assertCanCreateTenant();

  const id = crypto.randomUUID();
  await db.run(
    'INSERT INTO tenants (id, account_sub, name, slug) VALUES (?, ?, ?, ?)',
    id,
    ACCOUNT_SUB_PLACEHOLDER,
    name,
    slug
  );
  return (await getTenant(id))!;
};

export const suspendTenant = async (id: string): Promise<void> => {
  const db = getDB();
  const result = await db.run(
    `UPDATE tenants SET status = 'suspended' WHERE id = ? AND deleted_at IS NULL`,
    id
  );
  if (!result.changes) throw new NotFoundError('Tenant not found');
};

export const reactivateTenant = async (id: string): Promise<void> => {
  const db = getDB();
  const result = await db.run(
    `UPDATE tenants SET status = 'active' WHERE id = ? AND deleted_at IS NULL`,
    id
  );
  if (!result.changes) throw new NotFoundError('Tenant not found');
};

export const deleteTenant = async (id: string): Promise<void> => {
  const db = getDB();
  const result = await db.run('UPDATE tenants SET deleted_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL', id);
  if (!result.changes) throw new NotFoundError('Tenant not found');
  await db.run('UPDATE subtenants SET deleted_at = CURRENT_TIMESTAMP WHERE tenant_id = ? AND deleted_at IS NULL', id);
};

export const listSubtenants = async (tenantId?: string): Promise<Subtenant[]> => {
  const db = getDB();
  if (tenantId) {
    return db.all(
      'SELECT * FROM subtenants WHERE tenant_id = ? AND deleted_at IS NULL ORDER BY created_at DESC',
      tenantId
    );
  }
  return db.all('SELECT * FROM subtenants WHERE deleted_at IS NULL ORDER BY created_at DESC');
};

export const getSubtenant = async (id: string): Promise<Subtenant | undefined> => {
  const db = getDB();
  return db.get('SELECT * FROM subtenants WHERE id = ? AND deleted_at IS NULL', id);
};

export const resolveSubtenantByOrgId = async (orgId: string): Promise<Subtenant | undefined> => {
  const db = getDB();
  return db.get('SELECT * FROM subtenants WHERE org_id = ? AND deleted_at IS NULL', orgId);
};

export const createSubtenant = async (
  tenantId: string,
  name: string,
  slug: string
): Promise<Subtenant> => {
  if (!name || typeof name !== 'string') throw new ValidationError('name is required');
  assertSlug(slug, 'slug');

  const db = getDB();
  const tenant = await getTenant(tenantId);
  if (!tenant) throw new NotFoundError('Tenant not found');

  const existing = await db.get(
    'SELECT id FROM subtenants WHERE tenant_id = ? AND slug = ? AND deleted_at IS NULL',
    tenantId,
    slug
  );
  if (existing) throw new ConflictError(`Subtenant slug '${slug}' already exists in this tenant`);

  const orgId = `${tenant.slug}.${slug}`;
  const orgIdTaken = await db.get('SELECT id FROM subtenants WHERE org_id = ? AND deleted_at IS NULL', orgId);
  if (orgIdTaken) throw new ConflictError(`Org ID '${orgId}' already exists`);

  await assertCanCreateSubtenant();

  const id = crypto.randomUUID();
  await db.run(
    'INSERT INTO subtenants (id, tenant_id, name, slug, org_id) VALUES (?, ?, ?, ?, ?)',
    id,
    tenantId,
    name,
    slug,
    orgId
  );
  return (await getSubtenant(id))!;
};

export const suspendSubtenant = async (id: string): Promise<void> => {
  const db = getDB();
  const result = await db.run(
    `UPDATE subtenants SET status = 'suspended' WHERE id = ? AND deleted_at IS NULL`,
    id
  );
  if (!result.changes) throw new NotFoundError('Subtenant not found');
};

export const reactivateSubtenant = async (id: string): Promise<void> => {
  const db = getDB();
  const result = await db.run(
    `UPDATE subtenants SET status = 'active' WHERE id = ? AND deleted_at IS NULL`,
    id
  );
  if (!result.changes) throw new NotFoundError('Subtenant not found');
};

export const deleteSubtenant = async (id: string): Promise<void> => {
  const db = getDB();
  const result = await db.run('UPDATE subtenants SET deleted_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL', id);
  if (!result.changes) throw new NotFoundError('Subtenant not found');
};
