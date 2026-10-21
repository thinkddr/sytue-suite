/* SPDX-License-Identifier: Apache-2.0 */
export interface MatrixBootstrap {
  actor: string;
  tenant: string;
  homeserver_url: string;
  sso_provider_id: string;
  matrix_user_id: string;
  controller_user_id: string;
  channels: { channel_id: string; matrix_room_id: string }[];
  encryption: 'm.megolm.v1.aes-sha2';
  draft_room_id?: string;
  draft_room_ids?: string[];
  source_url?: string;
}
export interface MatrixReference {
  room_id: string;
  event_id: string;
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
function boundedIdentifier(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= 255 &&
    !/\s/.test(value) &&
    Array.from(value).every(
      (character) => character.charCodeAt(0) >= 32 && character.charCodeAt(0) !== 127,
    )
  );
}
export function isMatrixDeviceId(value: unknown): value is string {
  return boundedIdentifier(value);
}
/** Room v12 IDs are a SHA-256 base64url digest; earlier room versions include a server name. */
export function isMatrixRoomId(value: unknown): value is string {
  return (
    boundedIdentifier(value) && (/^![A-Za-z0-9_-]{43}$/.test(value) || /^![^:]+:.+$/.test(value))
  );
}

export function validateBootstrap(input: MatrixBootstrap): MatrixBootstrap {
  const home = new URL(input.homeserver_url);
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
    input.matrix_user_id !== `@sytue_${input.actor}:${home.host}` ||
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
