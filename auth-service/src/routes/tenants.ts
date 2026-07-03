import { Router } from 'express';
import { authenticate } from '../middleware/auth';
import * as tenants from '../controllers/tenantController';

const router = Router();

// All tenant/subtenant management is platform-admin only (protected).
router.get('/tenants', authenticate, tenants.listTenants);
router.post('/tenants', authenticate, tenants.createTenant);
router.post('/tenants/:id/suspend', authenticate, tenants.suspendTenant);
router.post('/tenants/:id/reactivate', authenticate, tenants.reactivateTenant);
router.delete('/tenants/:id', authenticate, tenants.deleteTenant);

router.get('/tenants/:tenantId/subtenants', authenticate, tenants.listSubtenantsForTenant);
router.post('/tenants/:tenantId/subtenants', authenticate, tenants.createSubtenant);

router.get('/subtenants', authenticate, tenants.listAllSubtenants);
router.post('/subtenants/:id/suspend', authenticate, tenants.suspendSubtenant);
router.post('/subtenants/:id/reactivate', authenticate, tenants.reactivateSubtenant);
router.delete('/subtenants/:id', authenticate, tenants.deleteSubtenant);

export default router;
