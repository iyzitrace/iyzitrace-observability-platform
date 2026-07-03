import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  verifyLicenseToken,
  setPublicKeysForTest,
  LicenseVerificationError
} from '../licenseTokenVerifier';
import { createTestLicenseSigner, defaultTestPayload } from '../../test-utils/licenseTestHelper';

describe('licenseTokenVerifier', () => {
  afterEach(() => {
    setPublicKeysForTest(null);
  });

  it('verifies a correctly signed token and returns its claims', () => {
    const signer = createTestLicenseSigner();
    setPublicKeysForTest(signer.publicKeyMap);

    const payload = defaultTestPayload();
    const token = signer.sign(payload);

    const claims = verifyLicenseToken(token);
    expect(claims.sub).toBe(payload.sub);
    expect(claims.limits.tenants.max).toBe(2);
    expect(claims.limits.subtenants.max_total).toBe(3);
  });

  it('rejects a token with a single tampered character in the payload', () => {
    const signer = createTestLicenseSigner();
    setPublicKeysForTest(signer.publicKeyMap);

    const token = signer.sign(defaultTestPayload());
    const parts = token.split('.');
    // Flip one character in the payload segment — signature no longer matches.
    const tamperedPayload = parts[1].slice(0, -1) + (parts[1].slice(-1) === 'A' ? 'B' : 'A');
    const tampered = [parts[0], tamperedPayload, parts[2]].join('.');

    expect(() => verifyLicenseToken(tampered)).toThrow(LicenseVerificationError);
  });

  it('rejects an unknown kid', () => {
    const signer = createTestLicenseSigner('kid-a');
    setPublicKeysForTest(signer.publicKeyMap);

    const otherSigner = createTestLicenseSigner('kid-b');
    const token = otherSigner.sign(defaultTestPayload());

    expect(() => verifyLicenseToken(token)).toThrow(/Unknown license signing key/);
  });

  it('rejects a non-EdDSA alg header', () => {
    setPublicKeysForTest({ 'any-kid': 'irrelevant' });
    const base64url = (input: string) =>
      Buffer.from(input).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
    const header = base64url(JSON.stringify({ alg: 'HS256', kid: 'any-kid' }));
    const payload = base64url(JSON.stringify(defaultTestPayload()));
    const token = `${header}.${payload}.fakesignature`;

    expect(() => verifyLicenseToken(token)).toThrow(/Unsupported license algorithm/);
  });

  it('rejects a malformed token', () => {
    expect(() => verifyLicenseToken('not-a-jwt')).toThrow(LicenseVerificationError);
  });

  it('rejects a token missing required limits claims', () => {
    const signer = createTestLicenseSigner();
    setPublicKeysForTest(signer.publicKeyMap);

    const token = signer.sign({
      sub: 'acc_test',
      exp: Math.floor(Date.now() / 1000) + 1000,
      limits: { tenants: { max: 1 } }
    } as any);

    expect(() => verifyLicenseToken(token)).toThrow(/missing required claims/);
  });
});
