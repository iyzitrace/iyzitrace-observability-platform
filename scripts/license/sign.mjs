#!/usr/bin/env node
// Dev-only license token signer. Builds a compact EdDSA (Ed25519) JWT matching
// the contract in docs/architecture/multitenancy-licensing.md §4, using the
// keypair produced by keygen.mjs. Uses only Node core `crypto` — no dependency
// on jsonwebtoken, so this stays usable standalone outside the auth-service
// package (and mirrors exactly what the real License Authority is expected
// to produce, minus the KMS).
//
// Usage:
//   node scripts/license/sign.mjs <kid> [payload.json] [output.lic]
//
// If payload.json is omitted, a sample payload (scripts/license/sample-payload.json)
// is used. Output defaults to scripts/license/license.lic (gitignored).

import { createPrivateKey, sign as cryptoSign } from 'crypto';
import { readFileSync, writeFileSync, existsSync } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));

const base64url = (input) =>
  Buffer.from(input)
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');

const kid = process.argv[2];
if (!kid) {
  console.error('Usage: node scripts/license/sign.mjs <kid> [payload.json] [output.lic]');
  process.exit(1);
}

const payloadPath = process.argv[3] || join(__dirname, 'sample-payload.json');
const outputPath = process.argv[4] || join(__dirname, 'license.lic');
const privateKeyPath = join(__dirname, 'dev-private.pem');

if (!existsSync(privateKeyPath)) {
  console.error(`Missing ${privateKeyPath} — run: node scripts/license/keygen.mjs ${kid}`);
  process.exit(1);
}

if (!existsSync(payloadPath)) {
  console.error(`Missing payload file: ${payloadPath}`);
  process.exit(1);
}

const payload = JSON.parse(readFileSync(payloadPath, 'utf-8'));

if (!payload.limits?.tenants?.max || !payload.limits?.subtenants?.max_total) {
  console.error('payload.limits.tenants.max and payload.limits.subtenants.max_total are required');
  process.exit(1);
}
if (!payload.exp) {
  console.error('payload.exp (unix seconds) is required');
  process.exit(1);
}

const header = { alg: 'EdDSA', typ: 'JWT', kid };
const signingInput = `${base64url(JSON.stringify(header))}.${base64url(JSON.stringify(payload))}`;

const privateKey = createPrivateKey(readFileSync(privateKeyPath, 'utf-8'));
const signature = cryptoSign(null, Buffer.from(signingInput), privateKey);

const token = `${signingInput}.${base64url(signature)}`;

writeFileSync(outputPath, token);
console.log(`Signed license -> ${outputPath}`);
console.log(`  kid: ${kid}`);
console.log(`  sub: ${payload.sub}`);
console.log(`  exp: ${new Date(payload.exp * 1000).toISOString()}`);
console.log(`  limits: tenants.max=${payload.limits.tenants.max} subtenants.max_total=${payload.limits.subtenants.max_total}`);
