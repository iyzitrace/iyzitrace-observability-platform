const DEV_DEFAULT_JWT_SECRET = 'dev-secret-change-me';

let warned = false;

/**
 * JWT_SECRET is required in production — it signs session tokens AND derives
 * the HMAC used for license clock-rollback tamper detection. Missing it in
 * production must fail startup loudly rather than silently falling back to a
 * publicly-known default (see docs/architecture/multitenancy-licensing.md §6).
 */
export const getJwtSecret = (): string => {
  const secret = process.env.JWT_SECRET;
  if (secret) return secret;

  if (process.env.NODE_ENV === 'production') {
    throw new Error(
      'JWT_SECRET must be set in production (see .secrets.env). Refusing to start with an insecure default.'
    );
  }

  if (!warned) {
    console.warn(
      '[SECURITY] JWT_SECRET not set — using an insecure development default. ' +
      'Set JWT_SECRET via .secrets.env before deploying to production.'
    );
    warned = true;
  }
  return DEV_DEFAULT_JWT_SECRET;
};
