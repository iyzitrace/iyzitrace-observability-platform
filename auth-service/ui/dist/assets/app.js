const API_BASE = '/api/v1/platform/auth';

const app = document.getElementById('app');

// State
let state = {
    token: localStorage.getItem('auth_token'),
    user: JSON.parse(localStorage.getItem('auth_user') || 'null'),
    config: { agent_api_key: false, external_api_key: false, setup_complete: false },
    keys: [],
    tenants: [],
    subtenants: [],
    license: null,
    systemStatus: null,
    autoRefresh: JSON.parse(localStorage.getItem('ui_auto_refresh') || 'false'),
    theme: localStorage.getItem('ui_theme') || 'dark'
};

// Theme Logic
// Theme Logic
const initTheme = () => {
    const html = document.documentElement;
    const themeCheckbox = document.getElementById('themeCheckbox');

    const applyTheme = (theme) => {
        html.setAttribute('data-theme', theme);
        state.theme = theme;
        localStorage.setItem('ui_theme', theme);

        // Sync Checkbox: Dark = Checked (Right), Light = Unchecked (Left)
        if (theme === 'dark') {
            if (themeCheckbox) themeCheckbox.checked = true;
        } else {
            if (themeCheckbox) themeCheckbox.checked = false;
        }
    };

    // Initial Apply
    if (state.theme === 'dark') {
        if (themeCheckbox) themeCheckbox.checked = true;
        applyTheme('dark');
    } else {
        if (themeCheckbox) themeCheckbox.checked = false;
        applyTheme('light');
    }

    // Event Listener
    if (themeCheckbox) {
        themeCheckbox.onchange = (e) => {
            const newTheme = e.target.checked ? 'dark' : 'light';
            applyTheme(newTheme);
        };
    }
};

// Initialize Theme immediately
document.addEventListener('DOMContentLoaded', initTheme);

// Utils
const fetchAPI = async (endpoint, method = 'GET', body = null, base = API_BASE) => {
    const headers = { 'Content-Type': 'application/json' };
    if (state.token) headers['Authorization'] = `Bearer ${state.token}`;

    const opts = { method, headers };
    if (body) opts.body = JSON.stringify(body);

    const res = await fetch(`${base}${endpoint}`, opts);
    if (res.status === 401) {
        logout();
        return null; // Handle logout
    }
    return res;
};

// ... (skip fetchAPI calls in setup/login, they use default base)

const logout = () => {
    stopAutoRefresh();
    state.token = null;
    state.user = null;
    localStorage.removeItem('auth_token');
    localStorage.removeItem('auth_user');
    init();
};

const showModal = (title, message, type = 'neutral') => {
    const modal = document.getElementById('modalContainer');
    // Ensure modal container exists (it might not be in DOM if renderDashboard hasn't run yet, 
    // but app.innerHTML usually wipes it. We need a persistent modal container or append it)

    // Check if modalContainer exists, if not, append it to app or body? 
    // Actually, renderSetup/Login/Dashboard all overwrite app.innerHTML. 
    // We should probably append modalContainer to document.body if it doesn't exist, 
    // OR ensure every render function includes it. 
    // Current code: renderDashboard includes <div id="modalContainer"></div>. 
    // renderSetup/Login DO NOT. This is another bug: Setup/Login can't show modals!

    // FIX: Let's create a getOrCreateModalContainer helper or just append it to body if missing.
    let container = document.getElementById('modalContainer');
    if (!container) {
        container = document.createElement('div');
        container.id = 'modalContainer';
        document.body.appendChild(container);
    }

    const color = type === 'error' ? 'var(--error-color)' : type === 'success' ? 'var(--success-color)' : 'var(--text-primary)';

    container.innerHTML = `
        <div class="modal-overlay" onclick="closeModal(event)">
            <div class="card" onclick="event.stopPropagation()" style="max-width:500px">
                <h2 style="color:${color}; margin-top:0">${title}</h2>
                <p style="line-height:1.6; color:var(--text-primary)">${message}</p>
                <div style="margin-top:20px; text-align:right">
                    <button onclick="closeModal()" style="justify-content:center; display:inline-flex; align-items:center; gap:8px">
                        <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
                        I Understand
                    </button>
                </div>
            </div>
        </div>
    `;
    document.body.style.overflow = 'hidden';
};

const showConfirmModal = (title, message, onConfirm) => {
    let container = document.getElementById('modalContainer');
    if (!container) {
        container = document.createElement('div');
        container.id = 'modalContainer';
        document.body.appendChild(container);
    }

    // Generate a unique ID for the confirm button to attach listener
    const confirmBtnId = 'confirmBtn_' + Date.now();

    container.innerHTML = `
        <div class="modal-overlay" onclick="closeModal(event)">
            <div class="card" onclick="event.stopPropagation()" style="max-width:500px">
                <h2 style="color:var(--text-primary); margin-top:0">${title}</h2>
                <p style="line-height:1.6; color:var(--text-secondary)">${message}</p>
                <div style="margin-top:24px; display:flex; justify-content:flex-end; gap:12px">
                    <button class="secondary" onclick="closeModal()" style="width:auto">Cancel</button>
                    <button class="danger" id="${confirmBtnId}" style="width:auto">Confirm</button>
                </div>
            </div>
        </div>
    `;
    document.body.style.overflow = 'hidden';

    document.getElementById(confirmBtnId).onclick = () => {
        onConfirm();
        closeModal();
    };
};

// Components
const renderSetup = () => {
    app.innerHTML = `
        <div class="card login-card">
            <div class="login-brand">
                <div class="brand-icon">
                   <svg width="32" height="32" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
                </div>
            </div>
            <h2 style="margin-bottom:8px">Setup Account</h2>
            <p style="text-align:center; color:var(--text-secondary); margin-bottom:32px; font-size:14px">Create your admin account to secure the platform.</p>
            
            <div class="alert alert-error" id="error" style="display:none"></div>
            
            <form id="setupForm">
                <div class="form-group">
                    <label>Username</label>
                    <div class="input-wrapper">
                        <svg class="input-icon" width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/></svg>
                        <input type="text" name="username" class="input-with-icon" required autocomplete="off" placeholder="Enter your username">
                    </div>
                </div>
                <div class="form-group">
                    <label>Password</label>
                    <div class="input-wrapper">
                        <svg class="input-icon" width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
                        <input type="password" name="password" class="input-with-icon" required minlength="8" placeholder="Enter your password">
                    </div>
                </div>
                <button type="submit" class="btn-login" style="display:flex; justify-content:center; align-items:center; gap:8px; margin-top:24px">
                    <svg width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
                    Complete Setup
                </button>
            </form>
            <div class="login-footer">
                Secured by Observability Platform
            </div>
        </div>
    `;

    document.getElementById('setupForm').onsubmit = async (e) => {
        e.preventDefault();
        const fd = new FormData(e.target);
        const data = Object.fromEntries(fd);

        const res = await fetchAPI('/setup', 'POST', data);
        if (res.ok) {
            showModal('Success', 'Setup complete! Please login.', 'success');
            init();
        } else {
            const err = await res.json();
            showError(err.error || 'Setup failed');
        }
    };
};

