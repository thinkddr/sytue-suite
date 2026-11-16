/* SPDX-License-Identifier: Apache-2.0 */
import { stringify } from 'another-json';
import { isMigrationEpoch, isStorageIdentifier } from '@sytue/config/matrix';
import type {
  StorageControlIntent,
  StorageControlAction,
  StorageRootAuthority,
  StorageRootEnrollment,
  StorageRootService,
  SignedStorageControlAction,
} from '@sytue/config/matrix';
import { MatrixSecurityError } from './types.js';
import { storagePublicationTarget, storageScope } from './storage.js';

function denied(): never {
  throw new MatrixSecurityError('unverified');
}
function generation(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) > 0;
}
export function publicStorageKey(value: unknown): value is string {
  if (typeof value !== 'string' || !/^[A-Za-z0-9+/]{43}$/.test(value)) return false;
  try {
    return atob(value).length === 32 && btoa(atob(value)).replace(/=+$/, '') === value;
  } catch {
    return false;
  }
}
export async function storageControlDigest(request: unknown): Promise<string> {
  const bytes = new TextEncoder().encode(stringify(request));
  const digest = await crypto.subtle.digest('SHA-256', bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
}
export async function storageControlIntent(
  request: StorageRootEnrollment,
  epoch: string,
  kind: 'root.enroll' | 'root.resume',
): Promise<StorageControlIntent> {
  if (
    !isMigrationEpoch(epoch) ||
    !generation(request.generation) ||
    !publicStorageKey(request.root_key)
  )
    return denied();
  const authorityGeneration =
    kind === 'root.resume' ? request.previous_generation : request.generation;
  if (!generation(authorityGeneration)) return denied();
  return Object.freeze({
    authority_generation: authorityGeneration,
    scope: Object.freeze(storageScope(request.scope)),
    epoch,
    kind,
    target: Object.freeze(storagePublicationTarget(request.target)),
    request_sha256: await storageControlDigest(request),
  });
}
/** Copy only the fixed public transcript before asynchronous SDK calls. */
export function checkedStorageAction(
  value: StorageControlAction,
  expected: StorageControlIntent,
  root: string,
  now = Date.now(),
): StorageControlAction {
  const kinds = [
    'root.enroll',
    'root.resume',
    'root.rotate',
    'key.enroll',
    'task.claim',
    'task.ack',
    'task.handoff',
  ];
  if (
    Object.keys(value).length !== 11 ||
    value.v !== 1 ||
    value.root_key !== root ||
    !publicStorageKey(root) ||
    !generation(value.authority_generation) ||
    !isMigrationEpoch(value.epoch) ||
    !kinds.includes(value.kind) ||
    !/^[a-f0-9]{64}$/.test(value.request_sha256) ||
    !isStorageIdentifier(value.nonce) ||
    !Number.isSafeInteger(value.issued_at) ||
    !Number.isSafeInteger(value.expires_at) ||
    value.issued_at <= 0 ||
    value.expires_at <= value.issued_at ||
    value.expires_at - value.issued_at > 300_000 ||
    value.expires_at <= now ||
    value.issued_at > now + 30_000
  )
    return denied();
  const scope = Object.freeze(storageScope(value.scope)),
    target = Object.freeze(storagePublicationTarget(value.target));
  const intent = {
    authority_generation: value.authority_generation,
    scope,
    epoch: value.epoch,
    kind: value.kind,
    target,
    request_sha256: value.request_sha256,
  };
  if (stringify(intent) !== stringify(expected)) return denied();
  return Object.freeze({
    v: 1,
    ...intent,
    root_key: root,
    nonce: value.nonce,
    issued_at: value.issued_at,
    expires_at: value.expires_at,
  });
}
export function checkedStorageAuthority(
  authority: StorageRootAuthority,
  expected: StorageRootEnrollment,
): StorageRootAuthority {
  if (
    authority.v !== 1 ||
    stringify(storageScope(authority.scope)) !== stringify(expected.scope) ||
    authority.user_id !== expected.user_id ||
    authority.root_key !== expected.root_key ||
    !publicStorageKey(authority.root_key) ||
    !generation(authority.generation) ||
    (authority.status !== 'active' && authority.status !== 'revoked')
  )
    return denied();
  return Object.freeze({
    v: 1,
    scope: Object.freeze(storageScope(authority.scope)),
    user_id: authority.user_id,
    root_key: authority.root_key,
    generation: authority.generation,
    status: authority.status,
  });
}

export interface StorageRootDevice {
  ownRoot: (signal: AbortSignal) => Promise<StorageRootEnrollment>;
  approve: (request: StorageRootEnrollment, signal: AbortSignal) => Promise<void>;
  sign: (
    expected: StorageControlIntent,
    action: StorageControlAction,
    signal: AbortSignal,
  ) => Promise<SignedStorageControlAction>;
  assertOpen: (signal: AbortSignal) => void;
}
/** The human wait holds no device job or key material. Every later step keeps
 * the original device and generation; a different root requires recovery. */
export async function ensureStorageRoot(
  device: StorageRootDevice,
  service: StorageRootService,
  signal: AbortSignal,
): Promise<StorageRootAuthority> {
  device.assertOpen(signal);
  const own = await device.ownRoot(signal);
  let request = own,
    kind: 'root.enroll' | 'root.resume' = 'root.enroll';
  try {
    const authority = checkedStorageAuthority(await service.read(signal), own);
    device.assertOpen(signal);
    if (authority.status === 'active') return authority;
    kind = 'root.resume';
    request = Object.freeze({
      ...own,
      generation: authority.generation + 1,
      previous_generation: authority.generation,
      previous_root_key: authority.root_key,
    });
    if (!generation(request.generation)) return denied();
  } catch (error) {
    if (!service.enrollmentRequired(error)) throw error;
  }
  device.assertOpen(signal);
  await device.approve(request, signal);
  device.assertOpen(signal);
  const epoch = crypto.randomUUID(),
    intent = await storageControlIntent(request, epoch, kind);
  const challenge = await service.challenge(epoch, kind, request, signal);
  device.assertOpen(signal);
  if (stringify(challenge.request) !== stringify(request)) return denied();
  const action = checkedStorageAction(challenge.action, intent, request.root_key);
  const proof = await device.sign(intent, action, signal);
  device.assertOpen(signal);
  const result = checkedStorageAuthority(await service.confirm(proof, signal), request);
  device.assertOpen(signal);
  if (result.status !== 'active' || result.generation !== request.generation) return denied();
  return result;
}
