/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Wrapping of a Matrix recovery key in the browser (ADR 0082, format v1). The service
 * only ever stores the output; the wrap key never leaves this module.
 *
 *   sytue-id: ikm = the person's unlock key from their Sytue ID sign-in (ADR 0084)
 *   password: ikm = Argon2id(password UTF-8, salt, t, m, p, 32 bytes, v0x13)  (ADR 0082)
 *   passkey:  ikm = WebAuthn PRF `first` output (32 bytes)                       (ADR 0082)
 *   wrapKey  = HKDF-SHA256(ikm, salt = empty, info = "sytue-matrix-recovery-wrap-v1/<kind>", 32)
 *   AEAD     = AES-256-GCM, random 12-byte nonce, AAD =
 *              "sytue-matrix-recovery-wrap-v1" 0 kind 0 account 0 key_id 0 credential_id
 *
 * The shared vectors (apps/nucleo/recoverywrap/testdata/vectors.json) pin it against
 * the desktop implementation.
 */
import { MatrixCustodyError } from './custody.js';

export const recoveryWrapLabel = 'sytue-matrix-recovery-wrap-v1';

export type RecoveryWrapperKDF =
  | { alg: 'argon2id'; m: number; t: number; p: number; salt: string }
  | { alg: 'webauthn-prf-hkdf-sha256'; prf_salt: string }
  /** `hint` is the public fingerprint of the unlock key it was sealed with. */
  | { alg: 'sytue-id-unlock-hkdf-sha256'; hint: string };

export interface RecoveryWrapper {
  id?: string;
  v: 1;
  kind: 'sytue-id' | 'password' | 'passkey';
  key_id: string;
  credential_id: string;
  kdf: RecoveryWrapperKDF;
  nonce: string;
  ciphertext: string;
  created_at?: string;
}

/** Password parameters of the ADR 0082 wrappers: the service's floor. */
export const passwordKDF = { m: 65536, t: 3, p: 1 } as const;

/** Label of the unlock key derivations (ADR 0084, apps/nucleo/recoverywrap/desbloqueo.go). */
export const unlockLabel = 'sytue-e2e-unlock-v1';

const encoder = new TextEncoder();

