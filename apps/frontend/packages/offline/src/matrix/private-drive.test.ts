/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import {
  privateDriveFixture as fixture,
  scope,
  room,
  bytes,
  precise,
} from './private-drive-fixture.js';
import { StorageRepository } from './storage-repository.js';
import { PrivateDriveError, PrivateDriveRepository } from './private-drive.js';
import { encodeStorageJSON } from './storage.js';
import {
  driveObject,
  driveItemKind,
  verifyProjection,
  summary,
  bodyPointer,
} from './private-drive-codec.js';
import { digest } from './storage-file-codec.js';
test('private browser reader requires complete import, preserves bytes and sensitivity, and cancels close', async () => {
  const { drive, state } = await fixture();
  expect(() => drive.root()).toThrow(PrivateDriveError);
  await drive.initialize(state);
  expect((await drive.root()).id).toBe('root');
  expect(new TextDecoder().decode(await drive.readRaw('file', 'file'))).toBe(precise);
  expect((await drive.file('file')).label).toEqual({
    name: 'Restricted',
    block_external_share: true,
  });
  const children = await drive.children('root');
  expect(children.files.map((x) => x.id)).toEqual(['file']);
  const results = await Promise.allSettled([drive.file('file'), drive.close()]);
  expect(results[0]!.status).toBe('rejected');
  expect(() => drive.root()).toThrow(PrivateDriveError);
});
test('missing or conflicting completion, foreign scopes and a wrong product never expose partial imports', async () => {
  const { memory, drive, state } = await fixture();
  expect(() => drive.initialize({ ...state, scope: { ...scope, subject: 'other' } })).toThrow();
  await expect(drive.initialize({ ...state, rows: 3 })).rejects.toThrow();
  memory.indexes.pop();
  await expect(drive.initialize(state)).rejects.toThrow('not_ready');
  expect(() => drive.file('file')).toThrow('not_ready');
  expect(
    () => new PrivateDriveRepository(new StorageRepository({ ...scope, product: 'mail' }, memory)),
  ).toThrow();
  await drive.close();
});
test('failed refresh retains its prior authenticated snapshot; divergent file heads stay conflicts', async () => {
  const { memory, drive, state } = await fixture();
  await drive.initialize(state);
  memory.failed = true;
  await expect(drive.refresh()).rejects.toThrow('interrupted history');
  expect((await drive.file('file')).id).toBe('file');
  memory.failed = false;
  const first = memory.indexes[1]!,
    changed = structuredClone(first);
  changed.reference.event_id = '$branch';
  changed.record.revision_id = 'branch';
  memory.indexes.push(changed);
  await drive.refresh();
  await expect(drive.file('file')).rejects.toThrow('conflict');
  expect((await drive.root()).id).toBe('root');
  await drive.close();
});
test('native projection preserves timezone precision and rejects an altered nanosecond or binary origin', async () => {
  const { memory, storage } = await fixture(),
    versions = await storage.catalog(),
    version = versions.find((v) => v.object_id === memory.indexes[1]!.record.object_id)!;
  const item = await summary(version);
  await verifyProjection(item, 'live', bytes(precise));
  for (const extra of [
    { created_at: 'invalid-time' },
    { parent_id: 123 },
    { size_bytes: 'invalid-size' },
  ]) {
    const changed = bytes(JSON.stringify({ ...JSON.parse(precise), ...extra }));
    await expect(
      verifyProjection({ ...item, raw_sha256: await digest(changed) }, 'live', changed),
    ).rejects.toThrow();
  }

  await expect(
    verifyProjection(
      item,
      'live',
      bytes(precise.replace('900719925474099512345', '900719925474099512346')),
    ),
  ).rejects.toThrow();
  await expect(
    verifyProjection(
      { ...item, updated_at: '2026-10-27T08:00:00.123456788Z' },
      'live',
      bytes(precise),
    ),
  ).rejects.toThrow();
  const origin = { kind: 'version', id: 'archived', epoch: '' },
    pointer = {
      object_id: await driveObject('version_binary', 'archived'),
      reference: { room_id: room, event_id: '$physical' },
      origin,
    };
  expect((await bodyPointer(pointer, item, 'binary')).origin).toEqual(origin);
  await expect(bodyPointer(pointer, item, 'metadata')).rejects.toThrow();
  await expect(
    bodyPointer({ ...pointer, origin: { ...origin, id: 'foreign' } }, item, 'binary'),
  ).rejects.toThrow();
});
test('a rejected download discards its caller staging once without reading foreign media', async () => {
  const { drive, state } = await fixture();
  await drive.initialize(state);
  let aborted = 0,
    wrote = false,
    committed = false;
  await expect(
    drive.download('foreign', {
      write: async () => {
        wrote = true;
      },
      commit: async () => {
        committed = true;
      },
      abort: async () => {
        aborted++;
      },
    }),
  ).rejects.toThrow('not_found');
  expect({ aborted, wrote, committed }).toEqual({ aborted: 1, wrote: false, committed: false });
  await drive.close();
});

