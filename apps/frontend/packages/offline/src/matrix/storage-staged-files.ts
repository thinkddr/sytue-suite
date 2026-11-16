/* SPDX-License-Identifier: Apache-2.0 */
/** Durable immutable input in the existing encrypted local cache; not another E2E format. */
import { streamCaptureSource, type StorageCaptureSource } from './storage-stream-source.js';
import { createSHA256 } from 'hash-wasm';
import { maxCipherBlockBytes, type MatrixStorageScope } from '@sytue/config/matrix';
import { storageScope, storageIdentifier } from './storage.js';
import { fields, hash, digest, invalid, maxStorageFileBytes } from './storage-file-codec.js';
import {
  storageBlobSource,
  storageSourceBlockBytes,
  type StorageFileSource,
} from './storage-file-upload.js';
import type { StorageWorkflowJournal } from './storage-repository.js';
export interface StorageStagedFile {
  v: 1;
  scope: MatrixStorageScope;
  id: string;
  size_bytes: number;
  sha256: string;
}
type Work = <T>(action: (signal: AbortSignal) => Promise<T>, parent?: AbortSignal) => Promise<T>;
const encoder = new TextEncoder();
export function storageStagedFile(input: unknown, expected: MatrixStorageScope): StorageStagedFile {
  const row = fields(input),
    scope = storageScope(row.scope);
  if (
    row.v !== 1 ||
    JSON.stringify(scope) !== JSON.stringify(storageScope(expected)) ||
    !storageIdentifier(row.id) ||
    !Number.isSafeInteger(row.size_bytes) ||
    (row.size_bytes as number) < 0 ||
    (row.size_bytes as number) > maxStorageFileBytes ||
    !hash(row.sha256)
  )
    return invalid();
  return { v: 1, scope, id: row.id, size_bytes: row.size_bytes as number, sha256: row.sha256 };
}

