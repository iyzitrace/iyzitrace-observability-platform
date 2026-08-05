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

// The capability nginx sets on the Thanos/Tempo/Loki read-path locations
// (/query/v1/*). Distinguishing it from INGEST_CAPABILITY is what lets us
// enforce the Agent/Reader role split below: writing telemetry needs
// "agent", reading it back needs "reader".
const EXTERNAL_QUERY_CAPABILITY = 'external_query';

// An API key's `role` column holds a comma-separated list (e.g.
// "agent,reader" — a key can write and read). Admin/platform JWT sessions
// carry no role list and are never subject to this check (see checkApiKey).
const keyHasRole = (roleColumn: string | null | undefined, needed: 'agent' | 'reader'): boolean =>
    (roleColumn || '').split(',').map(r => r.trim()).includes(needed);

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

    // Populated by checkApiKey on a rejection so the two call sites below can
    // return a more useful message than a bare "Unauthorized".
    let authFailureReason: string | undefined;

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
            // JWT session (admin/platform credential — carries no tenant scope,
            // and is exempt from the Agent/Reader role check below: platform
            // operators aren't scoped API keys).
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
                if (!(await bcrypt.compare(token, k.key_hash))) continue;

                // Agent/Reader role gate — applies to every key regardless of
                // scope. "ingest" needs write (agent); "external_query" needs
                // read (reader). Other capabilities (inventory, alerting,
                // agent_management) aren't part of this write/read split.
                if (capability === INGEST_CAPABILITY && !keyHasRole(k.role, 'agent')) {
                    authFailureReason = 'This API key does not have the Agent (write) role required to ingest telemetry.';
                    return false;
                }
                if (capability === EXTERNAL_QUERY_CAPABILITY && !keyHasRole(k.role, 'reader')) {
                    authFailureReason = 'This API key does not have the Reader (query) role required to query data.';
                    return false;
                }

                // Multi-subtenant scoping (GitHub PAT-style: one key, many
                // subtenants — see api_key_subtenants / migration 4). A key
                // bound to a suspended (or deleted) tenant/subtenant must not
                // authenticate for it.
                const scoped = await db.all(
                    `SELECT s.id, s.org_id, s.tenant_id, s.status AS sub_status, t.status AS tenant_status
                     FROM api_key_subtenants aks
                     JOIN subtenants s ON s.id = aks.subtenant_id AND s.deleted_at IS NULL
                     JOIN tenants t ON t.id = s.tenant_id AND t.deleted_at IS NULL
                     WHERE aks.api_key_id = ?`,
                    k.id
                );

                if (scoped.length > 0) {
                    const requestedOrgId = req.header('X-Subtenant-Id');
                    let match: typeof scoped[number] | undefined;

                    if (requestedOrgId) {
                        match = scoped.find(s => s.org_id === requestedOrgId);
                        if (!match) {
                            authFailureReason = 'X-Subtenant-Id is not one of the subtenants this API key is scoped to.';
                            return false;
                        }
                    } else if (scoped.length === 1) {
                        // Unambiguous — a single-subtenant key doesn't need the
                        // caller to spell out which subtenant it is.
                        match = scoped[0];
                    } else {
                        authFailureReason = 'This API key is scoped to multiple subtenants — send X-Subtenant-Id to say which one this request is for.';
                        return false;
                    }

                    if (match.sub_status !== 'active' || match.tenant_status !== 'active') return false;

                    req.user = {
                        id: k.id,
                        role: k.role,
                        type: 'api_key',
                        tenantId: match.tenant_id,
                        subtenantId: match.id,
                        orgId: match.org_id
                    };
                    return true;
                }

                // Legacy tenant-wide key (tenant_id set, no specific
                // subtenant) or a platform key (neither set).
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
        } catch (e) { console.error('Auth check error', e); }
        return false;
    };

    // --- Logic: Agent Context ---
    if (authContext === 'agent') {
        // 1. If OFF -> Open Access
        if (!agentKey) return next();

        // 2. Check API Key
        if (await checkApiKey()) return next();

        return res.status(401).json({ error: authFailureReason || 'Unauthorized: API Key required' });
    }

    // --- Logic: External Context (or unknown/default) ---
    // Treat unknown as external for safety

    // 1. If OFF -> Open Access
    if (!externalKey) return next();

    // 2. Check API Key
    if (await checkApiKey()) return next();

    return res.status(401).json({ error: authFailureReason || 'Unauthorized: API Key required' });
};
