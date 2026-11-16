/* SPDX-License-Identifier: Apache-2.0 */
/** One bounded native file.v1 codec shared by readers and resumable writers. */
import {
  decodeCipherBlobReceipt,
  maxCipherBlockBytes,
  type CipherBlobReceipt,
} from '@sytue/config/matrix';
import type { MatrixFile, MatrixReference, MatrixStorageScope } from './types.js';
import type { StorageJobRunner } from './storage-jobs.js';
import {
  StorageRecordError,
  storageJSONBytes,
  storageIdentifier,
  storageReference,
  type StorageRecord,
} from './storage.js';
export const maxStorageFileBytes = (1 << 24) * maxCipherBlockBytes;
export const fanout = 16,
  levels = 6;
export interface StorageFileRequest {
  object_id: string;
  revision_id: string;
  size_bytes: number;
  /** Original UTF-8 JSON metadata bytes in base64, matching Go []byte. */
  metadata: string;
  previous?: string[];
}
export interface StorageBlobPort {
  /** Same immutable ciphertext ID and bytes on every retry. */
  write?: (
    id: string,
    ciphertext: Uint8Array<ArrayBuffer>,
    signal?: AbortSignal,
  ) => Promise<CipherBlobReceipt>;
  /** Returns a bounded complete ciphertext block; never accept a name, media type or file key. */
  read: (id: string, signal?: AbortSignal) => Promise<Uint8Array<ArrayBuffer>>;
}
export interface StorageFileRecords {
  run?: StorageJobRunner;
  readRecord: (
    ref: MatrixReference,
    object: string,
    role: 'manifest',
    signal?: AbortSignal,
  ) => Promise<StorageRecord>;
  assertOpen: () => void;
}
export interface FileBlock {
  blob: CipherBlobReceipt;
  matrix_file: Omit<MatrixFile, 'url'>;
  plain_sha256: string;
}
export interface FileLink {
  block?: FileBlock;
  event?: MatrixReference;
}
export interface FilePage {
  kind: 'org.sytue.file.page.v1';
  level: number;
  index: number;
  nodes: FileLink[];
}
export interface PrivateFile {
  kind: 'org.sytue.file.v1';
  request: StorageFileRequest;
  sha256: string;
  root: unknown;
}
/** Sink owns staging. It must expose plaintext to consumers only after commit; abort discards it. */
export interface StorageFileSink {
  write: (authenticated: Uint8Array<ArrayBuffer>) => Promise<void>;
  commit: () => Promise<void>;
  abort: () => Promise<void>;
}
export function invalid(): never {
  throw new StorageRecordError();
}
export function fields(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return invalid();
  return value as Record<string, unknown>;
}
export function hash(value: unknown): value is string {
  return typeof value === 'string' && /^[0-9a-f]{64}$/.test(value);
}
function rawBase64(value: unknown, length: number, url = false): Uint8Array<ArrayBuffer> {
  if (
    typeof value !== 'string' ||
    value.includes('=') ||
    value.length !== Math.ceil((length * 8) / 6) ||
    !(url ? /^[A-Za-z0-9_-]+$/ : /^[A-Za-z0-9+/]+$/).test(value)
  )
    return invalid();
  const normal = url ? value.replaceAll('-', '+').replaceAll('_', '/') : value;
  let decoded: string;
  try {
    decoded = atob(normal);
  } catch {
    return invalid();
  }
  if (decoded.length !== length || btoa(decoded).replace(/=+$/, '') !== normal) return invalid();
  return Uint8Array.from(decoded, (character) => character.charCodeAt(0));
}
export function block(value: unknown, scope: MatrixStorageScope, wanted: number): FileBlock {
  const input = fields(value),
    file = fields(input.matrix_file),
    key = fields(file.key),
    hashes = fields(file.hashes);
  const receipt = decodeCipherBlobReceipt(input.blob, scope.product);
  if (
    receipt.size_bytes !== wanted ||
    !hash(input.plain_sha256) ||
    file.v !== 'v2' ||
    (file.url !== undefined && file.url !== '') ||
    key.kty !== 'oct' ||
    key.alg !== 'A256CTR' ||
    key.ext !== true ||
    !Array.isArray(key.key_ops) ||
    key.key_ops.length !== 2 ||
    !key.key_ops.includes('encrypt') ||
    !key.key_ops.includes('decrypt')
  )
    return invalid();
  const secret = rawBase64(key.k, 32, true);
  secret.fill(0);
  rawBase64(file.iv, 16);
  const cipherHash = rawBase64(hashes.sha256, 32);
  if (
    [...cipherHash].map((value) => value.toString(16).padStart(2, '0')).join('') !== receipt.sha256
  )
    return invalid();
  return {
    blob: receipt,
    matrix_file: {
      key: {
        kty: 'oct',
        alg: 'A256CTR',
        ext: true,
        key_ops: ['encrypt', 'decrypt'],
        k: key.k as string,
      },
      iv: file.iv as string,
      hashes: { sha256: hashes.sha256 as string },
      v: 'v2',
    },
    plain_sha256: input.plain_sha256,
  };
}
export function count(size: number): number {
  return Math.max(1, Math.ceil(size / maxCipherBlockBytes));
}
export function span(level: number): number {
  return fanout ** (level + 1);
}
export function rootLevel(size: number): number {
  let level = 0;
  while (count(size) > span(level)) level++;
  return level;
}
export function blockBytes(size: number, index: number): number {
  return Math.min(maxCipherBlockBytes, Math.max(0, size - index * maxCipherBlockBytes));
}
export function page(
  value: unknown,
  file: PrivateFile,
  scope: MatrixStorageScope,
  level: number,
  index: number,
): FilePage {
  const input = fields(value),
    blocks = count(file.request.size_bytes),
    first = index * span(level),
    childSpan = span(level) / fanout;
  if (
    !Number.isSafeInteger(level) ||
    level < 0 ||
    level >= levels ||
    !Number.isSafeInteger(index) ||
    index < 0 ||
    index > Math.floor((blocks - 1) / span(level)) ||
    input.kind !== 'org.sytue.file.page.v1' ||
    input.level !== level ||
    input.index !== index ||
    !Array.isArray(input.nodes) ||
    input.nodes.length !== Math.ceil(Math.min(span(level), blocks - first) / childSpan)
  )
    return invalid();
  const nodes = (input.nodes as unknown[]).map((value, offset): FileLink => {
    const link = fields(value);
    if (level === 0) {
      if (link.event !== undefined && link.event !== null) return invalid();
      return {
        block: block(link.block, scope, blockBytes(file.request.size_bytes, first + offset)),
      };
    }
    if (link.block !== undefined && link.block !== null) return invalid();
    return { event: storageReference(link.event) };
  });
  return { kind: 'org.sytue.file.page.v1', level, index, nodes };
}
export function parse(record: StorageRecord): unknown {
  return JSON.parse(
    new TextDecoder('utf-8', { fatal: true }).decode(storageJSONBytes(record.payload)),
  ) as unknown;
}
export function request(value: unknown, record: StorageRecord): StorageFileRequest {
  const input = fields(value);
  if (
    input.object_id !== record.object_id ||
    input.revision_id !== record.revision_id ||
    !storageIdentifier(input.object_id) ||
    !storageIdentifier(input.revision_id) ||
    typeof input.size_bytes !== 'number' ||
    !Number.isSafeInteger(input.size_bytes) ||
    input.size_bytes < 0 ||
    input.size_bytes > maxStorageFileBytes ||
    typeof input.metadata !== 'string'
  )
    return invalid();
  storageJSONBytes(input.metadata, 8 << 10);
  const previous = input.previous === undefined || input.previous === null ? [] : input.previous;
  if (
    !Array.isArray(previous) ||
    JSON.stringify(previous) !== JSON.stringify(record.previous ?? [])
  )
    return invalid();
  return {
    object_id: input.object_id,
    revision_id: input.revision_id,
    size_bytes: input.size_bytes,
    metadata: input.metadata,
    ...(previous.length ? { previous: [...(previous as string[])] } : {}),
  };
}
export async function digest(bytes: Uint8Array<ArrayBuffer>): Promise<string> {
  const sum = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes));
  return [...sum].map((value) => value.toString(16).padStart(2, '0')).join('');
}
