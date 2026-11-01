/* SPDX-License-Identifier: Apache-2.0 */
import { MatrixSecurityError, isMatrixRoomId, type MatrixStorageScope } from './types.js';
import {
  StorageRecordError,
  maxStorageRecordBytes,
  storageIdentifier,
  storageScope,
} from './storage.js';
import { sha256Hex } from './storage-blocks.js';

export const ssssVaultEventPrefix = 'org.sytue.vault.';
export const ssssVaultEnvelopeKind = 'org.sytue.ssss.v1';
export const storageSessionsKind = 'org.sytue.storage.sessions.v1';
const maxAttestedSessionRows = 512;

export interface SsssVaultEnvelope {
  kind: typeof ssssVaultEnvelopeKind;
  id: string;
  scope: MatrixStorageScope;
  revision: number;
  payload: string;
  sha256: string;
}

export interface AttestedStorageSession {
  room_id: string;
  session_id: string;
  sender_key: string;
}

const encoder = new TextEncoder();
const decoder = new TextDecoder('utf-8', { fatal: true });

function bytesToBase64(bytes: Uint8Array, unpadded = false): string {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  const encoded = btoa(binary);
  return unpadded ? encoded.replace(/=+$/, '') : encoded;
}

function base64ToBytes(value: string, strictPadded: boolean): Uint8Array<ArrayBuffer> {
  if (strictPadded) {
    if (value === '') return new Uint8Array(0);
    if (
      value.length % 4 !== 0 ||
      !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)
    ) {
      throw new MatrixSecurityError('identity');
    }
    const decoded = atob(value);
    if (btoa(decoded) !== value) throw new MatrixSecurityError('identity');
    const out = new Uint8Array(decoded.length);
    for (let i = 0; i < decoded.length; i++) out[i] = decoded.charCodeAt(i);
    return out;
  }
  const trimmed = value.replace(/=+$/, '');
  if (trimmed === '') return new Uint8Array(0);
  if (!/^[A-Za-z0-9+/]+$/.test(trimmed) || trimmed.length % 4 === 1) {
    throw new MatrixSecurityError('identity');
  }
  const padded = trimmed + '='.repeat((4 - (trimmed.length % 4)) % 4);
  const decoded = atob(padded);
  if (btoa(decoded).replace(/=+$/, '') !== trimmed) {
    throw new MatrixSecurityError('identity');
  }
  const out = new Uint8Array(decoded.length);
  for (let i = 0; i < decoded.length; i++) out[i] = decoded.charCodeAt(i);
  return out;
}

export async function ssssEnvelopeDigest(
  secretId: string,
  scope: MatrixStorageScope,
  revision: number,
  payload: Uint8Array,
): Promise<string> {
  const prefix = encoder.encode(
    `${ssssVaultEnvelopeKind}\u0000${secretId}\u0000${scope.tenant}\u0000${scope.subject}\u0000${scope.product}\u0000${revision}\u0000`,
  );
  const combined = new Uint8Array(prefix.byteLength + payload.byteLength);
  combined.set(prefix, 0);
  combined.set(payload, prefix.byteLength);
  try {
    return await sha256Hex(combined);
  } finally {
    combined.fill(0);
  }
}

/**
 * Encodes a context-bound SSSS vault secret as unpadded standard base64 for
 * `client.secretStorage.store`, matching Go's `SSSS.SetEncryptedAccountData`.
 */
export async function encodeSsssVaultSecret(
  secretId: string,
  scopeInput: MatrixStorageScope,
  revision: number,
  payload: Uint8Array,
): Promise<string> {
  const scope = storageScope(scopeInput);
  if (
    !storageIdentifier(secretId) ||
    !Number.isSafeInteger(revision) ||
    revision < 1 ||
    payload.byteLength > maxStorageRecordBytes
  ) {
    throw new StorageRecordError();
  }
  const sha256 = await ssssEnvelopeDigest(secretId, scope, revision, payload);
  const envelope: SsssVaultEnvelope = {
    kind: ssssVaultEnvelopeKind,
    id: secretId,
    scope,
    revision,
    payload: bytesToBase64(payload, false),
    sha256,
  };
  const jsonBytes = encoder.encode(JSON.stringify(envelope));
  try {
    return bytesToBase64(jsonBytes, true);
  } finally {
    jsonBytes.fill(0);
  }
}

