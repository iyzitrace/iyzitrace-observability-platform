import { Router } from 'express';
import { getSystemStatus } from '../controllers/healthController';
import { authenticate } from '../middleware/auth';

const router = Router();

// Retrieve System Status (Protected)
router.get('/system/status', authenticate, getSystemStatus);

export default router;
