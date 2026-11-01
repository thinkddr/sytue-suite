/* SPDX-License-Identifier: Apache-2.0 */
import {
  deleteOfflineValue,
  exclusiveOfflineRecord,
  isOfflineStorageEncrypted,
  readOfflineValue,
  writeOfflineValue,
} from '../storage.js';
import { MatrixSecurityError, type MatrixReference, type MatrixStorageScope } from './types.js';
import {
  StorageRecordError,
  createStorageRecord,
  decodeStorageRecord,
  storageIdentifier,
  storageJSONBytes,
  storageReference,
  storageScope,
  type StorageRecord,
} from './storage.js';
import {
  StorageSourceChangedError,
  createBlobId,
  downloadStorageBlock,
  isValidSha256Hex,
  maxCipherBlockBytes,
  prepareStorageBlock,
  putPreparedStorageBlock,
  restorePendingStorageBlock,
  sha256Hex,
  validateBlobReceipt,
  validateMatrixFileDescriptor,
  validateStorageBlock,
  type BlobReceipt,
  type CipherBlobGateway,
  type StorageBlock,
  type StorageMatrixFile,
} from './storage-blocks.js';

export const fileFanout = 16;
export const fileLevels = 6;
export const maxFileBlocks = 16_777_216;
export const maxStorageFileBytes = maxFileBlocks * maxCipherBlockBytes;
export const maxFileMetadataBytes = 8 << 10;
export const fileKind = 'org.sytue.file.v1';
export const filePageKind = 'org.sytue.file.page.v1';
export const pendingUploadsNamespace = 'storage-file-uploads-v1';

/**
 * PRIVATE file request metadata. `metadata` is standard base64 of UTF-8 JSON bytes
 * (matching Go's `[]byte` JSON encoding) so domain numbers > 2^53 are never rounded.
 */
export interface StorageFileRequest {
  object_id: string;
  revision_id: string;
  size_bytes: number;
  metadata: string;
  previous?: string[];
}

export interface StorageFileBlockDescriptor {
  blob: BlobReceipt;
  matrix_file: StorageMatrixFile;
  plain_sha256: string;
}

export interface StorageFileLink {
  block?: StorageFileBlockDescriptor;
  event?: MatrixReference;
}

export interface StorageFilePage {
  kind: typeof filePageKind;
  level: number;
  index: number;
  nodes: StorageFileLink[];
}

export interface StorageFileManifest {
  kind: typeof fileKind;
  request: StorageFileRequest;
  sha256: string;
  root: StorageFilePage;
}

/** Bounded random-access immutable source (analogous to Go `io.ReaderAt`). */
export interface StorageFileSource {
  readonly size: number;
  slice: (offset: number, length: number, signal?: AbortSignal) => Promise<Uint8Array<ArrayBuffer>>;
}

export function bytesStorageFileSource(bytes: Uint8Array): StorageFileSource {
  return {
    get size() {
      return bytes.byteLength;
    },
    slice(offset, length, signal) {
      try {
        signal?.throwIfAborted();
        if (offset < 0 || length < 0 || offset > bytes.byteLength) {
          throw new StorageRecordError();
        }
        if (offset + length > bytes.byteLength) {
          throw new StorageSourceChangedError();
        }
        const chunk = new Uint8Array(length);
        chunk.set(bytes.subarray(offset, offset + length));
        return Promise.resolve(chunk);
      } catch (err) {
        return Promise.reject(err instanceof Error ? err : new Error(String(err)));
      }
    },
  };
}

export function blobStorageFileSource(blob: Blob): StorageFileSource {
  return {
    get size() {
      return blob.size;
    },
    async slice(offset, length, signal) {
      signal?.throwIfAborted();
      if (offset < 0 || length < 0) throw new StorageRecordError();
      if (offset > blob.size || offset + length > blob.size) {
        throw new StorageSourceChangedError();
      }
      const buffer = await blob.slice(offset, offset + length).arrayBuffer();
      signal?.throwIfAborted();
      if (buffer.byteLength !== length) throw new StorageSourceChangedError();
      return new Uint8Array(buffer);
    },
  };
}

export interface StorageUploadJournal {
  isEncrypted: () => boolean;
  exclusive: <R>(
    owner: string,
    namespace: string,
    id: string,
    task: () => Promise<R>,
    signal?: AbortSignal,
  ) => Promise<R>;
  read: <T>(owner: string, namespace: string, id: string) => Promise<T | null>;
  write: <T>(owner: string, namespace: string, id: string, value: T) => Promise<void>;
  delete: (owner: string, namespace: string, id: string) => Promise<void>;
}

export const defaultStorageUploadJournal: StorageUploadJournal = {
  isEncrypted: isOfflineStorageEncrypted,
  exclusive: exclusiveOfflineRecord,
  read: readOfflineValue,
  write: writeOfflineValue,
  delete: deleteOfflineValue,
};

