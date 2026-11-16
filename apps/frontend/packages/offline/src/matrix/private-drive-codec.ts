/* SPDX-License-Identifier: Apache-2.0 */
/** Native Drive wire projection. Source JSON bytes are retained, never re-encoded. */
import { parse as parseLossless, stringify as stringifyLossless } from 'lossless-json';
import {
  isMigrationEpoch,
  legacySourceCursor,
  maxLegacySourceBytes,
  isStorageText,
} from '@sytue/config/matrix';
import { storageJSONBytes, storageReference, type StorageObjectVersion } from './storage.js';
import { fields, hash, digest, invalid, maxStorageFileBytes } from './storage-file-codec.js';
import type { MatrixReference } from './types.js';

export const driveItemKind = 'org.sytue.drive.item.v1';
export const driveBodyKind = 'org.sytue.drive.body.v1';
export const driveMigrationKind = 'org.sytue.drive.migration.v1';
const kinds = ['folder', 'file', 'version', 'share', 'favorite', 'request', 'provenance'] as const;
export type DriveSourceKind = (typeof kinds)[number];
const zeroTime = '0001-01-01T00:00:00Z';
export function text(row: Record<string, unknown>, key: string): string {
  const value = row[key];
  if (value == null) return '';
  if (typeof value !== 'string') return invalid();
  return value;
}
function integer(row: Record<string, unknown>, key: string): number {
  const value = row[key];
  if (value == null) return 0;
  if (typeof value !== 'number' || !Number.isSafeInteger(value)) return invalid();
  return value;
}
function boolean(row: Record<string, unknown>, key: string): boolean {
  const value = row[key];
  if (value == null) return false;
  if (typeof value !== 'boolean') return invalid();
  return value;
}
export function json(bytes: Uint8Array<ArrayBuffer>): Record<string, unknown> {
  try {
    return fields(
      JSON.parse(new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes)),
    );
  } catch {
    return invalid();
  }
}
/** Preserve nanoseconds; Date.parse alone would accept an altered sub-millisecond index. */
function instant(value: string): bigint {
  const m =
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(
      value,
    );
  if (!m) return invalid();
  const y = Number(m[1]),
    mo = Number(m[2]),
    d = Number(m[3]),
    h = Number(m[4]),
    mi = Number(m[5]),
    se = Number(m[6]);
  const leap = y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0),
    days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (mo < 1 || mo > 12 || d < 1 || d > days[mo - 1]! || h > 23 || mi > 59 || se > 59)
    return invalid();
  const zone = m[8]!;
  if (zone !== 'Z' && (Number(zone.slice(1, 3)) > 23 || Number(zone.slice(4)) > 59))
    return invalid();
  const whole = value.replace(/\.\d+(?=Z|[+-])/, '');
  const ms = Date.parse(whole);
  if (!Number.isFinite(ms)) return invalid();
  return BigInt(ms) * 1_000_000n + BigInt((m[7] ?? '').padEnd(9, '0'));
}
/** Keep native nanosecond ordering; Date.parse alone rounds distinct revisions. */
export function compareDriveTime(a: string, b: string): number {
  const left = instant(a),
    right = instant(b);
  return left < right ? -1 : left > right ? 1 : 0;
}
function time(row: Record<string, unknown>, key: string): string {
  const value = text(row, key) || zeroTime;
  instant(value);
  return value;
}
export interface DriveSummary {
  kind: typeof driveItemKind;
  source_kind: DriveSourceKind;
  source_id: string;
  epoch: string;
  raw_sha256: string;
  baseline_sha256: string;
  seed: boolean;
  parent_id: string;
  name: string;
  size_bytes: number;
  content_type: string;
  checksum: string;
  version: number;
  is_root: boolean;
  updated_at: string;
  deleted_at: string | null;
}
export interface DriveBodyPointer {
  object_id: string;
  reference: MatrixReference;
  origin?: { kind: DriveSourceKind; id: string; epoch: string };
}
export interface DriveEntry {
  raw?: Uint8Array<ArrayBuffer>;
  metadata?: DriveBodyPointer;
  binary?: DriveBodyPointer;
}
export function binary(kind: string): boolean {
  return kind === 'file' || kind === 'version';
}
export function sourceKind(value: unknown): DriveSourceKind {
  if (!kinds.includes(value as DriveSourceKind)) return invalid();
  return value as DriveSourceKind;
}
export async function driveObject(kind: string, id: string): Promise<string> {
  return 'drive_' + (await digest(new TextEncoder().encode(kind + '\0' + id)));
}
export function parseSummary(value: unknown): DriveSummary {
  const row = fields(value),
    kind = sourceKind(row.source_kind),
    id = text(row, 'source_id'),
    epoch = text(row, 'epoch');
  legacySourceCursor({ kind, id });
  const raw = text(row, 'raw_sha256'),
    base = text(row, 'baseline_sha256'),
    seed = boolean(row, 'seed'),
    sum = text(row, 'checksum'),
    size = integer(row, 'size_bytes');
  if (
    row.kind !== driveItemKind ||
    !hash(raw) ||
    (base !== '' && !hash(base)) ||
    (sum !== '' && !hash(sum)) ||
    size < 0 ||
    size > maxStorageFileBytes ||
    (epoch !== '' && !isMigrationEpoch(epoch))
  )
    return invalid();
  if (epoch === '' ? base !== '' || seed : !hash(base) || (seed && base !== raw)) return invalid();
  return {
    kind: driveItemKind,
    source_kind: kind,
    source_id: id,
    epoch,
    raw_sha256: raw,
    baseline_sha256: base,
    seed,
    parent_id: text(row, 'parent_id'),
    name: text(row, 'name'),
    size_bytes: size,
    content_type: text(row, 'content_type'),
    checksum: sum,
    version: integer(row, 'version'),
    is_root: boolean(row, 'is_root'),
    updated_at: time(row, 'updated_at'),
    deleted_at: row.deleted_at == null ? null : time(row, 'deleted_at'),
  };
}
export async function summary(version: StorageObjectVersion): Promise<DriveSummary> {
  const result = parseSummary(json(storageJSONBytes(version.commit.metadata, 8 << 10)));
  if (version.object_id !== (await driveObject(result.source_kind, result.source_id)))
    return invalid();
  return result;
}
export async function bodyPointer(
  value: unknown,
  item: DriveSummary,
  purpose: 'binary' | 'metadata',
): Promise<DriveBodyPointer> {
  const row = fields(value),
    object = text(row, 'object_id'),
    ref = storageReference(row.reference);
  let kind = item.source_kind,
    id = item.source_id,
    epoch = item.epoch,
    origin: DriveBodyPointer['origin'];
  if (row.origin != null) {
    const physical = fields(row.origin),
      k = text(physical, 'kind'),
      i = text(physical, 'id'),
      e = text(physical, 'epoch');
    if (k !== '' || i !== '' || e !== '') {
      if (purpose !== 'binary' || !binary(k) || (e !== '' && !isMigrationEpoch(e)))
        return invalid();
      kind = sourceKind(k);
      id = i;
      epoch = e;
      legacySourceCursor({ kind, id });
      origin = { kind, id, epoch };
    }
  }
  if (object !== (await driveObject(kind + '_' + purpose, id))) return invalid();
  return { object_id: object, reference: ref, ...(origin ? { origin } : {}) };
}
export async function entry(payload: string, item: DriveSummary): Promise<DriveEntry> {
  const row = json(storageJSONBytes(payload));
  if (
    row.kind !== 'org.sytue.drive.entry.v1' ||
    row.source_kind !== item.source_kind ||
    row.source_id !== item.source_id ||
    (row.raw == null) === (row.metadata == null) ||
    binary(item.source_kind) !== (row.binary != null)
  )
    return invalid();
  const raw = row.raw == null ? undefined : storageJSONBytes(row.raw, maxLegacySourceBytes);
  const metadata =
    row.metadata == null ? undefined : await bodyPointer(row.metadata, item, 'metadata');
  const body = row.binary == null ? undefined : await bodyPointer(row.binary, item, 'binary');
  return {
    ...(raw ? { raw } : {}),
    ...(metadata ? { metadata } : {}),
    ...(body ? { binary: body } : {}),
  };
}
function projectDriveRow(kind: DriveSourceKind, id: string, raw: Uint8Array<ArrayBuffer>) {
  if (raw.length > maxLegacySourceBytes) invalid();
  const row = json(raw);
  // Go decodes every known column before projecting the relevant kind. A bad
  // created_at must fail even when updated_at already supplies the displayed time.
  for (const key of ['id', 'parent_id', 'folder_id', 'file_id', 'name', 'content_type', 'checksum'])
    text(row, key);
  for (const key of ['size_bytes', 'version']) integer(row, key);
  boolean(row, 'is_root');
  const created = time(row, 'created_at');
  if ((kind === 'folder' || binary(kind)) && row.id !== id) return invalid();
  let parent = '',
    name = '',
    size = 0,
    type = '',
    version = 0,
    isRoot = false;
  if (kind === 'folder') {
    parent = text(row, 'parent_id');
    name = text(row, 'name');
    isRoot = boolean(row, 'is_root');
  }
  if (kind === 'file') {
    parent = text(row, 'folder_id');
    name = text(row, 'name');
  }
  if (kind === 'version') parent = text(row, 'file_id');
  if (binary(kind)) {
    size = integer(row, 'size_bytes');
    type = text(row, 'content_type');
    version = integer(row, 'version');
  }
  let updated = time(row, 'updated_at');
  if (instant(updated) === instant(zeroTime)) updated = created;
  const deleted = row.deleted_at == null ? null : time(row, 'deleted_at'),
    sum = binary(kind) ? text(row, 'checksum').toLowerCase() : '';
  return { row, parent, name, size, type, version, isRoot, updated, deleted, sum };
}
/** Same native projection for frozen source imports and subsequent authenticated reads. */
export async function summarizeDriveRow(
  kind: DriveSourceKind,
  id: string,
  epoch: string,
  raw: Uint8Array<ArrayBuffer>,
): Promise<DriveSummary> {
  const view = projectDriveRow(kind, id, raw),
    sha = await digest(raw);
  return parseSummary({
    kind: driveItemKind,
    source_kind: kind,
    source_id: id,
    epoch,
    raw_sha256: sha,
    baseline_sha256: epoch ? sha : '',
    seed: epoch !== '',
    parent_id: view.parent,
    name: view.name,
    size_bytes: view.size,
    content_type: view.type,
    checksum: view.sum,
    version: view.version,
    is_root: view.isRoot,
    updated_at: view.updated,
    deleted_at: view.deleted,
  });
}
export async function verifyProjection(
  item: DriveSummary,
  state: string,
  raw: Uint8Array<ArrayBuffer>,
): Promise<Record<string, unknown>> {
  if (raw.length > maxLegacySourceBytes || (await digest(raw)) !== item.raw_sha256) invalid();
  const { row, parent, name, size, type, version, isRoot, updated, deleted, sum } = projectDriveRow(
    item.source_kind,
    item.source_id,
    raw,
  );
  if (
    parent !== item.parent_id ||
    name !== item.name ||
    size !== item.size_bytes ||
    type !== item.content_type ||
    version !== item.version ||
    isRoot !== item.is_root ||
    sum !== item.checksum ||
    instant(updated) !== instant(item.updated_at) ||
    (deleted === null) !== (item.deleted_at === null) ||
    (deleted !== null && instant(deleted) !== instant(item.deleted_at!)) ||
    (deleted === null ? 'live' : 'trash') !== state
  )
    return invalid();
  return row;
}
export function fileView(row: Record<string, unknown>, id: string): Record<string, unknown> {
  if (row.id !== id) return invalid();
  const labelID = text(row, 'label_id'),
    name = text(row, 'label_name'),
    blocked = boolean(row, 'label_block_external_share');
  let label: Record<string, unknown> | undefined;
  if (row.label != null) {
    label = fields(row.label);
    text(label, 'name');
    boolean(label, 'block_external_share');
  }
  if (labelID !== '' || name !== '' || blocked) {
    if (
      labelID === '' ||
      name === '' ||
      (label &&
        (text(label, 'name') !== name || boolean(label, 'block_external_share') !== blocked))
    )
      return invalid();
    label = { name, block_external_share: blocked };
  }
  return { ...row, ...(label ? { label: { ...label } } : {}) };
}

