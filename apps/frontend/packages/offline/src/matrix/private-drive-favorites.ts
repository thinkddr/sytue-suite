/* SPDX-License-Identifier: Apache-2.0 */
/** Personal stars use the native identity and the shared encrypted entry journal. */
import {
  legacySourceCursor,
  maxLegacySourceBytes,
  type MatrixStorageScope,
} from '@sytue/config/matrix';
import {
  encodeStorageJSON,
  storageJSONBytes,
  storageScope,
  storageReference,
  storageIdentifier,
  type StorageObjectVersion,
  type StoragePublicationTarget,
} from './storage.js';
import { fields, digest, invalid } from './storage-file-codec.js';
import {
  compareDriveTime,
  driveObject,
  json,
  parseSummary,
  patchDriveJSON,
  text,
  summarizeDriveRow,
  verifyProjection,
  type DriveSummary,
} from './private-drive-codec.js';
import {
  decodeDriveEntryCheckpoints,
  publishDriveEntry,
  saveDriveWriteIntent,
  type DriveEntryHost,
  type DriveWriteIntent,
} from './private-drive-metadata.js';

export interface DriveFavoriteRequest {
  kind: 'file' | 'folder';
  id: string;
  on: boolean;
}
export interface DriveFavoriteRow {
  kind: 'file' | 'folder';
  id: string;
  created: string;
  removed: boolean;
}
export interface DriveFavoriteCapture {
  item: DriveSummary;
  raw: Uint8Array<ArrayBuffer>;
  base?: StorageObjectVersion;
}
export interface DriveFavoriteHost extends DriveEntryHost {
  capture: (
    request: DriveFavoriteRequest,
    signal: AbortSignal,
  ) => Promise<DriveFavoriteCapture | null>;
  validate: (
    request: DriveFavoriteRequest,
    intent: DriveWriteIntent,
    signal: AbortSignal,
  ) => Promise<void>;
}
interface FavoriteIntent extends DriveWriteIntent {
  v: 1;
  operation: 'favorite';
  input: string;
  request: DriveFavoriteRequest;
}
const encoder = new TextEncoder();
const equal = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

export function driveFavoriteRow(
  item: DriveSummary,
  row: Record<string, unknown>,
  subject: string,
): DriveFavoriteRow {
  const file = text(row, 'file_id'),
    folder = text(row, 'folder_id'),
    created = text(row, 'created_at');
  if (
    item.source_kind !== 'favorite' ||
    row.id !== item.source_id ||
    row.subject !== subject ||
    (file === '') === (folder === '') ||
    !created ||
    compareDriveTime(created, '0001-01-01T00:00:00Z') === 0
  )
    invalid();
  const kind = file ? 'file' : 'folder',
    id = file || folder;
  legacySourceCursor({ kind, id });
  return { kind, id, created, removed: item.deleted_at !== null };
}

/** RFC 9562 UUIDv5, URL namespace, matching google/uuid.NewSHA1 in Fyne.
 * SHA-1 is used only for this interoperable identifier, never for encryption. */
