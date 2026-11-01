/* SPDX-License-Identifier: Apache-2.0 */
import { MatrixSecurityError, type MatrixReference, type MatrixStorageScope } from './types.js';
import {
  StorageRecordError,
  createStorageRecord,
  decodeStorageRecord,
  decodeStorageCommit,
  decodeStorageObject,
  sameStorageRecord,
  storageCatalogHeads,
  storageIdentifier,
  storageReference,
  storageScope,
  type StorageRecord,
  type StorageRecordEntry,
  type StorageObjectCommit,
  type StorageObjectVersion,
} from './storage.js';
import {
  downloadStorageBlock,
  uploadStorageBlock,
  type CipherBlobGateway,
  type StorageBlock,
} from './storage-blocks.js';
import {
  downloadStorageFile,
  readStorageFileRange,
  storageFileInfo,
  uploadStorageFile,
  type StorageFileRequest,
  type StorageFileSource,
  type StorageManifestRecordIO,
  type StorageUploadJournal,
} from './storage-files.js';

/** Implemented by the maintained SDK adapter; these records are already authenticated client data. */
export interface StorageRecordTransport {
  readonly rooms: readonly string[];
  assertOpen: () => void;
  put: (transaction: string, record: StorageRecord) => Promise<MatrixReference>;
  read: (reference: MatrixReference) => Promise<StorageRecord>;
  history: (
    room: string,
    cursor: string | null,
    limit: number,
  ) => Promise<{ entries: StorageRecordEntry[]; scanned: number; next: string | null }>;
}
/** Catalog operations share native wire contracts; they never delete a source or choose conflict winners. */
export class StorageRepository {
  readonly scope: Readonly<MatrixStorageScope>;
  private readonly rooms: readonly string[];
  constructor(
    scope: MatrixStorageScope,
    private readonly transport: StorageRecordTransport,
  ) {
    this.scope = Object.freeze(storageScope(scope));
    this.rooms = [...transport.rooms];
    if (
      this.rooms.length === 0 ||
      this.rooms.length > 100 ||
      new Set(this.rooms).size !== this.rooms.length
    )
      throw new StorageRecordError();
    for (const room of this.rooms) storageReference({ room_id: room, event_id: '$validation' });
  }
  private check(signal?: AbortSignal): void {
    signal?.throwIfAborted();
    this.transport.assertOpen();
  }
  private ref(value: MatrixReference): MatrixReference {
    const ref = storageReference(value);
    if (!this.rooms.includes(ref.room_id)) throw new MatrixSecurityError('reference');
    return ref;
  }
  private manifestIO(): StorageManifestRecordIO {
    return {
      scope: this.scope,
      assertOpen: () => this.transport.assertOpen(),
      putRecord: (transaction, record, signal) => this.putRecord(transaction, record, signal),
      readRecord: (reference, object, role, signal) =>
        this.readRecord(reference, object, role, signal),
    };
  }
  async readRecord(
    reference: MatrixReference,
    object: string,
    role: StorageRecord['role'],
    signal?: AbortSignal,
  ): Promise<StorageRecord> {
    const ref = this.ref(reference);
    this.check(signal);
    const record = decodeStorageRecord(await this.transport.read(ref), this.scope);
    this.check(signal);
    if (record.object_id !== object || record.role !== role) throw new StorageRecordError();
    return record;
  }
  /** Read back the exact authenticated revision before reporting a write as complete. */
  async putRecord(
    transaction: string,
    input: StorageRecord,
    signal?: AbortSignal,
  ): Promise<MatrixReference> {
    if (!storageIdentifier(transaction)) throw new StorageRecordError();
    const record = decodeStorageRecord(input, this.scope); // freeze caller-owned arrays before awaiting
    this.check(signal);
    const ref = this.ref(await this.transport.put(transaction, record));
    this.check(signal);
    const got = await this.readRecord(ref, record.object_id, record.role, signal);
    if (!sameStorageRecord(got, record)) throw new StorageRecordError();
    this.check(signal);
    return ref;
  }
  async uploadBlock(
    gateway: CipherBlobGateway,
    object: string,
    revision: string,
    part: number,
    plaintext: Uint8Array,
    signal?: AbortSignal,
  ): Promise<StorageBlock> {
    this.check(signal);
    const block = await uploadStorageBlock(
      gateway,
      this.scope,
      object,
      revision,
      part,
      plaintext,
      signal,
    );
    this.check(signal);
    return block;
  }
  async downloadBlock(
    gateway: CipherBlobGateway,
    block: StorageBlock,
    object: string,
    revision: string,
    part: number,
    signal?: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer>> {
    this.check(signal);
    const plain = await downloadStorageBlock(
      gateway,
      this.scope,
      block,
      object,
      revision,
      part,
      signal,
    );
    try {
      this.check(signal);
      return plain;
    } catch (error) {
      plain.fill(0);
      throw error;
    }
  }
  async fileInfo(
    reference: MatrixReference,
    object: string,
    revision: string,
    signal?: AbortSignal,
  ): Promise<StorageFileRequest> {
    const ref = this.ref(reference);
    this.check(signal);
    return storageFileInfo(this.manifestIO(), ref, object, revision, signal);
  }
  async uploadFile(
    gateway: CipherBlobGateway,
    job: string,
    request: StorageFileRequest,
    source: StorageFileSource,
    options?: {
      journal?: StorageUploadJournal;
      signal?: AbortSignal;
    },
  ): Promise<MatrixReference> {
    this.check(options?.signal);
    const ref = await uploadStorageFile(this.manifestIO(), gateway, job, request, source, options);
    this.check(options?.signal);
    return this.ref(ref);
  }
  async downloadFile(
    gateway: CipherBlobGateway,
    reference: MatrixReference,
    object: string,
    revision: string,
    sink?: (chunk: Uint8Array<ArrayBuffer>, blockIndex: number) => void | Promise<void>,
    signal?: AbortSignal,
  ): Promise<StorageFileRequest> {
    const ref = this.ref(reference);
    this.check(signal);
    return downloadStorageFile(this.manifestIO(), gateway, ref, object, revision, sink, signal);
  }
  async readFileRange(
    gateway: CipherBlobGateway,
    reference: MatrixReference,
    object: string,
    revision: string,
    offset: number,
    length: number,
    signal?: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer>> {
    const ref = this.ref(reference);
    this.check(signal);
    return readStorageFileRange(
      this.manifestIO(),
      gateway,
      ref,
      object,
      revision,
      offset,
      length,
      signal,
    );
  }
  async object(
    reference: MatrixReference,
    object: string,
    signal?: AbortSignal,
  ): Promise<StorageObjectVersion> {
    const ref = this.ref(reference);
    const record = await this.readRecord(ref, object, 'index', signal);
    return decodeStorageObject({ reference: ref, record }, this.scope);
  }
  async putObject(
    transaction: string,
    object: string,
    revision: string,
    input: StorageObjectCommit,
    signal?: AbortSignal,
  ): Promise<StorageObjectVersion> {
    if (
      !storageIdentifier(transaction) ||
      !storageIdentifier(object) ||
      !storageIdentifier(revision)
    )
      throw new StorageRecordError();
    const commit = decodeStorageCommit(input);
    this.check(signal);
    for (const parent of commit.parents ?? []) {
      const previous = await this.object(parent, object, signal);
      if (previous.revision_id === revision) throw new StorageRecordError();
    }
    if (commit.content) {
      if (commit.content.kind === 'file') {
        await this.fileInfo(commit.content.reference, object, commit.content.revision_id, signal);
      } else {
        const content = await this.readRecord(commit.content.reference, object, 'content', signal);
        if (content.revision_id !== commit.content.revision_id) throw new StorageRecordError();
      }
    }
    const record = createStorageRecord(
      this.scope,
      object,
      revision,
      'index',
      new TextEncoder().encode(JSON.stringify(commit)),
      (commit.parents ?? []).map((parent) => parent.event_id),
    );
    const ref = await this.putRecord(transaction, record, signal);
    return decodeStorageObject({ reference: ref, record }, this.scope);
  }
  /** Exhaust every pinned room; repeated tokens, corrupt records and partial history fail visibly. */
  async catalog(signal?: AbortSignal, maximumEvents = 1_000_000): Promise<StorageObjectVersion[]> {
    if (!Number.isSafeInteger(maximumEvents) || maximumEvents < 1) throw new StorageRecordError();
    const entries: StorageRecordEntry[] = [];
    let scanned = 0;
    for (const room of this.rooms) {
      let cursor: string | null = null;
      const seen = new Set<string>();
      for (;;) {
        this.check(signal);
        const page = await this.transport.history(room, cursor, 100);
        this.check(signal);
        if (
          !Number.isSafeInteger(page.scanned) ||
          page.scanned < page.entries.length ||
          page.scanned > 100 ||
          (scanned += page.scanned) > maximumEvents
        )
          throw new StorageRecordError('incomplete');
        for (const entry of page.entries) {
          const reference = this.ref(entry.reference);
          if (reference.room_id !== room) throw new StorageRecordError();
          const record = decodeStorageRecord(entry.record, this.scope);
          if (record.role !== 'index') continue;
          decodeStorageObject({ reference, record }, this.scope);
          entries.push({ reference, record });
        }
        if (page.next === null || page.next === '') break;
        if (
          page.scanned === 0 ||
          typeof page.next !== 'string' ||
          page.next === cursor ||
          seen.has(page.next)
        )
          throw new StorageRecordError('incomplete');
        seen.add(page.next);
        cursor = page.next;
      }
    }
    this.check(signal);
    return storageCatalogHeads(this.scope, entries);
  }
}
