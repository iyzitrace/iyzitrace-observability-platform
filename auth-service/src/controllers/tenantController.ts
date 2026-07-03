import { Request, Response } from 'express';
import * as tenantService from '../services/tenantService';
import { LicenseLimitError } from '../services/licenseService';
import { regenerateOverrides } from '../services/tenancyOverridesService';

const handleError = (err: unknown, res: Response): Response => {
  if (err instanceof tenantService.ValidationError) return res.status(400).json({ error: err.message });
  if (err instanceof tenantService.NotFoundError) return res.status(404).json({ error: err.message });
  if (err instanceof tenantService.ConflictError) return res.status(409).json({ error: err.message });
  if (err instanceof LicenseLimitError) {
    return res.status(409).json({ error: err.message, reason: 'license_limit' });
  }
  console.error('Tenant/subtenant operation failed', err);
  return res.status(500).json({ error: 'Internal error' });
};

export const listTenants = async (req: Request, res: Response) => {
  res.json(await tenantService.listTenants());
};

export const createTenant = async (req: Request, res: Response) => {
  try {
    const { name, slug } = req.body;
    const tenant = await tenantService.createTenant(name, slug);
    res.status(201).json(tenant);
  } catch (err) {
    handleError(err, res);
  }
};

export const suspendTenant = async (req: Request, res: Response) => {
  try {
    await tenantService.suspendTenant(req.params.id);
    await regenerateOverrides();
    res.json({ status: 'suspended' });
  } catch (err) {
    handleError(err, res);
  }
};

export const reactivateTenant = async (req: Request, res: Response) => {
  try {
    await tenantService.reactivateTenant(req.params.id);
    await regenerateOverrides();
    res.json({ status: 'active' });
  } catch (err) {
    handleError(err, res);
  }
};

export const deleteTenant = async (req: Request, res: Response) => {
  try {
    await tenantService.deleteTenant(req.params.id);
    await regenerateOverrides();
    res.json({ status: 'deleted' });
  } catch (err) {
    handleError(err, res);
  }
};

export const listAllSubtenants = async (req: Request, res: Response) => {
  res.json(await tenantService.listSubtenants());
};

export const listSubtenantsForTenant = async (req: Request, res: Response) => {
  res.json(await tenantService.listSubtenants(req.params.tenantId));
};

export const createSubtenant = async (req: Request, res: Response) => {
  try {
    const { name, slug } = req.body;
    const subtenant = await tenantService.createSubtenant(req.params.tenantId, name, slug);
    await regenerateOverrides();
    res.status(201).json(subtenant);
  } catch (err) {
    handleError(err, res);
  }
};

export const suspendSubtenant = async (req: Request, res: Response) => {
  try {
    await tenantService.suspendSubtenant(req.params.id);
    await regenerateOverrides();
    res.json({ status: 'suspended' });
  } catch (err) {
    handleError(err, res);
  }
};

export const reactivateSubtenant = async (req: Request, res: Response) => {
  try {
    await tenantService.reactivateSubtenant(req.params.id);
    await regenerateOverrides();
    res.json({ status: 'active' });
  } catch (err) {
    handleError(err, res);
  }
};

export const deleteSubtenant = async (req: Request, res: Response) => {
  try {
    await tenantService.deleteSubtenant(req.params.id);
    await regenerateOverrides();
    res.json({ status: 'deleted' });
  } catch (err) {
    handleError(err, res);
  }
};