export async function driveFavoriteID(
  scope: MatrixStorageScope,
  request: DriveFavoriteRequest,
): Promise<string> {
  const namespace = Uint8Array.of(
      0x6b,
      0xa7,
      0xb8,
      0x11,
      0x9d,
      0xad,
      0x11,
      0xd1,
      0x80,
      0xb4,
      0x00,
      0xc0,
      0x4f,
      0xd4,
      0x30,
      0xc8,
    ),
    name = encoder.encode(
      scope.tenant +
        '\0' +
        scope.subject +
        '\0' +
        scope.product +
        '\0favorite\0' +
        request.kind +
        '\0' +
        request.id,
    ),
    input = new Uint8Array(namespace.length + name.length);
  input.set(namespace);
  input.set(name, namespace.length);
  const bytes = new Uint8Array(await crypto.subtle.digest('SHA-1', input)).slice(0, 16);
  bytes[6] = (bytes[6]! & 0x0f) | 0x50;
  bytes[8] = (bytes[8]! & 0x3f) | 0x80;
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  return [
    hex.slice(0, 8),
    hex.slice(8, 12),
    hex.slice(12, 16),
    hex.slice(16, 20),
    hex.slice(20),
  ].join('-');
}
function newBytes(id: string, subject: string, request: DriveFavoriteRequest, now: string) {
  return encoder.encode(
    JSON.stringify({
      id,
      subject,
      [request.kind + '_id']: request.id,
      created_at: now,
      updated_at: now,
    }),
  );
}
export function favoritePatch(request: DriveFavoriteRequest, now: string): Record<string, unknown> {
  return {
    updated_at: now,
    deleted_at: request.on ? null : now,
    ...(request.on ? { created_at: now } : {}),
  };
}
export async function prepareDriveFavorite(
  scope: MatrixStorageScope,
  request: DriveFavoriteRequest,
  previous?: { item: DriveSummary; raw: Uint8Array<ArrayBuffer>; version: StorageObjectVersion },
): Promise<DriveFavoriteCapture> {
  const now = new Date().toISOString(),
    id = previous?.item.source_id ?? (await driveFavoriteID(scope, request)),
    raw = previous
      ? patchDriveJSON(previous.raw, favoritePatch(request, now))
      : newBytes(id, scope.subject, request, now);
  try {
    const item = await summarizeDriveRow('favorite', id, previous?.item.epoch ?? '', raw);
    item.seed = false;
    item.baseline_sha256 = previous?.item.baseline_sha256 ?? '';
    return { item, raw, ...(previous ? { base: previous.version } : {}) };
  } catch (error) {
    raw.fill(0);
    throw error;
  }
}
async function decode(
  bytes: Uint8Array<ArrayBuffer>,
  host: DriveFavoriteHost,
  request: DriveFavoriteRequest,
  input: string,
  signal: AbortSignal,
): Promise<FavoriteIntent> {
  if (bytes.length > 256 << 10) invalid();
  const value = fields(
      JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)) as unknown,
    ),
    item = parseSummary(value.item),
    scope = storageScope(value.scope);
  if (
    value.v !== 1 ||
    value.operation !== 'favorite' ||
    value.input !== input ||
    !equal(value.request, request) ||
    !equal(scope, host.repository.scope) ||
    !storageIdentifier(value.revision) ||
    item.source_kind !== 'favorite' ||
    item.seed ||
    value.binary != null ||
    value.binary_work != null
  )
    invalid();
  const raw = storageJSONBytes(value.raw, maxLegacySourceBytes);
  try {
    const row = await verifyProjection(item, request.on ? 'live' : 'trash', raw),
      mark = driveFavoriteRow(item, row, scope.subject);
    if (mark.kind !== request.kind || mark.id !== request.id || mark.removed === request.on)
      invalid();
    const base = value.base == null ? undefined : storageReference(value.base);
    let expected: Uint8Array<ArrayBuffer>;
    if (base) {
      const prior = await host.read(
        await host.repository.object(base, await driveObject('favorite', item.source_id), signal),
        signal,
      );
      try {
        const previous = driveFavoriteRow(prior.item, json(prior.raw), scope.subject);
        if (
          previous.kind !== mark.kind ||
          previous.id !== mark.id ||
          prior.item.epoch !== item.epoch ||
          prior.item.baseline_sha256 !== item.baseline_sha256
        )
          invalid();
        expected = patchDriveJSON(prior.raw, favoritePatch(request, item.updated_at));
      } finally {
        prior.raw.fill(0);
      }
    } else {
      if (
        !request.on ||
        item.epoch !== '' ||
        item.baseline_sha256 !== '' ||
        item.source_id !== (await driveFavoriteID(scope, request))
      )
        invalid();
      expected = newBytes(item.source_id, scope.subject, request, item.updated_at);
    }
    try {
      if (encodeStorageJSON(expected) !== value.raw) invalid();
    } finally {
      expected.fill(0);
    }
    const intent: FavoriteIntent = {
      v: 1,
      operation: 'favorite',
      scope,
      input,
      request: { ...request },
      target: host.repository.publicationTarget(value.target as StoragePublicationTarget),
      revision: value.revision,
      item,
      raw: value.raw,
      ...(base ? { base } : {}),
    };
    decodeDriveEntryCheckpoints(value, intent, raw.length);
    await host.validate(request, intent, signal);
    return intent;
  } finally {
    raw.fill(0);
  }
}
/** Caller owns the same catalog queue as all other Drive writes. */
export async function setDriveFavorite(
  host: DriveFavoriteHost,
  request: DriveFavoriteRequest,
  signal: AbortSignal,
): Promise<void> {
  const repository = host.repository,
    { journal } = repository.writeContext(),
    input = await digest(
      encoder.encode(JSON.stringify({ scope: repository.scope, operation: 'favorite', request })),
    ),
    key = 'drive_favorite_' + input;
  let stored = await journal.read(key);
  if (stored === null) {
    const capture = await host.capture(request, signal);
    if (!capture) return;
    try {
      const intent: FavoriteIntent = {
        v: 1,
        operation: 'favorite',
        scope: { ...repository.scope },
        input,
        request: { ...request },
        target: repository.publicationTarget(),
        revision: crypto.randomUUID(),
        item: capture.item,
        raw: encodeStorageJSON(capture.raw),
        ...(capture.base ? { base: capture.base.reference } : {}),
      };
      await host.validate(request, intent, signal);
      await saveDriveWriteIntent(journal, key, intent, signal);
      stored = encoder.encode(JSON.stringify(intent));
    } finally {
      capture.raw.fill(0);
    }
  }
  let intent: FavoriteIntent;
  try {
    intent = await decode(stored, host, request, input, signal);
  } finally {
    stored.fill(0);
  }
  await publishDriveEntry(host, journal, key, intent, signal);
  await journal.delete(key);
}