export interface StorageManifestRecordIO {
  readonly scope: MatrixStorageScope;
  assertOpen: () => void;
  putRecord: (
    transaction: string,
    record: StorageRecord,
    signal?: AbortSignal,
  ) => Promise<MatrixReference>;
  readRecord: (
    reference: MatrixReference,
    object: string,
    role: StorageRecord['role'],
    signal?: AbortSignal,
  ) => Promise<StorageRecord>;
}

/**
 * Streaming FIPS 180-4 SHA-256 state so multi-gigabyte file uploads and downloads
 * verify whole-file digests with O(1) memory, matching Go's `sha256.New()`.
 */
const SHA256_K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);

export class StreamingSha256 {
  private readonly h = new Uint32Array([
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
  ]);
  private readonly block = new Uint8Array(64);
  private readonly words = new Uint32Array(64);
  private buffered = 0;
  private totalBytes = 0;

  update(chunk: Uint8Array): void {
    let offset = 0;
    this.totalBytes += chunk.byteLength;
    if (this.buffered > 0) {
      const take = Math.min(64 - this.buffered, chunk.byteLength);
      this.block.set(chunk.subarray(0, take), this.buffered);
      this.buffered += take;
      offset += take;
      if (this.buffered === 64) {
        this.compress(this.block, 0);
        this.buffered = 0;
      }
    }
    while (offset + 64 <= chunk.byteLength) {
      this.compress(chunk, offset);
      offset += 64;
    }
    if (offset < chunk.byteLength) {
      this.block.set(chunk.subarray(offset), 0);
      this.buffered = chunk.byteLength - offset;
    }
  }

  digestHex(): string {
    const state = new Uint32Array(this.h);
    const tail = new Uint8Array(64);
    tail.set(this.block.subarray(0, this.buffered));
    tail[this.buffered] = 0x80;
    const view = new DataView(tail.buffer);
    if (this.buffered >= 56) {
      this.compressInto(state, tail, 0);
      tail.fill(0);
    }
    const bitHi = Math.floor(this.totalBytes / 0x20000000);
    const bitLo = (this.totalBytes << 3) >>> 0;
    view.setUint32(56, bitHi, false);
    view.setUint32(60, bitLo, false);
    this.compressInto(state, tail, 0);
    tail.fill(0);
    let hex = '';
    for (const word of state) {
      hex += (word >>> 0).toString(16).padStart(8, '0');
    }
    return hex;
  }

  clear(): void {
    this.block.fill(0);
    this.words.fill(0);
  }

  private compress(source: Uint8Array, offset: number): void {
    this.compressInto(this.h, source, offset);
  }

  private compressInto(state: Uint32Array, source: Uint8Array, offset: number): void {
    const w = this.words;
    for (let i = 0; i < 16; i++) {
      const base = offset + i * 4;
      w[i] =
        ((source[base]! << 24) |
          (source[base + 1]! << 16) |
          (source[base + 2]! << 8) |
          source[base + 3]!) >>>
        0;
    }
    for (let i = 16; i < 64; i++) {
      const w15 = w[i - 15]!;
      const w2 = w[i - 2]!;
      const s0 = (((w15 >>> 7) | (w15 << 25)) ^ ((w15 >>> 18) | (w15 << 14)) ^ (w15 >>> 3)) >>> 0;
      const s1 = (((w2 >>> 17) | (w2 << 15)) ^ ((w2 >>> 19) | (w2 << 13)) ^ (w2 >>> 10)) >>> 0;
      w[i] = (w[i - 16]! + s0 + w[i - 7]! + s1) >>> 0;
    }
    let a = state[0]!;
    let b = state[1]!;
    let c = state[2]!;
    let d = state[3]!;
    let e = state[4]!;
    let f = state[5]!;
    let g = state[6]!;
    let h = state[7]!;
    for (let i = 0; i < 64; i++) {
      const s1 =
        (((e >>> 6) | (e << 26)) ^ ((e >>> 11) | (e << 21)) ^ ((e >>> 25) | (e << 7))) >>> 0;
      const ch = ((e & f) ^ (~e & g)) >>> 0;
      const temp1 = (h + s1 + ch + SHA256_K[i]! + w[i]!) >>> 0;
      const s0 =
        (((a >>> 2) | (a << 30)) ^ ((a >>> 13) | (a << 19)) ^ ((a >>> 22) | (a << 10))) >>> 0;
      const maj = ((a & b) ^ (a & c) ^ (b & c)) >>> 0;
      const temp2 = (s0 + maj) >>> 0;
      h = g;
      g = f;
      f = e;
      e = (d + temp1) >>> 0;
      d = c;
      c = b;
      b = a;
      a = (temp1 + temp2) >>> 0;
    }
    state[0] = (state[0]! + a) >>> 0;
    state[1] = (state[1]! + b) >>> 0;
    state[2] = (state[2]! + c) >>> 0;
    state[3] = (state[3]! + d) >>> 0;
    state[4] = (state[4]! + e) >>> 0;
    state[5] = (state[5]! + f) >>> 0;
    state[6] = (state[6]! + g) >>> 0;
    state[7] = (state[7]! + h) >>> 0;
  }
}

