/* SPDX-License-Identifier: Apache-2.0 */
/** Resumable native file.v1 publication. Cache is disposable; callers retain the original. */
import { Attachment, initAsync } from '@matrix-org/matrix-sdk-crypto-wasm';
import { createSHA256 } from 'hash-wasm';
import { decodeCipherBlobReceipt, maxCipherBlockBytes } from '@sytue/config/matrix';
import type { EncryptedOfflineValues } from '../encrypted-storage.js';
import { MatrixSecurityError, type MatrixReference, type MatrixStorageScope } from './types.js';
import type { StorageJobRunner } from './storage-jobs.js';
import {
  createStorageRecord,
  storageIdentifier,
  storagePublicationTarget,
  storageReference,
  storageScope,
  type StoragePublicationTarget,
  type StorageRecord,
} from './storage.js';
import {
  block,
  blockBytes,
  count,
  digest,
  fanout,
  fields,
  hash,
  invalid,
  levels,
  page,
  request,
  rootLevel,
  type FileBlock,
  type FileLink,
  type FilePage,
  type PrivateFile,
  type StorageBlobPort,
  type StorageFileRequest,
} from './storage-file-codec.js';
export const storageSourceBlockBytes = 256 << 10;
/** Owned read buffers; a retained source supplies its previously authenticated checksum. */
export interface StorageFileSource {
  readonly size: number;
  readonly sha256?: string;
  read: (offset: number, length: number, signal: AbortSignal) => Promise<Uint8Array<ArrayBuffer>>;
}
/** Snapshot the immutable Blob range before any asynchronous work or caller override. */
export function storageBlobSource(input: Blob): StorageFileSource {
  const captured = Blob.prototype.slice.call(input, 0, input.size);
  return Object.freeze({
    size: captured.size,
    read: async (offset: number, length: number) =>
      new Uint8Array(await captured.slice(offset, offset + length).arrayBuffer()),
  });
}
interface UploadState {
  v: 1;
  scope: MatrixStorageScope;
  request: StorageFileRequest;
  target: StoragePublicationTarget;
  salt: string;
  sha256: string;
  next: number;
  phase: 'upload' | 'finalize';
  frontier: FileLink[][];
  pages: number[];
  pending?: FileBlock;
  file?: PrivateFile;
  published?: MatrixReference;
}
export interface StorageUploadRecords {
  target: StoragePublicationTarget;
  run: StorageJobRunner;
  assertOpen: () => void;
  assertTarget: (target: StoragePublicationTarget) => void;
  putRecord: (
    transaction: string,
    record: StorageRecord,
    target: StoragePublicationTarget,
    signal: AbortSignal,
  ) => Promise<MatrixReference>;
  verifyCatalog?: (
    reference: MatrixReference,
    published: MatrixReference,
    request: StorageFileRequest,
    sha256: string,
    signal: AbortSignal,
  ) => Promise<void>;
  verify: (
    ref: MatrixReference,
    object: string,
    revision: string,
    signal: AbortSignal,
  ) => Promise<void>;
}
type WritableBlobs = StorageBlobPort & Required<Pick<StorageBlobPort, 'write'>>;
const encoder = new TextEncoder(),
  decoder = new TextDecoder('utf-8', { fatal: true });