test('private children keep native Unicode ordering, page beyond 500 files and pin cursors to a folder', async () => {
  const { memory, drive, state } = await fixture();
  const added = [
    ...Array.from({ length: 499 }, (_, i) => ({
      id: 'bulk_' + String(i).padStart(3, '0'),
      name: 'z' + String(i).padStart(3, '0'),
    })),
    { id: 'case-a', name: 'İZ' },
    { id: 'case-b', name: 'IZ' },
    { id: 'sigma-a', name: 'ΟΣ' },
    { id: 'sigma-b', name: 'ος' },
  ];
  for (const { id, name } of added) {
    const raw = bytes(JSON.stringify({ id, folder_id: 'root', name, size_bytes: 0, version: 1 }));
    memory.add(
      await driveObject('file', id),
      {
        kind: driveItemKind,
        source_kind: 'file',
        source_id: id,
        parent_id: 'root',
        name,
        version: 1,
        raw_sha256: await digest(raw),
        updated_at: '0001-01-01T00:00:00Z',
      },
      {
        kind: 'org.sytue.drive.entry.v1',
        source_kind: 'file',
        source_id: id,
        raw: encodeStorageJSON(raw),
        binary: {
          object_id: await driveObject('file_binary', id),
          reference: { room_id: room, event_id: '$unused_' + id },
        },
      },
      id,
    );
  }
  await drive.initialize(state);
  const first = await drive.children('root');
  expect(first.files.length).toBe(500);
  expect(first.next_cursor).not.toBe('');
  expect(first.files.slice(0, 3).map((f) => f.id)).toEqual(['case-a', 'case-b', 'file']);
  const next = await drive.children('root', first.next_cursor);
  expect(next.next_cursor).toBe('');
  expect(next.files.map((f) => f.id)).toEqual(['bulk_497', 'bulk_498', 'sigma-b', 'sigma-a']);
  const foreign = encodeStorageJSON(bytes(JSON.stringify({ Folder: 'foreign', After: 'file' })))
    .replaceAll('+', '-')
    .replaceAll('/', '_')
    .replace(/=+$/, '');
  await expect(drive.children('root', foreign)).rejects.toThrow();
  await drive.close();
});

test('a divergent completion is an explicit conflict and cannot replace the last private snapshot', async () => {
  const { memory, drive, state } = await fixture();
  await drive.initialize(state);
  const receipt = structuredClone(memory.indexes.at(-1)!);
  receipt.reference.event_id = '$completion_branch';
  receipt.record.revision_id = 'completion_branch';
  // Otro inventario de la misma época: dos importaciones que no leyeron lo mismo.
  const commit = JSON.parse(atob(receipt.record.payload)) as { metadata: string };
  const proof = JSON.parse(atob(commit.metadata)) as { inventory_sha256: string };
  proof.inventory_sha256 = '0'.repeat(64);
  commit.metadata = encodeStorageJSON(bytes(JSON.stringify(proof)));
  receipt.record.payload = encodeStorageJSON(bytes(JSON.stringify(commit)));
  memory.indexes.push(receipt);
  await expect(drive.refresh()).rejects.toThrow('conflict');
  expect((await drive.file('file')).id).toBe('file');
  await drive.close();
});

test('two identical completions of the same frozen source (two imports at once) still open', async () => {
  const { memory, drive, state } = await fixture();
  const receipt = structuredClone(memory.indexes.at(-1)!);
  receipt.reference.event_id = '$completion_branch';
  receipt.record.revision_id = 'completion_branch';
  memory.indexes.push(receipt);
  await drive.initialize(state);
  expect((await drive.root()).id).toBe('root');
  expect((await drive.file('file')).id).toBe('file');
  await drive.close();
});

test('late private read cancellation zeroes bytes before device cleanup completes', async () => {
  const { drive, storage, state } = await fixture();
  await drive.initialize(state);
  const original = storage.withJob.bind(storage);
  let captured: Uint8Array | undefined, closing: Promise<void> | undefined;
  storage.withJob = async <T>(
    action: (signal: AbortSignal) => Promise<T>,
    signal?: AbortSignal,
  ): Promise<T> => {
    const value = await original(action, signal);
    if (value instanceof Uint8Array) {
      captured = value;
      closing = drive.close();
    }
    return value;
  };
  await expect(drive.readRaw('file', 'file')).rejects.toThrow();
  await closing;
  expect(captured!.length).toBeGreaterThan(0);
  expect(captured!.every((byte) => byte === 0)).toBe(true);
});
