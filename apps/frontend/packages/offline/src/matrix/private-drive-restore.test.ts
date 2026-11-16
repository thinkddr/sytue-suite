/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { privateDriveWriteFixture, bytes, room, epoch } from './private-drive-fixture.js';
import { driveObject, json, type DriveBodyPointer } from './private-drive-codec.js';
import { encodeStorageJSON, storageJSONBytes } from './storage.js';
import { digest } from './storage-file-codec.js';

async function fixture() {
  const f = await privateDriveWriteFixture(),
    drive = f.open();
  await drive.initialize(f.state);
  const folder = await drive.createFolder('root', 'History'),
    first = await drive.upload(folder.id as string, 'History.txt', 'text/plain', new Blob(['A']));
  await drive.upload(folder.id as string, 'History.txt', 'text/plain', new Blob(['B']));
  return {
    f,
    drive,
    id: first.id as string,
    folder: folder.id as string,
    close: async () => {
      await drive.close();
      await f.close();
    },
  };
}
async function content(
  drive: Awaited<ReturnType<typeof fixture>>['drive'],
  id: string,
  version?: number,
) {
  const chunks: Uint8Array[] = [],
    sink = {
      write: async (chunk: Uint8Array) => {
        chunks.push(chunk.slice());
      },
      commit: async () => {},
      abort: async () => {
        throw new Error('unexpected aborted read');
      },
    };
  if (version) await drive.downloadVersion(id, version, sink);
  else await drive.download(id, sink);
  return new TextDecoder().decode(Buffer.concat(chunks));
}
test.each(['content', 'index'])(
  'restore publishes one new version across lost %s ACK, restart and room rotation',
  async (role) => {
    const x = await fixture(),
      { f, id } = x;
    let lost = false;
    f.hooks.recordAck = async (record) => {
      if (!lost && record.role === role) {
        lost = true;
        throw new Error('lost ACK');
      }
    };
    await expect(x.drive.restoreVersion(id, 1)).rejects.toThrow('lost ACK');
    await x.drive.close();
    delete f.hooks.recordAck;
    const drive = f.open({ room_id: '!rotated:matrix.test', device_id: 'DEVICE' });
    await drive.initialize(f.state);
    const before = f.writes.length;
    try {
      await drive.restoreVersion(id, 1);
      expect((await drive.versions(id)).versions.map((v) => v.version)).toEqual([3, 2, 1]);
      expect(await content(drive, id)).toBe('A');
      expect(await content(drive, id, 2)).toBe('B');
      expect(f.writes).toHaveLength(before);
      expect(f.sends.every((s) => s.target.room_id === room)).toBe(true);
      expect(
        (await f.cache.listIDs()).filter((k) => k.startsWith('drive_restore_version_')),
      ).toEqual([]);
    } finally {
      await drive.close();
      await f.close();
    }
  },
);
test('restoring an imported revision preserves exact large metadata, classification and migration lineage', async () => {
  const x = await fixture(),
    { f, drive } = x,
    object = await driveObject('file', x.id),
    first = f.entries().find((e) => e.record.object_id === object && e.record.role === 'content')!,
    source = json(storageJSONBytes(first.record.payload)),
    binary = {
      ...(source.binary as DriveBodyPointer),
      origin: { kind: 'file' as const, id: x.id, epoch: '' },
    },
    raw =
      '{"id":"imported","folder_id":"root","name":"Imported.txt","size_bytes":1,"content_type":"text/plain","version":1,"updated_at":"2026-01-01T00:00:00.123456789Z","label_id":"restricted","label_name":"Restricted","label_block_external_share":true,"extension":{"integer":900719925474099512345},"padding":"' +
      'x'.repeat(17000) +
      '"}';
  await f.addFileRow('imported', raw, binary, epoch);
  await drive.refresh();
  try {
    await drive.upload('root', 'Imported.txt', 'text/plain', new Blob(['B']));
    await drive.restoreVersion('imported', 1);
    const restored = await drive.readRaw('file', 'imported');
    try {
      expect(new TextDecoder().decode(restored)).toContain('900719925474099512345');
    } finally {
      restored.fill(0);
    }
    expect(await content(drive, 'imported')).toBe('A');
    expect((await drive.file('imported')).label).toEqual({
      name: 'Restricted',
      block_external_share: true,
    });
    expect((await drive.file('imported')).label_id).toBe('restricted');
    const imported = await driveObject('file', 'imported'),
      current = (await f.repository().catalog()).find((v) => v.object_id === imported);
    expect(json(storageJSONBytes(current!.commit.metadata)).baseline_sha256).toBe(
      await digest(bytes(raw)),
    );
  } finally {
    await x.close();
  }
});
test.each(['parent', 'newer'])(
  'retained restore revalidates %s before publishing',
  async (changed) => {
    const x = await fixture(),
      { f, drive, id } = x;
    try {
      f.hooks.recordAck = async () => {
        throw new Error('lost ACK');
      };
      await expect(drive.restoreVersion(id, 1)).rejects.toThrow();
      delete f.hooks.recordAck;
      if (changed === 'parent') await drive.delete('folder', x.folder);
      else await drive.upload(x.folder, 'History.txt', 'text/plain', new Blob(['C']));
      const before = f.sends.length;
      await expect(drive.restoreVersion(id, 1)).rejects.toThrow(
        changed === 'parent' ? 'not_found' : 'conflict',
      );
      expect(f.sends).toHaveLength(before);
      if (changed === 'parent') {
        await drive.restore('folder', x.folder);
        await drive.restoreVersion(id, 1);
        expect(await content(drive, id)).toBe('A');
      }
    } finally {
      await x.close();
    }
  },
);
test.each(['raw', 'source', 'base', 'target'])(
  'altered restore %s journal cannot add a remote effect',
  async (field) => {
    const x = await fixture(),
      { f, drive, id } = x;
    try {
      f.hooks.recordAck = async () => {
        throw new Error('lost ACK');
      };
      await expect(drive.restoreVersion(id, 1)).rejects.toThrow();
      delete f.hooks.recordAck;
      const key = (await f.cache.listIDs()).find((k) => k.startsWith('drive_restore_version_'))!,
        saved = (await f.cache.read(key))!,
        intent = json(saved);
      saved.fill(0);
      if (field === 'raw') {
        const value = storageJSONBytes(intent.raw as string),
          row = json(value);
        value.fill(0);
        row.name = 'Substituted';
        const next = bytes(JSON.stringify(row));
        intent.raw = encodeStorageJSON(next);
        (intent.item as Record<string, unknown>).name = 'Substituted';
        (intent.item as Record<string, unknown>).raw_sha256 = await digest(next);
        next.fill(0);
      }
      if (field === 'source')
        (intent.restored as Record<string, unknown>).object_id = await driveObject(
          'file',
          'foreign',
        );
      if (field === 'base') intent.base = { room_id: room, event_id: '$file' };
      if (field === 'target') (intent.target as Record<string, unknown>).device_id = 'OTHER';
      const next = bytes(JSON.stringify(intent));
      await f.cache.write(key, next);
      next.fill(0);
      const before = f.sends.length;
      await expect(drive.restoreVersion(id, 1)).rejects.toThrow();
      expect(f.sends).toHaveLength(before);
    } finally {
      await x.close();
    }
  },
);
