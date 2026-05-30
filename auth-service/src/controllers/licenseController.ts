import { Request, Response } from 'express';

const GCP_VALIDATE_URL = process.env.GCP_VALIDATE_URL ?? '';
const GCP_INTERNAL_SECRET = process.env.GCP_INTERNAL_SECRET ?? '';
const PLUGIN_LATEST_VERSION = process.env.PLUGIN_LATEST_VERSION ?? '1.0.4';
const CACHE_TTL_MS = 5 * 60 * 1000; // 5 dakika

interface CachedEntry {
  data: LicenseApiResponse;
  cachedAt: number;
}

interface LicenseApiResponse {
  status: string;
  plan: string;
  expire_remain_days: number;
  expires_at: string | null;
  latest_version: string;
}

interface GcpValidateResponse {
  status: string;
  plan: string;
  expires_at: string | null;
  customer_email: string;
  company_name: string;
}

// In-memory cache: licenseKey → cached response
const cache = new Map<string, CachedEntry>();

function computeExpireRemainDays(expiresAt: string | null): number {
  if (!expiresAt) return -1; // sonsuz
  const diff = new Date(expiresAt).getTime() - Date.now();
  return Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)));
}

export const getLicenseStatus = async (req: Request, res: Response): Promise<void> => {
  const licenseKey = (req.headers['x-license-key'] as string | undefined)?.trim();

  if (!licenseKey) {
    res.status(400).json({ error: 'X-License-Key header is required' });
    return;
  }

  // Cache kontrolü
  const cached = cache.get(licenseKey);
  if (cached && Date.now() - cached.cachedAt < CACHE_TTL_MS) {
    res.status(200).json(cached.data);
    return;
  }

  // GCP'ye git
  if (!GCP_VALIDATE_URL) {
    // GCP URL tanımlı değilse cache'i döndür, yoksa 503
    if (cached) {
      res.status(200).json(cached.data);
      return;
    }
    res.status(503).json({ error: 'License service not configured' });
    return;
  }

  let gcpResponse: GcpValidateResponse;
  try {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 8000);

    const response = await fetch(GCP_VALIDATE_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(GCP_INTERNAL_SECRET ? { 'X-Internal-Secret': GCP_INTERNAL_SECRET } : {}),
      },
      body: JSON.stringify({ licenseKey }),
      signal: controller.signal,
    });

    clearTimeout(timeoutId);

    if (response.status === 404) {
      res.status(404).json({ error: 'License not found' });
      return;
    }

    if (response.status === 403) {
      res.status(403).json({ error: 'License revoked' });
      return;
    }

    if (!response.ok) {
      throw new Error(`GCP returned ${response.status}`);
    }

    gcpResponse = await response.json() as GcpValidateResponse;
  } catch (err: any) {
    // GCP erişilemez — cache'i döndür
    if (cached) {
      res.status(200).json(cached.data);
      return;
    }
    res.status(503).json({ error: 'License service temporarily unavailable' });
    return;
  }

  const apiResponse: LicenseApiResponse = {
    status: gcpResponse.status,
    plan: gcpResponse.plan,
    expire_remain_days: computeExpireRemainDays(gcpResponse.expires_at),
    expires_at: gcpResponse.expires_at,
    latest_version: PLUGIN_LATEST_VERSION,
  };

  // Cache'e yaz
  cache.set(licenseKey, { data: apiResponse, cachedAt: Date.now() });

  res.status(200).json(apiResponse);
};
