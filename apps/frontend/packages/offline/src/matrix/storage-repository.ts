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
      // File publication stays disabled until the paged file verifier is available in this adapter.
      if (commit.content.kind === 'file') throw new MatrixSecurityError('unsupported');
      const content = await this.readRecord(commit.content.reference, object, 'content', signal);
      if (content.revision_id !== commit.content.revision_id) throw new StorageRecordError();
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
