import { createCipheriv, createDecipheriv, randomBytes } from 'crypto';

const ALGO = 'aes-256-gcm';

function key(): Buffer {
  const hex = process.env.CONFIG_ENCRYPTION_KEY;
  if (!hex || hex.length !== 64) {
    throw new Error('CONFIG_ENCRYPTION_KEY must be a 32-byte hex string (openssl rand -hex 32)');
  }
  return Buffer.from(hex, 'hex');
}

export function encryptJson(value: unknown): string {
  const iv = randomBytes(12);
  const cipher = createCipheriv(ALGO, key(), iv);
  const plaintext = Buffer.from(JSON.stringify(value), 'utf8');
  const enc = Buffer.concat([cipher.update(plaintext), cipher.final()]);
  const tag = cipher.getAuthTag();
  return Buffer.concat([iv, tag, enc]).toString('base64');
}

export function decryptJson<T = unknown>(payload: string): T {
  const buf = Buffer.from(payload, 'base64');
  const iv = buf.subarray(0, 12);
  const tag = buf.subarray(12, 28);
  const enc = buf.subarray(28);
  const decipher = createDecipheriv(ALGO, key(), iv);
  decipher.setAuthTag(tag);
  const dec = Buffer.concat([decipher.update(enc), decipher.final()]);
  return JSON.parse(dec.toString('utf8')) as T;
}

export function hasEncryptionKey(): boolean {
  const hex = process.env.CONFIG_ENCRYPTION_KEY;
  return !!hex && hex.length === 64;
}
