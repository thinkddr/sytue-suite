/* SPDX-License-Identifier: Apache-2.0 */
/** Synthetic Drive source models, shared by read and write contract tests. */
import { storageWriteFixture } from './storage-write-fixture.js';
import { StorageRepository, type StorageRecordTransport } from './storage-repository.js';
import { PrivateDriveRepository } from './private-drive.js';
import {
  createStorageRecord,
  encodeStorageJSON,
  type StorageRecord,
  type StorageRecordEntry,
} from './storage.js';
import {
  driveObject,
  driveItemKind,
  driveMigrationKind,
  summarizeDriveRow,
  type DriveBodyPointer,
} from './private-drive-codec.js';
import { digest } from './storage-file-codec.js';
import type { MatrixReference } from './types.js';
export const scope = { tenant: 'acme', subject: 'owner', product: 'drive' as const },
  room = '!private:fixture',
  epoch = '12345678-1234-4234-8234-123456789abc';
export const bytes = (text: string) => new TextEncoder().encode(text),
  wire = (value: unknown) => encodeStorageJSON(bytes(JSON.stringify(value)));
export const precise =
  '{"id":"file","folder_id":"root","name":"Private file","size_bytes":0,"version":1,"updated_at":"2026-10-27T10:00:00.123456789+02:00","label_id":"restricted","label_name":"Restricted","label_block_external_share":true,"extension":{"integer":900719925474099512345}}';
class Memory implements StorageRecordTransport {
  readonly rooms = [room];
  readonly records = new Map<string, StorageRecord>();
  readonly indexes: StorageRecordEntry[] = [];
  reads = 0;
  failed = false;
  closed = false;
  assertOpen = () => {
    if (this.closed) throw new Error('closed fixture');
  };
  put = (): Promise<MatrixReference> => Promise.reject(new Error('read-only fixture'));
  read = (ref: MatrixReference) => {
    this.reads++;
    const value = this.records.get(ref.event_id);
    if (!value) throw new Error('missing fixture');
    return Promise.resolve(structuredClone(value));
  };
  history = (_room: string, cursor: string | null, limit: number) => {
    if (this.failed) throw new Error('interrupted history');
    const start = Number(cursor ?? 0),
      entries = this.indexes.slice(start, start + limit);
    return Promise.resolve({
      entries,
      scanned: entries.length,
      next: start + entries.length < this.indexes.length ? String(start + entries.length) : null,
    });
  };
  add(object: string, metadata: unknown, payload: unknown, name: string) {
    const ref = { room_id: room, event_id: '$' + name },
      content = { room_id: room, event_id: '$' + name + '_content' };
    const record = createStorageRecord(
      scope,
      object,
      name + '_content',
      'content',
      bytes(JSON.stringify(payload)),
    );
    this.records.set(content.event_id, record);
    const index = createStorageRecord(
      scope,
      object,
      name,
      'index',
      bytes(
        JSON.stringify({
          kind: 'org.sytue.object.v1',
          state: 'live',
          metadata: wire(metadata),
          content: { kind: 'record', revision_id: record.revision_id, reference: content },
        }),
      ),
    );
    this.records.set(ref.event_id, index);
    this.indexes.push({ reference: ref, record: index });
  }
}
export async function privateDriveFixture(rawFile = precise) {
  const memory = new Memory();
  const root = '{"id":"root","name":"Private root","is_root":true}';
  for (const [kind, id, raw] of [
    ['folder', 'root', root],
    ['file', 'file', rawFile],
  ]) {
    const rawBytes = bytes(raw!),
      meta = {
        kind: driveItemKind,
        source_kind: kind,
        source_id: id,
        raw_sha256: await digest(rawBytes),
        ...(kind === 'folder'
          ? { name: 'Private root', is_root: true, updated_at: '0001-01-01T00:00:00Z' }
          : {
              parent_id: 'root',
              name: 'Private file',
              version: 1,
              updated_at: '2026-10-27T08:00:00.123456789Z',
            }),
      };
    memory.add(
      await driveObject(kind!, id!),
      meta,
      {
        kind: 'org.sytue.drive.entry.v1',
        source_kind: kind,
        source_id: id,
        raw: encodeStorageJSON(rawBytes),
        ...(kind === 'file'
          ? {
              binary: {
                object_id: await driveObject('file_binary', id!),
                reference: { room_id: room, event_id: '$body' },
              },
            }
          : {}),
      },
      id!,
    );
  }
  const receipt = {
    kind: driveMigrationKind,
    epoch,
    rows: 2,
    blobs: 1,
    inventory_sha256: 'a'.repeat(64),
  };
  memory.add('drive_migration_' + (await digest(bytes(epoch))), receipt, receipt, 'complete');
  const state = {
    scope,
    mode: 'frozen',
    epoch,
    rows: 2,
    blobs: 1,
    started_at: '2026-10-27T10:00:00Z',
  };
  const storage = new StorageRepository(scope, memory),
    drive = new PrivateDriveRepository(storage);
  return { memory, storage, drive, state };
}

export async function privateDriveWriteFixture(raw = precise) {
  const base = await privateDriveFixture(raw);
  await base.drive.close();
  const device = storageWriteFixture(scope, { room_id: room, device_id: 'DEVICE' });
  for (const [event_id, record] of base.memory.records)
    device.seed({ reference: { room_id: room, event_id }, record });
  const open = (target = { room_id: room, device_id: 'DEVICE' }) =>
    new PrivateDriveRepository(device.repository(target));
  const addRow = async (
    kind: 'file' | 'folder' | 'favorite' | 'version',
    id: string,
    row: Record<string, unknown> | string,
    sourceEpoch = '',
    binary?: DriveBodyPointer,
  ) => {
    const raw = bytes(typeof row === 'string' ? row : JSON.stringify(row));
    const payload: Record<string, unknown> = {
      kind: 'org.sytue.drive.entry.v1',
      source_kind: kind,
      source_id: id,
      raw: encodeStorageJSON(raw),
    };
    if (kind === 'file' || kind === 'version')
      payload.binary = binary ?? {
        object_id: await driveObject(kind + '_binary', id),
        reference: { room_id: room, event_id: '$body_' + id },
      };
    base.memory.add(
      await driveObject(kind, id),
      await summarizeDriveRow(kind, id, sourceEpoch, raw),
      payload,
      id,
    );
    for (const [event_id, record] of base.memory.records)
      if (!device.records.has(event_id))
        device.seed({ reference: { room_id: room, event_id }, record });
  };
  const addFolder = (id: string, parent: string, name: string) =>
    addRow('folder', id, { id, parent_id: parent, name });
  const addFile = (id: string, folder: string, name: string, updated_at: string) =>
    addRow('file', id, { id, folder_id: folder, name, updated_at, version: 1, size_bytes: 0 });
  const addFavorite = (id: string, row: Record<string, unknown> | string, sourceEpoch = '') =>
    addRow('favorite', id, row, sourceEpoch);
  const addVersion = (id: string, row: Record<string, unknown>, binary: DriveBodyPointer) =>
    addRow('version', id, row, '', binary);
  const addFileRow = (
    id: string,
    row: Record<string, unknown> | string,
    binary: DriveBodyPointer,
    sourceEpoch = '',
  ) => addRow('file', id, row, sourceEpoch, binary);
  return {
    ...device,
    open,
    state: base.state,
    addFolder,
    addFile,
    addFavorite,
    addVersion,
    addFileRow,
  };
}