const encoder = new TextEncoder();
const decoder = new TextDecoder('utf-8', { fatal: true });

export function fileBlocks(sizeBytes: number): number {
  if (sizeBytes === 0) return 1;
  return 1 + Math.floor((sizeBytes - 1) / maxCipherBlockBytes);
}

export function fileBlockBytes(sizeBytes: number, index: number): number {
  return Math.min(maxCipherBlockBytes, Math.max(0, sizeBytes - index * maxCipherBlockBytes));
}

export function filePageSpan(level: number): number {
  let span = 1;
  for (let i = 0; i < level + 1; i++) span *= fileFanout;
  return span;
}

export function fileRootLevel(count: number): number {
  let level = 0;
  while (count > filePageSpan(level)) level++;
  return level;
}

export function validateStorageFileRequest(
  value: unknown,
  expectedScope: MatrixStorageScope,
): StorageFileRequest {
  const scope = storageScope(expectedScope);
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new StorageRecordError();
  const raw = value as Record<string, unknown>;
  if (
    typeof raw.size_bytes !== 'number' ||
    !Number.isSafeInteger(raw.size_bytes) ||
    raw.size_bytes < 0 ||
    raw.size_bytes > maxStorageFileBytes
  ) {
    throw new StorageRecordError();
  }
  storageJSONBytes(raw.metadata, maxFileMetadataBytes);
  const previous = raw.previous === undefined || raw.previous === null ? [] : raw.previous;
  if (
    !Array.isArray(previous) ||
    typeof raw.object_id !== 'string' ||
    typeof raw.revision_id !== 'string'
  ) {
    throw new StorageRecordError();
  }
  // Delegate identifier + previous validation to `createStorageRecord` just like Go.
  const probe = createStorageRecord(
    scope,
    raw.object_id,
    raw.revision_id,
    'manifest',
    encoder.encode('{}'),
    previous as string[],
  );
  return {
    object_id: probe.object_id,
    revision_id: probe.revision_id,
    size_bytes: raw.size_bytes,
    metadata: raw.metadata as string,
    ...(probe.previous?.length ? { previous: [...probe.previous] } : {}),
  };
}

function validateFileBlockDescriptor(
  value: unknown,
  scope: MatrixStorageScope,
): StorageFileBlockDescriptor {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new StorageRecordError();
  const raw = value as Record<string, unknown>;
  if (!isValidSha256Hex(raw.plain_sha256)) throw new StorageRecordError();
  const blob = validateBlobReceipt(raw.blob, scope.product);
  const matrixFile = validateMatrixFileDescriptor(raw.matrix_file, blob.sha256);
  return {
    blob,
    matrix_file: matrixFile,
    plain_sha256: raw.plain_sha256,
  };
}

export function validateStorageFilePage(
  value: unknown,
  scope: MatrixStorageScope,
  request: StorageFileRequest,
  level: number,
  index: number,
): StorageFilePage {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new StorageRecordError();
  const page = value as Record<string, unknown>;
  const count = fileBlocks(request.size_bytes);
  if (
    level < 0 ||
    level >= fileLevels ||
    index < 0 ||
    index > count ||
    page.kind !== filePageKind ||
    page.level !== level ||
    page.index !== index ||
    !Array.isArray(page.nodes)
  ) {
    throw new StorageRecordError();
  }
  const span = filePageSpan(level);
  if (index > Math.floor((count - 1) / span)) {
    throw new StorageRecordError();
  }
  const first = index * span;
  const childSpan = span / fileFanout;
  const wanted = 1 + Math.floor((Math.min(span, count - first) - 1) / childSpan);
  if (page.nodes.length !== wanted) {
    throw new StorageRecordError();
  }
  const nodes: StorageFileLink[] = [];
  for (const rawNode of page.nodes as unknown[]) {
    if (!rawNode || typeof rawNode !== 'object' || Array.isArray(rawNode)) {
      throw new StorageRecordError();
    }
    const node = rawNode as Record<string, unknown>;
    if (level === 0) {
      if (node.event !== undefined && node.event !== null) throw new StorageRecordError();
      nodes.push({ block: validateFileBlockDescriptor(node.block, scope) });
    } else {
      if (node.block !== undefined && node.block !== null) throw new StorageRecordError();
      nodes.push({ event: storageReference(node.event) });
    }
  }
  return {
    kind: filePageKind,
    level,
    index,
    nodes,
  };
}