export function toBase64(bytes: Uint8Array): string {
  let text = '';
  for (const byte of bytes) text += String.fromCharCode(byte);
  return btoa(text);
}
export function fromBase64(value: string): Uint8Array<ArrayBuffer> {
  const text = atob(value.replace(/-/g, '+').replace(/_/g, '/'));
  const out = new Uint8Array(text.length);
  for (let i = 0; i < text.length; i++) out[i] = text.charCodeAt(i);
  return out;
}
export function toBase64Url(bytes: Uint8Array): string {
  return toBase64(bytes).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function recoveryWrapAAD(
  kind: RecoveryWrapper['kind'],
  account: string,
  keyID: string,
  credentialID: string,
): Uint8Array<ArrayBuffer> {
  return encoder.encode([recoveryWrapLabel, kind, account, keyID, credentialID].join('\0'));
}

/** Argon2id of the password exactly as typed (no normalization). */
export async function passwordIKM(
  password: string,
  kdf: Extract<RecoveryWrapperKDF, { alg: 'argon2id' }>,
): Promise<Uint8Array<ArrayBuffer>> {
  // Loaded on demand: the callback pages import this module's neighbours.
  const { argon2id } = await import('hash-wasm');
  const out = await argon2id({
    password: encoder.encode(password),
    salt: fromBase64(kdf.salt),
    iterations: kdf.t,
    memorySize: kdf.m,
    parallelism: kdf.p,
    hashLength: 32,
    outputType: 'binary',
  });
  return new Uint8Array(out);
}

/** The raw 32-byte wrap key; exposed for the shared vectors only. */
export async function recoveryWrapKeyBytes(
  kind: RecoveryWrapper['kind'],
  ikm: Uint8Array<ArrayBuffer>,
): Promise<Uint8Array<ArrayBuffer>> {
  const base = await crypto.subtle.importKey('raw', ikm, 'HKDF', false, ['deriveBits']);
  const bits = await crypto.subtle.deriveBits(
    {
      name: 'HKDF',
      hash: 'SHA-256',
      salt: new Uint8Array(0),
      info: encoder.encode(`${recoveryWrapLabel}/${kind}`),
    },
    base,
    256,
  );
  return new Uint8Array(bits);
}

async function aesKey(
  kind: RecoveryWrapper['kind'],
  ikm: Uint8Array<ArrayBuffer>,
): Promise<CryptoKey> {
  const raw = await recoveryWrapKeyBytes(kind, ikm);
  try {
    return await crypto.subtle.importKey('raw', raw, 'AES-GCM', false, ['encrypt', 'decrypt']);
  } finally {
    raw.fill(0);
  }
}

export async function sealRecoveryKey(input: {
  kind: RecoveryWrapper['kind'];
  account: string;
  keyID: string;
  credentialID: string;
  kdf: RecoveryWrapperKDF;
  ikm: Uint8Array<ArrayBuffer>;
  recoveryKey: string;
  /** Tests and vectors only; a fresh random nonce otherwise. */
  nonce?: Uint8Array<ArrayBuffer>;
}): Promise<RecoveryWrapper> {
  const nonce = input.nonce ?? crypto.getRandomValues(new Uint8Array(12));
  const key = await aesKey(input.kind, input.ikm);
  const ciphertext = await crypto.subtle.encrypt(
    {
      name: 'AES-GCM',
      iv: nonce,
      additionalData: recoveryWrapAAD(input.kind, input.account, input.keyID, input.credentialID),
    },
    key,
    encoder.encode(input.recoveryKey),
  );
  return {
    v: 1,
    kind: input.kind,
    key_id: input.keyID,
    credential_id: input.credentialID,
    kdf: input.kdf,
    nonce: toBase64(nonce),
    ciphertext: toBase64(new Uint8Array(ciphertext)),
  };
}

/** Rejects `wrong` when the factor (or the bound account/key) does not open it. */
export async function openRecoveryWrapper(
  wrapper: RecoveryWrapper,
  account: string,
  ikm: Uint8Array<ArrayBuffer>,
): Promise<string> {
  const key = await aesKey(wrapper.kind, ikm);
  let plain: ArrayBuffer;
  try {
    plain = await crypto.subtle.decrypt(
      {
        name: 'AES-GCM',
        iv: fromBase64(wrapper.nonce),
        additionalData: recoveryWrapAAD(
          wrapper.kind,
          account,
          wrapper.key_id,
          wrapper.credential_id,
        ),
      },
      key,
      fromBase64(wrapper.ciphertext),
    );
  } catch {
    throw new MatrixCustodyError('wrong');
  }
  return new TextDecoder('utf-8', { fatal: true }).decode(plain);
}

/** The public fingerprint of an unlock key (recoverywrap.Hint): which key sealed a wrapper. */
export async function unlockHint(unlockKey: Uint8Array<ArrayBuffer>): Promise<string> {
  const base = await crypto.subtle.importKey('raw', unlockKey, 'HKDF', false, ['deriveBits']);
  const bits = await crypto.subtle.deriveBits(
    {
      name: 'HKDF',
      hash: 'SHA-256',
      salt: new Uint8Array(0),
      info: encoder.encode(`${unlockLabel}/hint`),
    },
    base,
    128,
  );
  return toBase64(new Uint8Array(bits));
}

/** Wraps a recovery key with the unlock key of the person's Sytue ID sign-in (ADR 0084). */
export async function sealWithUnlockKey(input: {
  unlockKey: Uint8Array<ArrayBuffer>;
  account: string;
  keyID: string;
  recoveryKey: string;
  /** Tests and vectors only. */
  nonce?: Uint8Array<ArrayBuffer>;
}): Promise<RecoveryWrapper> {
  return sealRecoveryKey({
    kind: 'sytue-id',
    account: input.account,
    keyID: input.keyID,
    credentialID: '',
    kdf: { alg: 'sytue-id-unlock-hkdf-sha256', hint: await unlockHint(input.unlockKey) },
    ikm: input.unlockKey,
    recoveryKey: input.recoveryKey,
    ...(input.nonce ? { nonce: input.nonce } : {}),
  });
}
