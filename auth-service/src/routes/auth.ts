import { Router } from 'express';
import * as auth from '../controllers/authController';
import { authenticate } from '../middleware/auth';

const router = Router();

// Public
router.post('/login', auth.login);
router.post('/setup', auth.setup);
router.get('/config', auth.getConfig); // Used by frontend to check if setup needed

// Protected
router.get('/validate', authenticate, auth.validate);
router.post('/config', authenticate, auth.updateConfig);
router.post('/config/ssl', authenticate, auth.updateSSL);
router.get('/keys', authenticate, auth.getKeys);
router.post('/keys', authenticate, auth.createKey);
router.delete('/keys/:id', authenticate, auth.revokeKey);

export default router;