const maxJournalBytes = 256 << 10;
export function storageFileRequest(input: unknown, scope: MatrixStorageScope): StorageFileRequest {
  const value = fields(input);
  const record = createStorageRecord(
    scope,
    value.object_id as string,
    value.revision_id as string,
    'manifest',
    encoder.encode('{}'),
    value.previous as string[] | undefined,
  );
  return request(input, record);
}
function equal(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}
/** Strict bounded journals; all descriptors, filenames and target bindings remain encrypted locally. */
function stateFrom(
  bytes: Uint8Array<ArrayBuffer>,
  scope: MatrixStorageScope,
  wanted: StorageFileRequest,
  target: StoragePublicationTarget,
): UploadState {
  if (bytes.length > maxJournalBytes) return invalid();
  const value = fields(JSON.parse(decoder.decode(bytes)) as unknown);
  const savedRequest = storageFileRequest(value.request, scope),
    savedScope = storageScope(value.scope),
    savedTarget = storagePublicationTarget(value.target);
  if (
    value.v !== 1 ||
    !equal(savedScope, scope) ||
    !equal(savedRequest, wanted) ||
    savedTarget.device_id !== target.device_id ||
    !storageIdentifier(value.salt) ||
    !hash(value.sha256) ||
    !Number.isSafeInteger(value.next) ||
    (value.next as number) < 0 ||
    (value.next as number) > count(wanted.size_bytes) ||
    (value.phase !== 'upload' && value.phase !== 'finalize') ||
    !Array.isArray(value.frontier) ||
    value.frontier.length !== levels ||
    !Array.isArray(value.pages) ||
    value.pages.length !== levels
  )
    return invalid();
  const state: UploadState = {
    v: 1,
    scope: savedScope,
    request: savedRequest,
    target: savedTarget,
    salt: value.salt,
    sha256: value.sha256,
    next: value.next as number,
    phase: value.phase,
    frontier: [],
    pages: [],
  };
  if (state.phase === 'finalize' && state.next !== count(wanted.size_bytes)) return invalid();
  let children = state.next;
  for (let level = 0; level < levels; level++) {
    const nodes: unknown = value.frontier[level],
      pages: unknown = value.pages[level];
    if (
      !Array.isArray(nodes) ||
      nodes.length > fanout ||
      !Number.isSafeInteger(pages) ||
      (pages as number) < 0 ||
      (pages as number) > count(wanted.size_bytes) ||
      nodes.length > children
    )
      return invalid();
    const flushed = children - nodes.length;
    if (
      (state.phase === 'upload' && flushed % fanout !== 0) ||
      pages !== Math.ceil(flushed / fanout)
    )
      return invalid();
    state.pages.push(pages);
    state.frontier.push(
      nodes.map((item: unknown, offset: number): FileLink => {
        const link = fields(item);
        if (level === 0) {
          if (link.event !== undefined) return invalid();
          const index = Math.min(state.next, pages * fanout) + offset;
          return { block: block(link.block, scope, blockBytes(wanted.size_bytes, index)) };
        }
        if (link.block !== undefined) return invalid();
        return { event: storageReference(link.event) };
      }),
    );
    children = pages;
  }
  if (value.pending !== undefined) {
    if (state.phase !== 'upload' || state.next >= count(wanted.size_bytes)) return invalid();
    state.pending = block(value.pending, scope, blockBytes(wanted.size_bytes, state.next));
  }
  if (value.file !== undefined) {
    const file = fields(value.file);
    if (
      state.phase !== 'finalize' ||
      state.pending ||
      file.kind !== 'org.sytue.file.v1' ||
      file.sha256 !== state.sha256 ||
      !equal(storageFileRequest(file.request, scope), wanted)
    )
      return invalid();
    state.file = {
      kind: 'org.sytue.file.v1',
      request: savedRequest,
      sha256: state.sha256,
      root: file.root,
    };
    state.file.root = page(file.root, state.file, scope, rootLevel(wanted.size_bytes), 0);
  }
  if (value.published !== undefined) {
    if (!state.file) return invalid();
    state.published = storageReference(value.published);
    if (state.published.room_id !== savedTarget.room_id) return invalid();
  }
  return state;
}
/** One active job per leased Matrix device, across all repositories of that device. */
export class StorageFileUploads {
  private readonly scope: MatrixStorageScope;
  private readonly target: StoragePublicationTarget;
  constructor(
    scope: MatrixStorageScope,
    private readonly records: StorageUploadRecords,
    private readonly blobs: WritableBlobs,
    private readonly cache: EncryptedOfflineValues,
  ) {
    this.scope = Object.freeze(storageScope(scope));
    this.target = Object.freeze(storagePublicationTarget(records.target));
  }
  private check(signal: AbortSignal): void {
    signal.throwIfAborted();
    this.records.assertOpen();
  }
  private async checkpoint(job: string, state: UploadState, signal: AbortSignal): Promise<void> {
    this.check(signal);
    const bytes = encoder.encode(JSON.stringify(state));
    try {
      if (bytes.length > maxJournalBytes) return invalid();
      await this.cache.write('j_' + job, bytes);
      this.check(signal);
    } finally {
      bytes.fill(0);
    }
  }
  private async sourceHash(source: StorageFileSource, signal: AbortSignal): Promise<string> {
    const hasher = await createSHA256();
    try {
      for (let offset = 0; offset < source.size; offset += storageSourceBlockBytes) {
        this.check(signal);
        const bytes = await source.read(
          offset,
          Math.min(storageSourceBlockBytes, source.size - offset),
          signal,
        );
        try {
          this.check(signal);
          if (bytes.length !== Math.min(storageSourceBlockBytes, source.size - offset))
            return invalid();
          hasher.update(bytes);
        } finally {
          bytes.fill(0);
        }
      }
      this.check(signal);
      return hasher.digest('hex');
    } finally {
      hasher.init().update(new Uint8Array(storageSourceBlockBytes)).digest();
    }
  }
  private async save(
    state: UploadState,
    kind: string,
    level: number,
    index: number,
    value: unknown,
    signal: AbortSignal,
  ): Promise<MatrixReference> {
    this.check(signal);
    const transaction =
      'file_' + (await digest(encoder.encode(JSON.stringify([state.salt, kind, level, index]))));
    const record = createStorageRecord(
      this.scope,
      state.request.object_id,
      state.request.revision_id,
      'manifest',
      encoder.encode(JSON.stringify(value)),
      kind === 'root' ? state.request.previous : [],
    );
    this.check(signal);
    const ref = storageReference(
      await this.records.putRecord(transaction, record, state.target, signal),
    );
    this.check(signal);
    if (ref.room_id !== state.target.room_id) return invalid();
    return ref;
  }
  private async flush(
    job: string,
    state: UploadState,
    level: number,
    signal: AbortSignal,
  ): Promise<void> {
    if (level >= levels - 1 || !state.frontier[level]!.length) return invalid();
    if (state.frontier[level + 1]!.length === fanout)
      await this.flush(job, state, level + 1, signal);
    const value: FilePage = {
      kind: 'org.sytue.file.page.v1',
      level,
      index: state.pages[level]!,
      nodes: state.frontier[level]!,
    };
    const ref = await this.save(state, 'page', level, value.index, value, signal);
    state.pages[level]!++;
    state.frontier[level] = [];
    state.frontier[level + 1]!.push({ event: ref });
    await this.checkpoint(job, state, signal);
  }
  private async prepare(
    job: string,
    state: UploadState,
    source: StorageFileSource,
    signal: AbortSignal,
  ): Promise<void> {
    if (state.pending) return;
    await initAsync();
    this.check(signal);
    const offset = state.next * maxCipherBlockBytes;
    const data = await source.read(offset, blockBytes(source.size, state.next), signal);
    let ciphertext: Uint8Array<ArrayBuffer> | undefined;
    try {
      this.check(signal);
      if (data.length !== blockBytes(source.size, state.next)) return invalid();
      const plainHash = await digest(data);
      this.check(signal);
      const encrypted = Attachment.encrypt(data);
      try {
        ciphertext = encrypted.encryptedData as Uint8Array<ArrayBuffer>;
        const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
        const id = [...crypto.getRandomValues(new Uint8Array(32))]
          .map((value) => alphabet[value & 31])
          .join('');
        const descriptor = block(
          {
            blob: {
              id,
              product: this.scope.product,
              size_bytes: ciphertext.length,
              sha256: await digest(ciphertext),
            },
            matrix_file: JSON.parse(encrypted.mediaEncryptionInfo!) as unknown,
            plain_sha256: plainHash,
          },
          this.scope,
          data.length,
        );
        this.check(signal);
        // Data first: a crash can leave an unused encrypted cache entry, never an intent without retry bytes.
        await this.cache.write('b_' + job, ciphertext);
        this.check(signal);
        state.pending = descriptor;
        await this.checkpoint(job, state, signal);
      } finally {
        encrypted.free();
      }
    } finally {
      data.fill(0);
      ciphertext?.fill(0);
    }
  }
  private async leaf(
    job: string,
    state: UploadState,
    source: StorageFileSource,
    signal: AbortSignal,
  ): Promise<void> {
    if (state.frontier[0]!.length === fanout) await this.flush(job, state, 0, signal);
    await this.prepare(job, state, source, signal);
    const pending = state.pending ?? invalid();
    let data = await this.cache.read('b_' + job);
    try {
      this.check(signal);
      if (data === null) {
        // A missing disposable cache can recover the exact provider copy. Never regenerate using an old key/IV.
        data = await this.blobs.read(pending.blob.id, signal);
        this.check(signal);
      }
      if (data.length !== pending.blob.size_bytes || (await digest(data)) !== pending.blob.sha256)
        return invalid();
      this.check(signal);
      const receipt = decodeCipherBlobReceipt(
        await this.blobs.write(pending.blob.id, data, signal),
        this.scope.product,
        pending.blob.id,
      );
      this.check(signal);
      if (!equal(receipt, pending.blob)) return invalid();
      state.frontier[0]!.push({ block: pending });
      delete state.pending;
      state.next++;
      await this.checkpoint(job, state, signal);
      await this.cache.delete('b_' + job);
      this.check(signal);
    } finally {
      data?.fill(0);
    }
  }
  /** Forget retry keys only after the exact application-owned file catalog is authenticated. */
  acknowledge(
    job: string,
    input: StorageFileRequest,
    reference: MatrixReference,
    parent?: AbortSignal,
  ): Promise<boolean> {
    if (!storageIdentifier(job) || !this.records.verifyCatalog)
      throw new MatrixSecurityError('unsupported');
    const wanted = storageFileRequest(input, this.scope),
      catalog = storageReference(reference),
      verify = this.records.verifyCatalog;
    return this.records.run(
      'file-upload-' + job,
      async (signal) => {
        this.check(signal);
        const bytes = await this.cache.read('j_' + job);
        if (bytes === null) return false;
        let state: UploadState;
        try {
          state = stateFrom(bytes, this.scope, wanted, this.target);
        } finally {
          bytes.fill(0);
        }
        this.records.assertTarget(state.target);
        if (!state.published || state.phase !== 'finalize') return invalid();
        await verify(catalog, state.published, wanted, state.sha256, signal);
        this.check(signal);
        await this.cache.delete('j_' + job);
        this.check(signal);
        await this.cache.delete('b_' + job);
        this.check(signal);
        return true;
      },
      parent,
    );
  }
  /** A completed receipt stays journaled until the app durably commits its catalog reference. */
  upload(
    job: string,
    input: StorageFileRequest,
    source: Blob,
    parent?: AbortSignal,
  ): Promise<MatrixReference> {
    return this.uploadSource(job, input, storageBlobSource(source), parent);
  }
  /** Random reads allow the existing encrypted cache to retain a source across reloads. */
  uploadSource(
    job: string,
    input: StorageFileRequest,
    source: StorageFileSource,
    parent?: AbortSignal,
  ): Promise<MatrixReference> {
    if (!storageIdentifier(job)) return Promise.reject(new MatrixSecurityError('reference'));
    const wanted = storageFileRequest(input, this.scope),
      original = Object.freeze({
        size: source.size,
        ...(source.sha256 === undefined ? {} : { sha256: source.sha256 }),
        read: source.read.bind(source),
      });
    if (
      original.size !== wanted.size_bytes ||
      (original.sha256 !== undefined && !hash(original.sha256))
    )
      return Promise.reject(new MatrixSecurityError('reference'));
    return this.records.run(
      'file-upload-' + job,
      async (signal) => {
        this.check(signal);
        const bytes = await this.cache.read('j_' + job);
        let restored: UploadState | undefined;
        try {
          if (bytes !== null) restored = stateFrom(bytes, this.scope, wanted, this.target);
        } finally {
          bytes?.fill(0);
        }
        if (restored) this.records.assertTarget(restored.target);
        const sourceHash = await this.sourceHash(original, signal);
        if (original.sha256 !== undefined && original.sha256 !== sourceHash) return invalid();
        const state: UploadState = restored ?? {
          v: 1,
          scope: this.scope,
          request: wanted,
          target: this.target,
          salt: crypto.randomUUID(),
          sha256: sourceHash,
          next: 0,
          phase: 'upload',
          frontier: Array.from({ length: levels }, () => []),
          pages: Array.from({ length: levels }, () => 0),
        };
        if (state.sha256 !== sourceHash) return invalid();
        this.records.assertTarget(state.target);
        if (bytes === null) await this.checkpoint(job, state, signal);
        while (state.next < count(wanted.size_bytes)) await this.leaf(job, state, original, signal);
        if (!state.file) {
          state.phase = 'finalize';
          await this.checkpoint(job, state, signal);
          const level = rootLevel(wanted.size_bytes);
          for (let index = 0; index < level; index++)
            if (state.frontier[index]!.length) await this.flush(job, state, index, signal);
          state.file = {
            kind: 'org.sytue.file.v1',
            request: wanted,
            sha256: state.sha256,
            root: {
              kind: 'org.sytue.file.page.v1',
              level,
              index: 0,
              nodes: state.frontier[level]!,
            },
          };
          state.file.root = page(state.file.root, state.file, this.scope, level, 0);
          await this.checkpoint(job, state, signal);
        }
        if (!state.published) {
          state.published = await this.save(state, 'root', 0, 0, state.file, signal);
          await this.checkpoint(job, state, signal);
        }
        await this.records.verify(state.published, wanted.object_id, wanted.revision_id, signal);
        this.check(signal);
        return storageReference(state.published);
      },
      parent,
    );
  }
}