export async function decodeSsssVaultSecret(
  rawSecretBase64: string,
  expectedId: string,
  expectedScopeInput: MatrixStorageScope,
  minRevision = 1,
): Promise<{ payload: Uint8Array<ArrayBuffer>; revision: number }> {
  const expectedScope = storageScope(expectedScopeInput);
  if (
    !storageIdentifier(expectedId) ||
    !Number.isSafeInteger(minRevision) ||
    minRevision < 1 ||
    typeof rawSecretBase64 !== 'string' ||
    rawSecretBase64.length === 0 ||
    rawSecretBase64.length > maxStorageRecordBytes * 3
  ) {
    throw new StorageRecordError();
  }
  const jsonBytes = base64ToBytes(rawSecretBase64, false);
  let parsed: unknown;
  try {
    parsed = JSON.parse(decoder.decode(jsonBytes));
  } catch {
    throw new MatrixSecurityError('identity');
  } finally {
    jsonBytes.fill(0);
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new MatrixSecurityError('identity');
  }
  const env = parsed as Record<string, unknown>;
  if (
    env.kind !== ssssVaultEnvelopeKind ||
    env.id !== expectedId ||
    typeof env.revision !== 'number' ||
    !Number.isSafeInteger(env.revision) ||
    env.revision < 1 ||
    env.revision < minRevision ||
    typeof env.payload !== 'string' ||
    typeof env.sha256 !== 'string'
  ) {
    throw new MatrixSecurityError('identity');
  }
  let scope: MatrixStorageScope;
  try {
    scope = storageScope(env.scope);
  } catch {
    throw new MatrixSecurityError('identity');
  }
  if (
    scope.tenant !== expectedScope.tenant ||
    scope.subject !== expectedScope.subject ||
    scope.product !== expectedScope.product
  ) {
    throw new MatrixSecurityError('identity');
  }
  const payload = base64ToBytes(env.payload, true);
  if (payload.byteLength > maxStorageRecordBytes) {
    payload.fill(0);
    throw new MatrixSecurityError('identity');
  }
  const expectedDigest = await ssssEnvelopeDigest(expectedId, expectedScope, env.revision, payload);
  if (env.sha256 !== expectedDigest) {
    payload.fill(0);
    throw new MatrixSecurityError('identity');
  }
  return { payload, revision: env.revision };
}

export async function storageSessionsSecretId(scopeInput: MatrixStorageScope): Promise<string> {
  const scope = storageScope(scopeInput);
  const digest = await sha256Hex(
    encoder.encode(
      `${storageSessionsKind}\u0000${scope.tenant}\u0000${scope.subject}\u0000${scope.product}`,
    ),
  );
  return `sessions_${digest.slice(0, 32)}`;
}

export function isValidAttestedStorageSession(value: unknown): value is AttestedStorageSession {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const entry = value as Record<string, unknown>;
  return (
    typeof entry.room_id === 'string' &&
    isMatrixRoomId(entry.room_id) &&
    typeof entry.session_id === 'string' &&
    entry.session_id.length > 0 &&
    entry.session_id.length <= 255 &&
    !/\s/.test(entry.session_id) &&
    typeof entry.sender_key === 'string' &&
    entry.sender_key.length > 0 &&
    entry.sender_key.length <= 255 &&
    !/\s/.test(entry.sender_key)
  );
}

export function normalizeAttestedStorageSessions(
  entries: readonly AttestedStorageSession[],
): AttestedStorageSession[] {
  const seen = new Set<string>();
  const out: AttestedStorageSession[] = [];
  for (const entry of entries) {
    if (!isValidAttestedStorageSession(entry)) {
      throw new MatrixSecurityError('identity');
    }
    const key = `${entry.room_id}\u0000${entry.session_id}\u0000${entry.sender_key}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push({
      room_id: entry.room_id,
      session_id: entry.session_id,
      sender_key: entry.sender_key,
    });
  }
  out.sort((a, b) =>
    a.room_id < b.room_id
      ? -1
      : a.room_id > b.room_id
        ? 1
        : a.session_id < b.session_id
          ? -1
          : a.session_id > b.session_id
            ? 1
            : a.sender_key < b.sender_key
              ? -1
              : a.sender_key > b.sender_key
                ? 1
                : 0,
  );
  return out.length > maxAttestedSessionRows ? out.slice(-maxAttestedSessionRows) : out;
}

export function encodeAttestedStorageSessions(
  entries: readonly AttestedStorageSession[],
): Uint8Array<ArrayBuffer> {
  const sessions = normalizeAttestedStorageSessions(entries);
  return encoder.encode(
    JSON.stringify({
      kind: storageSessionsKind,
      sessions,
    }),
  );
}

export function decodeAttestedStorageSessions(payload: Uint8Array): AttestedStorageSession[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(decoder.decode(payload));
  } catch {
    throw new MatrixSecurityError('identity');
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new MatrixSecurityError('identity');
  }
  const obj = parsed as Record<string, unknown>;
  if (obj.kind !== storageSessionsKind || !Array.isArray(obj.sessions)) {
    throw new MatrixSecurityError('identity');
  }
  return normalizeAttestedStorageSessions(obj.sessions as AttestedStorageSession[]);
}
