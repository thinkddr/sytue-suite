/* SPDX-License-Identifier: Apache-2.0 */
import { StorageFiles, type StorageBlobPort } from './storage-files.js';
import { StorageStagedFiles } from './storage-staged-files.js';
import { StorageFileUploads, storageFileRequest } from './storage-file-upload.js';
import { hash, type StorageFileRequest } from './storage-file-codec.js';
import type { EncryptedOfflineValues } from '../encrypted-storage.js';
import { storageWork, type StorageJobRunner } from './storage-jobs.js';
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
  storagePublicationTarget,
  type StoragePublicationTarget,
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

/** Implemented by the maintained SDK adapter; these records are already authenticated client data. */
export type StorageWorkflowJournal = Pick<
  EncryptedOfflineValues,
  'read' | 'write' | 'delete' | 'listIDs'
>;
export interface StorageWriteContext {
  target: StoragePublicationTarget;
  journal: StorageWorkflowJournal;
}
/** An immutable original-operation policy. The maintained SDK calls it again
 * for every fragment/root HTTP publication; the origin owns durable admission. */
export interface StoragePublicationPolicy {
  readonly target: Readonly<StoragePublicationTarget>;
  readonly signal: AbortSignal;
  assertOpen: () => void;
  authorize: (
    transaction: string,
    target: StoragePublicationTarget,
    signal: AbortSignal,
  ) => Promise<void>;
}
export interface StorageRecordTransport {
  recoverable?: (target: StoragePublicationTarget, signal: AbortSignal) => Promise<void>;
  putAuthorized?: (
    transaction: string,
    record: StorageRecord,
    target: StoragePublicationTarget,
    policy: StoragePublicationPolicy,
    signal?: AbortSignal,
  ) => Promise<MatrixReference>;
  prepare?: (signal: AbortSignal) => Promise<void>;
  exclusive?: <T>(action: (signal: AbortSignal) => Promise<T>, signal?: AbortSignal) => Promise<T>;
  run?: StorageJobRunner;
  readonly publication?: StoragePublicationTarget;
  readonly rooms: readonly string[];
  assertOpen: () => void;
  put: (
    transaction: string,
    record: StorageRecord,
    target?: StoragePublicationTarget,
    signal?: AbortSignal,
  ) => Promise<MatrixReference>;
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
  private readonly blobs: StorageBlobPort | undefined;
  private readonly privateFiles: StorageFiles | undefined;
  constructor(
    scope: MatrixStorageScope,
    private readonly transport: StorageRecordTransport,
    blobs?: StorageBlobPort,
    private readonly uploadCache?: EncryptedOfflineValues,
  ) {
    this.blobs = blobs;
    this.scope = Object.freeze(storageScope(scope));
    this.rooms = [...transport.rooms];
    if (
      this.rooms.length === 0 ||
      this.rooms.length > 100 ||
      new Set(this.rooms).size !== this.rooms.length
    )
      throw new StorageRecordError();
    for (const room of this.rooms) storageReference({ room_id: room, event_id: '$validation' });
    if (blobs)
      this.privateFiles = new StorageFiles(
        this.scope,
        {
          ...(transport.run ? { run: transport.run } : {}),
          assertOpen: () => this.check(),
          readRecord: (ref, object, role, signal) => this.readRecord(ref, object, role, signal),
        },
        blobs,
      );
  }
  /** Immutable authenticated routing, without substituting catalog heads for history. */
  binding(): Readonly<{
    scope: Readonly<MatrixStorageScope>;
    rooms: readonly string[];
    target?: Readonly<StoragePublicationTarget>;
  }> {
    this.check();
    const target = this.transport.publication;
    return Object.freeze({
      scope: Object.freeze({ ...this.scope }),
      rooms: Object.freeze([...this.rooms]),
      ...(target ? { target: Object.freeze(this.publicationTarget(target)) } : {}),
    });
  }
  /** One REAL history page. Scanned includes non-index events and is never the filtered length. */
  async catalogHistory(
    room: string,
    cursor: string | null,
    limit: number,
    signal?: AbortSignal,
  ): Promise<{ entries: StorageRecordEntry[]; scanned: number; next: string | null }> {
    return this.indexHistory(room, cursor, limit, true, signal);
  }
  /** Authenticated raw index envelopes for the shared Go catalog validator.
   * Keeps original payload bytes; batch interpretation belongs to Go only. */
  async rawCatalogHistory(
    room: string,
    cursor: string | null,
    limit: number,
    signal?: AbortSignal,
  ): Promise<{ entries: StorageRecordEntry[]; scanned: number; next: string | null }> {
    return this.indexHistory(room, cursor, limit, false, signal);
  }
  private async indexHistory(
    room: string,
    cursor: string | null,
    limit: number,
    objectsOnly: boolean,
    signal?: AbortSignal,
  ): Promise<{ entries: StorageRecordEntry[]; scanned: number; next: string | null }> {
    if (
      !this.rooms.includes(room) ||
      !Number.isSafeInteger(limit) ||
      limit < 1 ||
      limit > 100 ||
      (cursor !== null &&
        (typeof cursor !== 'string' || cursor.length > 16384 || /[\0\r\n]/u.test(cursor)))
    )
      throw new StorageRecordError();
    const from = cursor || null;
    return this.withJob(async (child) => {
      this.check(child);
      const page = await this.transport.history(room, from, limit);
      this.check(child);
      if (
        !Number.isSafeInteger(page.scanned) ||
        page.scanned < page.entries.length ||
        page.scanned > limit ||
        page.scanned < 0 ||
        (page.next !== null &&
          (typeof page.next !== 'string' ||
            page.next.length > 16384 ||
            /[\0\r\n]/u.test(page.next))) ||
        (page.next && (page.scanned === 0 || page.next === from))
      )
        throw new StorageRecordError('incomplete');
      const entries: StorageRecordEntry[] = [];
      for (const input of page.entries) {
        const reference = this.ref(input.reference);
        if (reference.room_id !== room) throw new StorageRecordError();
        const record = decodeStorageRecord(input.record, this.scope);
        if (record.role !== 'index') continue;
        if (objectsOnly) decodeStorageObject({ reference, record }, this.scope);
        entries.push({ reference, record });
      }
      this.check(child);
      return { entries, scanned: page.scanned, next: page.next || null };
    }, signal);
  }
  /** A separate transport view, never a mutable current/global policy. */
  withPublicationPolicy(input: StoragePublicationPolicy): StorageRepository {
    this.check(input.signal);
    input.assertOpen();
    const target = Object.freeze(this.publicationTarget(input.target));
    const authorize = input.authorize.bind(input),
      assert = input.assertOpen.bind(input),
      signal = input.signal,
      put = this.transport.putAuthorized?.bind(this.transport);
    if (!put) throw new MatrixSecurityError('unsupported');
    const check = (): void => {
      signal.throwIfAborted();
      assert();
      this.check(signal);
      this.publicationTarget(target);
    };
    const policy: StoragePublicationPolicy = Object.freeze<StoragePublicationPolicy>({
      target,
      signal,
      assertOpen: check,
      authorize: async (transaction, destination, child) => {
        check();
        child.throwIfAborted();
        const wanted = this.publicationTarget(destination);
        if (wanted.room_id !== target.room_id || wanted.device_id !== target.device_id)
          throw new MatrixSecurityError('reference');
        await authorize(transaction, wanted, child);
        check();
        child.throwIfAborted();
      },
    });
    const original = this.transport;
    const transport: StorageRecordTransport = {
      ...(original.prepare ? { prepare: original.prepare.bind(original) } : {}),
      ...(original.recoverable ? { recoverable: original.recoverable.bind(original) } : {}),
      ...(original.run ? { run: original.run.bind(original) } : {}),
      ...(original.exclusive ? { exclusive: original.exclusive.bind(original) } : {}),
      rooms: [...this.rooms],
      publication: target,
      assertOpen: check,
      put: (transaction, record, destination, child) => {
        check();
        const wanted = this.publicationTarget(destination ?? target);
        if (wanted.room_id !== target.room_id || wanted.device_id !== target.device_id)
          throw new MatrixSecurityError('reference');
        return put(transaction, record, wanted, policy, child);
      },
      read: original.read.bind(original),
      history: original.history.bind(original),
    };
    const blobs = this.blobs;
    return new StorageRepository(
      this.scope,
      transport,
      blobs
        ? {
            read: blobs.read.bind(blobs),
            ...(blobs.write
              ? {
                  write: async (id, bytes, child) => {
                    check();
                    const current = child ?? signal;
                    await policy.authorize('blob_' + id, target, current);
                    const receipt = await blobs.write!(id, bytes, current);
                    check();
                    current.throwIfAborted();
                    return receipt;
                  },
                }
              : {}),
          }
        : undefined,
      this.uploadCache,
    );
  }
  async recoverable(target: StoragePublicationTarget, signal?: AbortSignal): Promise<void> {
    const wanted = this.publicationTarget(target),
      ready = this.transport.recoverable;
    if (!ready) throw new MatrixSecurityError('unsupported');
    return this.withJob(async (child) => {
      await this.prepare(child);
      this.publicationTarget(wanted);
      await ready(wanted, child);
      this.check(child);
      this.publicationTarget(wanted);
    }, signal);
  }
  /** Domain operations participate in device shutdown before keys/cache are erased. */
  withJob<T>(action: (signal: AbortSignal) => Promise<T>, signal?: AbortSignal): Promise<T> {
    return storageWork(this.transport.run, () => this.check(), signal, action);
  }
  prepare(signal?: AbortSignal): Promise<void> {
    const prepare = this.transport.prepare;
    if (!prepare) throw new MatrixSecurityError('unsupported');
    return this.withJob(async (child) => {
      await prepare(child);
      this.publicationTarget();
    }, signal);
  }
  assertOpen(signal?: AbortSignal): void {
    this.check(signal);
  }
  /** The host owns one exclusive SDK device lease; queued writes drain on shutdown. */
  exclusive<T>(action: (signal: AbortSignal) => Promise<T>, signal?: AbortSignal): Promise<T> {
    this.check(signal);
    const run = this.transport.exclusive;
    if (!run) throw new MatrixSecurityError('unsupported');
    return this.withJob((child) => run(action, child), signal);
  }
  publicationTarget(value?: StoragePublicationTarget): StoragePublicationTarget {
    this.check();
    const current = this.transport.publication;
    if (!current) throw new MatrixSecurityError('unsupported');
    const target = storagePublicationTarget(value ?? current);
    if (!this.rooms.includes(target.room_id) || target.device_id !== current.device_id)
      throw new MatrixSecurityError('reference');
    return target;
  }
  writeContext(value?: StoragePublicationTarget): StorageWriteContext {
    this.check();
    if (!this.uploadCache || !this.transport.run) throw new MatrixSecurityError('unsupported');
    const target = this.publicationTarget(value),
      cache = this.uploadCache;
    return {
      target,
      journal: {
        listIDs: (maximum, prefix) => cache.listIDs(maximum, prefix),
        read: (id) => cache.read(id),
        write: (id, bytes) => cache.write(id, bytes),
        delete: (id) => cache.delete(id),
      },
    };
  }
  stagedFiles(): StorageStagedFiles {
    const { journal } = this.writeContext();
    return new StorageStagedFiles(
      this.scope,
      journal,
      (action, parent) => this.withJob(action, parent),
      (signal) => this.assertOpen(signal),
    );
  }
  files(): StorageFiles {
    this.check();
    if (!this.privateFiles) throw new MatrixSecurityError('unsupported');
    return this.privateFiles;
  }
  /** The original source remains owned by the caller, including after verification. */
  uploads(destination?: StoragePublicationTarget): StorageFileUploads {
    this.check();
    if (
      !this.blobs?.write ||
      !this.uploadCache ||
      !this.transport.publication ||
      !this.transport.run
    )
      throw new MatrixSecurityError('unsupported');
    return new StorageFileUploads(
      this.scope,
      {
        target: this.publicationTarget(destination),
        assertTarget: (target) => {
          this.publicationTarget(target);
        },
        run: this.transport.run,
        assertOpen: () => this.check(),
        putRecord: (transaction, record, target, signal) =>
          this.putRecord(transaction, record, signal, target),
        verify: (ref, object, revision, signal) =>
          this.files().verify(ref, object, revision, signal),
        verifyCatalog: (reference, published, request, sha256, signal) =>
          this.verifyFileCatalog(reference, published, request, sha256, signal),
      },
      this.blobs as StorageBlobPort & Required<Pick<StorageBlobPort, 'write'>>,
      this.uploadCache,
    );
  }
  verifyFileCatalog(
    reference: MatrixReference,
    published: MatrixReference,
    input: StorageFileRequest,
    sha256: string,
    parent?: AbortSignal,
  ): Promise<void> {
    const wanted = storageFileRequest(input, this.scope),
      ref = storageReference(reference),
      root = storageReference(published);
    if (!hash(sha256)) throw new StorageRecordError();
    return this.withJob(async (signal) => {
      const version = await this.object(ref, wanted.object_id, signal),
        content = version.commit.content;
      if (
        version.commit.state !== 'live' ||
        content?.kind !== 'file' ||
        content.revision_id !== wanted.revision_id ||
        content.reference.room_id !== root.room_id ||
        content.reference.event_id !== root.event_id
      )
        throw new StorageRecordError();
      const checked = await this.files().commitment(
        root,
        wanted.object_id,
        wanted.revision_id,
        signal,
      );
      if (checked.sha256 !== sha256 || JSON.stringify(checked.request) !== JSON.stringify(wanted))
        throw new StorageRecordError();
    }, parent);
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
    return storageWork(
      this.transport.run,
      () => this.check(),
      signal,
      async (signal) => {
        const ref = this.ref(reference);
        this.check(signal);
        const record = decodeStorageRecord(await this.transport.read(ref), this.scope);
        this.check(signal);
        if (record.object_id !== object || record.role !== role) throw new StorageRecordError();
        return record;
      },
    );
  }
  /** Read back the exact authenticated revision before reporting a write as complete. */
  async putRecord(
    transaction: string,
    input: StorageRecord,
    signal?: AbortSignal,
    destination?: StoragePublicationTarget,
  ): Promise<MatrixReference> {
    return storageWork(
      this.transport.run,
      () => this.check(),
      signal,
      async (signal) => {
        if (!storageIdentifier(transaction)) throw new StorageRecordError();
        const record = decodeStorageRecord(input, this.scope); // freeze caller-owned arrays before awaiting
        const target = destination ? storagePublicationTarget(destination) : undefined;
        if (
          target &&
          (!this.rooms.includes(target.room_id) ||
            target.device_id !== this.transport.publication?.device_id)
        )
          throw new StorageRecordError();
        this.check(signal);
        const ref = this.ref(await this.transport.put(transaction, record, target, signal));
        this.check(signal);
        if (target && ref.room_id !== target.room_id) throw new StorageRecordError();
        const got = await this.readRecord(ref, record.object_id, record.role, signal);
        if (!sameStorageRecord(got, record)) throw new StorageRecordError();
        this.check(signal);
        return ref;
      },
    );
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
  async object(
    reference: MatrixReference,
    object: string,
    signal?: AbortSignal,
  ): Promise<StorageObjectVersion> {
    return storageWork(
      this.transport.run,
      () => this.check(),
      signal,
      async (signal) => {
        const ref = this.ref(reference);
        const record = await this.readRecord(ref, object, 'index', signal);
        return decodeStorageObject({ reference: ref, record }, this.scope);
      },
    );
  }
  async putObject(
    transaction: string,
    object: string,
    revision: string,
    input: StorageObjectCommit,
    signal?: AbortSignal,
    destination?: StoragePublicationTarget,
  ): Promise<StorageObjectVersion> {
    return storageWork(
      this.transport.run,
      () => this.check(),
      signal,
      async (signal) => {
        if (
          !storageIdentifier(transaction) ||
          !storageIdentifier(object) ||
          !storageIdentifier(revision)
        )
          throw new StorageRecordError();
        const commit = decodeStorageCommit(input);
        const target = destination ? this.publicationTarget(destination) : undefined;
        this.check(signal);
        for (const parent of commit.parents ?? []) {
          const previous = await this.object(parent, object, signal);
          if (previous.revision_id === revision) throw new StorageRecordError();
        }
        if (commit.content) {
          if (commit.content.kind === 'file')
            await this.files().verify(
              commit.content.reference,
              object,
              commit.content.revision_id,
              signal,
            );
          else {
            const content = await this.readRecord(
              commit.content.reference,
              object,
              'content',
              signal,
            );
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
        const ref = await this.putRecord(transaction, record, signal, target);
        return decodeStorageObject({ reference: ref, record }, this.scope);
      },
    );
  }
  /** Exhaust every pinned room; repeated tokens, corrupt records and partial history fail visibly. */
  async catalog(signal?: AbortSignal, maximumEvents = 1_000_000): Promise<StorageObjectVersion[]> {
    return storageWork(
      this.transport.run,
      () => this.check(),
      signal,
      async (signal) => {
        if (!Number.isSafeInteger(maximumEvents) || maximumEvents < 1)
          throw new StorageRecordError();
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
      },
    );
  }
}
