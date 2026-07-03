import { Request, Response } from 'express';
import bcrypt from 'bcryptjs';
import jwt from 'jsonwebtoken';
import { getDB } from '../config/database';
import { getJwtSecret } from '../config/secrets';
import { AuthRequest } from '../middleware/auth';
import crypto from 'crypto';
import fs from 'fs';
import tls from 'tls';

const JWT_SECRET = getJwtSecret();

// --- Auth Controller ---

export const setup = async (req: Request, res: Response) => {
    try {
        const db = getDB();
        const existing = await db.get('SELECT count(*) as count FROM users');
        if (existing && existing.count > 0) {
            return res.status(409).json({ error: 'Setup already complete' });
        }

        const { username, password } = req.body;
        if (!username || !password || password.length < 8) {
            return res.status(400).json({ error: 'Invalid username or password (min 8 chars)' });
        }

        const hash = await bcrypt.hash(password, 10);
        await db.run('INSERT INTO users (username, password_hash) VALUES (?, ?)', username, hash);

        res.json({ status: 'setup complete' });
    } catch (err: any) {
        res.status(500).json({ error: err.message });
    }
};

export const login = async (req: Request, res: Response) => {
    try {
        const db = getDB();
        const { username, password } = req.body;

        const user = await db.get('SELECT * FROM users WHERE username = ?', username);
        if (!user) {
            return res.status(401).json({ error: 'Invalid credentials' });
        }

        const valid = await bcrypt.compare(password, user.password_hash);
        if (!valid) {
            return res.status(401).json({ error: 'Invalid credentials' });
        }

        const token = jwt.sign({ id: user.id, username: user.username, role: 'admin' }, JWT_SECRET, { expiresIn: '24h' });

        res.json({
            token,
            user: { id: user.id, username: user.username, role: 'admin' }
        });
    } catch (err: any) {
        res.status(500).json({ error: err.message });
    }
};

export const validate = (req: AuthRequest, res: Response) => {
    // Middleware already validated security/token. Surface the resolved
    // tenancy scope as response headers so nginx's auth_request_set can
    // forward it downstream as X-Scope-OrgID (see
    // docs/architecture/multitenancy-licensing.md §7). Platform/admin
    // credentials resolve no scope and set nothing.
    const user = req.user;
    if (user?.orgId) res.setHeader('X-Scope-OrgID', user.orgId);
    if (user?.tenantId) res.setHeader('X-Tenant-Id', user.tenantId);
    if (user?.subtenantId) res.setHeader('X-Subtenant-Id', user.subtenantId);
    res.status(200).send('OK');
};

export const getConfig = async (req: Request, res: Response) => {
    const db = getDB();
    const rows = await db.all('SELECT key, value FROM settings');
    const settings = rows.reduce((acc, row) => ({ ...acc, [row.key]: row.value === 'true' }), {});

    const userCount = await db.get('SELECT count(*) as count FROM users');

    res.json({
        agent_api_key: settings['security.agent.api_key'] || false,
        external_api_key: settings['security.external.api_key'] || false,
        force_ssl: settings['security.force_ssl'] || false,
        setup_complete: userCount?.count > 0
    });
};

export const updateConfig = async (req: Request, res: Response) => {
    const { agent_api_key, external_api_key, force_ssl } = req.body;
    const db = getDB();

    // Safety Check: Require at least one active API Key before enforcing it
    if (String(agent_api_key) === 'true' || String(external_api_key) === 'true') {
        const keyCount = await db.get('SELECT count(*) as count FROM api_keys WHERE revoked_at IS NULL');
        if (!keyCount || keyCount.count === 0) {
            return res.status(400).json({
                error: 'You must create at least one active API key before enforcing API key authentication.'
            });
        }
    }

    const stmt = await db.prepare('INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)');
    if (agent_api_key !== undefined) await stmt.run('security.agent.api_key', String(agent_api_key));
    if (external_api_key !== undefined) await stmt.run('security.external.api_key', String(external_api_key));
    if (force_ssl !== undefined) await stmt.run('security.force_ssl', String(force_ssl));
    await stmt.finalize();

    res.json({ status: 'updated' });
};

