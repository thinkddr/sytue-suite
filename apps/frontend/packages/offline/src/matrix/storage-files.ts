/* SPDX-License-Identifier: Apache-2.0 */
/** Native file.v1 authenticated, bounded reads. */
import { createSHA256, type IHasher } from 'hash-wasm';
import { Attachment, EncryptedAttachment, initAsync } from '@matrix-org/matrix-sdk-crypto-wasm';
import { maxCipherBlockBytes } from '@sytue/config/matrix';
import type { MatrixReference, MatrixStorageScope } from './types.js';
import { storageWork } from './storage-jobs.js';
import {
  storageScope,
  decodeStorageRecord,
  storageReference,
  type StorageRecord,
} from './storage.js';
import {
  fields,
  hash,
  count,
  span,
  rootLevel,
  page,
  parse,
  request,
  invalid,
  digest,
  fanout,
  type FileBlock,
  type PrivateFile,
  type StorageFileRequest,
  type StorageFileSink,
  type StorageFileRecords,
  type StorageBlobPort,
} from './storage-file-codec.js';
export {
  maxStorageFileBytes,
  type StorageFileRequest,
  type StorageFileSink,
  type StorageFileRecords,
  type StorageBlobPort,
} from './storage-file-codec.js';
/** Paged bounded reads; records are authenticated by the host's maintained Matrix adapter. */
export class StorageFiles {
  private readonly scope: MatrixStorageScope;
  constructor(
    scope: MatrixStorageScope,
    private readonly records: StorageFileRecords,
    private readonly blobs: StorageBlobPort,
  ) {
    this.scope = Object.freeze(storageScope(scope));
  }
  private check(signal?: AbortSignal): void {
    signal?.throwIfAborted();
    this.records.assertOpen();
  }
  private async record(
    ref: MatrixReference,
    object: string,
    revision: string,
    signal?: AbortSignal,
  ): Promise<StorageRecord> {
    this.check(signal);
    const result = decodeStorageRecord(
      await this.records.readRecord(storageReference(ref), object, 'manifest', signal),
      this.scope,
    );
    this.check(signal);
    if (
      result.object_id !== object ||
      result.revision_id !== revision ||
      result.role !== 'manifest' ||
      result.scope.product !== this.scope.product ||
      result.scope.tenant !== this.scope.tenant ||
      result.scope.subject !== this.scope.subject
    )
      return invalid();
    return result;
  }
  private async root(
    ref: MatrixReference,
    object: string,
    revision: string,
    signal?: AbortSignal,
  ): Promise<PrivateFile> {
    const record = await this.record(ref, object, revision, signal),
      input = fields(parse(record));
    if (input.kind !== 'org.sytue.file.v1' || !hash(input.sha256)) return invalid();
    const file: PrivateFile = {
      kind: 'org.sytue.file.v1',
      request: request(input.request, record),
      sha256: input.sha256,
      root: input.root,
    };
    file.root = page(file.root, file, this.scope, rootLevel(file.request.size_bytes), 0);
    return file;
  }
  async info(
    ref: MatrixReference,
    object: string,
    revision: string,
    signal?: AbortSignal,
  ): Promise<StorageFileRequest> {
    return (await this.commitment(ref, object, revision, signal)).request;
  }
  /** Authenticated immutable manifest commitment, matching the native SDK. */
  async commitment(
    ref: MatrixReference,
    object: string,
    revision: string,
    signal?: AbortSignal,
  ): Promise<{ request: StorageFileRequest; sha256: string }> {
    return storageWork(
      this.records.run,
      () => this.check(),
      signal,
      async (signal) => {
        const file = await this.root(ref, object, revision, signal);
        return { request: file.request, sha256: file.sha256 };
      },
    );
  }

