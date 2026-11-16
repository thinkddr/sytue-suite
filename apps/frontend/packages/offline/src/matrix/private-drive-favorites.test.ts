/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { privateDriveWriteFixture, bytes, scope, room, epoch } from './private-drive-fixture.js';
import { driveFavoriteID } from './private-drive-favorites.js';
import { driveObject, json } from './private-drive-codec.js';
import { createStorageRecord, encodeStorageJSON, storageJSONBytes } from './storage.js';
import { digest } from './storage-file-codec.js';
const at = '2026-10-29T10:00:00.123456789Z';
const mark = (id: string, kind = 'file', target = 'file') => ({
  id,
  subject: scope.subject,
  [kind + '_id']: target,
  created_at: at,
});

test('favorite UUID matches the native URL-namespace golden vector', async () => {
  expect(await driveFavoriteID(scope, { kind: 'file', id: 'file', on: true })).toBe(
    '293af8db-4607-5a09-b331-2bc38d7523d4',
  );
});
test('personal stars preserve imported IDs, unknown exact numbers and their baseline across removal and re-add', async () => {
  const f = await privateDriveWriteFixture(),
    raw = JSON.stringify(mark('imported')).replace(
      '}',
      ',"extension":{"integer":900719925474099512345},"padding":"' + 'x'.repeat(17000) + '"}',
    );
  await f.addFavorite('imported', raw, epoch);
  const drive = f.open();
  await drive.initialize(f.state);
  try {
    expect((await drive.favorites()).map(({ row }) => row.id)).toEqual(['file']);
    const before = f.sends.length;
    await drive.setFavorite('file', 'file', true);
    expect(f.sends).toHaveLength(before);
    await drive.setFavorite('file', 'file', false);
    expect(await drive.favorites()).toEqual([]);
    const removed = await drive.readRaw('favorite', 'imported');
    try {
      expect(new TextDecoder().decode(removed)).toContain('900719925474099512345');
      expect(json(removed).deleted_at).toBeString();
    } finally {
      removed.fill(0);
    }
    await drive.setFavorite('file', 'file', true);
    const added = await drive.readRaw('favorite', 'imported');
    try {
      expect(new TextDecoder().decode(added)).toContain('900719925474099512345');
      expect(json(added).deleted_at).toBeNull();
    } finally {
      added.fill(0);
    }
    const object = await driveObject('favorite', 'imported'),
      version = (await f.repository().catalog()).find((v) => v.object_id === object)!;
    expect(json(storageJSONBytes(version.commit.metadata)).baseline_sha256).toBe(
      await digest(bytes(raw)),
    );
    expect((await f.cache.listIDs()).filter((k) => k.startsWith('drive_favorite_'))).toEqual([]);
  } finally {
    await drive.close();
    await f.close();
  }
});
test('favorites sort folders first, retain foreign targets, and hide children of deleted ancestors', async () => {
  const f = await privateDriveWriteFixture();
  await f.addFolder('parent', 'root', '100%');
  await f.addFolder('child', 'parent', 'Child');
  await f.addFile('nested', 'child', 'Nested', at);
  await f.addFavorite('foreign', mark('foreign', 'file', 'foreign-file'));
  const drive = f.open();
  await drive.initialize(f.state);
  try {
    await drive.setFavorite('file', 'file', true);
    await drive.setFavorite('file', 'nested', true);
    await drive.setFavorite('folder', 'child', true);
    const rows = await drive.favorites();
    expect(rows[0]?.path).toBe('/100%/Child');
    expect(rows.find((v) => v.row.id === 'nested')?.path).toBe('/100%/Child/Nested');
    expect(rows).toHaveLength(3);
    await expect(drive.setFavorite('folder', 'root', true)).rejects.toThrow('immutable_root');
    await drive.delete('folder', 'parent');
    expect((await drive.favorites()).map((v) => v.row.id)).toEqual(['file']);
    await expect(drive.setFavorite('file', 'nested', true)).rejects.toThrow('not_found');
    await drive.setFavorite('file', 'foreign-file', false);
    await drive.restore('folder', 'parent');
    expect(await drive.favorites()).toHaveLength(3);
  } finally {
    await drive.close();
    await f.close();
  }
});
test.each(['subject', 'duplicate', 'fork', 'two_targets', 'zero_created'])(
  'invalid %s favorite is rejected before publication',
  async (field) => {
    const f = await privateDriveWriteFixture(),
      row = mark('first');
    if (field === 'subject') row.subject = 'foreign';
    if (field === 'two_targets') row.folder_id = 'root';
    if (field === 'zero_created') row.created_at = '0001-01-01T00:00:00Z';
    await f.addFavorite('first', row);
    if (field === 'duplicate') await f.addFavorite('second', mark('second'));
    if (field === 'fork') {
      const object = await driveObject('favorite', 'first'),
        index = f
          .entries()
          .find((e) => e.record.object_id === object && e.record.role === 'index')!;
      f.seed({
        reference: { room_id: room, event_id: '$fork' },
        record: createStorageRecord(
          scope,
          object,
          'fork',
          'index',
          storageJSONBytes(index.record.payload),
        ),
      });
    }
    const drive = f.open();
    await drive.initialize(f.state);
    try {
      await expect(drive.favorites()).rejects.toThrow();
      await expect(drive.setFavorite('file', 'file', false)).rejects.toThrow();
      expect(f.sends).toHaveLength(0);
    } finally {
      await drive.close();
      await f.close();
    }
  },
);
test.each(['content', 'index'])(
  'favorite survives lost %s ACK, restart and storage-room rotation',
  async (role) => {
    const f = await privateDriveWriteFixture(),
      first = f.open();
    await first.initialize(f.state);
    let lost = false;
    f.hooks.recordAck = async (record) => {
      if (!lost && record.role === role) {
        lost = true;
        throw new Error('lost ACK');
      }
    };
    await expect(first.setFavorite('file', 'file', true)).rejects.toThrow('lost ACK');
    await first.close();
    delete f.hooks.recordAck;
    const drive = f.open({ room_id: '!rotated:matrix.test', device_id: 'DEVICE' });
    await drive.initialize(f.state);
    try {
      await drive.setFavorite('file', 'file', true);
      expect((await drive.favorites()).map((v) => v.row.id)).toEqual(['file']);
      expect(f.sends.every((s) => s.target.room_id === room)).toBe(true);
      expect((await f.cache.listIDs()).filter((k) => k.startsWith('drive_favorite_'))).toEqual([]);
    } finally {
      await drive.close();
      await f.close();
    }
  },
);
test.each(['scope', 'raw', 'target', 'base'])(
  'altered favorite journal %s cannot cause a new remote effect',
  async (field) => {
    const f = await privateDriveWriteFixture(),
      drive = f.open();
    await drive.initialize(f.state);
    f.hooks.recordAck = async () => {
      throw new Error('lost ACK');
    };
    await expect(drive.setFavorite('file', 'file', true)).rejects.toThrow();
    delete f.hooks.recordAck;
    const key = (await f.cache.listIDs()).find((k) => k.startsWith('drive_favorite_'))!,
      saved = (await f.cache.read(key))!,
      intent = json(saved);
    saved.fill(0);
    if (field === 'scope') (intent.scope as Record<string, unknown>).subject = 'foreign';
    if (field === 'target') (intent.target as Record<string, unknown>).device_id = 'OTHER';
    if (field === 'base') intent.base = { room_id: room, event_id: '$file' };
    if (field === 'raw') {
      const raw = storageJSONBytes(intent.raw as string),
        row = json(raw);
      raw.fill(0);
      row.subject = 'foreign';
      const changed = bytes(JSON.stringify(row));
      intent.raw = encodeStorageJSON(changed);
      (intent.item as Record<string, unknown>).raw_sha256 = await digest(changed);
      changed.fill(0);
    }
    const changed = bytes(JSON.stringify(intent));
    await f.cache.write(key, changed);
    changed.fill(0);
    const before = f.sends.length;
    try {
      await expect(drive.setFavorite('file', 'file', true)).rejects.toThrow();
      expect(f.sends).toHaveLength(before);
    } finally {
      await drive.close();
      await f.close();
    }
  },
);
test('a retained favorite cannot publish into a deleted ancestor or overwrite a newer star', async () => {
  const f = await privateDriveWriteFixture();
  await f.addFolder('parent', 'root', 'Parent');
  await f.addFile('nested', 'parent', 'Nested', at);
  const drive = f.open();
  await drive.initialize(f.state);
  try {
    f.hooks.recordAck = async () => {
      throw new Error('lost ACK');
    };
    await expect(drive.setFavorite('file', 'nested', true)).rejects.toThrow();
    delete f.hooks.recordAck;
    await drive.delete('folder', 'parent');
    const before = f.sends.length;
    await expect(drive.setFavorite('file', 'nested', true)).rejects.toThrow('not_found');
    expect(f.sends).toHaveLength(before);
    await drive.restore('folder', 'parent');
    await drive.setFavorite('file', 'nested', true);
    f.hooks.recordAck = async () => {
      throw new Error('lost ACK');
    };
    await expect(drive.setFavorite('file', 'nested', false)).rejects.toThrow();
    delete f.hooks.recordAck;
    // A second device has an independent encrypted workflow journal.
    const other = await privateDriveWriteFixture();
    for (const entry of f.entries())
      if (!other.records.has(entry.reference.event_id)) other.seed(entry);
    const second = other.open();
    await second.initialize(f.state);
    try {
      await second.setFavorite('file', 'nested', false);
      await second.setFavorite('file', 'nested', true);
      for (const entry of other.entries())
        if (!f.records.has(entry.reference.event_id)) f.seed(entry);
    } finally {
      await second.close();
      await other.close();
    }
    const after = f.sends.length;
    await expect(drive.setFavorite('file', 'nested', false)).rejects.toThrow('conflict');
    expect(f.sends).toHaveLength(after);
  } finally {
    await drive.close();
    await f.close();
  }
});
