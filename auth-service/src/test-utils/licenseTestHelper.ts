import { generateKeyPairSync, createPrivateKey, sign as cryptoSign } from 'crypto';

const base64url = (input: Buffer | string): string =>
  Buffer.from(input)
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');

export interface TestLicensePayload {
  sub: string;
  customer?: string;
  edition?: string;
  iat?: number;
  exp: number;
  grace_days?: number;
  limits: {
    tenants: { max: number };
    subtenants: { max_total: number };
  };
  features?: string[];
  binding?: string;
}

/**
 * Generates a fresh Ed25519 keypair and returns a helper to sign test
 * license tokens with it, plus the public key map shape expected by
 * licenseTokenVerifier.setPublicKeysForTest.
 */
export const createTestLicenseSigner = (kid = 'test-kid') => {
  const { publicKey, privateKey } = generateKeyPairSync('ed25519');
  const publicKeyPem = publicKey.export({ type: 'spki', format: 'pem' }).toString();
  const privateKeyObj = createPrivateKey(privateKey.export({ type: 'pkcs8', format: 'pem' }));

  const sign = (payload: TestLicensePayload): string => {
    const header = { alg: 'EdDSA', typ: 'JWT', kid };
    const signingInput = `${base64url(JSON.stringify(header))}.${base64url(JSON.stringify(payload))}`;
    const signature = cryptoSign(null, Buffer.from(signingInput), privateKeyObj);
    return `${signingInput}.${base64url(signature)}`;
  };

  return { kid, publicKeyPem, sign, publicKeyMap: { [kid]: publicKeyPem } };
};

export const defaultTestPayload = (overrides: Partial<TestLicensePayload> = {}): TestLicensePayload => ({
  sub: 'acc_test',
  customer: 'Test Customer',
  edition: 'enterprise',
  iat: Math.floor(Date.now() / 1000) - 3600,
  exp: Math.floor(Date.now() / 1000) + 3600 * 24 * 365,
  grace_days: 14,
  limits: { tenants: { max: 2 }, subtenants: { max_total: 3 } },
  features: ['crm'],
  ...overrides
});