export function decodeStorageFileManifest(
  recordInput: StorageRecord,
  expectedScope: MatrixStorageScope,
  expectedObject: string,
  expectedRevision: string,
): StorageFileManifest {
  const scope = storageScope(expectedScope);
  const record = decodeStorageRecord(recordInput, scope);
  if (
    record.object_id !== expectedObject ||
    record.revision_id !== expectedRevision ||
    record.role !== 'manifest'
  ) {
    throw new StorageRecordError();
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(decoder.decode(storageJSONBytes(record.payload)));
  } catch {
    throw new StorageRecordError();
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new StorageRecordError();
  }
  const raw = parsed as Record<string, unknown>;
  if (raw.kind !== fileKind || !isValidSha256Hex(raw.sha256)) {
    throw new StorageRecordError();
  }
  const request = validateStorageFileRequest(raw.request, scope);
  if (
    request.object_id !== expectedObject ||
    request.revision_id !== expectedRevision ||
    JSON.stringify(request.previous ?? []) !== JSON.stringify(record.previous ?? [])
  ) {
    throw new StorageRecordError();
  }
  const rootLevel = fileRootLevel(fileBlocks(request.size_bytes));
  const root = validateStorageFilePage(raw.root, scope, request, rootLevel, 0);
  return {
    kind: fileKind,
    request,
    sha256: raw.sha256,
    root,
  };
}

function descriptorToBlock(
  descriptor: StorageFileBlockDescriptor,
  scope: MatrixStorageScope,
  object: string,
  revision: string,
  part: number,
): StorageBlock {
  return validateStorageBlock(
    {
      scope,
      object_id: object,
      revision_id: revision,
      part,
      blob: descriptor.blob,
      matrix_file: descriptor.matrix_file,
    },
    scope,
    object,
    revision,
    part,
  );
}

interface PendingFileBlock {
  block: StorageBlock;
  plain_sha256: string;
}

interface StorageFileUploadState {
  v: 1;
  scope: MatrixStorageScope;
  request: StorageFileRequest;
  salt: string;
  source_sha256: string;
  prefix_sha256: string;
  next: number;
  frontier: StorageFileLink[][];
  pages: number[];
  pending?: PendingFileBlock;
  file?: StorageFileManifest;
  published?: MatrixReference;
}

function journalOwner(scope: MatrixStorageScope): string {
  return `${scope.tenant}\u0000${scope.subject}\u0000${scope.product}`;
}

async function uploadTransaction(
  salt: string,
  kind: 'page' | 'root',
  level: number,
  index: number,
): Promise<string> {
  return `file_${await sha256Hex(encoder.encode(`${salt}\u0000${kind}\u0000${level}\u0000${index}`))}`;
}

function sameRequest(left: StorageFileRequest, right: StorageFileRequest): boolean {
  return (
    left.object_id === right.object_id &&
    left.revision_id === right.revision_id &&
    left.size_bytes === right.size_bytes &&
    left.metadata === right.metadata &&
    JSON.stringify(left.previous ?? []) === JSON.stringify(right.previous ?? [])
  );
}

