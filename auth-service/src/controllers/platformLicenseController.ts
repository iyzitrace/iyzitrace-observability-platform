import { Request, Response } from 'express';
import * as licenseService from '../services/licenseService';
import { LicenseVerificationError } from '../services/licenseTokenVerifier';
import { regenerateOverrides } from '../services/tenancyOverridesService';

// --- Platform License (tenancy gating) ---
//
// Distinct from controllers/licenseController.ts, which proxies an external
// per-plugin license check against IyziTrace's GCP licensing endpoint. This
// controller owns the offline, Ed25519-signed platform license that gates
// how many tenants/subtenants this install may create — see
// docs/architecture/multitenancy-licensing.md.

export const installLicense = async (req: Request, res: Response) => {
  const { token } = req.body;
  if (!token || typeof token !== 'string') {
    return res.status(400).json({ error: 'token is required' });
  }

  try {
    const summary = await licenseService.installLicense(token);
    // New traces/logs limits may apply to existing subtenants — refresh the
    // Tempo/Loki override files immediately rather than waiting on the next
    // subtenant CRUD operation.
    await regenerateOverrides();
    res.json(summary);
  } catch (err: unknown) {
    if (err instanceof LicenseVerificationError) {
      return res.status(400).json({ error: err.message });
    }
    console.error('License install failed', err);
    res.status(500).json({ error: 'Failed to install license' });
  }
};

export const getLicenseSummary = async (req: Request, res: Response) => {
  const summary = await licenseService.getSummary();
  res.json(summary);
};
