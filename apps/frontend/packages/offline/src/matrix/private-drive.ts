/* SPDX-License-Identifier: Apache-2.0 */
/** Private Drive on verified web devices. No legacy content fallback. */
import {
  decodeLegacyMigrationState,
  legacySourceCursor,
  type LegacyMigrationState,
  maxLegacySourceBytes,
  compareLegacySourceCursor,
} from '@sytue/config/matrix';
import {
  mutateDriveMetadata,
  metadataPatch,
  type DriveMetadataRequest,
  type DriveMetadataCapture,
} from './private-drive-metadata.js';
import {
  uploadDriveFile,
  resumeDriveUploads,
  driveUploadRequest,
  type DriveUploadHost,
} from './private-drive-upload.js';
import { migrateDriveSource, type DriveMigrationSource } from './private-drive-migration.js';
import { createDriveFolder } from './private-drive-create.js';
import { restoreDriveVersion } from './private-drive-restore.js';
import {
  driveFavoriteRow,
  prepareDriveFavorite,
  setDriveFavorite,
  type DriveFavoriteRow,
} from './private-drive-favorites.js';
import { driveCursor, encodeDriveCursor } from './private-drive-cursor.js';
import type { StorageRepository } from './storage-repository.js';
import { StorageJobs } from './storage-jobs.js';
import { storageJSONBytes, type StorageObjectVersion } from './storage.js';
import {
  hash,
  digest,
  invalid,
  type StorageFileSink,
  type StorageFileRequest,
} from './storage-file-codec.js';
import {
  driveName,
  compareDriveTime,
  patchDriveJSON,
  driveItemKind,
  driveBodyKind,
  driveMigrationKind,
  driveObject,
  summary,
  entry,
  verifyProjection,
  fileView,
  json,
  text,
  type DriveSummary,
  type DriveEntry,
  type DriveBodyPointer,
  type DriveSourceKind,
} from './private-drive-codec.js';

export class PrivateDriveError extends Error {
  constructor(
    readonly reason: 'not_ready' | 'not_found' | 'conflict' | 'cycle' | 'closed' | 'immutable_root',
  ) {
    super('drive_private_' + reason);
    this.name = 'PrivateDriveError';
  }
}
type Node = { item: DriveSummary; version: StorageObjectVersion };
type HistoryEntry = {
  node: Node;
  binary: DriveBodyPointer;
  checksum: string;
  view: {
    version: number;
    size_bytes: number;
    content_type: string;
    author_sub: string;
    created_at: string;
    is_current: boolean;
  };
};
type Projection = { heads: Map<string, StorageObjectVersion[]>; nodes: Map<string, Node[]> };
const empty = (): Projection => ({ heads: new Map(), nodes: new Map() });
const notFound = (): never => {
  throw new PrivateDriveError('not_found');
};
const conflict = (): never => {
  throw new PrivateDriveError('conflict');
};
const zero = (value: Uint8Array) => value.fill(0);
/** Go uses simple lowercasing; ECMAScript's contextual/full mappings differ. */
const childKey = (item: DriveSummary) =>
  Array.from(item.name, (c) => (c === 'İ' ? 'i' : c.toLowerCase())).join('') +
  '\0' +
  item.source_id;
const compare = (a: string, b: string) =>
  compareLegacySourceCursor({ kind: '', id: a }, { kind: '', id: b });

