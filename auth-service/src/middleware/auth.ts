import { Request, Response, NextFunction } from 'express';
import jwt from 'jsonwebtoken';
import { getDB } from '../config/database';
import bcrypt from 'bcryptjs';

const JWT_SECRET = process.env.JWT_SECRET || 'dev-secret-change-me';

export interface AuthRequest extends Request {
    user?: any;
}

export const authenticate = async (req: AuthRequest, res: Response, next: NextFunction) => {
    const db = getDB();
    const authContext = req.header('X-Auth-Context'); // 'agent' or 'external'

    // Fetch settings
    const settingsRows = await db.all('SELECT key, value FROM settings');
    const settings = settingsRows.reduce((acc, row) => ({ ...acc, [row.key]: row.value === 'true' }), {});

    const agentKey = settings['security.agent.api_key'];
    const externalKey = settings['security.external.api_key'];
    const forceSsl = settings['security.force_ssl'];

    // --- Global SSL Check (API/Agents) ---
    if (forceSsl) {
        const proto = req.header('X-Forwarded-Proto') || 'http';
        // Note: internal Nginx auth requests come from local, but X-Original-URI is passed.
        // The important header is X-Forwarded-Proto passed by Nginx.
        if (proto !== 'https') {
            return res.status(403).json({ error: 'SSL Required: Platform is configured to reject non-HTTPS traffic.' });
        }
    }

    // --- Helper: Check API Key ---
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
            // Try JWT
            if (token.includes('.')) {
                const decoded = jwt.verify(token, JWT_SECRET);
                req.user = decoded;
                return true;
            }
            // Try API Key
            const keys = await db.all('SELECT * FROM api_keys');
            for (const k of keys) {
                if (await bcrypt.compare(token, k.key_hash)) {
                    req.user = { id: k.id, role: k.role, type: 'api_key' };
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