const renderLogin = () => {
    app.innerHTML = `
        <div class="card login-card">
            <div class="login-brand">
                 <div class="brand-icon">
                    <svg width="32" height="32" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>
                 </div>
            </div>
            <h2 style="margin-bottom:8px">Welcome Back</h2>
            <p style="text-align:center; color:var(--text-secondary); margin-bottom:32px; font-size:14px">Sign in to access the Observability Platform.</p>

            <div class="alert alert-error" id="error" style="display:none"></div>
            
            <form id="loginForm">
                <div class="form-group">
                    <label>Username</label>
                    <div class="input-wrapper">
                        <svg class="input-icon" width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/></svg>
                        <input type="text" name="username" class="input-with-icon" required placeholder="Enter your username">
                    </div>
                </div>
                <div class="form-group">
                    <label>Password</label>
                    <div class="input-wrapper">
                        <svg class="input-icon" width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
                        <input type="password" name="password" class="input-with-icon" required placeholder="Enter your password">
                    </div>
                </div>
                <button type="submit" class="btn-login" style="display:flex; justify-content:center; align-items:center; gap:8px; margin-top:24px">
                    <svg width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 16l-4-4m0 0l4-4m-4 4h14m-5 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h7a3 3 0 013 3v1"/></svg>
                    Sign In
                </button>
            </form>
            <div class="login-footer">
                Secured by Observability Platform
            </div>
        </div>
    `;

    document.getElementById('loginForm').onsubmit = async (e) => {
        e.preventDefault();
        const fd = new FormData(e.target);
        const data = Object.fromEntries(fd);

        // Direct fetch to handle 401 differently
        const res = await fetch(`${API_BASE}/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data)
        });

        if (res.ok) {
            const body = await res.json();
            state.token = body.token;
            state.user = body.user;
            localStorage.setItem('auth_token', state.token);
            localStorage.setItem('auth_user', JSON.stringify(state.user));
            init();
        } else {
            showError('Invalid credentials');
        }
    };
};

const LICENSE_STATUS_LABEL = {
    none: 'No License Installed',
    active: 'Active',
    grace: 'Grace Period',
    expired: 'Expired',
    restricted: 'Restricted (clock check failed)'
};

const licenseStatusClass = (status) => {
    if (status === 'active') return 'status-online';
    if (status === 'grace') return 'status-online';
    return 'status-offline';
};

const LICENSE_FEATURE_LABEL = {
    inventory: 'Inventory / Topology Discovery',
    agent_management: 'Agent Management (Lawrence remote config)',
    external_query: 'External Query API (Tempo/Loki/Thanos)',
    alerting: 'Alerting (Alertmanager)'
};

// Telemetry ingestion (traces/logs/metrics) is NOT one of the optional
// features above — it requires a genuinely usable license (active/grace)
// unconditionally, with no "unlisted = open" fallback. See
// docs/architecture/multitenancy-licensing.md §16.0.
const INGESTION_ALLOWED_STATUSES = ['active', 'grace'];

const renderLicenseSection = () => {
    const lic = state.license;
    const status = lic?.status || 'none';
    const tenantsMax = lic?.limits?.tenants?.max ?? '—';
    const subtenantsMax = lic?.limits?.subtenants?.max_total ?? '—';
    const tenantsUsed = lic?.tenantsUsed ?? 0;
    const subtenantsUsed = lic?.subtenantsUsed ?? 0;
    const tracesRetention = lic?.limits?.traces?.retention_days;
    const logsRetention = lic?.limits?.logs?.retention_days;
    const features = lic?.features ?? [];
    const ingestionAllowed = INGESTION_ALLOWED_STATUSES.includes(status);

    return `
        <div class="section">
            <div class="section-header" style="display:flex; justify-content:space-between; align-items:center">
                <h3>
                    <svg width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>
                    License
                </h3>
                <button class="btn-create" style="width:auto; display:flex; align-items:center; gap:8px" onclick="showInstallLicenseModal()">
                    <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/></svg>
                    Install License
                </button>
            </div>
            <div style="display:flex; flex-wrap:wrap; gap:24px; align-items:center; margin-bottom:16px">
                <span class="status-badge ${licenseStatusClass(status)}">${LICENSE_STATUS_LABEL[status] || status}</span>
                <span class="status-badge ${ingestionAllowed ? 'status-online' : 'status-offline'}">
                    ${ingestionAllowed ? '&check;' : '&times;'} Telemetry Ingestion (traces/logs/metrics)
                </span>
                ${lic?.customer ? `<span style="color:var(--text-secondary)">${lic.customer} &middot; ${lic.edition || ''}</span>` : ''}
                ${lic?.exp ? `<span style="color:var(--text-secondary)">Expires ${new Date(lic.exp * 1000).toLocaleDateString()}</span>` : ''}
                ${lic?.remoteRevoked ? `<span style="color:var(--error-color)">Revoked remotely via license heartbeat</span>` : ''}
            </div>
            ${!ingestionAllowed ? `<p style="color:var(--error-color); font-size:13px">No agent or API client can send traces, logs, or metrics right now — telemetry ingestion always requires an active or grace-period license.</p>` : ''}
            ${lic ? `
            <table>
                <thead><tr><th>Resource</th><th>Used</th><th>Limit</th></tr></thead>
                <tbody>
                    <tr><td>Tenants</td><td>${tenantsUsed}</td><td>${tenantsMax}</td></tr>
                    <tr><td>Subtenants</td><td>${subtenantsUsed}</td><td>${subtenantsMax}</td></tr>
                    ${tracesRetention ? `<tr><td>Trace retention</td><td colspan="2">${tracesRetention} days</td></tr>` : ''}
                    ${logsRetention ? `<tr><td>Log retention</td><td colspan="2">${logsRetention} days</td></tr>` : ''}
                </tbody>
            </table>
            <div style="margin-top:16px">
                <strong style="font-size:13px; color:var(--text-secondary)">Licensed Optional Modules</strong>
                <div style="display:flex; flex-wrap:wrap; gap:8px; margin-top:8px">
                    ${Object.keys(LICENSE_FEATURE_LABEL).map(key => `
                        <span class="status-badge ${features.includes(key) ? 'status-online' : 'status-offline'}" title="${LICENSE_FEATURE_LABEL[key]}">
                            ${features.includes(key) ? '&check;' : '&times;'} ${LICENSE_FEATURE_LABEL[key]}
                        </span>
                    `).join('')}
                </div>
            </div>
            ` : '<p style="color:var(--text-secondary)">No license installed — telemetry ingestion is blocked and no tenants/subtenants can be created until one is uploaded. The optional modules above (Inventory, Agent Management, External Query, Alerting) remain open until a license restricts them.</p>'}
        </div>
    `;
};

const renderTenantsSection = () => {
    const subtenantsByTenant = (tenantId) => state.subtenants.filter(s => s.tenant_id === tenantId);

    return `
        <div class="section">
            <div class="section-header" style="display:flex; justify-content:space-between; align-items:center">
                <h3>
                    <svg width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 6v-3a1 1 0 011-1h2a1 1 0 011 1v3"/></svg>
                    Tenants &amp; Subtenants
                </h3>
                <button class="btn-create" style="width:auto; display:flex; align-items:center; gap:8px" onclick="showCreateTenantModal()">
                    <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
                    New Tenant
                </button>
            </div>
            ${state.tenants.length > 0 ? state.tenants.map(t => `
                <div style="border:1px solid var(--border-color); border-radius:8px; padding:12px 16px; margin-bottom:12px">
                    <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px">
                        <div>
                            <strong>${t.name}</strong>
                            <code style="margin-left:8px">${t.slug}</code>
                            <span class="status-badge ${t.status === 'active' ? 'status-online' : 'status-offline'}" style="margin-left:8px">${t.status}</span>
                        </div>
                        <div style="display:flex; gap:8px">
                            <button class="secondary" style="width:auto; padding:4px 8px; font-size:12px" onclick="showCreateSubtenantModal('${t.id}')">+ Subtenant</button>
                            ${t.status === 'active'
                                ? `<button class="secondary" style="width:auto; padding:4px 8px; font-size:12px" onclick="suspendTenant('${t.id}')">Suspend</button>`
                                : `<button class="secondary" style="width:auto; padding:4px 8px; font-size:12px" onclick="reactivateTenant('${t.id}')">Reactivate</button>`}
                            <button class="danger" style="width:auto; padding:4px 8px; font-size:12px" onclick="deleteTenant('${t.id}')">Delete</button>
                        </div>
                    </div>
                    ${subtenantsByTenant(t.id).length > 0 ? `
                    <table style="margin-top:12px">
                        <thead><tr><th>Subtenant</th><th>Org ID (X-Scope-OrgID)</th><th>Status</th><th>Action</th></tr></thead>
                        <tbody>
                            ${subtenantsByTenant(t.id).map(s => `
                                <tr>
                                    <td>${s.name} <code>${s.slug}</code></td>
                                    <td><code>${s.org_id}</code></td>
                                    <td><span class="status-badge ${s.status === 'active' ? 'status-online' : 'status-offline'}">${s.status}</span></td>
                                    <td>
                                        ${s.status === 'active'
                                            ? `<button class="secondary" style="width:auto; padding:4px 8px; font-size:12px" onclick="suspendSubtenant('${s.id}')">Suspend</button>`
                                            : `<button class="secondary" style="width:auto; padding:4px 8px; font-size:12px" onclick="reactivateSubtenant('${s.id}')">Reactivate</button>`}
                                        <button class="danger" style="width:auto; padding:4px 8px; font-size:12px" onclick="deleteSubtenant('${s.id}')">Delete</button>
                                    </td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                    ` : '<p style="color:var(--text-secondary); margin:8px 0 0">No subtenants yet.</p>'}
                </div>
            `).join('') : '<p style="color:var(--text-secondary)">No tenants created yet.</p>'}
        </div>
    `;
};

const renderDashboard = async () => {
    // Fetch data
    const [configRes, keysRes, statusRes, licenseRes, tenantsRes, subtenantsRes] = await Promise.all([
        fetchAPI('/config'),
        fetchAPI('/keys'),
        fetchAPI('/system/status', 'GET', null, '/api/v1/platform'), // Maps to /system/status
        fetchAPI('/license/status'),
        fetchAPI('/tenants'),
        fetchAPI('/subtenants')
    ]);

    if (!configRes || !keysRes || !statusRes || !configRes.ok || !keysRes.ok || !statusRes.ok) return;

    state.config = await configRes.json();
    state.keys = await keysRes.json() || [];
    state.systemStatus = await statusRes.json();
    state.license = (licenseRes && licenseRes.ok) ? await licenseRes.json() : null;
    state.tenants = (tenantsRes && tenantsRes.ok) ? (await tenantsRes.json() || []) : [];
    state.subtenants = (subtenantsRes && subtenantsRes.ok) ? (await subtenantsRes.json() || []) : [];
    const systemStatus = state.systemStatus;

    app.innerHTML = `
        <div class="dashboard-container">
            <div class="header">
                <div>
                    <div style="display:flex; align-items:center; gap:12px; margin-bottom:8px">
                         <div style="width:32px; height:32px; background:linear-gradient(135deg, var(--accent-color) 0%, #4f46e5 100%); border-radius:8px; display:flex; align-items:center; justify-content:center; box-shadow:0 4px 12px rgba(79, 70, 229, 0.3); color:white">
                            <svg width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
                         </div>
                        <h1 style="margin:0">Observability Platform</h1>
                    </div>
                    <p style="margin:0; color:var(--text-secondary)">Welcome back, ${state.user.username}</p>
                </div>
                <button class="secondary" style="width:auto; display:flex; align-items:center; gap:8px" onclick="logout()">
                    <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"/></svg>
                    Logout
                </button>
            </div>

            <!-- System Status -->
             <div class="section">
                <div class="section-header" style="display:flex; justify-content:space-between; align-items:center">
                    <h3>
                        <svg width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                        System Health
                    </h3>
                    <div style="display:flex; align-items:center; gap:16px">
                        <div style="display:flex; align-items:center; gap:8px">
                            <span style="font-size:13px; color:var(--text-secondary); font-weight:500">Auto-Refresh</span>
                            <label class="switch" style="transform:scale(0.8)">
                                <input type="checkbox" id="refreshToggle" ${state.autoRefresh ? 'checked' : ''}>
                                <span class="slider"></span>
                            </label>
                        </div>
                        <span id="statusTime" style="font-size:12px; color:var(--text-secondary); border-left:1px solid var(--border-color); padding-left:16px">Last updated: ${new Date(systemStatus.updated_at).toLocaleTimeString()}</span>
                    </div>
                </div>
                <div class="status-grid" id="statusGrid">
                    ${renderStatusGrid(systemStatus.components)}
                </div>
            </div>

            <!-- Modern Bento Grid System Endpoints -->
            <div class="section" style="max-width: 100%; box-sizing: border-box;">
                <div class="section-header">
                    <h3>
                        <svg width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"/></svg>
                        System Infrastructure Endpoints
                    </h3>
                </div>

                <div style="display:grid; grid-template-columns: repeat(12, 1fr); gap:24px; margin-top:16px; width:100%; box-sizing:border-box;">
                    
                    <!-- HERO CARD: Ingestion -->
                    <div class="card" style="box-sizing: border-box; max-width:100%; grid-column: span 12; padding:32px; background: linear-gradient(135deg, var(--card-bg) 0%, rgba(52, 152, 219, 0.05) 100%); border-radius:16px; position:relative; box-shadow: 0 10px 30px rgba(0,0,0,0.1); overflow:hidden;">
                        <!-- Glassmorphic Decorative Background -->
                        <div style="position:absolute; top:-100px; right:-100px; width:300px; height:300px; background:var(--accent-glow); filter:blur(80px); border-radius:50%; opacity:0.25; pointer-events:none;"></div>
                        <div style="position:absolute; bottom:-50px; left:-50px; width:200px; height:200px; background:rgba(139, 92, 246, 0.15); filter:blur(60px); border-radius:50%; pointer-events:none;"></div>

                        <div style="position:relative; z-index:1; display:flex; flex-wrap:wrap; gap:40px; justify-content:space-between; align-items:center;">
                            <div style="flex:1; min-width:300px;">
                                <div style="display:flex; align-items:center; gap:12px; margin-bottom:16px;">
                                    <div style="display:flex; padding:10px; background:var(--accent-glow); color:var(--accent-color); border-radius:12px;">
                                        <svg width="26" height="26" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
                                    </div>
                                    <span style="font-size:14px; font-weight:800; color:var(--accent-color); text-transform:uppercase; letter-spacing:2px;">Data Ingestion</span>
                                </div>
                                <div style="display:flex; align-items:center; gap:12px; margin-bottom:16px;">
                                    <h2 style="font-size:36px; margin:0; letter-spacing:-1.5px; color:var(--text-primary); font-weight:800;">OpenTelemetry OTLP</h2>
                                    <span class="status-badge" style="background:rgba(52, 152, 219, 0.15); color:#3498db; border:none; font-size:10px; font-weight:700; padding:4px 10px;">AGENT</span>
                                </div>
                                <p style="font-size:16px; color:var(--text-secondary); line-height:1.6; max-width:550px; margin-bottom:28px;">
                                    The central gateway for all telemetry signals. Configure your collectors to point at this base endpoint for seamless data routing.
                                </p>
                                
                                <div style="display:flex; align-items:center; gap:16px; background:rgba(0,0,0,0.25); padding:16px; border-radius:14px; border:1px solid rgba(255,255,255,0.05); box-shadow: inset 0 2px 10px rgba(0,0,0,0.2); overflow:hidden;">
                                    <code id="baseOtlpCode" style="font-size:16px; color:var(--text-primary); font-weight:400; font-family:'JetBrains Mono', monospace; flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${window.location.origin}/ingest/otlp</code>
                                    <button class="btn-create" style="display:flex; align-items:center; justify-content:center; gap:8px; padding:12px 24px; border-radius:10px; font-weight:600; flex-shrink:0; width:auto;" onclick="copyText('${window.location.origin}/ingest/otlp', this)">
                                        <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg> 
                                        <span class="btn-text">Copy</span>
                                    </button>
                                </div>
                            </div>

                            <div style="display:grid; grid-template-columns: 1fr; gap:16px; min-width:300px;">
                                <div style="background:rgba(15, 23, 42, 0.4); backdrop-filter: blur(8px); border:1px solid rgba(255,255,255,0.05); padding:20px; border-radius:14px; display:flex; flex-direction:column; gap:12px; position:relative; overflow:hidden;">
                                    <div style="position:absolute; top:0; left:0; width:4px; height:100%; background:#06b6d4;"></div>
                                    <div style="display:flex; gap:8px;">
                                        <div style="color:#06b6d4;"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6"/></svg></div>
                                        <span style="font-size:13px; font-weight:700; color:var(--text-primary); text-transform:uppercase;">Traces</span>
                                    </div>
                                    <code style="font-size:11px; color:var(--text-primary); background:rgba(0,0,0,0.2); padding:8px; border-radius:8px; border:1px solid rgba(255,255,255,0.03);">/v1/traces</code>
                                    <button class="secondary" style="display:flex; align-items:center; justify-content:center; gap:8px; width:100%; font-size:11px; font-weight:600; padding:8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyText('${window.location.origin}/ingest/otlp/v1/traces/', this)">
                                        <svg width="12" height="12" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                        <span class="btn-text">Copy</span>
                                    </button>
                                </div>

                                <!-- Thanos -->
                                <div style="background:rgba(15, 23, 42, 0.4); backdrop-filter: blur(8px); border:1px solid rgba(255,255,255,0.05); padding:20px; border-radius:14px; display:flex; flex-direction:column; gap:12px; position:relative; overflow:hidden;">
                                    <div style="position:absolute; top:0; left:0; width:4px; height:100%; background:#e11d48;"></div>
                                    <div style="display:flex; gap:8px;">
                                        <div style="color:#e11d48;"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/></svg></div>
                                        <span style="font-size:13px; font-weight:700; color:var(--text-primary); text-transform:uppercase;">Metrics</span>
                                    </div>
                                    <code style="font-size:11px; color:var(--text-primary); background:rgba(0,0,0,0.2); padding:8px; border-radius:8px; border:1px solid rgba(255,255,255,0.03);">/v1/metrics</code>
                                    <button class="secondary" style="display:flex; align-items:center; justify-content:center; gap:8px; width:100%; font-size:11px; font-weight:600; padding:8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyText('${window.location.origin}/ingest/otlp/v1/metrics/', this)">
                                        <svg width="12" height="12" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                        <span class="btn-text">Copy</span>
                                    </button>
                                </div>

                                <!-- Loki -->
                                <div style="background:rgba(15, 23, 42, 0.4); backdrop-filter: blur(8px); border:1px solid rgba(255,255,255,0.05); padding:20px; border-radius:14px; display:flex; flex-direction:column; gap:12px; position:relative; overflow:hidden;">
                                    <div style="position:absolute; top:0; left:0; width:4px; height:100%; background:#f97316;"></div>
                                    <div style="display:flex; gap:8px;">
                                        <div style="color:#f97316;"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16"/></svg></div>
                                        <span style="font-size:13px; font-weight:700; color:var(--text-primary); text-transform:uppercase;">Logs</span>
                                    </div>
                                    <code style="font-size:11px; color:var(--text-primary); background:rgba(0,0,0,0.2); padding:8px; border-radius:8px; border:1px solid rgba(255,255,255,0.03);">/v1/logs</code>
                                    <button class="secondary" style="display:flex; align-items:center; justify-content:center; gap:8px; width:100%; font-size:11px; font-weight:600; padding:8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyText('${window.location.origin}/ingest/otlp/v1/logs/', this)">
                                        <svg width="12" height="12" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                        <span class="btn-text">Copy</span>
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- HERO CARD: Query Endpoints -->
                    <div class="card" style="box-sizing: border-box; max-width:100%; grid-column: span 12; padding:32px; background: linear-gradient(135deg, var(--card-bg) 0%, rgba(52, 152, 219, 0.05) 100%); border-radius:16px; position:relative; box-shadow: 0 10px 30px rgba(0,0,0,0.1); overflow:hidden;">
                        <!-- Glassmorphic Decorative Background -->
                        <div style="position:absolute; top:-100px; right:-100px; width:300px; height:300px; background:rgba(52, 152, 219, 0.1); filter:blur(80px); border-radius:50%; opacity:0.3; pointer-events:none;"></div>
                        
                        <div style="position:relative; z-index:1; display:flex; flex-wrap:wrap; gap:40px; justify-content:space-between; align-items:center;">
                            <div style="flex:1; min-width:300px;">
                                <div style="display:flex; align-items:center; gap:12px; margin-bottom:16px;">
                                    <div style="display:flex; padding:10px; background:rgba(52, 152, 219, 0.1); color:#3498db; border-radius:12px;">
                                        <svg width="26" height="26" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>
                                    </div>
                                    <span style="font-size:14px; font-weight:800; color:#3498db; text-transform:uppercase; letter-spacing:2px;">Data Retrieval</span>
                                </div>
                                <div style="display:flex; align-items:center; gap:12px; margin-bottom:16px;">
                                    <h2 style="font-size:36px; margin:0; letter-spacing:-1.5px; color:var(--text-primary); font-weight:800;">Query & Analytical APIs</h2>
                                    <span class="status-badge" style="background:rgba(52, 152, 219, 0.15); color:#3498db; border:none; font-size:10px; font-weight:700; padding:4px 10px;">EXTERNAL</span>
                                </div>
                                <p style="font-size:16px; color:var(--text-secondary); line-height:1.6; max-width:550px; margin-bottom:28px;">
                                    High-performance interfaces for querying traces, metrics, and logs. Powered by Tempo, Thanos, and Loki for deep analytical insights into your infrastructure.
                                </p>
                            </div>

                                <div style="display:grid; gap:16px; flex:0; min-width:300px;">
                                    <!-- Tempo -->
                                <div style="background:rgba(15, 23, 42, 0.4); backdrop-filter: blur(8px); border:1px solid rgba(255,255,255,0.05); padding:20px; border-radius:14px; display:flex; flex-direction:column; gap:12px; position:relative; overflow:hidden;">
                                    <div style="position:absolute; top:0; left:0; width:4px; height:100%; background:#06b6d4;"></div>
                                    <div style="display:flex; gap:8px;">
                                        <div style="color:#06b6d4;"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6"/></svg></div>
                                        <span style="font-size:13px; font-weight:700; color:var(--text-primary); text-transform:uppercase;">Traces</span>
                                    </div>
                                    <code style="font-size:11px; color:var(--text-primary); background:rgba(0,0,0,0.2); padding:8px; border-radius:8px; border:1px solid rgba(255,255,255,0.03);">${window.location.origin}/query/v1/traces/</code>
                                    <button class="secondary" style="display:flex; align-items:center; justify-content:center; gap:8px; width:100%; font-size:11px; font-weight:600; padding:8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyText('${window.location.origin}/query/v1/traces/', this)">
                                        <svg width="12" height="12" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                        <span class="btn-text">Copy</span>
                                    </button>
                                </div>

                                <!-- Thanos -->
                                <div style="background:rgba(15, 23, 42, 0.4); backdrop-filter: blur(8px); border:1px solid rgba(255,255,255,0.05); padding:20px; border-radius:14px; display:flex; flex-direction:column; gap:12px; position:relative; overflow:hidden;">
                                    <div style="position:absolute; top:0; left:0; width:4px; height:100%; background:#e11d48;"></div>
                                    <div style="display:flex; gap:8px;">
                                        <div style="color:#e11d48;"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/></svg></div>
                                        <span style="font-size:13px; font-weight:700; color:var(--text-primary); text-transform:uppercase;">Metrics</span>
                                    </div>
                                    <code style="font-size:11px; color:var(--text-primary); background:rgba(0,0,0,0.2); padding:8px; border-radius:8px; border:1px solid rgba(255,255,255,0.03);">${window.location.origin}/query/v1/metrics/</code>
                                    <button class="secondary" style="display:flex; align-items:center; justify-content:center; gap:8px; width:100%; font-size:11px; font-weight:600; padding:8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyText('${window.location.origin}/query/v1/metrics/', this)">
                                        <svg width="12" height="12" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                        <span class="btn-text">Copy</span>
                                    </button>
                                </div>

                                <!-- Loki -->
                                <div style="background:rgba(15, 23, 42, 0.4); backdrop-filter: blur(8px); border:1px solid rgba(255,255,255,0.05); padding:20px; border-radius:14px; display:flex; flex-direction:column; gap:12px; position:relative; overflow:hidden;">
                                    <div style="position:absolute; top:0; left:0; width:4px; height:100%; background:#f97316;"></div>
                                    <div style="display:flex; gap:8px;">
                                        <div style="color:#f97316;"><svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16"/></svg></div>
                                        <span style="font-size:13px; font-weight:700; color:var(--text-primary); text-transform:uppercase;">Logs</span>
                                    </div>
                                    <code style="font-size:11px; color:var(--text-primary); background:rgba(0,0,0,0.2); padding:8px; border-radius:8px; border:1px solid rgba(255,255,255,0.03);">${window.location.origin}/query/v1/logs/</code>
                                    <button class="secondary" style="display:flex; align-items:center; justify-content:center; gap:8px; width:100%; font-size:11px; font-weight:600; padding:8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyText('${window.location.origin}/query/v1/logs/', this)">
                                        <svg width="12" height="12" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                        <span class="btn-text">Copy</span>
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- OpAMP WebSocket -->
                        <div class="card" style="grid-column: span 6; max-width: none; padding:32px; border-radius:16px; background: linear-gradient(135deg, var(--card-bg) 0%, rgba(142, 68, 173, 0.05) 100%); box-shadow: 0 10px 30px rgba(0,0,0,0.1); display:flex; flex-direction:column; position:relative; overflow:visible; box-sizing:border-box;">
                            <div style="position:absolute; top:-50px; right:-50px; width:150px; height:150px; background:rgba(142, 68, 173, 0.1); filter:blur(50px); border-radius:50%; pointer-events:none;"></div>
                            
                            <div style="position:relative; z-index:1;">
                                <div style="display:flex; align-items:center; gap:12px; margin-bottom:16px;">
                                    <div style="padding:10px; background:rgba(142, 68, 173, 0.1); color:#8e44ad; border-radius:12px; display:flex;">
                                        <svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071a9.05 9.05 0 0112.728 0m-3.536 3.536a1.5 1.5 0 01-2.122 0"/></svg>
                                    </div>
                                    <span style="font-size:12px; font-weight:800; color:#8e44ad; text-transform:uppercase; letter-spacing:2px;">Device Management</span>
                                </div>
                                <h4 style="margin:0 0 12px 0; font-size:24px; color:var(--text-primary); font-weight:800; letter-spacing:-0.5px;">OpAMP WebSocket</h4>
                                <p style="font-size:14px; color:var(--text-secondary); line-height:1.5; margin-bottom:20px;">
                                    The Open Agent Management Protocol gateway. Enables remote configuration and health monitoring for distributed collectors.
                                </p>
                                <div style="display:flex; flex-direction:column; gap:12px;">
                                    <!-- Primary WebSocket -->
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">WebSocket Ingress</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/ingest/opamp/v1/ws</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Primary agent gateway for configuration and health reporting over secure WebSocket.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ingest/opamp/v1/ws', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                    <!-- Telemetry -->
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Telemetry Mirror</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/ingest/opamp/v1/telemetry/</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Real-time telemetry bridge for inspecting raw traffic arriving from distributed agents.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}/ingest/opamp/v1/telemetry/', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                    <!-- Management APIs -->
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Agents API</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/api/v1/platform/opamp/agents</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Management API for active collectors. Retrieve connection stats and entity mapping.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}/api/v1/platform/opamp/agents', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Config List</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/api/v1/platform/opamp/configs</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Fleet-wide configuration repository to manage collector settings and policies remotely.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}/api/v1/platform/opamp/configs', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Groups API</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/api/v1/platform/opamp/groups</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Segmentation engine to group collectors for bulk configuration and mass operations.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}/api/v1/platform/opamp/groups', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>

                        <!-- Platform APIs -->
                        <div class="card" style="grid-column: span 6; padding:32px; border-radius:16px; max-width: none; background: linear-gradient(135deg, var(--card-bg) 0%, rgba(139, 92, 246, 0.05) 100%); box-shadow: 0 10px 30px rgba(0,0,0,0.1); position:relative; overflow:visible; box-sizing:border-box;">
                            <div style="position:absolute; bottom:-50px; left:-50px; width:150px; height:150px; background:rgba(139, 92, 246, 0.1); filter:blur(50px); border-radius:50%; pointer-events:none;"></div>
                            
                            <div style="position:relative; z-index:1;">
                                <div style="display:flex; align-items:center; gap:12px; margin-bottom:16px;">
                                    <div style="padding:10px; background:rgba(139, 92, 246, 0.1); color:#8b5cf6; border-radius:12px; display:flex;">
                                        <svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
                                    </div>
                                    <span style="font-size:12px; font-weight:800; color:#8b5cf6; text-transform:uppercase; letter-spacing:2px;">Platform Core</span>
                                </div>
                                <h4 style="margin:0 0 12px 0; font-size:24px; color:var(--text-primary); font-weight:800; letter-spacing:-0.5px;">System Internal</h4>
                                <p style="font-size:14px; color:var(--text-secondary); line-height:1.5; margin-bottom:20px;">
                                    Administrative endpoints for platform discovery and core operations. Restricted to authorized internal services.
                                </p>
                                
                                <div style="display:flex; flex-direction:column; gap:12px;">
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Platform Status</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/api/v1/platform/system/status</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Unified health oversight reporting on internal platform service availability.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}/api/v1/platform/system/status', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Swagger UI</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/console/swagger/</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <a class="secondary" href="${window.location.origin}${window.location.pathname.startsWith('/console') ? '/console/swagger/' : '/swagger/'}" target="_blank" rel="noopener noreferrer" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary); text-decoration:none;" title="Open interactive Swagger UI for the platform gateway API.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 3h7m0 0v7m0-7L10 14"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 7v12h12v-5"/></svg>
                                            </a>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}${window.location.pathname.startsWith('/console') ? '/console/swagger/' : '/swagger/'}', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Inventory & Topology</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/inventory/api/v1/topology</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Relationship discovery service mapping architectural dependencies into a graph view.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}/inventory/api/v1/topology', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Entities List</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/inventory/api/v1/entities</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Global directory of infrastructure components, services, hosts, and containers.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}/inventory/api/v1/entities', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Relations List</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/inventory/api/v1/relations</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Dependency analysis interface tracking connections between all discovered entities.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}/inventory/api/v1/relations', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                    <div style="display:flex; align-items:center; justify-content:space-between; padding:12px; background:rgba(0,0,0,0.2); border-radius:12px; border:1px solid rgba(255,255,255,0.05);">
                                        <div>
                                            <span style="font-size:10px; color:#8e44ad; text-transform:uppercase; font-weight:800; letter-spacing:0.5px;">Storage Stats</span>
                                            <div style="font-size:11px; color:var(--text-primary); font-family: monospace; font-weight:600; opacity:0.8;">/inventory/api/v1/stats</div>
                                        </div>
                                        <div style="display:flex; gap:8px;">
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0; border:none; background:rgba(255,255,255,0.05); color:var(--text-primary);" data-tooltip="Internal storage health metrics monitoring disk usage and indexing performance.">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                                            </button>
                                            <button class="secondary" style="display:flex; align-items:center; justify-content:center; height:32px; width:32px; padding:0 8px; border:none; background:rgba(255,255,255,0.05);" onclick="copyIconOnly('${window.location.origin}/inventory/api/v1/stats', this)">
                                                <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                                            </button>
                                        </div>
                                    </div>
                                </div>
                            </div>
                    </div>
            </div>
            </div>

            <!-- Access Control -->
            <div class="section">
                <div class="section-header" style="margin-bottom:20px;">
                    <div style="display:flex; align-items:center; gap:12px;">
                        <div style="padding:10px; background:var(--accent-glow); color:var(--accent-color); border-radius:12px; display:flex;">
                            <svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
                        </div>
                        <div>
                            <span style="font-size:12px; font-weight:800; color:var(--accent-color); text-transform:uppercase; letter-spacing:2px;">Security Policies</span>
                            <h3 style="margin:0; font-size:24px; font-weight:800; color:var(--text-primary);">Access Control</h3>
                        </div>
                    </div>
                </div>
                
                <div class="card" style="margin-top:16px; padding:16px; display:flex; justify-content:space-between; align-items:center; background:linear-gradient(135deg, var(--card-bg) 0%, rgba(255,255,255,0.02) 100%); border-radius:16px; box-shadow: 0 4px 20px rgba(0,0,0,0.05);">
                     <div>
                        <h4 style="margin:0; color:var(--text-primary); font-size:16px;">Force HTTPS (Global)</h4>
                        <p style="font-size:12px; color:var(--text-secondary); margin:4px 0 0 0">Redirects all UI traffic to HTTPS and rejects insecure API calls.</p>
                     </div>
                     <label class="switch">
                        <input type="checkbox" id="toggleForceSsl" ${state.config.force_ssl ? 'checked' : ''}>
                        <span class="slider"></span>
                     </label>
                </div>

                <div style="border: 1px solid var(--card-border); margin-top: 20px; margin-bottom: 20px;"></div>
                
                <div style="display:grid; grid-template-columns: 1fr 1fr; gap:20px; margin-top:16px">
                    <!-- Agents Group -->
                    <div class="card" style="padding:16px">
                        <h4 style="margin-top:0; color:var(--text-primary)">Agents (Ingest)</h4>
                        <p style="font-size:12px; color:var(--text-secondary); margin-bottom:16px">Controls for /ingest/otlp/v1/* (OTLP) and /ingest/opamp/v1/* ingestion.</p>
                        

                        <div style="display:flex; justify-content:space-between; align-items:center">
                            <span>Enforce API Key</span>
                            <label class="switch">
                                <input type="checkbox" id="toggleAgentKey" ${state.config.agent_api_key ? 'checked' : ''}>
                                <span class="slider"></span>
                            </label>
                        </div>
                    </div>

                    <!-- External Consumers Group -->
                    <div class="card" style="padding:16px">
                        <h4 style="margin-top:0; color:var(--text-primary)">External Consumers (Query)</h4>
                        <p style="font-size:12px; color:var(--text-secondary); margin-bottom:16px">Controls for /query/v1/* (Plugins, Dashboards).</p>
                        
                        <div style="display:flex; justify-content:space-between; align-items:center">
                            <span>Enforce API Key</span>
                            <label class="switch">
                                <input type="checkbox" id="toggleExternalKey" ${state.config.external_api_key ? 'checked' : ''}>
                                <span class="slider"></span>
                            </label>
                        </div>
                    </div>
                </div>
            </div>

            <!-- SSL Management -->
            <div class="section">
                <div class="section-header" style="margin-bottom:20px;">
                    <div style="display:flex; align-items:center; gap:12px;">
                        <div style="padding:10px; background:rgba(139, 92, 246, 0.1); color:#8b5cf6; border-radius:12px; display:flex;">
                            <svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
                        </div>
                        <div>
                            <span style="font-size:12px; font-weight:800; color:#8b5cf6; text-transform:uppercase; letter-spacing:2px;">Encryption</span>
                            <h3 style="margin:0; font-size:24px; font-weight:800; color:var(--text-primary);">SSL Management</h3>
                        </div>
                    </div>
                </div>
                
                <div class="card" style="margin-top:16px; padding:24px; max-width:100%; box-sizing:border-box; border-radius:16px; background:linear-gradient(135deg, var(--card-bg) 0%, rgba(139, 92, 246, 0.02) 100%);">
                    <p style="font-size:13px; color:var(--text-secondary); margin:0 0 20px 0;">
                        Apply your custom SSL Certificate and Private Key. You can upload the files or paste their PEM-encoded content.
                    </p>
                    
                    <!-- Tabs instead of standard radios -->
                    <div style="display:flex; gap:10px; margin-bottom:24px; padding:4px; background:var(--input-bg); border-radius:8px; width:max-content; border:1px solid var(--card-border);">
                        <label id="sslTabFile" style="display:flex; align-items:center; gap:8px; cursor:pointer; font-size:13px; font-weight:600; padding:8px 16px; border-radius:6px; background:var(--accent-glow); color:var(--accent-color); transition:all 0.2s; margin-bottom:0;">
                            <input type="radio" name="sslInputMethod" id="sslMethodFile" value="file" checked style="display:none;"> 
                            <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12"/></svg>
                            Upload Files
                        </label>
                        <label id="sslTabText" style="display:flex; align-items:center; gap:8px; cursor:pointer; font-size:13px; font-weight:600; padding:8px 16px; border-radius:6px; color:var(--text-secondary); transition:all 0.2s; margin-bottom:0;">
                            <input type="radio" name="sslInputMethod" id="sslMethodText" value="text" style="display:none;"> 
                            <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>
                            Paste Text
                        </label>
                    </div>

                    <form id="sslForm">
                        <!-- File Upload Mode -->
                        <div id="sslFileContainer" style="display:grid; grid-template-columns: 1fr 1fr; gap:24px;">
                            <div class="form-group" style="margin-bottom:0">
                                <label style="margin-bottom:12px;">SSL Certificate (.crt / .pem)</label>
                                <label for="uploadCert" style="display:flex; flex-direction:column; align-items:center; justify-content:center; gap:8px; padding:32px 16px; border:2px dashed var(--card-border); border-radius:12px; background:var(--input-bg); cursor:pointer; transition:all 0.2s; text-align:center;">
                                    <svg width="32" height="32" fill="none" viewBox="0 0 24 24" stroke="var(--text-secondary)" style="margin-bottom:8px;"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>
                                    <span style="font-size:14px; font-weight:500; color:var(--text-primary);" id="uploadCertLabel">Click to browse</span>
                                    <span style="font-size:12px; color:var(--text-secondary);">Select certificate file</span>
                                    <input type="file" id="uploadCert" accept=".crt,.pem,.txt" style="display:none;">
                                </label>
                            </div>
                            <div class="form-group" style="margin-bottom:0">
                                <label style="margin-bottom:12px;">Private Key (.key)</label>
                                <label for="uploadKey" style="display:flex; flex-direction:column; align-items:center; justify-content:center; gap:8px; padding:32px 16px; border:2px dashed var(--card-border); border-radius:12px; background:var(--input-bg); cursor:pointer; transition:all 0.2s; text-align:center;">
                                    <svg width="32" height="32" fill="none" viewBox="0 0 24 24" stroke="var(--text-secondary)" style="margin-bottom:8px;"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/></svg>
                                    <span style="font-size:14px; font-weight:500; color:var(--text-primary);" id="uploadKeyLabel">Click to browse</span>
                                    <span style="font-size:12px; color:var(--text-secondary);">Select private key file</span>
                                    <input type="file" id="uploadKey" accept=".key,.pem,.txt" style="display:none;">
                                </label>
                            </div>
                        </div>

                        <!-- Text Area Mode -->
                        <div id="sslTextContainer" style="display:none; grid-template-columns: 1fr 1fr; gap:24px;">
                            <div class="form-group" style="margin-bottom:0">
                                <label style="margin-bottom:12px;">SSL Certificate (PEM Content)</label>
                                <textarea id="sslCertText" style="width:100%; box-sizing:border-box; height:220px; padding:16px; border:1px solid var(--card-border); border-radius:8px; background:var(--input-bg); color:var(--text-primary); font-family:'SF Mono', 'Roboto Mono', monospace; font-size:12px; resize:vertical; white-space:pre" placeholder="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"></textarea>
                            </div>
                            <div class="form-group" style="margin-bottom:0">
                                <label style="margin-bottom:12px;">Private Key (PEM Content)</label>
                                <textarea id="sslKeyText" style="width:100%; box-sizing:border-box; height:220px; padding:16px; border:1px solid var(--card-border); border-radius:8px; background:var(--input-bg); color:var(--text-primary); font-family:'SF Mono', 'Roboto Mono', monospace; font-size:12px; resize:vertical; white-space:pre" placeholder="-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"></textarea>
                            </div>
                        </div>

                        <div style="display:flex; justify-content:flex-end; margin-top:24px; padding-top:24px; border-top:1px solid var(--border-color);">
                            <button type="submit" class="btn-create" style="display:flex; align-items:center; gap:8px; width:auto; padding:12px 24px;">
                                <svg width="18" height="18" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
                                Apply Certificates
                            </button>
                        </div>
                    </form>
                </div>
            </div>

            <!-- License -->
            ${renderLicenseSection()}

            <!-- Tenants & Subtenants -->
            ${renderTenantsSection()}

            <!-- API Keys -->
            <div class="section">
                <div class="section-header">
                    <h3>
                        <svg width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/></svg>
                        API Keys
                    </h3>
                    <button class="btn-create" style="width:auto; display:flex; align-items:center; gap:8px" onclick="showCreateKeyModal()">
                        <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
                        Generate New Key
                    </button>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>Prefix</th>
                            <th>Role</th>
                            <th>Scope</th>
                            <th>Created</th>
                            <th>Status</th>
                            <th>Action</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${state.keys.length > 0 ? state.keys.map(k => `
                            <tr>
                                <td style="text-transform:capitalize">${k.name}</td>
                                <td><code>${k.prefix || 'sk-...'}...</code></td>
                                <td>${k.role}</td>
                                <td>${k.org_id ? `<code>${k.org_id}</code>` : (k.tenant_name ? `${k.tenant_name} (all subtenants)` : '<span style="color:var(--text-secondary)">platform</span>')}</td>
                                <td>${new Date(k.created_at).toLocaleDateString()}</td>
                                <td><span class="status-badge ${k.revoked_at ? 'status-offline' : 'status-online'}">${k.revoked_at ? 'Revoked' : 'Active'}</span></td>
                                <td>
                                    ${!k.revoked_at ? `<button class="danger" style="width:auto; padding:4px 8px; font-size:12px" onclick="revokeKey('${k.id}')">Revoke</button>` : ''}
                                </td>
                            </tr>
                        `).join('') : '<tr><td colspan="7" style="text-align:center">No API Keys created</td></tr>'}
                    </tbody>
                </table>
            </div>



        </div>
    </div>
    <div id="modalContainer"></div>
`;

    // Handlers

    const updateConfig = async (key, value, checkbox) => {
        const res = await fetchAPI('/config', 'POST', { [key]: value });

        if (res && res.ok) {
            state.config[key] = value;
            // Reload if Force SSL is changed to ensure protocol consistency
            if (key === 'force_ssl') {
                window.location.reload();
            }
        } else {
            // Revert switch visually
            checkbox.checked = !value;

            // Show error
            let errorMsg = 'Failed to update settings';
            try {
                const body = await res.json();
                if (body && body.error) errorMsg = body.error;
            } catch (e) { }

            showModal('Error', errorMsg, 'error');
        }
    };

    document.getElementById('toggleAgentKey').onchange = (e) => updateConfig('agent_api_key', e.target.checked, e.target);
    document.getElementById('toggleExternalKey').onchange = (e) => updateConfig('external_api_key', e.target.checked, e.target);
    document.getElementById('toggleForceSsl').onchange = (e) => updateConfig('force_ssl', e.target.checked, e.target);

    // SSL UI Toggle Logic
    const sslMethodFile = document.getElementById('sslMethodFile');
    const sslMethodText = document.getElementById('sslMethodText');
    const sslTabFile = document.getElementById('sslTabFile');
    const sslTabText = document.getElementById('sslTabText');
    const sslFileContainer = document.getElementById('sslFileContainer');
    const sslTextContainer = document.getElementById('sslTextContainer');

    const uploadCert = document.getElementById('uploadCert');
    const uploadCertLabel = document.getElementById('uploadCertLabel');
    const uploadKey = document.getElementById('uploadKey');
    const uploadKeyLabel = document.getElementById('uploadKeyLabel');

    const sslCertText = document.getElementById('sslCertText');
    const sslKeyText = document.getElementById('sslKeyText');

    if (uploadCert && uploadKey) {
        uploadCert.onchange = (e) => {
            if (e.target.files.length > 0) uploadCertLabel.innerText = e.target.files[0].name;
            else uploadCertLabel.innerText = 'Click to browse';
        };
        uploadKey.onchange = (e) => {
            if (e.target.files.length > 0) uploadKeyLabel.innerText = e.target.files[0].name;
            else uploadKeyLabel.innerText = 'Click to browse';
        };
    }

    if (sslMethodFile && sslMethodText) {
        sslMethodFile.onchange = (e) => {
            if (e.target.checked) {
                sslFileContainer.style.display = 'grid';
                sslTextContainer.style.display = 'none';
                sslCertText.required = false;
                sslKeyText.required = false;

                // Active tab styling
                sslTabFile.style.background = 'var(--accent-glow)';
                sslTabFile.style.color = 'var(--accent-color)';
                sslTabText.style.background = 'transparent';
                sslTabText.style.color = 'var(--text-secondary)';
            }
        };

        sslMethodText.onchange = (e) => {
            if (e.target.checked) {
                sslFileContainer.style.display = 'none';
                sslTextContainer.style.display = 'grid';
                sslCertText.required = true;
                sslKeyText.required = true;

                // Active tab styling
                sslTabText.style.background = 'var(--accent-glow)';
                sslTabText.style.color = 'var(--accent-color)';
                sslTabFile.style.background = 'transparent';
                sslTabFile.style.color = 'var(--text-secondary)';
            }
        };
    }

    const sslForm = document.getElementById('sslForm');
    if (sslForm) {
        sslForm.onsubmit = async (e) => {
            e.preventDefault();

            let certificate = '';
            let privateKey = '';

            const isFileMode = sslMethodFile.checked;

            if (isFileMode) {
                const certFile = uploadCert.files[0];
                const keyFile = uploadKey.files[0];

                if (!certFile || !keyFile) {
                    showModal('Validation Error', 'Please select both Certificate and Private Key files before applying.', 'error');
                    return;
                }

                const readAsText = (file) => new Promise(r => {
                    const reader = new FileReader();
                    reader.onload = ev => r(ev.target.result);
                    reader.readAsText(file);
                });

                certificate = await readAsText(certFile);
                privateKey = await readAsText(keyFile);
            } else {
                certificate = sslCertText.value;
                privateKey = sslKeyText.value;
            }

            // Send to backend
            const btn = e.target.querySelector('button[type="submit"]');
            const origText = btn.innerHTML;
            btn.innerHTML = 'Applying...';
            btn.disabled = true;

            const res = await fetchAPI('/config/ssl', 'POST', { certificate, privateKey });
            btn.disabled = false;
            btn.innerHTML = origText;

            if (res && res.ok) {
                showModal('Success', 'SSL Certificates have been applied successfully. The gateway will reload them automatically within a few seconds.', 'success');
                // Clear forms
                document.getElementById('sslCertText').value = '';
                document.getElementById('sslKeyText').value = '';
                document.getElementById('uploadCert').value = '';
                document.getElementById('uploadKey').value = '';
            } else {
                let errorMsg = 'Failed to apply certificates.';
                try {
                    const body = await res.json();
                    if (body && body.error) errorMsg = body.error;
                } catch (e) { }
                showModal('Error', errorMsg, 'error');
            }
        };
    }

    document.getElementById('refreshToggle').onchange = (e) => {
        state.autoRefresh = e.target.checked;
        localStorage.setItem('ui_auto_refresh', state.autoRefresh);
        if (state.autoRefresh) {
            updateStatus(); // Run immediately
            startAutoRefresh();
        } else {
            stopAutoRefresh();
        }
    };

    if (state.autoRefresh) startAutoRefresh();
};

const startAutoRefresh = () => {
    stopAutoRefresh();
    window.statusInterval = setInterval(updateStatus, 15000); // 15 seconds
};

const stopAutoRefresh = () => {
    if (window.statusInterval) clearInterval(window.statusInterval);
};

const getServiceIcon = (name) => {
    const n = name.toLowerCase();
    if (n.includes('loki')) return '<svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="#f97316"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17.657 18.657A8 8 0 016.343 7.343S7 9 9 10c0-2 .5-5 2.986-7C14 5 16.09 5.777 17.656 7.343A7.975 7.975 0 0120 13a7.975 7.975 0 01-2.343 5.657z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.879 16.121A3 3 0 1012.015 11L11 14H9c0 .768.293 1.536.879 2.121z"/></svg>';
    if (n.includes('tempo')) return '<svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="#06b6d4"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>';
    if (n.includes('prometheus')) return '<svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="#e11d48"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>';
    if (n.includes('thanos')) return '<svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="#8b5cf6"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"/></svg>';
    if (n.includes('otel') || n.includes('collector')) return '<svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="#3b82f6"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/></svg>';
    if (n.includes('seaweed')) return '<svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="#22c55e"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4"/></svg>';
    if (n.includes('nginx')) return '<svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="#10b981"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01"/></svg>';

    // Default
    return '<svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="currentColor" opacity="0.5"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01"/></svg>';
};

const renderStatusGrid = (components) => {
    return components.map(c => `
        <div class="status-card ${c.status}" onclick="showStatusDetail('${c.name}')" style="cursor:pointer">
            <div class="status-icon"></div>
            <div style="display:flex; align-items:center; gap:12px; width:100%">
                <div style="flex-shrink:0; opacity:0.9">
                    ${getServiceIcon(c.name)}
                </div>
                <div class="status-info">
                    <strong>${c.name}</strong>
                    <span>${c.status === 'online' ? 'Operational' : c.message || 'Offline'}</span>
                </div>
            </div>
            ${c.latency ? `
            <div class="latency-badge">
                <span style="opacity:0.7">⚡</span> ${c.latency}
            </div>` : ''}
        </div>
    `).join('');
};

const updateStatus = async () => {
    // Only update if dashboard is active (simple check: if statusGrid exists)
    const grid = document.getElementById('statusGrid');
    if (!grid) {
        if (window.statusInterval) clearInterval(window.statusInterval);
        return;
    }

    const res = await fetchAPI('/system/status', 'GET', null, '/api/v1/platform');
    if (res && res.ok) {
        const status = await res.json();
        state.systemStatus = status;

        // Update Grid
        grid.innerHTML = renderStatusGrid(status.components);

        // Update Time
        const timeSpan = document.getElementById('statusTime');
        if (timeSpan) timeSpan.innerText = `Last updated: ${new Date(status.updated_at).toLocaleTimeString()}`;
    }
};

// Actions
window.logout = logout;

window.showCreateKeyModal = () => {
    let container = document.getElementById('modalContainer');
    if (!container) {
        container = document.createElement('div');
        container.id = 'modalContainer';
        document.body.appendChild(container);
    }

    container.innerHTML = `
        <div class="modal-overlay" onclick="closeModal(event)">
            <div class="card" onclick="event.stopPropagation()">
                <h2>Generate New API Key</h2>
                <form id="createKeyForm">
                    <div class="form-group">
                        <label>Key Name (e.g. "Loki Agent")</label>
                        <input type="text" name="name" required placeholder="Production Server 1">
                    </div>
                    <div class="form-group">
                        <label>Role</label>
                        <select name="role" style="width:100%; padding:8px; background:rgba(0,0,0,0.2); color:white; border:1px solid var(--border-color); border-radius:6px">
                            <option value="agent">Agent (Write Data)</option>
                            <option value="reader">Reader (Query Data)</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>Subtenant scope (optional — binds this key's telemetry to X-Scope-OrgID)</label>
                        <select name="subtenant_id" style="width:100%; padding:8px; background:rgba(0,0,0,0.2); color:white; border:1px solid var(--border-color); border-radius:6px">
                            <option value="">Platform key (no tenant scope)</option>
                            ${state.subtenants.map(s => {
                                const tenant = state.tenants.find(t => t.id === s.tenant_id);
                                return `<option value="${s.id}" data-tenant-id="${s.tenant_id}">${tenant ? tenant.name + ' / ' : ''}${s.name} (${s.org_id})</option>`;
                            }).join('')}
                        </select>
                    </div>
                    <button type="submit" class="btn-generate" style="display:flex; justify-content:center; align-items:center; gap:8px">
                        <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
                        Generate
                    </button>
                </form>
            </div>
        </div>
        </div>
    `;
    document.body.style.overflow = 'hidden';

    document.getElementById('createKeyForm').onsubmit = async (e) => {
        e.preventDefault();
        const fd = new FormData(e.target);

        const res = await fetchAPI('/keys', 'POST', Object.fromEntries(fd));
        if (res.ok) {
            const data = await res.json();
            showKeyResult(data);
            // DO NOT refresh dashboard here, it wipes the modal.
            // We refresh when the user clicks 'Close' on the result modal.
        }
    };
};

window.closeAndRefresh = () => {
    closeModal();
    renderDashboard();
};

// --- License ---

window.showInstallLicenseModal = () => {
    let container = document.getElementById('modalContainer');
    if (!container) {
        container = document.createElement('div');
        container.id = 'modalContainer';
        document.body.appendChild(container);
    }

    container.innerHTML = `
        <div class="modal-overlay" onclick="closeModal(event)">
            <div class="card" onclick="event.stopPropagation()">
                <h2>Install License</h2>
                <p style="color:var(--text-secondary)">Upload the <code>license.lic</code> file you downloaded from the IyziTrace portal, or paste its contents directly.</p>
                <form id="installLicenseForm">
                    <div class="form-group">
                        <label>License File</label>
                        <label for="uploadLicenseFile" style="display:flex; flex-direction:column; align-items:center; justify-content:center; gap:8px; padding:24px 16px; border:2px dashed var(--card-border); border-radius:12px; background:var(--input-bg); cursor:pointer; text-align:center;">
                            <svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/></svg>
                            <span style="font-size:14px; font-weight:500; color:var(--text-primary);" id="uploadLicenseFileLabel">Click to browse for license.lic</span>
                        </label>
                        <input type="file" id="uploadLicenseFile" accept=".lic,.txt,.jwt" style="display:none;">
                    </div>
                    <div class="form-group">
                        <label>Or paste the token directly</label>
                        <textarea name="token" id="licenseTokenInput" rows="6" style="width:100%; padding:8px; background:rgba(0,0,0,0.2); color:white; border:1px solid var(--border-color); border-radius:6px; font-family:monospace; font-size:11px" placeholder="eyJhbGciOiJFZERTQSIs..."></textarea>
                    </div>
                    <button type="submit" class="btn-generate" style="display:flex; justify-content:center; align-items:center; gap:8px">Install</button>
                </form>
            </div>
        </div>
    `;
    document.body.style.overflow = 'hidden';

    const tokenInput = document.getElementById('licenseTokenInput');
    const uploadLicenseFile = document.getElementById('uploadLicenseFile');
    const uploadLicenseFileLabel = document.getElementById('uploadLicenseFileLabel');

    uploadLicenseFile.onchange = (e) => {
        const file = e.target.files[0];
        if (!file) {
            uploadLicenseFileLabel.innerText = 'Click to browse for license.lic';
            return;
        }
        uploadLicenseFileLabel.innerText = file.name;
        const reader = new FileReader();
        reader.onload = (ev) => { tokenInput.value = ev.target.result.trim(); };
        reader.readAsText(file);
    };

    document.getElementById('installLicenseForm').onsubmit = async (e) => {
        e.preventDefault();
        const token = tokenInput.value.trim();
        if (!token) {
            showModal('Validation Error', 'Upload a license.lic file or paste the token before installing.', 'error');
            return;
        }
        const res = await fetchAPI('/license/install', 'POST', { token });
        if (res && res.ok) {
            closeModal();
            renderDashboard();
        } else {
            let errorMsg = 'Failed to install license';
            try {
                const body = await res.json();
                if (body && body.error) errorMsg = body.error;
            } catch (e) { }
            showModal('License Error', errorMsg, 'error');
        }
    };
};

// --- Tenants & Subtenants ---

window.showCreateTenantModal = () => {
    let container = document.getElementById('modalContainer');
    if (!container) {
        container = document.createElement('div');
        container.id = 'modalContainer';
        document.body.appendChild(container);
    }

    container.innerHTML = `
        <div class="modal-overlay" onclick="closeModal(event)">
            <div class="card" onclick="event.stopPropagation()">
                <h2>New Tenant</h2>
                <form id="createTenantForm">
                    <div class="form-group">
                        <label>Name</label>
                        <input type="text" name="name" required placeholder="Acme Holding Turkey">
                    </div>
                    <div class="form-group">
                        <label>Slug (lowercase, dashes)</label>
                        <input type="text" name="slug" required pattern="[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?" placeholder="acme-tr">
                    </div>
                    <button type="submit" class="btn-generate" style="display:flex; justify-content:center; align-items:center; gap:8px">Create</button>
                </form>
            </div>
        </div>
    `;
    document.body.style.overflow = 'hidden';

    document.getElementById('createTenantForm').onsubmit = async (e) => {
        e.preventDefault();
        const fd = new FormData(e.target);
        const res = await fetchAPI('/tenants', 'POST', Object.fromEntries(fd));
        if (res && res.ok) {
            closeModal();
            renderDashboard();
        } else {
            let errorMsg = 'Failed to create tenant';
            try {
                const body = await res.json();
                if (body && body.error) errorMsg = body.error;
            } catch (e) { }
            showModal('Error', errorMsg, 'error');
        }
    };
};

window.showCreateSubtenantModal = (tenantId) => {
    let container = document.getElementById('modalContainer');
    if (!container) {
        container = document.createElement('div');
        container.id = 'modalContainer';
        document.body.appendChild(container);
    }

    const tenant = state.tenants.find(t => t.id === tenantId);

    container.innerHTML = `
        <div class="modal-overlay" onclick="closeModal(event)">
            <div class="card" onclick="event.stopPropagation()">
                <h2>New Subtenant${tenant ? ` in ${tenant.name}` : ''}</h2>
                <form id="createSubtenantForm">
                    <div class="form-group">
                        <label>Name</label>
                        <input type="text" name="name" required placeholder="Production">
                    </div>
                    <div class="form-group">
                        <label>Slug (lowercase, dashes)</label>
                        <input type="text" name="slug" required pattern="[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?" placeholder="prod">
                    </div>
                    <p style="color:var(--text-secondary); font-size:12px">Resulting X-Scope-OrgID: <code>${tenant ? tenant.slug : '...'}.&lt;slug&gt;</code></p>
                    <button type="submit" class="btn-generate" style="display:flex; justify-content:center; align-items:center; gap:8px">Create</button>
                </form>
            </div>
        </div>
    `;
    document.body.style.overflow = 'hidden';

    document.getElementById('createSubtenantForm').onsubmit = async (e) => {
        e.preventDefault();
        const fd = new FormData(e.target);
        const res = await fetchAPI(`/tenants/${tenantId}/subtenants`, 'POST', Object.fromEntries(fd));
        if (res && res.ok) {
            closeModal();
            renderDashboard();
        } else {
            let errorMsg = 'Failed to create subtenant';
            try {
                const body = await res.json();
                if (body && body.error) errorMsg = body.error;
            } catch (e) { }
            showModal('Error', errorMsg, 'error');
        }
    };
};

window.suspendTenant = (id) => {
    showConfirmModal('Suspend Tenant', 'API keys scoped to this tenant will stop authenticating. Continue?', async () => {
        await fetchAPI(`/tenants/${id}/suspend`, 'POST');
        renderDashboard();
    });
};

window.reactivateTenant = async (id) => {
    await fetchAPI(`/tenants/${id}/reactivate`, 'POST');
    renderDashboard();
};

window.deleteTenant = (id) => {
    showConfirmModal('Delete Tenant', 'This also removes all of its subtenants. This action cannot be undone. Continue?', async () => {
        await fetchAPI(`/tenants/${id}`, 'DELETE');
        renderDashboard();
    });
};

window.suspendSubtenant = (id) => {
    showConfirmModal('Suspend Subtenant', 'API keys scoped to this subtenant will stop authenticating. Continue?', async () => {
        await fetchAPI(`/subtenants/${id}/suspend`, 'POST');
        renderDashboard();
    });
};

window.reactivateSubtenant = async (id) => {
    await fetchAPI(`/subtenants/${id}/reactivate`, 'POST');
    renderDashboard();
};

window.deleteSubtenant = (id) => {
    showConfirmModal('Delete Subtenant', 'This action cannot be undone. Continue?', async () => {
        await fetchAPI(`/subtenants/${id}`, 'DELETE');
        renderDashboard();
    });
};



window.showStatusDetail = (name) => {
    const comp = state.systemStatus.components.find(c => c.name === name);
    if (!comp) return;

    let container = document.getElementById('modalContainer');
    if (!container) {
        container = document.createElement('div');
        container.id = 'modalContainer';
        document.body.appendChild(container);
    }

    container.innerHTML = `
        <div class="modal-overlay" onclick="closeModal(event)">
            <div class="card" onclick="event.stopPropagation()">
                <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:20px">
                    <h2 style="margin:0">${comp.name} Status</h2>
                    <span class="status-badge ${comp.status === 'online' ? 'status-online' : 'status-offline'}">${comp.status}</span>
                </div>
                
                <table style="margin-bottom:20px">
                    <tbody>
                        <tr><td><strong>Latency</strong></td><td>${comp.latency}</td></tr>
                        <tr><td><strong>Message</strong></td><td>${comp.message || 'OK'}</td></tr>
                        ${comp.details ? Object.entries(comp.details).map(([k, v]) => `
                            <tr>
                                <td style="text-transform:capitalize"><strong>${k.replace(/_/g, ' ')}</strong></td>
                                <td style="word-break:break-all; font-family:monospace">${v}</td>
                            </tr>
                        `).join('') : ''}
                    </tbody>
                </table>
                <button class="secondary" onclick="closeModal()" style="justify-content:center; display:flex; align-items:center; gap:8px">
                    <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
                    Close
                </button>
            </div>
        </div>
    `;
};

window.revokeKey = (id) => {
    showConfirmModal('Revoke API Key', 'Are you sure you want to revoke this API key? This action cannot be undone and any services using this key will immediately lose access.', async () => {
        await fetchAPI(`/keys/${id}`, 'DELETE');
        renderDashboard();
    });
};

window.closeModal = (e) => {
    if (e && e.target.className !== 'modal-overlay') return;
    if (e && e.target.className !== 'modal-overlay') return;
    const container = document.getElementById('modalContainer');
    if (container) container.innerHTML = '';
    document.body.style.overflow = '';
};

const showKeyResult = (data) => {
    const modal = document.getElementById('modalContainer');
    modal.innerHTML = `
        <div class="modal-overlay">
            <div class="card">
                <h2 style="color:var(--success-color)">Key Generated!</h2>
                <p>Make sure to copy your API Key now. You won't be able to see it again!</p>
                <div class="key-display-wrapper" style="display:flex; gap:8px; margin:16px 0">
                    <div class="key-display" style="flex:1; margin:0">${data.raw_key}</div>
                    <button class="secondary" style="display:flex; align-items:center; justify-content:center; gap:8px; width:auto" onclick="copyText('${data.raw_key}', this)">
                        <svg width="14" height="14" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
                        <span class="btn-text">Copy</span>
                    </button>
                </div>
                <button onclick="closeAndRefresh()" class="btn-confirm" style="display:flex; justify-content:center; align-items:center; gap:8px">
                    <svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
                    I have copied the key
                </button>
            </div>
        </div>
    `;
};

window.copyText = async (text, btn) => {
    try {
        await navigator.clipboard.writeText(text);
        const textSpan = btn.querySelector('.btn-text');
        const originalContent = textSpan ? textSpan.innerText : btn.innerText;
        const originalBg = btn.style.backgroundColor;

        if (textSpan) {
            textSpan.innerText = 'Copied!';
        } else {
            btn.innerText = 'Copied!';
        }

        btn.style.backgroundColor = 'var(--success-color)';
        btn.style.color = 'white';

        setTimeout(() => {
            if (textSpan) {
                textSpan.innerText = originalContent;
            } else {
                btn.innerText = originalContent;
            }
            btn.style.backgroundColor = originalBg;
            btn.style.color = '';
        }, 2000);
    } catch (err) {
        console.error('Failed to copy text', err);
        const textSpan = btn.querySelector('.btn-text');
        if (textSpan) textSpan.innerText = 'Error';
        else btn.innerText = 'Error';
    }
};

window.copyIconOnly = async (text, btn) => {
    try {
        await navigator.clipboard.writeText(text);
        const originalBg = btn.style.backgroundColor;
        const originalColor = btn.style.color;

        btn.style.backgroundColor = 'rgba(46, 204, 113, 0.2)';
        btn.style.color = '#2ecc71';

        setTimeout(() => {
            btn.style.backgroundColor = originalBg;
            btn.style.color = originalColor;
        }, 2000);
    } catch (err) {
        console.error('Failed to copy text', err);
    }
};

const showError = (msg) => {
    const el = document.getElementById('error');
    if (el) {
        el.innerText = msg;
        el.style.display = 'block';
    }
};

// Init
const init = async () => {
    // Direct fetch to avoid auth header for initial check if no token
    const res = await fetch(`${API_BASE}/config`);
    if (res.ok) {
        const config = await res.json();
        state.config = config;

        if (!config.setup_complete) {
            renderSetup();
            return;
        }

        if (state.token) {
            renderDashboard();
        } else {
            renderLogin();
        }
    } else {
        // API down?
        app.innerHTML = '<div style="color:red">Backend unavailable</div>';
    }
};

init();