export class PrivateDriveRepository {
  private readonly jobs = new StorageJobs();
  private closed = false;
  private state: LegacyMigrationState | undefined;
  private projection = empty();
  private tail: Promise<unknown> = Promise.resolve();
  constructor(private readonly repository: StorageRepository) {
    if (repository.scope.product !== 'drive') invalid();
  }
  private check(ready = true): void {
    if (this.closed) throw new PrivateDriveError('closed');
    this.repository.assertOpen();
    if (ready && !this.state) throw new PrivateDriveError('not_ready');
  }
  private work<T>(
    action: (signal: AbortSignal) => Promise<T>,
    signal?: AbortSignal,
    ready = true,
  ): Promise<T> {
    this.check(ready);
    return this.jobs.run(
      crypto.randomUUID(),
      (child) =>
        this.repository.withJob(async (signal) => {
          this.check(ready);
          const result = await action(signal);
          this.check(ready);
          return result;
        }, child),
      signal,
    );
  }
  async close(): Promise<void> {
    this.closed = true;
    await this.jobs.close();
    this.projection = empty();
    this.state = undefined;
  }
  private serialized<T>(action: () => Promise<T>): Promise<T> {
    const next = this.tail.then(action);
    this.tail = next.catch(() => {});
    return next;
  }
  /** A native device may already have imported. Missing completion never starts a plaintext read. */
  initialize(input: unknown, signal?: AbortSignal): Promise<void> {
    const state = decodeLegacyMigrationState(input, this.repository.scope);
    return this.work((signal) => this.serialized(() => this.load(state, signal)), signal, false);
  }
  migrate(source: DriveMigrationSource, signal?: AbortSignal): Promise<LegacyMigrationState> {
    return this.work(
      (signal) =>
        this.repository.exclusive(
          (signal) =>
            this.serialized(() =>
              migrateDriveSource(
                {
                  repository: this.repository,
                  read: async (version, signal) =>
                    this.read({ item: await summary(version), version }, signal),
                  completed: async (state, inventory, signal) => {
                    try {
                      await this.load(state, signal, inventory);
                      return true;
                    } catch (error) {
                      if (error instanceof PrivateDriveError && error.reason === 'not_ready')
                        return false;
                      throw error;
                    }
                  },
                  verifyBinary: async (item, pointer, signal) => {
                    await this.readBody(
                      pointer,
                      item,
                      'binary',
                      {
                        write: () => Promise.resolve(),
                        commit: () => Promise.resolve(),
                        abort: () => Promise.resolve(),
                      },
                      signal,
                    );
                  },
                },
                source,
                signal,
              ),
            ),
          signal,
        ),
      signal,
      false,
    );
  }
  refresh(signal?: AbortSignal): Promise<void> {
    return this.work((signal) => this.serialized(() => this.load(this.state!, signal)), signal);
  }
  private async load(
    state: LegacyMigrationState,
    signal: AbortSignal,
    inventory?: string,
  ): Promise<void> {
    this.check(false);
    signal.throwIfAborted();
    if (state.mode === 'legacy' || !state.epoch) throw new PrivateDriveError('not_ready');
    const catalog = await this.repository.catalog(signal),
      next = empty();
    for (const version of catalog) {
      const list = next.heads.get(version.object_id) ?? [];
      list.push(version);
      next.heads.set(version.object_id, list);
    }
    const completion =
      next.heads.get('drive_migration_' + (await digest(new TextEncoder().encode(state.epoch)))) ??
      [];
    if (completion.length === 0) throw new PrivateDriveError('not_ready');
    if (completion.length !== 1) conflict();
    const receipt = completion[0]!,
      raw = storageJSONBytes(receipt.commit.metadata, 8 << 10),
      proof = json(raw);
    if (
      proof.kind !== driveMigrationKind ||
      proof.epoch !== state.epoch ||
      proof.rows !== state.rows ||
      proof.blobs !== state.blobs ||
      !hash(proof.inventory_sha256) ||
      (inventory !== undefined && proof.inventory_sha256 !== inventory) ||
      receipt.commit.state !== 'live'
    )
      invalid();
    const pointer = receipt.commit.content;
    if (!pointer || pointer.kind !== 'record') invalid();
    const content = await this.repository.readRecord(
      pointer.reference,
      receipt.object_id,
      'content',
      signal,
    );
    if (content.revision_id !== pointer.revision_id || content.payload !== receipt.commit.metadata)
      invalid();
    let roots = 0;
    for (const version of catalog) {
      const descriptor = json(storageJSONBytes(version.commit.metadata, 8 << 10));
      if (descriptor.kind !== driveItemKind) continue;
      const item = await summary(version),
        list = next.nodes.get(version.object_id) ?? [];
      list.push({ item, version });
      next.nodes.set(version.object_id, list);
      if (item.source_kind === 'folder' && item.is_root && version.commit.state === 'live') roots++;
    }
    if (roots > 1) conflict();
    if (roots !== 1) invalid();
    this.check(false);
    signal.throwIfAborted();
    this.projection = next;
    this.state = { ...state, scope: { ...state.scope } };
  }
  private async node(projection: Projection, kind: DriveSourceKind, id: string): Promise<Node> {
    const versions = projection.nodes.get(await driveObject(kind, id)) ?? [];
    if (versions.length === 0) notFound();
    if (versions.length !== 1) conflict();
    const node = versions[0]!;
    if (node.version.commit.state === 'deleted') notFound();
    return node;
  }
  private async live(projection: Projection, node: Node, signal: AbortSignal): Promise<void> {
    if (node.version.commit.state !== 'live' || node.item.deleted_at !== null) notFound();
    let id = node.item.source_kind === 'folder' ? node.item.source_id : node.item.parent_id;
    const seen = new Set<string>();
    for (;;) {
      signal.throwIfAborted();
      if (id === '') invalid();
      if (seen.has(id)) throw new PrivateDriveError('cycle');
      seen.add(id);
      const parent = await this.node(projection, 'folder', id);
      if (parent.version.commit.state !== 'live' || parent.item.deleted_at !== null) notFound();
      if (parent.item.is_root) {
        if (parent.item.parent_id !== '') invalid();
        return;
      }
      id = parent.item.parent_id;
    }
  }
  private async read(
    node: Node,
    signal: AbortSignal,
  ): Promise<{
    item: DriveSummary;
    entry: DriveEntry;
    raw: Uint8Array<ArrayBuffer>;
    row: Record<string, unknown>;
  }> {
    const pointer = node.version.commit.content;
    if (!pointer || pointer.kind !== 'record') invalid();
    const record = await this.repository.readRecord(
      pointer.reference,
      node.version.object_id,
      'content',
      signal,
    );
    if (record.revision_id !== pointer.revision_id) invalid();
    const data = await entry(record.payload, node.item);
    let raw = data.raw;
    try {
      if (data.metadata) {
        const chunks: Uint8Array<ArrayBuffer>[] = [];
        let size = 0;
        try {
          await this.readBody(
            data.metadata,
            node.item,
            'metadata',
            {
              write: (chunk) => {
                if ((size += chunk.length) > maxLegacySourceBytes) invalid();
                chunks.push(chunk.slice());
                return Promise.resolve();
              },
              commit: async () => {},
              abort: () => {
                for (const chunk of chunks) zero(chunk);
                return Promise.resolve();
              },
            },
            signal,
          );
          raw = new Uint8Array(size);
          let offset = 0;
          for (const chunk of chunks) {
            raw.set(chunk, offset);
            offset += chunk.length;
          }
        } finally {
          for (const chunk of chunks) zero(chunk);
        }
      }
      if (!raw) invalid();
      const row = await verifyProjection(node.item, node.version.commit.state, raw);
      signal.throwIfAborted();
      this.check(false);
      return { item: node.item, entry: data, raw, row };
    } catch (error) {
      raw?.fill(0);
      throw error;
    }
  }
  private async bodyCommitment(
    pointer: DriveBodyPointer,
    item: DriveSummary,
    purpose: 'binary' | 'metadata',
    signal: AbortSignal,
  ) {
    const version = await this.repository.object(pointer.reference, pointer.object_id, signal),
      description = json(storageJSONBytes(version.commit.metadata, 8 << 10));
    const physical = pointer.origin ?? {
      kind: item.source_kind,
      id: item.source_id,
      epoch: item.epoch,
    };
    const size = description.size_bytes;
    if (
      description.kind !== driveBodyKind ||
      description.source_kind !== physical.kind ||
      description.source_id !== physical.id ||
      text(description, 'epoch') !== physical.epoch ||
      description.purpose !== purpose ||
      !hash(description.sha256) ||
      typeof size !== 'number' ||
      !Number.isSafeInteger(size) ||
      size < 0 ||
      (purpose === 'binary' &&
        (size !== item.size_bytes ||
          (item.checksum !== '' && description.sha256 !== item.checksum))) ||
      (purpose === 'metadata' && size > maxLegacySourceBytes) ||
      version.commit.state !== 'live'
    )
      invalid();
    const content = version.commit.content;
    if (!content || content.kind !== 'file') invalid();
    const files = this.repository.files(),
      commitment = await files.commitment(
        content.reference,
        pointer.object_id,
        content.revision_id,
        signal,
      ),
      info = commitment.request;
    if (
      info.object_id !== pointer.object_id ||
      info.revision_id !== content.revision_id ||
      info.size_bytes !== size ||
      commitment.sha256 !== description.sha256
    )
      invalid();
    return { files, content, info, size, checksum: description.sha256 };
  }
  private async readBody(
    pointer: DriveBodyPointer,
    item: DriveSummary,
    purpose: 'binary' | 'metadata',
    sink: StorageFileSink,
    signal: AbortSignal,
  ): Promise<StorageFileRequest> {
    let aborted = false;
    const abort = async () => {
      if (!aborted) {
        aborted = true;
        await sink.abort();
      }
    };
    try {
      const { files, content, info, size } = await this.bodyCommitment(
        pointer,
        item,
        purpose,
        signal,
      );
      let copied = 0,
        verified = false;
      const got = await files.download(
        content.reference,
        pointer.object_id,
        content.revision_id,
        {
          write: async (chunk) => {
            if ((copied += chunk.length) > size) invalid();
            await sink.write(chunk);
          },
          commit: () => {
            if (copied !== size) invalid();
            verified = true;
            return Promise.resolve();
          },
          abort,
        },
        signal,
      );
      if (
        !verified ||
        got.object_id !== info.object_id ||
        got.revision_id !== info.revision_id ||
        got.size_bytes !== size ||
        got.metadata !== info.metadata
      )
        invalid();
      signal.throwIfAborted();
      this.check(false);
      await sink.commit();
      return got;
    } catch (error) {
      try {
        await abort();
      } catch (cleanup) {
        throw new AggregateError([error, cleanup], 'drive_private_incomplete');
      }
      throw error;
    }
  }
  rename(kind: 'file' | 'folder', id: string, value: string, signal?: AbortSignal): Promise<void> {
    return this.mutate({ operation: 'rename', kind, id, value: driveName(value) }, signal);
  }
  move(kind: 'file' | 'folder', id: string, parent: string, signal?: AbortSignal): Promise<void> {
    return this.mutate({ operation: 'move', kind, id, value: parent }, signal);
  }
  delete(kind: 'file' | 'folder', id: string, signal?: AbortSignal): Promise<void> {
    return this.mutate({ operation: 'trash', kind, id, value: '' }, signal);
  }
  restore(kind: 'file' | 'folder', id: string, signal?: AbortSignal): Promise<void> {
    return this.mutate({ operation: 'restore', kind, id, value: '' }, signal);
  }
  private mutate(request: DriveMetadataRequest, signal?: AbortSignal): Promise<void> {
    if (!['file', 'folder'].includes(request.kind)) invalid();
    legacySourceCursor({ kind: request.kind, id: request.id });
    if (request.operation === 'move') legacySourceCursor({ kind: 'folder', id: request.value });
    return this.write(
      (signal) =>
        mutateDriveMetadata(
          {
            repository: this.repository,
            capture: (request, signal) => this.captureMetadata(request, signal),
            read: async (version, signal) =>
              this.read({ item: await summary(version), version }, signal),
          },
          request,
          signal,
        ),
      signal,
    );
  }
  createFolder(
    parent: string,
    value: string,
    signal?: AbortSignal,
  ): Promise<Record<string, unknown>> {
    legacySourceCursor({ kind: 'folder', id: parent });
    const name = driveName(value);
    return this.write(
      (signal) =>
        createDriveFolder(
          {
            repository: this.repository,
            read: async (version, signal) =>
              this.read({ item: await summary(version), version }, signal),
            validate: async (item, revision, signal) => {
              await this.live(
                this.projection,
                await this.node(this.projection, 'folder', parent),
                signal,
              );
              this.unique(item, parent, name);
              const heads =
                this.projection.heads.get(await driveObject('folder', item.source_id)) ?? [];
              if (heads.some((head) => head.revision_id !== revision + '_index')) conflict();
            },
          },
          parent,
          name,
          signal,
        ),
      signal,
    );
  }
  upload(
    parent: string,
    name: string,
    contentType: string,
    input: Blob,
    signal?: AbortSignal,
  ): Promise<Record<string, unknown>> {
    const source = Blob.prototype.slice.call(input, 0, input.size),
      request = driveUploadRequest({
        folder: parent,
        name,
        content_type: contentType,
        size_bytes: source.size,
      });
    return this.write(
      (signal) => uploadDriveFile(this.uploadHost(), request, source, signal),
      signal,
    );
  }
  resumeUploads(signal?: AbortSignal) {
    return this.write((signal) => resumeDriveUploads(this.uploadHost(), signal), signal);
  }
  private uploadHost(): DriveUploadHost {
    return {
      repository: this.repository,
      read: async (version, signal) => this.read({ item: await summary(version), version }, signal),
      refresh: (signal) => this.load(this.state!, signal),
      destination: async (request, signal) => {
        await this.live(
          this.projection,
          await this.node(this.projection, 'folder', request.folder),
          signal,
        );
        let result: StorageObjectVersion | undefined;
        for (const nodes of this.projection.nodes.values())
          for (const node of nodes) {
            if (
              node.item.source_kind !== 'file' ||
              node.item.parent_id !== request.folder ||
              node.item.name !== request.name ||
              node.version.commit.state !== 'live'
            )
              continue;
            if (nodes.length !== 1 || result) conflict();
            result = node.version;
          }
        return result;
      },
      validate: async (item, base, revision, signal) => {
        await this.live(
          this.projection,
          await this.node(this.projection, 'folder', item.parent_id),
          signal,
        );
        this.unique(item, item.parent_id, item.name);
        const heads = this.projection.heads.get(await driveObject('file', item.source_id)) ?? [];
        for (const head of heads)
          if (
            head.revision_id !== revision + '_index' &&
            (!base ||
              head.reference.room_id !== base.room_id ||
              head.reference.event_id !== base.event_id)
          )
            conflict();
      },
    };
  }
  private write<T>(action: (signal: AbortSignal) => Promise<T>, signal?: AbortSignal): Promise<T> {
    return this.work(
      (signal) =>
        this.repository.exclusive(
          (signal) =>
            this.serialized(async () => {
              await this.load(this.state!, signal);
              const value = await action(signal);
              await this.load(this.state!, signal);
              return value;
            }),
          signal,
        ),
      signal,
    );
  }
  private unique(item: DriveSummary, parent: string, name: string): void {
    for (const nodes of this.projection.nodes.values())
      for (const node of nodes) {
        if (
          node.item.source_kind === item.source_kind &&
          node.item.source_id !== item.source_id &&
          node.item.parent_id === parent &&
          node.item.name === name &&
          node.version.commit.state === 'live'
        )
          conflict();
      }
  }
  private async captureMetadata(
    request: DriveMetadataRequest,
    signal: AbortSignal,
  ): Promise<DriveMetadataCapture | null> {
    const projection = this.projection,
      node = await this.node(projection, request.kind, request.id),
      item = node.item;
    if (item.is_root) throw new PrivateDriveError('immutable_root');
    const now = new Date().toISOString();
    switch (request.operation) {
      case 'rename':
        await this.live(projection, node, signal);
        if (item.name === request.value) return null;
        this.unique(item, item.parent_id, request.value);
        break;
      case 'move': {
        await this.live(projection, node, signal);
        const destination = await this.node(projection, 'folder', request.value);
        await this.live(projection, destination, signal);
        if (request.kind === 'folder') {
          let id = destination.item.source_id;
          const seen = new Set<string>();
          for (;;) {
            if (id === item.source_id || seen.has(id)) throw new PrivateDriveError('cycle');
            seen.add(id);
            const parent = await this.node(projection, 'folder', id);
            if (parent.item.is_root) break;
            id = parent.item.parent_id;
          }
        }
        if (item.parent_id === request.value) return null;
        this.unique(item, request.value, item.name);
        break;
      }
      case 'trash':
        if (item.deleted_at !== null) return null;
        break;
      case 'restore':
        if (item.deleted_at === null) return null;
        await this.live(projection, await this.node(projection, 'folder', item.parent_id), signal);
        this.unique(item, item.parent_id, item.name);
        break;
    }
    const previous = await this.read(node, signal);
    let raw: Uint8Array<ArrayBuffer> | undefined;
    try {
      const next = {
        ...item,
        seed: false,
        updated_at: now,
        ...(request.operation === 'rename' ? { name: request.value } : {}),
        ...(request.operation === 'move' ? { parent_id: request.value } : {}),
        ...(request.operation === 'trash' ? { deleted_at: now } : {}),
        ...(request.operation === 'restore' ? { deleted_at: null } : {}),
      };
      raw = patchDriveJSON(previous.raw, metadataPatch(request, next));
      next.raw_sha256 = await digest(raw);
      await verifyProjection(next, next.deleted_at === null ? 'live' : 'trash', raw);
      return {
        base: node.version,
        item: next,
        raw,
        ...(previous.entry.binary ? { binary: previous.entry.binary } : {}),
      };
    } catch (error) {
      raw?.fill(0);
      throw error;
    } finally {
      previous.raw.fill(0);
    }
  }
  async readRaw(
    kind: DriveSourceKind,
    id: string,
    signal?: AbortSignal,
  ): Promise<Uint8Array<ArrayBuffer>> {
    let returned: Uint8Array<ArrayBuffer> | undefined;
    try {
      return await this.work(async (signal) => {
        returned = (await this.read(await this.node(this.projection, kind, id), signal)).raw;
        return returned;
      }, signal);
    } catch (error) {
      returned?.fill(0);
      throw error;
    }
  }
  root(signal?: AbortSignal): Promise<Record<string, unknown>> {
    return this.work(async (signal) => {
      const candidates = [...this.projection.nodes.values()]
        .flat()
        .filter(
          (n) =>
            n.item.source_kind === 'folder' && n.item.is_root && n.version.commit.state === 'live',
        );
      if (candidates.length === 0) notFound();
      if (candidates.length !== 1) conflict();
      const result = await this.read(candidates[0]!, signal);
      try {
        if (result.item.parent_id !== '') invalid();
        return result.row;
      } finally {
        zero(result.raw);
      }
    }, signal);
  }
  private resource(
    kind: 'file' | 'folder',
    id: string,
    signal?: AbortSignal,
  ): Promise<Record<string, unknown>> {
    return this.work(async (signal) => {
      const projection = this.projection,
        node = await this.node(projection, kind, id);
      await this.live(projection, node, signal);
      const result = await this.read(node, signal);
      try {
        return kind === 'file' ? fileView(result.row, id) : result.row;
      } finally {
        zero(result.raw);
      }
    }, signal);
  }
  folder(id: string, signal?: AbortSignal): Promise<Record<string, unknown>> {
    return this.resource('folder', id, signal);
  }
  file(id: string, signal?: AbortSignal): Promise<Record<string, unknown>> {
    return this.resource('file', id, signal);
  }
  children(
    id: string,
    cursor = '',
    signal?: AbortSignal,
  ): Promise<{
    folders: Record<string, unknown>[];
    files: Record<string, unknown>[];
    next_cursor: string;
  }> {
    return this.work(async (signal) => {
      const projection = this.projection,
        parent = await this.node(projection, 'folder', id);
      await this.live(projection, parent, signal);
      let after = '';
      if (cursor) {
        const pin = driveCursor(cursor);
        if (pin.Folder !== id || typeof pin.After !== 'string') invalid();
        const last = await this.node(projection, 'file', pin.After);
        if (last.item.parent_id !== id) invalid();
        after = childKey(last.item);
      }
      const candidates: Node[] = [];
      for (const nodes of projection.nodes.values())
        for (const node of nodes) {
          if (
            !['file', 'folder'].includes(node.item.source_kind) ||
            node.item.parent_id !== id ||
            node.version.commit.state !== 'live'
          )
            continue;
          if (nodes.length !== 1) conflict();
          candidates.push(node);
        }
      candidates.sort((a, b) =>
        a.item.source_kind === b.item.source_kind
          ? compare(childKey(a.item), childKey(b.item))
          : a.item.source_kind === 'folder'
            ? -1
            : 1,
      );
      const result: {
        folders: Record<string, unknown>[];
        files: Record<string, unknown>[];
        next_cursor: string;
      } = { folders: [], files: [], next_cursor: '' };
      for (const node of candidates) {
        if (node.item.source_kind === 'file') {
          if (compare(childKey(node.item), after) <= 0) continue;
          if (result.files.length === 500) {
            result.next_cursor = encodeDriveCursor({ Folder: id, After: result.files.at(-1)!.id });
            break;
          }
        }
        const read = await this.read(node, signal);
        try {
          if (node.item.source_kind === 'folder') result.folders.push(read.row);
          else result.files.push(fileView(read.row, node.item.source_id));
        } finally {
          zero(read.raw);
        }
      }
      return result;
    }, signal);
  }
  /** Resolve one captured private tree; hidden descendants never enter query results. */
  private async queryPath(
    projection: Projection,
    node: Node,
    signal: AbortSignal,
  ): Promise<string | null> {
    const names = [node.item.name],
      seen = new Set<string>();
    if (node.item.source_kind === 'folder') seen.add(node.item.source_id);
    let id = node.item.parent_id;
    for (;;) {
      signal.throwIfAborted();
      if (!id) invalid();
      if (seen.has(id)) throw new PrivateDriveError('cycle');
      seen.add(id);
      const parents = projection.nodes.get(await driveObject('folder', id));
      if (!parents?.length) invalid();
      if (parents.length !== 1) conflict();
      const parent = parents[0]!;
      if (parent.version.commit.state !== 'live' || parent.item.deleted_at !== null) return null;
      if (parent.item.is_root) {
        if (parent.item.parent_id !== '') invalid();
        return '/' + names.join('/');
      }
      names.unshift(parent.item.name);
      id = parent.item.parent_id;
    }
  }
  /** Same timestamp/ID order and account-pinned cursor as the native private repository. */
  recent(
    cursor = '',
    signal?: AbortSignal,
  ): Promise<{ items: { row: Record<string, unknown>; path: string }[]; next_cursor: string }> {
    return this.work(async (signal) => {
      const projection = this.projection,
        scope = this.repository.scope;
      const account = await digest(
        new TextEncoder().encode(scope.tenant + '\0' + scope.subject + '\0' + scope.product),
      );
      const decoded = cursor ? driveCursor(cursor) : undefined;
      let pin: { before: string; id: string } | undefined;
      if (decoded) {
        if (
          decoded.v !== 1 ||
          decoded.account !== account ||
          decoded.kind !== 'drive_recent' ||
          typeof decoded.before !== 'string' ||
          typeof decoded.id !== 'string'
        )
          invalid();
        legacySourceCursor({ kind: 'file', id: decoded.id });
        compareDriveTime(decoded.before, decoded.before);
        if (compareDriveTime(decoded.before, '0001-01-01T00:00:00Z') === 0) invalid();
        pin = { before: decoded.before, id: decoded.id };
      }
      const nodes: { node: Node; path: string }[] = [];
      for (const heads of projection.nodes.values())
        for (const node of heads) {
          signal.throwIfAborted();
          if (
            node.item.source_kind !== 'file' ||
            node.item.deleted_at !== null ||
            node.version.commit.state !== 'live'
          )
            continue;
          if (heads.length !== 1) conflict();
          if (pin) {
            const order = compareDriveTime(node.item.updated_at, pin.before);
            if (order > 0 || (order === 0 && compare(node.item.source_id, pin.id) >= 0)) continue;
          }
          const path = await this.queryPath(projection, node, signal);
          if (path !== null) nodes.push({ node, path });
        }
      nodes.sort(
        (a, b) =>
          compareDriveTime(b.node.item.updated_at, a.node.item.updated_at) ||
          compare(b.node.item.source_id, a.node.item.source_id),
      );
      const result: {
        items: { row: Record<string, unknown>; path: string }[];
        next_cursor: string;
      } = { items: [], next_cursor: '' };
      for (const { node, path } of nodes.slice(0, 50)) {
        const read = await this.read(node, signal);
        try {
          result.items.push({ row: fileView(read.row, node.item.source_id), path });
        } finally {
          zero(read.raw);
        }
      }
      if (nodes.length > 50) {
        const last = nodes[49]!.node.item;
        result.next_cursor = encodeDriveCursor({
          v: 1,
          account,
          kind: 'drive_recent',
          before: last.updated_at,
          id: last.source_id,
        });
      }
      return result;
    }, signal);
  }
  private async favoriteMarks(projection: Projection, signal: AbortSignal) {
    const marks = new Map<string, { node: Node; row: DriveFavoriteRow }>();
    for (const heads of projection.nodes.values())
      for (const node of heads) {
        signal.throwIfAborted();
        if (node.item.source_kind !== 'favorite') continue;
        if (heads.length !== 1) conflict();
        if (node.version.commit.state === 'deleted') continue;
        const read = await this.read(node, signal);
        try {
          const row = driveFavoriteRow(node.item, read.row, this.repository.scope.subject),
            key = await driveObject(row.kind, row.id);
          if (marks.has(key)) conflict();
          marks.set(key, { node, row });
        } finally {
          zero(read.raw);
        }
      }
    return marks;
  }
  /** Personal favorites resolve only inside this account's authenticated tree. */
  favorites(
    signal?: AbortSignal,
  ): Promise<{ kind: 'file' | 'folder'; row: Record<string, unknown>; path: string }[]> {
    return this.work(async (signal) => {
      const projection = this.projection,
        marks = await this.favoriteMarks(projection, signal),
        selected: { node: Node; path: string; created: string }[] = [];
      for (const heads of projection.nodes.values())
        for (const node of heads) {
          const mark = marks.get(node.version.object_id);
          if (
            !mark ||
            mark.row.removed ||
            node.item.is_root ||
            node.item.deleted_at !== null ||
            node.version.commit.state !== 'live'
          )
            continue;
          if (heads.length !== 1) conflict();
          const path = await this.queryPath(projection, node, signal);
          if (path !== null) selected.push({ node, path, created: mark.row.created });
        }
      selected.sort((a, b) =>
        a.node.item.source_kind === b.node.item.source_kind
          ? compareDriveTime(b.created, a.created) ||
            compare(a.node.item.source_id, b.node.item.source_id)
          : a.node.item.source_kind === 'folder'
            ? -1
            : 1,
      );
      const result: { kind: 'file' | 'folder'; row: Record<string, unknown>; path: string }[] = [];
      for (const { node, path } of selected) {
        const read = await this.read(node, signal),
          kind = node.item.source_kind;
        try {
          if (kind !== 'file' && kind !== 'folder') invalid();
          result.push({
            kind,
            row: kind === 'file' ? fileView(read.row, node.item.source_id) : read.row,
            path,
          });
        } finally {
          zero(read.raw);
        }
      }
      return result;
    }, signal);
  }
  setFavorite(
    kind: 'file' | 'folder',
    id: string,
    on: boolean,
    signal?: AbortSignal,
  ): Promise<void> {
    legacySourceCursor({ kind, id });
    if ((kind !== 'file' && kind !== 'folder') || typeof on !== 'boolean') invalid();
    return this.write(
      (signal) =>
        setDriveFavorite(
          {
            repository: this.repository,
            read: async (version, signal) =>
              this.read({ item: await summary(version), version }, signal),
            capture: async (request, signal) => {
              if (request.on) await this.favoriteTarget(request.kind, request.id, signal);
              const marks = await this.favoriteMarks(this.projection, signal),
                previous = marks.get(await driveObject(request.kind, request.id));
              if ((!previous && !request.on) || (previous && previous.row.removed !== request.on))
                return null;
              if (!previous) return prepareDriveFavorite(this.repository.scope, request);
              const read = await this.read(previous.node, signal);
              try {
                return await prepareDriveFavorite(this.repository.scope, request, {
                  ...read,
                  version: previous.node.version,
                });
              } finally {
                zero(read.raw);
              }
            },
            validate: async (request, intent, signal) => {
              if (request.on) await this.favoriteTarget(request.kind, request.id, signal);
              const marks = await this.favoriteMarks(this.projection, signal),
                prior = marks.get(await driveObject(request.kind, request.id));
              if (prior && prior.node.item.source_id !== intent.item.source_id) conflict();
              const heads =
                this.projection.heads.get(await driveObject('favorite', intent.item.source_id)) ??
                [];
              for (const head of heads)
                if (
                  head.revision_id !== intent.revision + '_index' &&
                  (!intent.base ||
                    head.reference.room_id !== intent.base.room_id ||
                    head.reference.event_id !== intent.base.event_id)
                )
                  conflict();
            },
          },
          { kind, id, on },
          signal,
        ),
      signal,
    );
  }
  private async favoriteTarget(
    kind: 'file' | 'folder',
    id: string,
    signal: AbortSignal,
  ): Promise<void> {
    const node = await this.node(this.projection, kind, id);
    if (node.item.is_root) throw new PrivateDriveError('immutable_root');
    await this.live(this.projection, node, signal);
  }
  /** Only top-level recoverable deletions; private retention is not activated here. */
  trash(
    signal?: AbortSignal,
  ): Promise<{ kind: 'file' | 'folder'; row: Record<string, unknown>; path: string }[]> {
    return this.work(async (signal) => {
      const projection = this.projection,
        nodes: { node: Node; path: string }[] = [];
      for (const heads of projection.nodes.values())
        for (const node of heads) {
          signal.throwIfAborted();
          if (
            (node.item.source_kind !== 'file' && node.item.source_kind !== 'folder') ||
            node.item.is_root ||
            node.item.deleted_at === null ||
            node.version.commit.state !== 'trash'
          )
            continue;
          if (heads.length !== 1) conflict();
          const path = await this.queryPath(projection, node, signal);
          if (path !== null) nodes.push({ node, path });
        }
      nodes.sort(
        (a, b) =>
          compareDriveTime(b.node.item.deleted_at!, a.node.item.deleted_at!) ||
          compare(childKey(a.node.item), childKey(b.node.item)),
      );
      const result: { kind: 'file' | 'folder'; row: Record<string, unknown>; path: string }[] = [];
      for (const { node, path } of nodes) {
        const read = await this.read(node, signal),
          kind = node.item.source_kind;
        try {
          if (kind !== 'file' && kind !== 'folder') invalid();
          result.push({
            kind,
            row: kind === 'file' ? fileView(read.row, node.item.source_id) : read.row,
            path,
          });
        } finally {
          zero(read.raw);
        }
      }
      return result;
    }, signal);
  }
  private async fileVersions(
    projection: Projection,
    id: string,
    signal: AbortSignal,
  ): Promise<HistoryEntry[]> {
    const current = await this.node(projection, 'file', id);
    await this.live(projection, current, signal);
    const queue = [current.version],
      seen = new Set<string>(),
      indexes: Node[] = [];
    while (queue.length) {
      signal.throwIfAborted();
      const version = queue.pop()!,
        key = JSON.stringify(version.reference);
      if (seen.has(key)) continue;
      if (seen.size >= 1_000_000 || version.object_id !== current.version.object_id) invalid();
      seen.add(key);
      indexes.push({ item: await summary(version), version });
      for (const parent of version.commit.parents ?? [])
        queue.push(await this.repository.object(parent, version.object_id, signal));
    }
    for (const heads of projection.nodes.values())
      for (const node of heads) {
        if (node.item.source_kind !== 'version' || node.item.parent_id !== id) continue;
        if (heads.length !== 1) conflict();
        if (node.version.commit.state !== 'live') invalid();
        indexes.push(node);
      }
    const byNumber = new Map<number, HistoryEntry>();
    for (const node of indexes) {
      const read = await this.read(node, signal);
      try {
        const item = read.item,
          binary = read.entry.binary;
        if (
          !binary ||
          item.version < 1 ||
          item.version > current.item.version ||
          !(
            (item.source_kind === 'file' && item.source_id === id) ||
            (item.source_kind === 'version' && item.parent_id === id)
          )
        )
          invalid();
        const { checksum } = await this.bodyCommitment(binary, item, 'binary', signal),
          prior = byNumber.get(item.version);
        if (prior) {
          if (
            prior.checksum !== checksum ||
            prior.node.item.size_bytes !== item.size_bytes ||
            prior.node.item.content_type !== item.content_type
          )
            conflict();
          if (compareDriveTime(item.updated_at, prior.node.item.updated_at) >= 0) continue;
        }
        byNumber.set(item.version, {
          node,
          binary,
          checksum,
          view: {
            version: item.version,
            size_bytes: item.size_bytes,
            content_type: item.content_type,
            author_sub: text(read.row, 'author_sub') || text(read.row, 'owner_sub'),
            created_at: item.updated_at,
            is_current: item.version === current.item.version,
          },
        });
      } finally {
        zero(read.raw);
      }
    }
    return [...byNumber.values()].sort((a, b) => b.view.version - a.view.version);
  }
  versions(id: string, cursor = '', signal?: AbortSignal) {
    legacySourceCursor({ kind: 'file', id });
    let before = 0;
    if (cursor) {
      if (!/^\d+$/.test(cursor)) invalid();
      before = Number(cursor);
      if (!Number.isSafeInteger(before) || before < 1) invalid();
    }
    return this.work(async (signal) => {
      const all = (await this.fileVersions(this.projection, id, signal)).filter(
          (v) => !before || v.view.version < before,
        ),
        page = all.slice(0, 500);
      return {
        versions: page.map((v) => v.view),
        next_cursor: all.length > 500 ? String(page.at(-1)!.view.version) : '',
      };
    }, signal);
  }
  downloadVersion(
    id: string,
    version: number,
    sink: StorageFileSink,
    signal?: AbortSignal,
  ): Promise<StorageFileRequest> {
    return this.downloadSelected(
      async (signal) => {
        legacySourceCursor({ kind: 'file', id });
        if (!Number.isSafeInteger(version) || version < 1) invalid();
        const selected =
          (await this.fileVersions(this.projection, id, signal)).find(
            (v) => v.view.version === version && !v.view.is_current,
          ) ?? notFound();
        return { item: selected.node.item, binary: selected.binary };
      },
      sink,
      signal,
    );
  }
  download(id: string, sink: StorageFileSink, signal?: AbortSignal): Promise<StorageFileRequest> {
    return this.downloadSelected(
      async (signal) => {
        const projection = this.projection,
          node = await this.node(projection, 'file', id);
        await this.live(projection, node, signal);
        const read = await this.read(node, signal);
        zero(read.raw);
        return { item: read.item, binary: read.entry.binary ?? notFound() };
      },
      sink,
      signal,
    );
  }
  restoreVersion(id: string, version: number, signal?: AbortSignal): Promise<void> {
    legacySourceCursor({ kind: 'file', id });
    if (!Number.isSafeInteger(version) || version < 1) invalid();
    return this.write(
      (signal) =>
        restoreDriveVersion(
          {
            repository: this.repository,
            read: async (version, signal) =>
              this.read({ item: await summary(version), version }, signal),
            capture: async (request, signal) => {
              const projection = this.projection,
                candidates = await this.fileVersions(projection, request.id, signal),
                selected =
                  candidates.find(
                    (v) => v.view.version === request.version && !v.view.is_current,
                  ) ?? notFound(),
                node = await this.node(projection, 'file', request.id),
                read = await this.read(node, signal);
              return {
                ...read,
                base: node.version,
                source: {
                  item: selected.node.item,
                  binary: selected.binary,
                  checksum: selected.checksum,
                },
                restored: selected.node.version,
              };
            },
            source: async (version, signal) => {
              const read = await this.read({ item: await summary(version), version }, signal);
              try {
                const binary = read.entry.binary ?? invalid(),
                  { checksum } = await this.bodyCommitment(binary, read.item, 'binary', signal);
                return { item: read.item, binary, checksum };
              } finally {
                zero(read.raw);
              }
            },
            validate: this.uploadHost().validate,
          },
          { id, version },
          signal,
        ),
      signal,
    );
  }
  private async downloadSelected(
    select: (signal: AbortSignal) => Promise<{ item: DriveSummary; binary: DriveBodyPointer }>,
    sink: StorageFileSink,
    signal?: AbortSignal,
  ): Promise<StorageFileRequest> {
    let aborted = false;
    const guarded: StorageFileSink = {
      write: (chunk) => sink.write(chunk),
      commit: () => sink.commit(),
      abort: async () => {
        if (!aborted) {
          aborted = true;
          await sink.abort();
        }
      },
    };
    try {
      return await this.work(async (signal) => {
        const { item, binary } = await select(signal);
        return this.readBody(binary, item, 'binary', guarded, signal);
      }, signal);
    } catch (error) {
      try {
        await guarded.abort();
      } catch (cleanup) {
        throw new AggregateError([error, cleanup], 'drive_private_incomplete');
      }
      throw error;
    }
  }
}
