#!/usr/bin/env node
// Dev-only Ed25519 keypair generator for the IyziTrace license token scheme.
//
// The REAL private key for production licenses lives in the License Authority's
// KMS/HSM (external, not this repo). This script only produces a local keypair
// for testing license verification end-to-end during development.
//
// Usage:
//   node scripts/license/keygen.mjs [kid]
//
// Writes:
//   scripts/license/dev-private.pem   (gitignored — never commit)
//   scripts/license/dev-public.pem
//   config/license/public-keys.json   (merges { <kid>: <public key PEM> })

import { generateKeyPairSync } from 'crypto';
import { writeFileSync, readFileSync, existsSync, mkdirSync } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(__dirname, '..', '..');

const kid = process.argv[2] || `iyzi-lic-dev-${new Date().toISOString().slice(0, 10)}`;

const { publicKey, privateKey } = generateKeyPairSync('ed25519');

const privatePem = privateKey.export({ type: 'pkcs8', format: 'pem' });
const publicPem = publicKey.export({ type: 'spki', format: 'pem' });

writeFileSync(join(__dirname, 'dev-private.pem'), privatePem, { mode: 0o600 });
writeFileSync(join(__dirname, 'dev-public.pem'), publicPem);

const publicKeysPath = join(repoRoot, 'config', 'license', 'public-keys.json');
mkdirSync(dirname(publicKeysPath), { recursive: true });

let keys = {};
if (existsSync(publicKeysPath)) {
  keys = JSON.parse(readFileSync(publicKeysPath, 'utf-8'));
}
keys[kid] = publicPem;
writeFileSync(publicKeysPath, JSON.stringify(keys, null, 2) + '\n');

console.log(`Generated dev Ed25519 keypair (kid=${kid})`);
console.log(`  private key -> scripts/license/dev-private.pem (gitignored)`);
console.log(`  public key  -> scripts/license/dev-public.pem`);
console.log(`  embedded    -> config/license/public-keys.json [${kid}]`);
console.log(`\nSign a license with:`);
console.log(`  node scripts/license/sign.mjs ${kid} payload.json`);