// --- API Keys ---

export const getKeys = async (req: Request, res: Response) => {
    const db = getDB();
    const keys = await db.all(`
        SELECT k.id, k.name, k.prefix, k.role, k.created_at, k.revoked_at,
               k.tenant_id, k.subtenant_id, s.org_id, s.name as subtenant_name, t.name as tenant_name
        FROM api_keys k
        LEFT JOIN subtenants s ON s.id = k.subtenant_id
        LEFT JOIN tenants t ON t.id = k.tenant_id
        ORDER BY k.created_at DESC
    `);
    res.json(keys);
};

export const createKey = async (req: Request, res: Response) => {
    try {
        const { name, role } = req.body;
        let { tenant_id, subtenant_id } = req.body;
        const db = getDB();

        // A key may be scoped to a subtenant (the hard telemetry-isolation
        // unit — see docs/architecture/multitenancy-licensing.md §3), to a
        // bare tenant (grouping only, no org_id), or to neither (platform
        // key). Validate the scope actually exists and is internally
        // consistent before minting a credential for it.
        if (subtenant_id) {
            const sub = await db.get(
                'SELECT * FROM subtenants WHERE id = ? AND deleted_at IS NULL',
                subtenant_id
            );
            if (!sub) return res.status(400).json({ error: 'Unknown subtenant_id' });
            if (tenant_id && sub.tenant_id !== tenant_id) {
                return res.status(400).json({ error: 'subtenant_id does not belong to tenant_id' });
            }
            // Auto-derive tenant_id from the subtenant so the key's scope
            // stays internally consistent even if the caller only sent
            // subtenant_id (the Console UI does this).
            tenant_id = sub.tenant_id;
        } else if (tenant_id) {
            const tenant = await db.get('SELECT * FROM tenants WHERE id = ? AND deleted_at IS NULL', tenant_id);
            if (!tenant) return res.status(400).json({ error: 'Unknown tenant_id' });
        }

        // Generate a random key
        const rawKey = 'sk-' + crypto.randomBytes(16).toString('hex');
        const prefix = rawKey.substring(0, 7); // sk-XXXX
        const hash = await bcrypt.hash(rawKey, 10);

        const result = await db.run(
            'INSERT INTO api_keys (name, prefix, key_hash, role, tenant_id, subtenant_id) VALUES (?, ?, ?, ?, ?, ?)',
            name, prefix, hash, role, tenant_id || null, subtenant_id || null
        );

        res.json({
            api_key: {
                id: result.lastID,
                name,
                prefix,
                role,
                tenant_id: tenant_id || null,
                subtenant_id: subtenant_id || null,
                created_at: new Date()
            },
            raw_key: rawKey
        });
    } catch (err: any) {
        res.status(500).json({ error: err.message });
    }
};

export const revokeKey = async (req: Request, res: Response) => {
    const db = getDB();
    await db.run('UPDATE api_keys SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?', req.params.id);
    res.json({ status: 'revoked' });
};

// --- SSL Config ---

export const updateSSL = async (req: Request, res: Response) => {
    const { certificate, privateKey } = req.body;

    if (!certificate || !privateKey) {
        return res.status(400).json({ error: 'Both certificate and private key are required.' });
    }

    try {
        // Validate the key pair before saving to avoid breaking Nginx
        tls.createSecureContext({
            cert: certificate,
            key: privateKey
        });
    } catch (err: any) {
        return res.status(400).json({ error: 'Invalid Certificate or Private Key: ' + err.message });
    }

    try {
        // Write the valid certificates to the mounted volume
        fs.writeFileSync('/etc/nginx/certs/server.crt', certificate);
        fs.writeFileSync('/etc/nginx/certs/server.key', privateKey);

        res.json({ status: 'ssl updated' });
    } catch (err: any) {
        return res.status(500).json({ error: 'Failed to write certificates: ' + err.message });
    }
};