function validateUploadState(
  raw: unknown,
  scope: MatrixStorageScope,
  expectedRequest: StorageFileRequest,
): StorageFileUploadState {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new StorageRecordError();
  const s = raw as Record<string, unknown>;
  const stateScope = storageScope(s.scope);
  if (
    s.v !== 1 ||
    stateScope.tenant !== scope.tenant ||
    stateScope.subject !== scope.subject ||
    stateScope.product !== scope.product ||
    !storageIdentifier(s.salt) ||
    !isValidSha256Hex(s.source_sha256) ||
    !isValidSha256Hex(s.prefix_sha256) ||
    typeof s.next !== 'number' ||
    !Number.isSafeInteger(s.next) ||
    !Array.isArray(s.frontier) ||
    s.frontier.length !== fileLevels ||
    !Array.isArray(s.pages) ||
    s.pages.length !== fileLevels
  ) {
    throw new StorageRecordError();
  }
  const request = validateStorageFileRequest(s.request, scope);
  if (!sameRequest(request, expectedRequest)) {
    throw new StorageSourceChangedError();
  }
  const totalBlocks = fileBlocks(request.size_bytes);
  if (s.next < 0 || s.next > totalBlocks) throw new StorageRecordError();

  const frontier: StorageFileLink[][] = [];
  const pages: number[] = [];
  for (let level = 0; level < fileLevels; level++) {
    const levelLinks = (s.frontier[level] as unknown[] | null | undefined) ?? [];
    const pageCount = (s.pages as unknown[])[level];
    if (
      !Array.isArray(levelLinks) ||
      levelLinks.length > fileFanout ||
      typeof pageCount !== 'number' ||
      !Number.isSafeInteger(pageCount) ||
      pageCount < 0 ||
      pageCount > maxFileBlocks
    ) {
      throw new StorageRecordError();
    }
    const parsedLinks: StorageFileLink[] = [];
    for (const rawNode of levelLinks) {
      if (!rawNode || typeof rawNode !== 'object' || Array.isArray(rawNode)) {
        throw new StorageRecordError();
      }
      const node = rawNode as Record<string, unknown>;
      if (level === 0) {
        if (node.event !== undefined && node.event !== null) throw new StorageRecordError();
        parsedLinks.push({ block: validateFileBlockDescriptor(node.block, scope) });
      } else {
        if (node.block !== undefined && node.block !== null) throw new StorageRecordError();
        parsedLinks.push({ event: storageReference(node.event) });
      }
    }
    frontier.push(parsedLinks);
    pages.push(pageCount);
  }

  let pending: PendingFileBlock | undefined;
  if (s.pending !== undefined && s.pending !== null) {
    if (typeof s.pending !== 'object' || Array.isArray(s.pending) || s.next >= totalBlocks) {
      throw new StorageRecordError();
    }
    const rawPending = s.pending as Record<string, unknown>;
    if (!isValidSha256Hex(rawPending.plain_sha256)) throw new StorageRecordError();
    const block = validateStorageBlock(
      rawPending.block,
      scope,
      request.object_id,
      request.revision_id,
      s.next,
    );
    if (block.blob.size_bytes !== fileBlockBytes(request.size_bytes, s.next)) {
      throw new StorageRecordError();
    }
    pending = { block, plain_sha256: rawPending.plain_sha256 };
  }

  let file: StorageFileManifest | undefined;
  if (s.file !== undefined && s.file !== null) {
    if (s.next !== totalBlocks || pending !== undefined) throw new StorageRecordError();
    if (typeof s.file !== 'object' || Array.isArray(s.file)) throw new StorageRecordError();
    const rawFile = s.file as Record<string, unknown>;
    if (rawFile.kind !== fileKind || rawFile.sha256 !== s.source_sha256) {
      throw new StorageRecordError();
    }
    const fileReq = validateStorageFileRequest(rawFile.request, scope);
    if (!sameRequest(fileReq, request)) throw new StorageRecordError();
    const rootLevel = fileRootLevel(totalBlocks);
    const root = validateStorageFilePage(rawFile.root, scope, request, rootLevel, 0);
    file = { kind: fileKind, request: fileReq, sha256: s.source_sha256, root };
  }

  let published: MatrixReference | undefined;
  if (s.published !== undefined && s.published !== null) {
    if (!file) throw new StorageRecordError();
    published = storageReference(s.published);
  }

  return {
    v: 1,
    scope,
    request,
    salt: s.salt,
    source_sha256: s.source_sha256,
    prefix_sha256: s.prefix_sha256,
    next: s.next,
    frontier,
    pages,
    ...(pending ? { pending } : {}),
    ...(file ? { file } : {}),
    ...(published ? { published } : {}),
  };
}

async function hashSourcePrefix(
  io: StorageManifestRecordIO,
  source: StorageFileSource,
  expectedTotalSize: number,
  prefixBytes: number,
  hasher: StreamingSha256,
  signal?: AbortSignal,
): Promise<void> {
  if (source.size !== expectedTotalSize) throw new StorageSourceChangedError();
  const chunkBudget = 64 << 10;
  for (let offset = 0; offset < prefixBytes;) {
    signal?.throwIfAborted();
    io.assertOpen();
    if (source.size !== expectedTotalSize) throw new StorageSourceChangedError();
    const wanted = Math.min(chunkBudget, prefixBytes - offset);
    const slice = await source.slice(offset, wanted, signal);
    if (slice.byteLength !== wanted) {
      slice.fill(0);
      throw new StorageSourceChangedError();
    }
    hasher.update(slice);
    slice.fill(0);
    offset += wanted;
  }
}

function assertExactSourceSize(
  io: StorageManifestRecordIO,
  source: StorageFileSource,
  expectedSize: number,
  signal?: AbortSignal,
): void {
  signal?.throwIfAborted();
  io.assertOpen();
  if (source.size !== expectedSize) throw new StorageSourceChangedError();
}

async function readManifestPage(
  io: StorageManifestRecordIO,
  reference: MatrixReference,
  request: StorageFileRequest,
  level: number,
  index: number,
  signal?: AbortSignal,
): Promise<StorageFilePage> {
  const record = await io.readRecord(reference, request.object_id, 'manifest', signal);
  if (record.revision_id !== request.revision_id) throw new StorageRecordError();
  let parsed: unknown;
  try {
    parsed = JSON.parse(decoder.decode(storageJSONBytes(record.payload)));
  } catch {
    throw new StorageRecordError();
  }
  return validateStorageFilePage(parsed, io.scope, request, level, index);
}

export async function readStorageFileRoot(
  io: StorageManifestRecordIO,
  reference: MatrixReference,
  object: string,
  revision: string,
  signal?: AbortSignal,
): Promise<StorageFileManifest> {
  signal?.throwIfAborted();
  io.assertOpen();
  const record = await io.readRecord(reference, object, 'manifest', signal);
  signal?.throwIfAborted();
  io.assertOpen();
  return decodeStorageFileManifest(record, io.scope, object, revision);
}