  private async leaf(
    file: PrivateFile,
    index: number,
    cache: Map<string, unknown>,
    signal?: AbortSignal,
  ): Promise<FileBlock> {
    let level = rootLevel(file.request.size_bytes),
      position = 0,
      current = page(file.root, file, this.scope, level, 0);
    while (level > 0) {
      this.check(signal);
      const child = Math.floor((index - position * span(level)) / span(level - 1)),
        ref = current.nodes[child]?.event;
      if (!ref) return invalid();
      const key = JSON.stringify([ref.room_id, ref.event_id]);
      let raw = cache.get(key);
      if (raw === undefined) {
        raw = parse(
          await this.record(ref, file.request.object_id, file.request.revision_id, signal),
        );
        cache.set(key, raw);
      }
      level--;
      position = position * fanout + child;
      current = page(raw, file, this.scope, level, position);
    }
    return current.nodes[index % fanout]?.block ?? invalid();
  }
  private async plaintext(
    descriptor: FileBlock,
    signal?: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer>> {
    this.check(signal);
    let clear: Uint8Array<ArrayBuffer> | undefined;
    const ciphertext = await this.blobs.read(descriptor.blob.id, signal);
    try {
      this.check(signal);
      if (
        ciphertext.byteLength !== descriptor.blob.size_bytes ||
        ciphertext.byteLength > maxCipherBlockBytes
      )
        return invalid();
      await initAsync();
      this.check(signal);
      const encrypted = new EncryptedAttachment(ciphertext, JSON.stringify(descriptor.matrix_file));
      try {
        clear = Attachment.decrypt(encrypted) as Uint8Array<ArrayBuffer>;
      } finally {
        encrypted.free();
      }
      if (
        clear.byteLength !== descriptor.blob.size_bytes ||
        (await digest(clear)) !== descriptor.plain_sha256
      )
        return invalid();
      this.check(signal);
      return clear;
    } catch (error) {
      clear?.fill(0);
      throw error;
    } finally {
      ciphertext.fill(0);
    }
  }
  async readRange(
    ref: MatrixReference,
    object: string,
    revision: string,
    offset: number,
    length: number,
    signal?: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer>> {
    let returned: Uint8Array<ArrayBuffer> | undefined;
    try {
      return await storageWork(
        this.records.run,
        () => this.check(),
        signal,
        async (signal) => {
          if (
            !Number.isSafeInteger(offset) ||
            !Number.isSafeInteger(length) ||
            offset < 0 ||
            length < 0 ||
            length > maxCipherBlockBytes
          )
            return invalid();
          const file = await this.root(ref, object, revision, signal);
          if (offset > file.request.size_bytes || length > file.request.size_bytes - offset)
            return invalid();
          const out = new Uint8Array(length),
            cache = new Map<string, unknown>();
          returned = out;
          try {
            let cursor = offset;
            while (cursor < offset + length) {
              const index = Math.floor(cursor / maxCipherBlockBytes),
                data = await this.plaintext(await this.leaf(file, index, cache, signal), signal);
              try {
                const start = cursor - index * maxCipherBlockBytes,
                  wanted = Math.min(data.length - start, offset + length - cursor);
                if (wanted <= 0) return invalid();
                out.set(data.subarray(start, start + wanted), cursor - offset);
                cursor += wanted;
              } finally {
                data.fill(0);
              }
            }
            this.check(signal);
            return out;
          } catch (error) {
            out.fill(0);
            throw error;
          }
        },
      );
    } catch (error) {
      returned?.fill(0);
      throw error;
    }
  }
  async download(
    ref: MatrixReference,
    object: string,
    revision: string,
    sink: StorageFileSink,
    signal?: AbortSignal,
  ): Promise<StorageFileRequest> {
    return storageWork(
      this.records.run,
      () => this.check(),
      signal,
      async (signal) => {
        let hasher: IHasher | undefined;
        try {
          const file = await this.root(ref, object, revision, signal);
          hasher = await createSHA256();
          this.check(signal);
          // Retain only one page path. Large files do not retain every visited descriptor in memory.
          const cache = new Map<string, unknown>();
          for (let index = 0; index < count(file.request.size_bytes); index++) {
            if (index % fanout === 0) cache.clear();
            const data = await this.plaintext(await this.leaf(file, index, cache, signal), signal);
            try {
              hasher.update(data);
              await sink.write(data);
              this.check(signal);
            } finally {
              data.fill(0);
            }
          }
          if (hasher.digest('hex') !== file.sha256) return invalid();
          this.check(signal);
          await sink.commit();
          this.check(signal);
          return file.request;
        } catch (error) {
          try {
            await sink.abort();
          } catch (cleanup) {
            throw new AggregateError([error, cleanup], 'storage_incomplete');
          }
          throw error;
        } finally {
          hasher
            ?.init()
            .update(new Uint8Array(64 << 10))
            .digest();
        }
      },
    );
  }
  async verify(
    ref: MatrixReference,
    object: string,
    revision: string,
    signal?: AbortSignal,
  ): Promise<void> {
    await this.download(
      ref,
      object,
      revision,
      { write: async () => {}, commit: async () => {}, abort: async () => {} },
      signal,
    );
  }
}