/** Match Go's last-key semantics and retain every numeric lexeme, including unknown extensions. */
export function patchDriveJSON(
  raw: Uint8Array<ArrayBuffer>,
  patch: Readonly<Record<string, unknown>>,
): Uint8Array<ArrayBuffer> {
  let result: Uint8Array<ArrayBuffer> | undefined;
  try {
    const value = fields(
      parseLossless(
        new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(raw),
        undefined,
        { onDuplicateKey: ({ newValue }) => newValue },
      ),
    );
    for (const [key, next] of Object.entries(patch))
      Object.defineProperty(value, key, {
        value: next,
        enumerable: true,
        writable: true,
        configurable: true,
      });
    const serialized = stringifyLossless(value);
    if (typeof serialized !== 'string') return invalid();
    result = new TextEncoder().encode(serialized);
    if (result.length > maxLegacySourceBytes) return invalid();
    json(result);
    return result;
  } catch {
    result?.fill(0);
    return invalid();
  }
}

/** Same Unicode-space and UTF-8 byte rule as storage.ValidDriveName in Go. */
export function driveName(value: string): string {
  const name = value.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '');
  if (
    !isStorageText(name, 255) ||
    name === '.' ||
    name === '..' ||
    Array.from(name).some((c) => c.charCodeAt(0) < 32 || c === '/' || c === '\\')
  )
    return invalid();
  return name;
}