export async function storageFileInfo(
  io: StorageManifestRecordIO,
  reference: MatrixReference,
  object: string,
  revision: string,
  signal?: AbortSignal,
): Promise<StorageFileRequest> {
  const manifest = await readStorageFileRoot(io, reference, object, revision, signal);
  return manifest.request;
}

async function walkStorageFile(
  io: StorageManifestRecordIO,
  manifest: StorageFileManifest,
  page: StorageFilePage,
  level: number,
  index: number,
  visitLeaf: (blockIndex: number, descriptor: StorageFileBlockDescriptor) => Promise<void>,
  signal?: AbortSignal,
): Promise<void> {
  signal?.throwIfAborted();
  io.assertOpen();
  validateStorageFilePage(page, io.scope, manifest.request, level, index);
  for (let j = 0; j < page.nodes.length; j++) {
    const nextIndex = index * fileFanout + j;
    const node = page.nodes[j]!;
    if (level === 0) {
      await visitLeaf(nextIndex, node.block!);
    } else {
      const child = await readManifestPage(
        io,
        node.event!,
        manifest.request,
        level - 1,
        nextIndex,
        signal,
      );
      await walkStorageFile(io, manifest, child, level - 1, nextIndex, visitLeaf, signal);
    }
  }
}

/**
 * Authenticates every manifest page, block position, block plaintext digest and
 * whole-file SHA-256 before completing. `sink` is optional (e.g., upload
 * verification discards plaintext immediately after authenticating each block).
 */
export async function downloadStorageFile(
  io: StorageManifestRecordIO,
  gateway: CipherBlobGateway,
  reference: MatrixReference,
  object: string,
  revision: string,
  sink?: (chunk: Uint8Array<ArrayBuffer>, blockIndex: number) => void | Promise<void>,
  signal?: AbortSignal,
): Promise<StorageFileRequest> {
  const manifest = await readStorageFileRoot(io, reference, object, revision, signal);
  const wholeHash = new StreamingSha256();
  try {
    await walkStorageFile(
      io,
      manifest,
      manifest.root,
      manifest.root.level,
      0,
      async (blockIndex, descriptor) => {
        const block = descriptorToBlock(descriptor, io.scope, object, revision, blockIndex);
        if (block.blob.size_bytes !== fileBlockBytes(manifest.request.size_bytes, blockIndex)) {
          throw new StorageRecordError();
        }
        const plain = await downloadStorageBlock(
          gateway,
          io.scope,
          block,
          object,
          revision,
          blockIndex,
          signal,
        );
        try {
          if ((await sha256Hex(plain)) !== descriptor.plain_sha256) {
            throw new MatrixSecurityError('identity');
          }
          signal?.throwIfAborted();
          io.assertOpen();
          wholeHash.update(plain);
          if (sink) {
            const copy = new Uint8Array(plain.byteLength);
            copy.set(plain);
            await sink(copy, blockIndex);
          }
        } finally {
          plain.fill(0);
        }
      },
      signal,
    );
    if (wholeHash.digestHex() !== manifest.sha256) {
      throw new MatrixSecurityError('identity');
    }
    signal?.throwIfAborted();
    io.assertOpen();
    return manifest.request;
  } finally {
    wholeHash.clear();
  }
}

async function leafAt(
  io: StorageManifestRecordIO,
  manifest: StorageFileManifest,
  blockIndex: number,
  cache: Map<string, StorageFilePage>,
  signal?: AbortSignal,
): Promise<StorageFileBlockDescriptor> {
  let page = manifest.root;
  let level = manifest.root.level;
  let position = 0;
  while (level > 0) {
    const span = filePageSpan(level - 1);
    const child = Math.floor((blockIndex - position * filePageSpan(level)) / span);
    const childNode = child >= 0 && child < page.nodes.length ? page.nodes[child] : undefined;
    if (!childNode?.event) {
      throw new StorageRecordError();
    }
    const ref = childNode.event;
    const cacheKey = JSON.stringify([ref.room_id, ref.event_id]);
    const nextPosition = position * fileFanout + child;
    let nextPage = cache.get(cacheKey);
    if (!nextPage) {
      nextPage = await readManifestPage(io, ref, manifest.request, level - 1, nextPosition, signal);
      cache.set(cacheKey, nextPage);
    } else {
      validateStorageFilePage(nextPage, io.scope, manifest.request, level - 1, nextPosition);
    }
    level--;
    position = nextPosition;
    page = nextPage;
  }
  const slot = blockIndex % fileFanout;
  const leafNode = slot >= 0 && slot < page.nodes.length ? page.nodes[slot] : undefined;
  if (!leafNode?.block) {
    throw new StorageRecordError();
  }
  return leafNode.block;
}

/**
 * Authenticated bounded random-access read (<= 8 MiB) matching Go's
 * `ReadStorageFileRange`, suitable for media seeking and range previews.
 */
