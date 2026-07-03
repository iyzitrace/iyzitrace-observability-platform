import { Router } from 'express';
import { authenticate } from '../middleware/auth';
import * as license from '../controllers/platformLicenseController';

const router = Router();

// Platform (tenancy) license — distinct from the /license plugin-license
// proxy in routes/license.ts. Admin-only.
router.post('/license/install', authenticate, license.installLicense);
router.get('/license/status', authenticate, license.getLicenseSummary);

export default router;
