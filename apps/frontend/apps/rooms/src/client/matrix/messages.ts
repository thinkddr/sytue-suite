/* SPDX-License-Identifier: Apache-2.0 */
import type { ChatMessage, Attachment, MessageCard } from '../../lib/chat.js';
import { roomsMatrix, type RoomsMatrix } from './client.js';
import { MatrixSecurityError } from './types.js';
let generation = 0;
interface Scope {
  client: RoomsMatrix;
  generation: number;
}
const messages = new Map<string, { scope: Scope; message: ChatMessage }>();
const uploads = new Map<
  string,
  { scope: Scope; channel: string; room: string; file: Attachment }
>();
export function captureMatrixScope(): Scope {
  return { client: roomsMatrix(), generation };
}
export function assertMatrixScope(scope: Scope): void {
  if (scope.generation !== generation || scope.client !== roomsMatrix())
    throw new MatrixSecurityError('identity');
}
export function rememberUpload(
  scope: Scope,
  channel: string,
  uploaded: { room: string; file: Attachment },
): Attachment {
  const { room, file } = uploaded;
  try {
    assertMatrixScope(scope);
    if (
      scope.client.bootstrap.channels.find((entry) => entry.channel_id === channel)
        ?.matrix_room_id !== room
    )
      throw new MatrixSecurityError('reference');
  } catch (error) {
    if (file.objectUrl) scope.client.releaseAttachment(file.objectUrl);
    throw error;
  }
  uploads.set(file.fileId, { scope, channel, room, file });
  return file;
}
export function encryptedFiles(
  channel: string,
  ids: string[],
): { attachments: Attachment[]; room: string } {
  let room = '';
  const attachments = ids.map((id) => {
    const entry = uploads.get(id);
    if (!entry?.file.matrixFile || entry.channel !== channel)
      throw new MatrixSecurityError('reference');
    assertMatrixScope(entry.scope);
    if (
      entry.scope.client.bootstrap.channels.find((item) => item.channel_id === channel)
        ?.matrix_room_id !== entry.room
    )
      throw new MatrixSecurityError('reference');
    if (room && room !== entry.room) throw new MatrixSecurityError('reference');
    room = entry.room;
    return entry.file;
  });
  return { attachments, room };
}
export function knownMessage(id: string): ChatMessage {
  const entry = messages.get(id);
  if (!entry) throw new MatrixSecurityError('reference');
  assertMatrixScope(entry.scope);
  return entry.message;
}
export function clearMatrixMessages(): void {
  generation++;
  messages.clear();
  uploads.clear();
}
export async function decryptMessage(message: ChatMessage): Promise<ChatMessage> {
  const scope = captureMatrixScope();
  if (!message.matrix_event) {
    const safe =
      message.encryption === 'legacy' || message.type === 'call'
        ? message
        : { ...message, body: '', attachments: [], decryptionFailed: true };
    messages.set(message.id, { scope, message: safe });
    return safe;
  }
  try {
    const content = await scope.client.read(
      message.channelId,
      message.matrix_event,
      message.author.id,
    );
    const decoded = {
      ...message,
      body: content.body,
      identityVerified: content.identityVerified === true,
      authenticityVerified: content.authenticityVerified === true,
      attachments: content.attachments ?? [],
      ...(content.mentions ? { mentions: content.mentions } : {}),
      ...(content.card ? { card: content.card as MessageCard } : {}),
      ...(content.priority ? { priority: content.priority } : {}),
      ...(content.requiresAck ? { requiresAck: true } : {}),
      decryptionFailed: false,
    };
    assertMatrixScope(scope);
    messages.set(message.id, { scope, message: decoded });
    return decoded;
  } catch {
    assertMatrixScope(scope);
    const unavailable = { ...message, body: '', attachments: [], decryptionFailed: true };
    messages.set(message.id, { scope, message: unavailable });
    return unavailable;
  }
}
/** REST payloads contain only references; hydrate messages after they reach this device. */
export async function decryptPayload<T>(payload: T): Promise<T> {
  async function visit(value: unknown): Promise<unknown> {
    if (Array.isArray(value)) return Promise.all(value.map(visit));
    if (!value || typeof value !== 'object') return value;
    const object = value as Record<string, unknown>;
    if (
      typeof object.id === 'string' &&
      typeof object.channelId === 'string' &&
      object.author &&
      typeof object.body === 'string'
    )
      return decryptMessage(value as ChatMessage);
    return Object.fromEntries(
      await Promise.all(
        Object.entries(object).map(async ([key, item]) => [key, await visit(item)]),
      ),
    );
  }
  return (await visit(payload)) as T;
}
