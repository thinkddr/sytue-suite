/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { privateDriveWriteFixture as fixture, room, bytes } from './private-drive-fixture.js';
import { json, driveItemKind } from './private-drive-codec.js';
import { digest } from './storage-file-codec.js';
import { storageJSONBytes, encodeStorageJSON } from './storage.js';
test.each(['content', 'index'])(
  'new folders retain their exact identity and timestamps across a lost %s ACK and restart',
  async (role) => {
    const f = await fixture(),
      first = f.open();
    await first.initialize(f.state);
    let lost = false;
    f.hooks.recordAck = (record) => {
      const body = json(storageJSONBytes(record.payload));
      if (
        !lost &&
        record.role === role &&
        (body.kind === 'org.sytue.drive.entry.v1' || body.kind === 'org.sytue.object.v1')
      ) {
        lost = true;
        return Promise.reject(new Error('accepted without ACK'));
      }
      return Promise.resolve();
    };
    await expect(first.createFolder('root', ' New folder ')).rejects.toThrow();
    expect(lost).toBe(true);
    await first.close();
    const key = (await f.cache.listIDs()).find((k) => k.startsWith('drive_create_folder_'))!,
      saved = (await f.cache.read(key))!,
      intent = JSON.parse(new TextDecoder().decode(saved)) as {
        item: { source_id: string };
        raw: string;
      };
    saved.fill(0);
    const drive = f.open({ room_id: '!rotated:matrix.test', device_id: 'DEVICE' });
    await drive.initialize(f.state);
    const created = await drive.createFolder('root', 'New folder');
    expect(created.id).toBe(intent.item.source_id);
    expect(created.name).toBe('New folder');
    expect(created.created_at).toBe(created.updated_at);
    expect(await drive.readRaw('folder', created.id as string)).toEqual(
      storageJSONBytes(intent.raw),
    );
    expect(f.sends.every((s) => s.target.room_id === room)).toBe(true);
    expect(f.writes).toHaveLength(0);
    expect(
      (await f.cache.listIDs()).filter((k) => k.startsWith('drive_create_folder_')),
    ).toHaveLength(0);
    await expect(drive.createFolder('root', 'New folder')).rejects.toThrow('conflict');
    const parent = created.id as string,
      child = await drive.createFolder(parent, 'Child');
    await drive.move('file', 'file', child.id as string);
    await drive.delete('folder', parent);
    await expect(drive.file('file')).rejects.toThrow('not_found');
    await drive.restore('folder', parent);
    expect((await drive.file('file')).folder_id).toBe(child.id);
    const rows = f.entries().filter((e) => {
      if (e.record.role !== 'index') return false;
      const body = json(storageJSONBytes(e.record.payload));
      return json(storageJSONBytes(body.metadata)).kind === driveItemKind;
    });
    expect(rows.length).toBeGreaterThan(2);
    await drive.close();
    await f.close();
  },
);
test('pending folder creation cannot publish into a parent trashed by another operation', async () => {
  const f = await fixture();
  await f.addFolder('parent', 'root', 'Parent');
  const drive = f.open();
  await drive.initialize(f.state);
  f.hooks.recordAck = () => Promise.reject(new Error('lost ACK'));
  await expect(drive.createFolder('parent', 'Pending')).rejects.toThrow();
  delete f.hooks.recordAck;
  await drive.delete('folder', 'parent');
  const before = f.sends.length;
  await expect(drive.createFolder('parent', 'Pending')).rejects.toThrow('not_found');
  expect(f.sends).toHaveLength(before);
  await drive.restore('folder', 'parent');
  expect((await drive.createFolder('parent', 'Pending')).parent_id).toBe('parent');
  await drive.close();
  await f.close();
});
test('altered folder journal owner, raw bytes and root flag are rejected before publication', async () => {
  for (const field of ['scope', 'raw', 'item']) {
    const f = await fixture(),
      drive = f.open();
    await drive.initialize(f.state);
    f.hooks.recordAck = () => Promise.reject(new Error('lost ACK'));
    await expect(drive.createFolder('root', 'Pending')).rejects.toThrow();
    delete f.hooks.recordAck;
    const key = (await f.cache.listIDs()).find((k) => k.startsWith('drive_create_folder_'))!,
      saved = (await f.cache.read(key))!,
      intent = JSON.parse(new TextDecoder().decode(saved)) as Record<string, unknown>;
    saved.fill(0);
    if (field === 'scope') (intent.scope as Record<string, unknown>).subject = 'foreign';
    if (field === 'raw') {
      const row = json(storageJSONBytes(intent.raw as string));
      row.owner_sub = 'foreign';
      const changed = bytes(JSON.stringify(row));
      intent.raw = encodeStorageJSON(changed);
      (intent.item as Record<string, unknown>).raw_sha256 = await digest(changed);
      changed.fill(0);
    }
    if (field === 'item') (intent.item as Record<string, unknown>).is_root = true;
    const changed = bytes(JSON.stringify(intent));
    await f.cache.write(key, changed);
    changed.fill(0);
    const before = f.sends.length;
    await expect(drive.createFolder('root', 'Pending')).rejects.toThrow();
    expect(f.sends).toHaveLength(before);
    await drive.close();
    await f.close();
  }
});
