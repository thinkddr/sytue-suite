/* SPDX-License-Identifier: Apache-2.0 */
/** Domain records carried INSIDE maintained Matrix encryption. No cipher or key exchange lives here. */
import { isStorageProduct, type MatrixReference, type MatrixStorageScope } from './types.js';

export const maxStorageRecordBytes = 40 << 10;
export const storageObjectKind = 'org.sytue.object.v1';
export type StorageRole =
  'content' | 'manifest' | 'preview' | 'thumbnail' | 'revision' | 'trash' | 'index' | 'task';
export interface StorageRecord {
  v: 1;
  scope: MatrixStorageScope;
  object_id: string;
  revision_id: string;
  role: StorageRole;
  previous?: string[];
  /** Original UTF-8 JSON bytes in standard base64, matching Go []byte. Never parse/re-encode domain numbers. */
  payload: string;
}
export interface StorageContentPointer {
  kind: 'record' | 'file';
  revision_id: string;
  reference: MatrixReference;
}
export interface StorageObjectCommit {
  kind: typeof storageObjectKind;
  state: 'live' | 'trash' | 'deleted';
  /** Opaque JSON bytes in base64; private titles and domain indexes belong here. */
  metadata: string;
  content?: StorageContentPointer;
  parents?: MatrixReference[];
}
export interface StorageRecordEntry {
  reference: MatrixReference;
  record: StorageRecord;
}
export interface StorageObjectVersion {
  reference: MatrixReference;
  object_id: string;
  revision_id: string;
  commit: StorageObjectCommit;
}
export class StorageRecordError extends Error {
  constructor(readonly reason: 'invalid' | 'incomplete' = 'invalid') {
    super(`storage_${reason}`);
    this.name = 'StorageRecordError';
  }
}
function fail(): never {
  throw new StorageRecordError();
}
function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fail();
  return value as Record<string, unknown>;
}
const encoder = new TextEncoder();
function text(value: unknown, maximum: number): value is string {
  if (typeof value !== 'string' || value.length === 0 || encoder.encode(value).length > maximum)
    return false;
  for (const character of value) {
    const unit = character.charCodeAt(0);
    if (character.length === 1 && unit >= 0xd800 && unit <= 0xdfff) return false;
  }
  return true;
}
export function storageIdentifier(value: unknown): value is string {
  return typeof value === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(value);
}
export function storageScope(value: unknown): MatrixStorageScope {
  const scope = object(value);
  if (
    !text(scope.tenant, 63) ||
    !text(scope.subject, 255) ||
    !isStorageProduct(scope.product) ||
    /[\0\r\n\t]/.test(scope.tenant + scope.subject)
  )
    return fail();
  return { tenant: scope.tenant, subject: scope.subject, product: scope.product };
}
function matrixID(value: unknown, sigil: string): value is string {
  return (
    text(value, 255) && value.length > 1 && value.startsWith(sigil) && !/[\0\r\n\t ]/.test(value)
  );
}
export function storageReference(value: unknown): MatrixReference {
  const ref = object(value);
  if (!matrixID(ref.room_id, '!') || !matrixID(ref.event_id, '$')) return fail();
  return { room_id: ref.room_id, event_id: ref.event_id };
}
function refKey(ref: MatrixReference): string {
  return JSON.stringify([ref.room_id, ref.event_id]);
}
function equalScope(left: MatrixStorageScope, right: MatrixStorageScope): boolean {
  return (
    left.tenant === right.tenant && left.subject === right.subject && left.product === right.product
  );
}
function eventIDs(value: unknown): string[] {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value) || value.length > 16) return fail();
  const ids: string[] = [];
  for (const id of value as unknown[]) {
    if (!matrixID(id, '$') || ids.includes(id)) return fail();
    ids.push(id);
  }
  return ids;
}
function jsonBytes(value: Uint8Array, maximum: number): void {
  if (value.byteLength > maximum) return fail();
  try {
    JSON.parse(new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(value));
  } catch {
    return fail();
  }
}
/** JSON validation does not replace the returned original bytes, including integers above 2^53. */
export function storageJSONBytes(
  value: unknown,
  maximum = maxStorageRecordBytes,
): Uint8Array<ArrayBuffer> {
  if (
    typeof value !== 'string' ||
    value.length > 4 * Math.ceil(maximum / 3) ||
    value.length % 4 !== 0 ||
    !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)
  )
    return fail();
  let decoded: string;
  try {
    decoded = atob(value);
  } catch {
    return fail();
  }
  if (btoa(decoded) !== value) return fail();
  const bytes = Uint8Array.from(decoded, (character) => character.charCodeAt(0));
  jsonBytes(bytes, maximum);
  return bytes;
}
export function encodeStorageJSON(value: Uint8Array, maximum = maxStorageRecordBytes): string {
  jsonBytes(value, maximum);
  // Records are bounded, but avoid Function argument limits for shared callers.
  let binary = '';
  for (const byte of value) binary += String.fromCharCode(byte);
  return btoa(binary);
}
/** Go's standard JSON encoder escapes HTML and these two separators. Account for the native wire limit too. */
function nativeJSONSize(value: unknown): number {
  const raw = JSON.stringify(value).replace(
    /[<>&\u2028\u2029]/g,
    (character) => `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`,
  );
  return encoder.encode(raw).length;
}
export function decodeStorageRecord(value: unknown, expected: MatrixStorageScope): StorageRecord {
  const input = object(value),
    scope = storageScope(input.scope);
  expected = storageScope(expected);
  if (
    input.v !== 1 ||
    !equalScope(scope, expected) ||
    !storageIdentifier(input.object_id) ||
    !storageIdentifier(input.revision_id) ||
    typeof input.role !== 'string' ||
    !['content', 'manifest', 'preview', 'thumbnail', 'revision', 'trash', 'index', 'task'].includes(
      input.role,
    )
  )
    return fail();
  storageJSONBytes(input.payload);
  const previous = eventIDs(input.previous);
  const record: StorageRecord = {
    v: 1,
    scope,
    object_id: input.object_id,
    revision_id: input.revision_id,
    role: input.role as StorageRole,
    ...(previous.length ? { previous } : {}),
    payload: input.payload as string,
  };
  if (nativeJSONSize(record) > maxStorageRecordBytes) return fail();
  return record;
}
export function createStorageRecord(
  scope: MatrixStorageScope,
  objectID: string,
  revisionID: string,
  role: StorageRole,
  payload: Uint8Array,
  previous: string[] = [],
): StorageRecord {
  return decodeStorageRecord(
    {
      v: 1,
      scope,
      object_id: objectID,
      revision_id: revisionID,
      role,
      payload: encodeStorageJSON(payload),
      previous,
    },
    scope,
  );
}
export function sameStorageRecord(left: StorageRecord, right: StorageRecord): boolean {
  return (
    left.v === right.v &&
    equalScope(left.scope, right.scope) &&
    left.object_id === right.object_id &&
    left.revision_id === right.revision_id &&
    left.role === right.role &&
    left.payload === right.payload &&
    JSON.stringify(left.previous ?? []) === JSON.stringify(right.previous ?? [])
  );
}
export function decodeStorageCommit(value: unknown): StorageObjectCommit {
  const input = object(value);
  if (
    input.kind !== storageObjectKind ||
    typeof input.state !== 'string' ||
    !['live', 'trash', 'deleted'].includes(input.state)
  )
    return fail();
  storageJSONBytes(input.metadata, 8 << 10);
  let content: StorageContentPointer | undefined;
  if (input.state === 'deleted') {
    if (input.content !== undefined && input.content !== null) return fail();
  } else {
    const pointer = object(input.content);
    if (
      (pointer.kind !== 'record' && pointer.kind !== 'file') ||
      !storageIdentifier(pointer.revision_id)
    )
      return fail();
    content = {
      kind: pointer.kind,
      revision_id: pointer.revision_id,
      reference: storageReference(pointer.reference),
    };
  }
  const parents = input.parents === undefined || input.parents === null ? [] : input.parents;
  if (!Array.isArray(parents) || parents.length > 16) return fail();
  const refs = parents.map(storageReference);
  if (new Set(refs.map((ref) => ref.event_id)).size !== refs.length) return fail();
  return {
    kind: storageObjectKind,
    state: input.state as StorageObjectCommit['state'],
    metadata: input.metadata as string,
    ...(content ? { content } : {}),
    ...(refs.length ? { parents: refs } : {}),
  };
}
export function decodeStorageObject(
  entry: StorageRecordEntry,
  scope: MatrixStorageScope,
): StorageObjectVersion {
  const ref = storageReference(entry.reference),
    record = decodeStorageRecord(entry.record, scope);
  if (record.role !== 'index') return fail();
  const commit = decodeStorageCommit(
    JSON.parse(new TextDecoder().decode(storageJSONBytes(record.payload))),
  );
  const parents = commit.parents ?? [],
    previous = record.previous ?? [];
  if (
    parents.length !== previous.length ||
    parents.some((parent, i) => refKey(parent) === refKey(ref) || parent.event_id !== previous[i])
  )
    return fail();
  return { reference: ref, object_id: record.object_id, revision_id: record.revision_id, commit };
}
/** Reduce EXHAUSTED history. Retain every concurrent head; no timestamp or server order wins. */
export function storageCatalogHeads(
  scope: MatrixStorageScope,
  entries: readonly StorageRecordEntry[],
): StorageObjectVersion[] {
  storageScope(scope);
  const versions = new Map<string, StorageObjectVersion>(),
    events = new Set<string>();
  for (const entry of entries) {
    const version = decodeStorageObject(entry, scope),
      key = refKey(version.reference);
    if (events.has(version.reference.event_id)) return fail();
    events.add(version.reference.event_id);
    versions.set(key, version);
  }
  const children = new Map<string, string[]>(),
    degrees = new Map<string, number>(),
    queue: string[] = [];
  for (const [key, version] of versions) {
    const parents = version.commit.parents ?? [];
    degrees.set(key, parents.length);
    if (!parents.length) queue.push(key);
    for (const ref of parents) {
      const parentKey = refKey(ref),
        parent = versions.get(parentKey);
      if (!parent) throw new StorageRecordError('incomplete');
      if (parent.object_id !== version.object_id || parent.revision_id === version.revision_id)
        return fail();
      const siblings = children.get(parentKey) ?? [];
      siblings.push(key);
      children.set(parentKey, siblings);
    }
  }
  for (let i = 0; i < queue.length; i++) {
    for (const child of children.get(queue[i]!) ?? []) {
      const degree = degrees.get(child)! - 1;
      degrees.set(child, degree);
      if (degree === 0) queue.push(child);
    }
  }
  if (queue.length !== versions.size) return fail();
  const heads = [...versions].filter(([key]) => !children.has(key)).map(([, version]) => version);
  return heads.sort((left, right) =>
    left.object_id < right.object_id
      ? -1
      : left.object_id > right.object_id
        ? 1
        : left.reference.event_id < right.reference.event_id
          ? -1
          : left.reference.event_id > right.reference.event_id
            ? 1
            : 0,
  );
}
