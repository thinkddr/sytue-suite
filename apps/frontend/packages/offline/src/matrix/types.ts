/* SPDX-License-Identifier: Apache-2.0 */
import {
  isStorageProduct,
  isMatrixRoomId,
  isMatrixDeviceId,
  type MatrixBootstrapMetadata as MatrixBootstrap,
  type MatrixStorageScope,
  type MatrixReference,
} from '@sytue/config/matrix';
export { isStorageProduct, isMatrixRoomId, isMatrixDeviceId } from '@sytue/config/matrix';
export type {
  MatrixBootstrapMetadata as MatrixBootstrap,
  MatrixStorageProduct,
  MatrixStorageScope,
  MatrixReference,
} from '@sytue/config/matrix';
function privateIdentity(value: string, maximum: number): boolean {
  if (!value || new TextEncoder().encode(value).length > maximum || /[\0\r\n\t]/.test(value))
    return false;
  for (const character of value) {
    const code = character.charCodeAt(0);
    if (character.length === 1 && code >= 0xd800 && code <= 0xdfff) return false;
  }
  return true;
}
export interface MatrixDraft {
  channelId: string;
  parentId?: string;
  body: string;
  updatedAt: string;
}
export interface MatrixSession {
  user_id: string;
  device_id: string;
  access_token: string;
  storage_scope?: MatrixStorageScope;
}
/** Keep the deployed Rooms name and the private product/device namespace byte-for-byte. */
export function matrixStorePrefix(input: MatrixBootstrap, device: string): string {
  const bootstrap = validateBootstrap(input);
  if (!isMatrixDeviceId(device)) throw new MatrixSecurityError('device');
  const user = bootstrap.matrix_user_id,
    home = bootstrap.homeserver_url;
  if (bootstrap.storage_product)
    return `sytue-storage-matrix-${encodeURIComponent(JSON.stringify([bootstrap.storage_product, bootstrap.tenant, bootstrap.actor, home, user, device]))}`;
  return `sytue-rooms-matrix-${encodeURIComponent(home)}-${encodeURIComponent(user)}-${encodeURIComponent(device)}`;
}

export interface MatrixFile {
  url: string;
  key: JsonWebKey;
  iv: string;
  hashes: { sha256: string };
  v: 'v2';
}
export interface LocalMatrixAttachment {
  fileId: string;
  name: string;
  contentType: string;
  sizeBytes: number;
  matrixFile?: MatrixFile;
  objectUrl?: string;
}
export interface RoomsEncryptedContent {
  identityVerified?: boolean;
  authenticityVerified?: boolean;
  body: string;
  mentions?: string[];
  attachments?: LocalMatrixAttachment[];
  card?: unknown;
  priority?: 'important' | 'urgent';
  requiresAck?: boolean;
}
export class MatrixSecurityError extends Error {
  constructor(
    public readonly reason:
      | 'login'
      | 'identity'
      | 'device'
      | 'room'
      | 'locked'
      | 'recovery'
      | 'unverified'
      | 'reference'
      | 'unsupported',
  ) {
    super(`matrix_${reason}`);
    this.name = 'MatrixSecurityError';
  }
}
/** Never turn an arbitrary rejection value (which may contain private input) into a message. */
export function matrixFailure(reason: unknown): Error {
  return reason instanceof Error ? reason : new MatrixSecurityError('device');
}