export async function readStorageFileRange(
  io: StorageManifestRecordIO,
  gateway: CipherBlobGateway,
  reference: MatrixReference,
  object: string,
  revision: string,
  offset: number,
  length: number,
  signal?: AbortSignal,
): Promise<Uint8Array<ArrayBuffer>> {
  if (
    !Number.isSafeInteger(offset) ||
    !Number.isSafeInteger(length) ||
    offset < 0 ||
    length < 0 ||
    length > maxCipherBlockBytes
  ) {
    throw new StorageRecordError();
  }
  const manifest = await readStorageFileRoot(io, reference, object, revision, signal);
  if (offset > manifest.request.size_bytes || length > manifest.request.size_bytes - offset) {
    throw new StorageRecordError();
  }
  const output = new Uint8Array(length);
  let written = 0;
  const cache = new Map<string, StorageFilePage>();
  const end = offset + length;
  try {
    for (let cursor = offset; cursor < end;) {
      const blockIndex = Math.floor(cursor / maxCipherBlockBytes);
      const descriptor = await leafAt(io, manifest, blockIndex, cache, signal);
      const block = descriptorToBlock(descriptor, io.scope, object, revision, blockIndex);
      if (block.blob.size_bytes !== fileBlockBytes(manifest.request.size_bytes, blockIndex)) {
        throw new StorageRecordError();
      }
      const data = await downloadStorageBlock(
        gateway,
        io.scope,
        block,
        object,
        revision,
        blockIndex,
        signal,
      );
      try {
        if ((await sha256Hex(data)) !== descriptor.plain_sha256) {
          throw new MatrixSecurityError('identity');
        }
        signal?.throwIfAborted();
        io.assertOpen();
        const start = cursor - blockIndex * maxCipherBlockBytes;
        const count = Math.min(data.byteLength - start, end - cursor);
        if (count <= 0) throw new StorageRecordError();
        output.set(data.subarray(start, start + count), written);
        written += count;
        cursor += count;
      } finally {
        data.fill(0);
      }
    }
    signal?.throwIfAborted();
    io.assertOpen();
    return output;
  } catch (error) {
    output.fill(0);
    throw error;
  }
}

/**
 * Resumable encrypted multi-block file upload matching Go's `UploadStorageFile`.
 * Checkpoints every descriptor in the sealed offline store before network I/O,
 * verifies the complete uploaded replacement end-to-end before retiring the
 * journal, and never deletes or mutates the caller's source.
 */
