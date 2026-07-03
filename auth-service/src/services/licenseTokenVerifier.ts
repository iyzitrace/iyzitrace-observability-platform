import crypto from 'crypto';
import fs from 'fs';
import path from 'path';

/**
 * Offline EdDSA (Ed25519) license token verification.
 *
 * jsonwebtoken@9 rejects ed25519 keys during verify() (`Unknown key type
 * "ed25519"`), so this parses/verifies the compact JWT manually via Node's
 * built-in `crypto` — the same primitive scripts/license/sign.mjs uses to
 * produce tokens, keeping signer and verifier symmetric with zero extra
 * runtime dependencies.
 *
 * See docs/architecture/multitenancy-licensing.md §4 for the token contract.
 */

export interface LicenseHeader {
  alg: string;
  typ?: string;
  kid?: string;
}

export interface LicenseLimits {
  tenants: { max: number };
  subtenants: { max_total: number };
  // Optional per-signal ingestion/retention caps, applied per-subtenant via
  // Tempo's per_tenant_override_config / Loki's runtime_config (see
  // services/tenancyOverridesService.ts and docs/architecture/
  // multitenancy-licensing.md §16). Absent = fall back to the backend's own
  // global defaults (unrestricted).
  traces?: {
    retention_days?: number;
    ingestion_rate_limit_bytes?: number;
    ingestion_burst_size_bytes?: number;
    max_traces_per_user?: number;
  };
  logs?: {
    retention_days?: number;
    ingestion_rate_mb?: number;
    ingestion_burst_size_mb?: number;
    max_streams_per_user?: number;
  };
}

export interface LicenseClaims {
  sub: string;
  customer?: string;
  edition?: string;
  iat?: number;
  exp: number;
  grace_days?: number;
  limits: LicenseLimits;
  features?: string[];
  binding?: string;
}

export class LicenseVerificationError extends Error {}

const base64urlDecode = (input: string): Buffer => {
  const normalized = input.replace(/-/g, '+').replace(/_/g, '/');
  const padLength = (4 - (normalized.length % 4)) % 4;
  return Buffer.from(normalized + '='.repeat(padLength), 'base64');
};

interface ParsedToken {
  header: LicenseHeader;
  claims: LicenseClaims;
  signingInput: string;
  signature: Buffer;
}

const parseToken = (token: string): ParsedToken => {
  const parts = token.trim().split('.');
  if (parts.length !== 3) {
    throw new LicenseVerificationError('Malformed license token');
  }
  const [headerPart, payloadPart, signaturePart] = parts;

  let header: LicenseHeader;
  let claims: LicenseClaims;
  try {
    header = JSON.parse(base64urlDecode(headerPart).toString('utf-8'));
    claims = JSON.parse(base64urlDecode(payloadPart).toString('utf-8'));
  } catch {
    throw new LicenseVerificationError('Malformed license token');
  }

  return {
    header,
    claims,
    signingInput: `${headerPart}.${payloadPart}`,
    signature: base64urlDecode(signaturePart)
  };
};

export type PublicKeyMap = Record<string, string>;

const DEFAULT_PUBLIC_KEYS_PATH =
  process.env.LICENSE_PUBLIC_KEYS_PATH ||
  path.join(__dirname, '../../../config/license/public-keys.json');

let publicKeysOverride: PublicKeyMap | null = null;

/** Test-only hook: inject an in-memory key map instead of reading from disk. */
export const setPublicKeysForTest = (keys: PublicKeyMap | null): void => {
  publicKeysOverride = keys;
};

export const loadPublicKeys = (): PublicKeyMap => {
  if (publicKeysOverride) return publicKeysOverride;
  try {
    const raw = fs.readFileSync(DEFAULT_PUBLIC_KEYS_PATH, 'utf-8');
    return JSON.parse(raw);
  } catch {
    return {};
  }
};

const assertClaimsShape = (claims: LicenseClaims): void => {
  if (
    !claims.sub ||
    typeof claims.exp !== 'number' ||
    !claims.limits?.tenants ||
    typeof claims.limits.tenants.max !== 'number' ||
    !claims.limits?.subtenants ||
    typeof claims.limits.subtenants.max_total !== 'number'
  ) {
    throw new LicenseVerificationError('License token missing required claims');
  }
};

/**
 * Verifies signature + shape. Does NOT check expiry/grace — that's a status
 * concern handled by licenseService against a tamper-resistant trusted clock.
 */
export const verifyLicenseToken = (token: string): LicenseClaims => {
  const { header, claims, signingInput, signature } = parseToken(token);

  if (header.alg !== 'EdDSA') {
    throw new LicenseVerificationError(`Unsupported license algorithm: ${header.alg}`);
  }
  if (!header.kid) {
    throw new LicenseVerificationError('License token missing kid');
  }

  const publicKeyPem = loadPublicKeys()[header.kid];
  if (!publicKeyPem) {
    throw new LicenseVerificationError(`Unknown license signing key: ${header.kid}`);
  }

  let publicKey: crypto.KeyObject;
  try {
    publicKey = crypto.createPublicKey(publicKeyPem);
  } catch {
    throw new LicenseVerificationError('Invalid embedded public key');
  }

  const signatureValid = crypto.verify(null, Buffer.from(signingInput), publicKey, signature);
  if (!signatureValid) {
    throw new LicenseVerificationError('License signature is invalid');
  }

  assertClaimsShape(claims);
  return claims;
};

export const decodeLicenseHeader = (token: string): LicenseHeader => parseToken(token).header;