export class StorageStagedFiles {
  private readonly scope: MatrixStorageScope;
  private readonly prefix: Promise<string>;
  constructor(
    scope: MatrixStorageScope,
    private readonly cache: StorageWorkflowJournal,
    private readonly work: Work,
    private readonly assertOpen: (signal: AbortSignal) => void,
  ) {
    this.scope = Object.freeze(storageScope(scope));
    this.prefix = digest(encoder.encode(JSON.stringify(this.scope))).then(
      (sum) => 'stage_' + sum + '_',
    );
  }
  private check(signal: AbortSignal): void {
    signal.throwIfAborted();
    this.assertOpen(signal);
  }
  capture(input: Blob, parent?: AbortSignal): Promise<StorageStagedFile> {
    const source = storageBlobSource(input);
    if (!Number.isSafeInteger(source.size) || source.size < 0 || source.size > maxStorageFileBytes)
      return invalid();
    return this.captureSource(() => source, parent);
  }
  async captureStream(
    input: ReadableStream<Uint8Array<ArrayBuffer>>,
    size: number,
    checksum: string | undefined,
    parent?: AbortSignal,
  ): Promise<StorageStagedFile> {
    try {
      if (
        !Number.isSafeInteger(size) ||
        size < 0 ||
        size > maxStorageFileBytes ||
        (checksum !== undefined && !hash(checksum))
      )
        invalid();
      return await this.captureSource(
        (signal) => streamCaptureSource(input, size, signal),
        parent,
        checksum,
      );
    } catch (error) {
      await input.cancel(error).catch(() => undefined);
      throw error;
    }
  }
  private captureSource(
    create: (signal: AbortSignal) => StorageCaptureSource,
    parent?: AbortSignal,
    checksum?: string,
  ): Promise<StorageStagedFile> {
    return this.work(async (signal) => {
      const prefix = await this.prefix,
        id = crypto.randomUUID(),
        key = prefix + id,
        hasher = await createSHA256(),
        source = create(signal);
      let written = 0;
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
            if (bytes.length !== Math.min(storageSourceBlockBytes, source.size - offset)) invalid();
            const block = written++;
            await this.cache.write(key + '_' + block, bytes);
            this.check(signal);
            hasher.update(bytes);
          } finally {
            bytes.fill(0);
          }
        }
        await source.finish?.(signal);
        const sum = hasher.digest('hex');
        if (checksum !== undefined && checksum !== sum) invalid();
        const file: StorageStagedFile = {
          v: 1,
          scope: { ...this.scope },
          id,
          size_bytes: source.size,
          sha256: sum,
        };
        this.check(signal);
        const raw = encoder.encode(JSON.stringify(file));
        try {
          await this.cache.write(key, raw);
          this.check(signal);
        } finally {
          raw.fill(0);
        }
        return file;
      } catch (error) {
        const errors: unknown[] = [error];
        // The marker write can have completed before reporting an error. Try every cleanup.
        for (let n = -1; n < written; n++) {
          try {
            await this.cache.delete(n === -1 ? key : key + '_' + n);
          } catch (cleanup) {
            errors.push(cleanup);
          }
        }
        if (errors.length > 1) throw new AggregateError(errors, 'storage_capture_incomplete');
        throw error;
      } finally {
        hasher.init().update(new Uint8Array(storageSourceBlockBytes)).digest();
        await source.close?.();
      }
    }, parent);
  }
  open(input: StorageStagedFile, parent?: AbortSignal): Promise<StorageFileSource> {
    const file = storageStagedFile(input, this.scope);
    return this.work(async (signal) => {
      const key = (await this.prefix) + file.id;
      this.check(signal);
      if (!(await this.stored(key, file, signal))) return invalid();
      return Object.freeze({
        size: file.size_bytes,
        sha256: file.sha256,
        read: (offset: number, length: number, parent: AbortSignal) =>
          this.read(key, file, offset, length, parent),
      });
    }, parent);
  }
  private async stored(
    key: string,
    file: StorageStagedFile,
    signal: AbortSignal,
  ): Promise<boolean> {
    const raw = await this.cache.read(key);
    if (raw === null) {
      this.check(signal);
      return false;
    }
    try {
      this.check(signal);
      const stored = storageStagedFile(
        JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(raw)) as unknown,
        this.scope,
      );
      if (JSON.stringify(stored) !== JSON.stringify(file)) invalid();
      return true;
    } finally {
      raw.fill(0);
    }
  }
  private async read(
    key: string,
    file: StorageStagedFile,
    offset: number,
    length: number,
    parent: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer>> {
    if (
      !Number.isSafeInteger(offset) ||
      !Number.isSafeInteger(length) ||
      offset < 0 ||
      length < 0 ||
      length > maxCipherBlockBytes ||
      offset > file.size_bytes - length
    )
      return invalid();
    const result = new Uint8Array(length);
    try {
      return await this.work(async (signal) => {
        let used = 0;
        while (used < length) {
          this.check(signal);
          const position = offset + used,
            index = Math.floor(position / storageSourceBlockBytes),
            start = position % storageSourceBlockBytes,
            data = await this.cache.read(key + '_' + index);
          if (data === null) invalid();
          try {
            this.check(signal);
            if (
              data.length !==
              Math.min(storageSourceBlockBytes, file.size_bytes - index * storageSourceBlockBytes)
            )
              invalid();
            const count = Math.min(length - used, data.length - start);
            result.set(data.subarray(start, start + count), used);
            used += count;
          } finally {
            data.fill(0);
          }
        }
        this.check(signal);
        return result;
      }, parent);
    } catch (error) {
      result.fill(0);
      throw error;
    }
  }
  /** Call after authenticating the remote catalog. A partial cleanup can be retried. */
  discard(input: StorageStagedFile, parent?: AbortSignal): Promise<void> {
    const file = storageStagedFile(input, this.scope);
    return this.work(async (signal) => {
      const key = (await this.prefix) + file.id;
      this.check(signal);
      if (!(await this.stored(key, file, signal))) return;
      for (let n = 0; n < Math.ceil(file.size_bytes / storageSourceBlockBytes); n++) {
        this.check(signal);
        await this.cache.delete(key + '_' + n);
      }
      this.check(signal);
      await this.cache.delete(key);
      this.check(signal);
    }, parent);
  }
}