export async function uploadStorageFile(
  io: StorageManifestRecordIO,
  gateway: CipherBlobGateway,
  job: string,
  requestInput: StorageFileRequest,
  source: StorageFileSource,
  options?: {
    journal?: StorageUploadJournal;
    signal?: AbortSignal;
  },
): Promise<MatrixReference> {
  const journal = options?.journal ?? defaultStorageUploadJournal;
  const signal = options?.signal;
  signal?.throwIfAborted();
  io.assertOpen();
  if (!journal.isEncrypted() || !storageIdentifier(job)) {
    throw new StorageRecordError();
  }
  const scope = storageScope(io.scope);
  const request = validateStorageFileRequest(requestInput, scope);
  const owner = journalOwner(scope);

  const checkpoint = async (state: StorageFileUploadState): Promise<void> => {
    signal?.throwIfAborted();
    io.assertOpen();
    await journal.write(owner, pendingUploadsNamespace, job, state);
  };

  const saveManifestRecord = async (
    transaction: string,
    payloadObject: unknown,
    previous: string[] = [],
  ): Promise<MatrixReference> => {
    signal?.throwIfAborted();
    io.assertOpen();
    const record = createStorageRecord(
      scope,
      request.object_id,
      request.revision_id,
      'manifest',
      encoder.encode(JSON.stringify(payloadObject)),
      previous,
    );
    const ref = await io.putRecord(transaction, record, signal);
    signal?.throwIfAborted();
    io.assertOpen();
    return ref;
  };

  const flushFrontier = async (state: StorageFileUploadState, level: number): Promise<void> => {
    if (level >= fileLevels - 1 || state.frontier[level]!.length === 0) {
      throw new StorageRecordError();
    }
    if (state.frontier[level + 1]!.length === fileFanout) {
      await flushFrontier(state, level + 1);
    }
    const page: StorageFilePage = {
      kind: filePageKind,
      level,
      index: state.pages[level]!,
      nodes: [...state.frontier[level]!],
    };
    const txn = await uploadTransaction(state.salt, 'page', level, page.index);
    const ref = await saveManifestRecord(txn, page);
    state.pages[level] = state.pages[level]! + 1;
    state.frontier[level] = [];
    state.frontier[level + 1]!.push({ event: ref });
    await checkpoint(state);
  };

  return journal.exclusive(
    owner,
    pendingUploadsNamespace,
    job,
    async () => {
      const existing = await journal.read<unknown>(owner, pendingUploadsNamespace, job);
      let state: StorageFileUploadState;
      if (existing !== null) {
        state = validateUploadState(existing, scope, request);
      } else {
        const sourceHasher = new StreamingSha256();
        try {
          await hashSourcePrefix(
            io,
            source,
            request.size_bytes,
            request.size_bytes,
            sourceHasher,
            signal,
          );
          assertExactSourceSize(io, source, request.size_bytes, signal);
          const emptySha = await sha256Hex(new Uint8Array(0));
          state = {
            v: 1,
            scope,
            request,
            salt: createBlobId(),
            source_sha256: sourceHasher.digestHex(),
            prefix_sha256: emptySha,
            next: 0,
            frontier: Array.from({ length: fileLevels }, () => []),
            pages: Array.from({ length: fileLevels }, () => 0),
          };
          await checkpoint(state);
        } finally {
          sourceHasher.clear();
        }
      }

      if (state.file) {
        const checkHasher = new StreamingSha256();
        try {
          await hashSourcePrefix(
            io,
            source,
            request.size_bytes,
            request.size_bytes,
            checkHasher,
            signal,
          );
          if (checkHasher.digestHex() !== state.source_sha256) {
            throw new StorageSourceChangedError();
          }
          assertExactSourceSize(io, source, request.size_bytes, signal);
        } finally {
          checkHasher.clear();
        }
      } else {
        const prefixHasher = new StreamingSha256();
        try {
          const doneBytes = Math.min(state.next * maxCipherBlockBytes, request.size_bytes);
          await hashSourcePrefix(io, source, request.size_bytes, doneBytes, prefixHasher, signal);
          if (prefixHasher.digestHex() !== state.prefix_sha256) {
            throw new StorageSourceChangedError();
          }
          const totalBlocks = fileBlocks(request.size_bytes);
          while (state.next < totalBlocks) {
            signal?.throwIfAborted();
            io.assertOpen();
            if (state.frontier[0]!.length === fileFanout) {
              await flushFrontier(state, 0);
            }
            const blockSize = fileBlockBytes(request.size_bytes, state.next);
            const blockOffset = state.next * maxCipherBlockBytes;
            if (source.size !== request.size_bytes) throw new StorageSourceChangedError();
            const plainChunk = await source.slice(blockOffset, blockSize, signal);
            if (plainChunk.byteLength !== blockSize) {
              plainChunk.fill(0);
              throw new StorageSourceChangedError();
            }
            prefixHasher.update(plainChunk);
            let ciphertext: Uint8Array<ArrayBuffer> | undefined;
            try {
              if (!state.pending) {
                const prepared = await prepareStorageBlock(
                  scope,
                  request.object_id,
                  request.revision_id,
                  state.next,
                  plainChunk,
                );
                ciphertext = prepared.ciphertext;
                state.pending = {
                  block: prepared.block,
                  plain_sha256: prepared.plainSha256,
                };
                await checkpoint(state);
              } else {
                ciphertext = await restorePendingStorageBlock(
                  scope,
                  state.pending.block,
                  state.pending.plain_sha256,
                  plainChunk,
                );
              }
              plainChunk.fill(0);
              signal?.throwIfAborted();
              io.assertOpen();
              await putPreparedStorageBlock(gateway, state.pending.block, ciphertext, signal);
              state.frontier[0]!.push({
                block: {
                  blob: state.pending.block.blob,
                  matrix_file: state.pending.block.matrix_file,
                  plain_sha256: state.pending.plain_sha256,
                },
              });
              delete state.pending;
              state.next++;
              state.prefix_sha256 = prefixHasher.digestHex();
              await checkpoint(state);
            } finally {
              plainChunk.fill(0);
              ciphertext?.fill(0);
            }
          }
          assertExactSourceSize(io, source, request.size_bytes, signal);
          if (prefixHasher.digestHex() !== state.source_sha256) {
            throw new StorageSourceChangedError();
          }
          const rootLevel = fileRootLevel(totalBlocks);
          for (let level = 0; level < rootLevel; level++) {
            if (state.frontier[level]!.length !== 0) {
              await flushFrontier(state, level);
            }
          }
          const root = validateStorageFilePage(
            {
              kind: filePageKind,
              level: rootLevel,
              index: 0,
              nodes: state.frontier[rootLevel]!,
            },
            scope,
            request,
            rootLevel,
            0,
          );
          state.file = {
            kind: fileKind,
            request,
            sha256: state.source_sha256,
            root,
          };
          await checkpoint(state);
        } finally {
          prefixHasher.clear();
        }
      }

      if (!state.published) {
        if (!state.file) throw new StorageRecordError();
        const rootTxn = await uploadTransaction(state.salt, 'root', 0, 0);
        const published = await saveManifestRecord(rootTxn, state.file, request.previous ?? []);
        state.published = published;
        await checkpoint(state);
      }

      const published = state.published;
      await downloadStorageFile(
        io,
        gateway,
        published,
        request.object_id,
        request.revision_id,
        undefined,
        signal,
      );
      await journal.delete(owner, pendingUploadsNamespace, job);
      return published;
    },
    signal,
  );
}
