import { Router } from 'express';
import { getLicenseStatus } from '../controllers/licenseController';

const router = Router();

// GET /api/v1/license
// Header: X-License-Key: {key}
// Auth: lisans key'in kendisi kimlik doğrulama, platform API key gerekmez
router.get('/', getLicenseStatus);

export default router;
