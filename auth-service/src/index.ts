import express from 'express';
import cors from 'cors';
import helmet from 'helmet';
import morgan from 'morgan';
import path from 'path';
import fs from 'fs';
import { initDB } from './config/database';
import authRoutes from './routes/auth';
import healthRoutes from './routes/health';
import licenseRoutes from './routes/license';
import tenantRoutes from './routes/tenants';
import platformLicenseRoutes from './routes/platformLicense';
import { checkRevocationHeartbeat } from './services/licenseService';
import { regenerateOverrides } from './services/tenancyOverridesService';
const app = express();
const PORT = process.env.PORT || 8080;
const uiPath = path.join(__dirname, '../ui/dist');
const openApiSpecCandidates = [
    path.join(__dirname, '../../docs/platform-api.openapi.yaml'),
    path.join(__dirname, '../platform-api.openapi.yaml'),
];

const resolveOpenApiSpecPath = (): string | null => {
    for (const candidate of openApiSpecCandidates) {
        if (fs.existsSync(candidate)) {
            return candidate;
        }
    }
    return null;
};

// Ensure data directory exists
const dataDir = path.join(__dirname, '../data');
if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir);
}

// Middleware
app.use(cors());
app.use(helmet({
    contentSecurityPolicy: false // Disable for simple UI usage
}));
app.use(morgan('combined'));
app.use(express.json());

// Global SSL Enforcement (Browser Redirect)
app.use(async (req, res, next) => {
    // Check if SSL is forced
    try {
        const db = await initDB(); // Ensure initialized
        const row = await db.get('SELECT value FROM settings WHERE key = ?', 'security.force_ssl');
        const forceSsl = row?.value === 'true';

        if (forceSsl) {
            // Skip redirect for API, Auth (Internal Nginx Subrequests), and OpAMP routes
            // Redirecting these causes Nginx (500 Error) or API failures.
            if (req.path.startsWith('/auth') || req.path.startsWith('/api') || req.path.startsWith('/opamp')) {
                // Do nothing, let the specific route or auth middleware handle it/reject it.
            } else {
                const proto = req.header('X-Forwarded-Proto') || 'http';
                if (proto !== 'https' && req.accepts('html')) {
                    // If it's a browser request (HTML), redirect
                    const host = req.header('Host');
                    return res.redirect(302, `https://${host}${req.originalUrl}`);
                }
            }
        }
    } catch (e) {
        console.error('SSL check failed', e);
    }
    next();
});

// Routes
app.use('/auth', authRoutes);
app.use('/auth', tenantRoutes);        // /auth/tenants, /auth/subtenants
app.use('/auth', platformLicenseRoutes); // /auth/license/install, /auth/license/status
app.use('/', healthRoutes);          // System health endpoints (e.g. /system/status)
app.use('/api/v1/license', licenseRoutes);  // direct access
app.use('/license', licenseRoutes);          // via nginx /api/v1/platform/ rewrite

app.get('/platform-api.openapi.yaml', (req, res) => {
    const specPath = resolveOpenApiSpecPath();
    if (!specPath) {
        return res.status(404).json({ error: 'OpenAPI spec not found' });
    }

    return res.type('application/yaml').sendFile(specPath);
});

app.get(['/swagger', '/swagger/'], (req, res) => {
    const html = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>IyziTrace Platform API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  <style>
    body { margin: 0; background: #0f172a; }
    .swagger-ui .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    const specUrl = window.location.pathname.startsWith('/console/')
      ? '/console/platform-api.openapi.yaml'
      : '/platform-api.openapi.yaml';

    window.ui = SwaggerUIBundle({
      url: specUrl,
      dom_id: '#swagger-ui',
      deepLinking: true,
      displayRequestDuration: true,
      docExpansion: 'list',
      filter: true,
      presets: [
        SwaggerUIBundle.presets.apis,
        SwaggerUIStandalonePreset,
      ],
      layout: 'BaseLayout',
    });
  </script>
</body>
</html>`;

    return res.type('html').send(html);
});

// Static UI
app.use(express.static(uiPath)); // Serve assets

// SPA Fallback
app.get('*', (req, res) => {
    if (req.path.startsWith('/api') || req.path.startsWith('/auth')) {
        return res.status(404).json({ error: 'Not Found' });
    }
    res.sendFile(path.join(uiPath, 'index.html'));
});

// Optional SaaS-ready license heartbeat (see services/licenseService.ts).
// Entirely inert unless LICENSE_HEARTBEAT_URL is configured — offline
// installs make zero network calls here.
const startLicenseHeartbeat = () => {
    const heartbeatUrl = process.env.LICENSE_HEARTBEAT_URL;
    if (!heartbeatUrl) return;

    const intervalMs = parseInt(process.env.LICENSE_HEARTBEAT_INTERVAL_MS || '', 10) || 6 * 60 * 60 * 1000;
    checkRevocationHeartbeat().catch(() => { });
    setInterval(() => {
        checkRevocationHeartbeat().catch(() => { });
    }, intervalMs);
};

// Start
const start = async () => {
    try {
        await initDB();
        await regenerateOverrides();
        startLicenseHeartbeat();
        app.listen(PORT, () => {
            console.log(`Auth Service running on port ${PORT}`);
        });
    } catch (err) {
        console.error('Failed to start server:', err);
        process.exit(1);
    }
};

start();
