/* SPDX-License-Identifier: Apache-2.0 */
import encryptSecret from 'matrix-js-sdk/lib/utils/encryptAESSecretStorageItem';
import decryptSecret from 'matrix-js-sdk/lib/utils/decryptAESSecretStorageItem';
import { decodeStorageRecord, type StorageRecord } from './storage.js';
import { MatrixSecurityError, type MatrixStorageScope, isMatrixRoomId } from './types.js';

export const ownerProofDomain = 'org.sytue.storage.auth.v1';
export const ownerProofAlgorithm = 'm.secret_storage.v1.aes-hmac-sha2';
const algorithm = ownerProofAlgorithm;
export interface StorageOwnerProof {
  v: 1;
  algorithm: typeof algorithm;
  key_id: string;
  iv: string;
  ciphertext: string;
  mac: string;
}
export interface StorageOwnerKey {
  id: string;
  key: Uint8Array<ArrayBuffer>;
}
const encoder = new TextEncoder();
function b64(value: Uint8Array): string {
  return btoa(Array.from(value, (byte) => String.fromCharCode(byte)).join(''));
}
export function ownerKeyID(value: unknown): value is string {
  return typeof value === 'string' && /^[\x21-\x7e]{1,255}$/.test(value);
}
function proofBytes(value: unknown, length: number): value is string {
  if (
    typeof value !== 'string' ||
    value.length > 4 * Math.ceil(length / 3) ||
    !/^[A-Za-z0-9+/]+={0,2}$/.test(value)
  )
    return false;
  try {
    const decoded = atob(value),
      raw = btoa(decoded),
      trimmed = raw.replace(/=+$/, '');
    return decoded.length === length && (value === raw || value === trimmed);
  } catch {
    return false;
  }
}
/** Reject unverifiable legacy key metadata before accepting or caching a recovery key. */
export function hasSecretStorageCheck(value: unknown): boolean {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const description = value as Record<string, unknown>;
  return (
    description.algorithm === algorithm &&
    proofBytes(description.iv, 16) &&
    proofBytes(description.mac, 32)
  );
}
export function decodeOwnerProof(value: unknown): StorageOwnerProof {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new MatrixSecurityError('unverified');
  const proof = value as Record<string, unknown>;
  if (
    proof.v !== 1 ||
    proof.algorithm !== algorithm ||
    !ownerKeyID(proof.key_id) ||
    !proofBytes(proof.iv, 16) ||
    !proofBytes(proof.mac, 32) ||
    !proofBytes(proof.ciphertext, 86)
  )
    throw new MatrixSecurityError('unverified');
  return {
    v: 1,
    algorithm,
    key_id: proof.key_id,
    iv: proof.iv,
    ciphertext: proof.ciphertext,
    mac: proof.mac,
  };
}
/** A content checksum, not a signature: ASCII tuple encoding matches storage.RecordDigest in Go. */
export async function storageRecordDigest(
  room: string,
  input: StorageRecord,
  scope: MatrixStorageScope,
): Promise<string> {
  const record = decodeStorageRecord(input, scope);
  if (!isMatrixRoomId(room)) throw new MatrixSecurityError('room');
  const tuple = [
    ownerProofDomain,
    ...[
      room,
      record.scope.tenant,
      record.scope.subject,
      record.scope.product,
      record.object_id,
      record.revision_id,
      record.role,
    ].map((value) => b64(encoder.encode(value))),
    record.payload,
    ...(record.previous ?? []).map((value) => b64(encoder.encode(value))),
  ];
  const digest = new Uint8Array(
    await crypto.subtle.digest('SHA-256', encoder.encode(JSON.stringify(tuple))),
  );
  return Array.from(digest, (byte) => byte.toString(16).padStart(2, '0')).join('');
}
/** All encryption/MAC is the maintained Matrix SSSS implementation; no application cipher lives here. */
export async function sealStorageOwnerRecord(
  room: string,
  input: StorageRecord,
  scope: MatrixStorageScope,
  owner: StorageOwnerKey,
): Promise<StorageOwnerProof> {
  if (!ownerKeyID(owner.id) || owner.key.byteLength !== 32)
    throw new MatrixSecurityError('recovery');
  const id = owner.id,
    key = owner.key.slice();
  try {
    const digest = await storageRecordDigest(room, input, scope);
    // mautrix's SSSS primitive base64-encodes raw bytes before encrypting.
    const encrypted = await encryptSecret(
      btoa(digest).replace(/=+$/, ''),
      key,
      ownerProofDomain + '.' + digest,
    );
    return decodeOwnerProof({ v: 1, algorithm, key_id: id, ...encrypted });
  } finally {
    key.fill(0);
  }
}
export async function verifyStorageOwnerRecord(
  room: string,
  input: StorageRecord,
  scope: MatrixStorageScope,
  value: unknown,
  owner: StorageOwnerKey,
): Promise<void> {
  const proof = decodeOwnerProof(value);
  if (proof.key_id !== owner.id || owner.key.byteLength !== 32)
    throw new MatrixSecurityError('unverified');
  const key = owner.key.slice();
  try {
    const digest = await storageRecordDigest(room, input, scope);
    const clear = await decryptSecret(proof, key, ownerProofDomain + '.' + digest);
    if (clear.replace(/=+$/, '') !== btoa(digest).replace(/=+$/, ''))
      throw new MatrixSecurityError('unverified');
  } catch {
    throw new MatrixSecurityError('unverified');
  } finally {
    key.fill(0);
  }
}
