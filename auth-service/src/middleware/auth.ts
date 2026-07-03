import { Request, Response, NextFunction } from 'express';
import jwt from 'jsonwebtoken';
import bcrypt from 'bcryptjs';
import { getDB } from '../config/database';
import { getJwtSecret } from '../config/secrets';
import { LicenseFeatureError, assertFeatureAllowed, assertIngestionAllowed } from '../services/licenseService';

// Reserved X-License-Capability value nginx sets on the three OTLP ingest
// locations (traces/logs/metrics). Uses assertIngestionAllowed's no-license
// default (blocked) rather than assertFeatureAllowed's (open) — see
// docs/architecture/multitenancy-licensing.md §16.0.
const INGEST_CAPABILITY = 'ingest';

const JWT_SECRET = getJwtSecret();

export interface AuthenticatedUser {
    id?: number;
    username?: string;
    role?: string;
    type?: 'jwt' | 'api_key';
    tenantId?: string;
    subtenantId?: string;
    orgId?: string;
}

export interface AuthRequest extends Request {
    user?: AuthenticatedUser;
}

export const authenticate = async (req: AuthRequest, res: Response, next: NextFunction) => {
    const db = getDB();
    const authContext = req.header('X-Auth-Context'); // 'agent' | 'external' | 'platform'

    // Fetch settings
    const settingsRows = await db.all('SELECT key, value FROM settings');
    const settings = settingsRows.reduce((acc, row) => ({ ...acc, [row.key]: row.value === 'true' }), {});

    const agentKey = settings['security.agent.api_key'];
    const externalKey = settings['security.external.api_key'];
    const forceSsl = settings['security.force_ssl'];

    // --- License Gate ---
    // Runs unconditionally, ahead of the agent/external toggles below: a
    // capability nginx marks (X-License-Capability) must be enforced even
    // when API-key enforcement is off. Two different rules apply —
    // "ingest" (traces/logs/metrics ingestion) requires a genuinely usable
    // license and blocks outright with none installed; every other named
    // capability ("inventory", "agent_management", "external_query",
    // "alerting") is a no-op with no license installed and only becomes an
    // allow-list once one exists. See services/licenseService.ts and
    // docs/architecture/multitenancy-licensing.md §16.
    const capability = req.header('X-License-Capability');
    if (capability) {
        try {
            if (capability === INGEST_CAPABILITY) {
                await assertIngestionAllowed();
            } else {
                await assertFeatureAllowed(capability);
            }
        } catch (err) {
            if (err instanceof LicenseFeatureError) {
                const reason = capability === INGEST_CAPABILITY ? 'license_required' : 'license_feature';
                return res.status(403).json({ error: err.message, reason, feature: capability });
            }
            throw err;
        }
    }

    // --- Global SSL Check (API/Agents) ---
    if (forceSsl) {
        const proto = req.header('X-Forwarded-Proto') || 'http';
        // Note: internal Nginx auth requests come from local, but X-Original-URI is passed.
        // The important header is X-Forwarded-Proto passed by Nginx.
        if (proto !== 'https') {
            return res.status(403).json({ error: 'SSL Required: Platform is configured to reject non-HTTPS traffic.' });
        }
    }

    // --- Helper: Check API Key / JWT, resolve tenancy scope ---
    const checkApiKey = async (): Promise<boolean> => {
        let token: string | undefined;

        // 1. Try modern header (direct token)
        const customKey = req.header('iyzitrace-api-key');
        if (customKey) {
            token = customKey;
        } else {
            // 2. Try legacy header (Bearer prefix)
            const authHeader = req.header('Authorization');
            if (authHeader) {
                const [type, t] = authHeader.split(' ');
                if (type === 'Bearer') {
                    token = t;
                }
            }
        }

        if (!token) return false;

        try {
            // JWT session (admin/platform credential — carries no tenant scope)
            if (token.includes('.')) {
                const decoded = jwt.verify(token, JWT_SECRET) as AuthenticatedUser;
                req.user = { ...decoded, type: 'jwt' };
                return true;
            }

            // API key: look up by prefix first (avoids an O(n) bcrypt scan over
            // every key on every request), and exclude revoked keys — a revoked
            // key must never authenticate again.
            const prefix = token.substring(0, 7);
            const candidates = await db.all(
                'SELECT * FROM api_keys WHERE prefix = ? AND revoked_at IS NULL',
                prefix
            );

            for (const k of candidates) {
                if (await bcrypt.compare(token, k.key_hash)) {
                    // A key bound to a suspended (or deleted) tenant/subtenant
                    // must not authenticate — status alone was previously
                    // stored but never enforced here.
                    if (k.subtenant_id) {
                        const sub = await db.get(
                            "SELECT org_id FROM subtenants WHERE id = ? AND deleted_at IS NULL AND status = 'active'",
                            k.subtenant_id
                        );
                        if (!sub) return false;
                        if (k.tenant_id) {
                            const tenant = await db.get(
                                "SELECT id FROM tenants WHERE id = ? AND deleted_at IS NULL AND status = 'active'",
                                k.tenant_id
                            );
                            if (!tenant) return false;
                        }
                        req.user = {
                            id: k.id,
                            role: k.role,
                            type: 'api_key',
                            tenantId: k.tenant_id || undefined,
                            subtenantId: k.subtenant_id,
                            orgId: sub.org_id
                        };
                        return true;
                    }

                    if (k.tenant_id) {
                        const tenant = await db.get(
                            "SELECT id FROM tenants WHERE id = ? AND deleted_at IS NULL AND status = 'active'",
                            k.tenant_id
                        );
                        if (!tenant) return false;
                    }

                    req.user = {
                        id: k.id,
                        role: k.role,
                        type: 'api_key',
                        tenantId: k.tenant_id || undefined
                    };
                    return true;
                }
            }
        } catch (e) { console.error('Auth check error', e); }
        return false;
    };

    // --- Logic: Agent Context ---
    if (authContext === 'agent') {
        // 1. If OFF -> Open Access
        if (!agentKey) return next();

        // 2. Check API Key
        if (await checkApiKey()) return next();

        return res.status(401).json({ error: 'Unauthorized: API Key required' });
    }

    // --- Logic: External Context (or unknown/default) ---
    // Treat unknown as external for safety

    // 1. If OFF -> Open Access
    if (!externalKey) return next();

    // 2. Check API Key
    if (await checkApiKey()) return next();

    return res.status(401).json({ error: 'Unauthorized: API Key required' });
};