export function validateBootstrap(input: MatrixBootstrap): MatrixBootstrap {
  const home = new URL(input.homeserver_url);
  const identityVersion =
    input.storage_identity_version === undefined || input.storage_identity_version === 0
      ? 1
      : input.storage_identity_version;
  const userID =
    identityVersion === 2 && input.storage_product
      ? `@sytue_storage_${input.storage_product}_${input.tenant}_${input.actor}:${home.host}`
      : `@sytue_${input.actor}:${home.host}`;
  if (
    home.protocol !== 'https:' ||
    home.username ||
    home.password ||
    home.search ||
    home.hash ||
    (home.pathname !== '/' && home.pathname !== '') ||
    input.encryption !== 'm.megolm.v1.aes-sha2' ||
    !input.actor ||
    !input.tenant ||
    !input.sso_provider_id ||
    (identityVersion !== 1 && identityVersion !== 2) ||
    (identityVersion === 2 && !input.storage_product) ||
    input.matrix_user_id !== userID ||
    input.controller_user_id !== `@rooms_controller:${home.host}` ||
    !Array.isArray(input.channels)
  )
    throw new MatrixSecurityError('identity');
  const seen = new Set<string>();
  for (const channel of input.channels) {
    if (
      !channel.channel_id ||
      !isMatrixRoomId(channel.matrix_room_id) ||
      seen.has(channel.channel_id)
    )
      throw new MatrixSecurityError('room');
    seen.add(channel.channel_id);
  }
  if (input.storage_product !== undefined) {
    const rooms = input.storage_room_ids;
    if (
      !isStorageProduct(input.storage_product) ||
      input.channels.length !== 0 ||
      !privateIdentity(input.tenant, 63) ||
      !privateIdentity(input.actor, 255) ||
      input.draft_room_id ||
      (input.draft_room_ids?.length ?? 0) !== 0 ||
      !isMatrixRoomId(input.storage_room_id) ||
      !Array.isArray(rooms) ||
      rooms.length < 1 ||
      rooms.length > 100 ||
      rooms.some((room) => !isMatrixRoomId(room)) ||
      new Set(rooms).size !== rooms.length ||
      !rooms.includes(input.storage_room_id)
    )
      throw new MatrixSecurityError('identity');
  } else if (
    input.storage_room_id ||
    (input.storage_room_ids?.length ?? 0) !== 0 ||
    (input.storage_identity_version ?? 0) !== 0
  ) {
    throw new MatrixSecurityError('identity');
  }
  return { ...input, homeserver_url: home.origin };
}

export function reference(value: unknown): MatrixReference | undefined {
  if (!value || typeof value !== 'object') return undefined;
  const object = value as Record<string, unknown>;
  return typeof object.room_id === 'string' &&
    isMatrixRoomId(object.room_id) &&
    typeof object.event_id === 'string' &&
    /^\$[^\s]+$/.test(object.event_id)
    ? { room_id: object.room_id, event_id: object.event_id }
    : undefined;
}

export interface MatrixCallMember {
  subject: string;
  device_id: string;
}
export interface MatrixCallScope {
  call_id: string;
  matrix_room_id: string;
  livekit_room: string;
  generation: number;
  expires_at_ms: number;
  leader_subject: string;
  leader_device_id: string;
  members: MatrixCallMember[];
}
export function validateCallScope(
  value: MatrixCallScope,
  callId: string,
  subject: string,
  device: string,
): MatrixCallScope {
  if (
    value.call_id !== callId ||
    typeof value.livekit_room !== 'string' ||
    value.livekit_room.length < 1 ||
    value.livekit_room.length > 255 ||
    !isMatrixRoomId(value.matrix_room_id) ||
    !Number.isSafeInteger(value.generation) ||
    value.generation < 1 ||
    !Number.isSafeInteger(value.expires_at_ms) ||
    value.expires_at_ms <= Date.now() ||
    value.expires_at_ms > Date.now() + 15 * 60 * 1000 ||
    !Array.isArray(value.members) ||
    value.members.length === 0 ||
    value.members.length > 1000
  )
    throw new MatrixSecurityError('reference');
  const seen = new Set<string>();
  for (const member of value.members) {
    if (
      !/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(member.subject) ||
      !isMatrixDeviceId(member.device_id) ||
      seen.has(member.subject)
    )
      throw new MatrixSecurityError('identity');
    seen.add(member.subject);
  }
  if (
    !value.members.some((member) => member.subject === subject && member.device_id === device) ||
    !value.members.some(
      (member) =>
        member.subject === value.leader_subject && member.device_id === value.leader_device_id,
    )
  )
    throw new MatrixSecurityError('identity');
  return value;
}
