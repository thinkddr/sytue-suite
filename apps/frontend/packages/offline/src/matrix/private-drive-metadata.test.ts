/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import {
  privateDriveWriteFixture as fixture,
  room,
  bytes,
  precise,
} from './private-drive-fixture.js';
import {
  patchDriveJSON,
  driveName,
  driveObject,
  driveItemKind,
  json,
} from './private-drive-codec.js';
import { storageJSONBytes, encodeStorageJSON, type StorageRecord } from './storage.js';
import { digest } from './storage-file-codec.js';
const descriptor = (record: StorageRecord) => json(storageJSONBytes(record.payload));
test('metadata patch keeps numeric lexemes, duplicate semantics and ordinary objects with number-like tags', () => {
  const input = bytes(
    '{"name":"old","name":"last","extension":{"n":900719925474099512345,"decimal":1.00000000000000000001,"minus":-0,"overflow":1e400,"tag":{"isLosslessNumber":true,"value":"12"}},"__proto__":{"safe":true}}',
  );
  const patched = patchDriveJSON(input, { name: 'new' }),
    text = new TextDecoder().decode(patched);
  for (const lexical of ['900719925474099512345', '1.00000000000000000001', '-0', '1e400'])
    expect(text).toContain(lexical);
  expect((JSON.parse(text) as { extension: { tag: unknown } }).extension.tag).toEqual({
    isLosslessNumber: true,
    value: '12',
  });
  expect((JSON.parse(text) as { name: string }).name).toBe('new');
  expect(Object.prototype).not.toHaveProperty('safe');
  for (const name of [' . ', '../no', 'a/b', 'a\\b', '\0', 'x'.repeat(256), '\ud800'])
    expect(() => driveName(name)).toThrow();
  expect(driveName('\u0085 good \u2003')).toBe('good');
  expect(driveName('\ufeffname')).toBe('\ufeffname');
});
test('private metadata retries retain room, binary reference, precise JSON and one catalog revision across both lost ACKs', async () => {
  const f = await fixture();
  let contentLost = false,
    indexLost = false;
  f.hooks.recordAck = async (record) => {
    const data = descriptor(record);
    if (record.role === 'content' && data.kind === 'org.sytue.drive.entry.v1' && !contentLost) {
      contentLost = true;
      throw new Error('lost content ACK');
    }
    if (record.role === 'index' && !indexLost) {
      const meta = json(storageJSONBytes(data.metadata));
      if (meta.kind === driveItemKind) {
        indexLost = true;
        throw new Error('lost index ACK');
      }
    }
  };
  const before = f.records.size;
  for (const stage of ['content', 'index', 'complete']) {
    const drive = f.open(
      stage === 'content' ? undefined : { room_id: '!rotated:matrix.test', device_id: 'DEVICE' },
    );
    await drive.initialize(f.state);
    if (stage === 'complete') await drive.rename('file', 'file', 'Renamed');
    else await expect(drive.rename('file', 'file', 'Renamed')).rejects.toThrow();
    await drive.close();
  }
  const drive = f.open();
  await drive.initialize(f.state);
  expect((await drive.file('file')).name).toBe('Renamed');
  expect(new TextDecoder().decode(await drive.readRaw('file', 'file'))).toContain(
    '900719925474099512345',
  );
  expect((await drive.file('file')).version).toBe(1);
  expect(f.records.size).toBe(before + 2);
  expect(f.writes).toHaveLength(0);
  expect(f.sends.every((s) => s.target.room_id === room)).toBe(true);
  expect((await f.cache.listIDs()).filter((id) => id.startsWith('drive_metadata_'))).toHaveLength(
    0,
  );
  await drive.close();
  await f.close();
});
test('private move/trash/restore rejects cycles and immutable root and hides a whole subtree without media transfer', async () => {
  const f = await fixture();
  await f.addFolder('parent', 'root', 'Parent');
  await f.addFolder('child', 'parent', 'Child');
  const drive = f.open();
  await drive.initialize(f.state);
  await drive.move('file', 'file', 'child');
  await expect(drive.move('folder', 'parent', 'child')).rejects.toThrow('cycle');
  await expect(drive.rename('folder', 'root', 'No')).rejects.toThrow('immutable_root');
  await drive.delete('folder', 'parent');
  await expect(drive.file('file')).rejects.toThrow('not_found');
  await expect(drive.rename('file', 'file', 'Hidden')).rejects.toThrow('not_found');
  await drive.restore('folder', 'parent');
  expect((await drive.file('file')).folder_id).toBe('child');
  expect(f.writes).toHaveLength(0);
  await drive.close();
  await f.close();
});
test('large private metadata is encrypted and acknowledged without replacing the original binary reference', async () => {
  const large = precise.replace(
      '"extension":{',
      '"padding":"' + 'x'.repeat(20 << 10) + '","extension":{',
    ),
    f = await fixture(large),
    drive = f.open();
  await drive.initialize(f.state);
  await drive.rename('file', 'file', 'Large renamed');
  expect(f.writes).toHaveLength(1);
  const raw = await drive.readRaw('file', 'file');
  expect(new TextDecoder().decode(raw)).toContain('900719925474099512345');
  expect(raw.length).toBeGreaterThan(16 << 10);
  expect((await drive.file('file')).version).toBe(1);
  expect(
    (await f.cache.listIDs()).filter(
      (id) => id.startsWith('drive_metadata_') || id.startsWith('j_drive_meta_body_'),
    ),
  ).toHaveLength(0);
  for (const body of f.blobs.values())
    expect(new TextDecoder().decode(body)).not.toContain('Large renamed');
  await drive.close();
  await f.close();
});
test('catalog writes from two repository instances serialize before capturing their parent', async () => {
  const f = await fixture(),
    a = f.open(),
    b = f.open();
  await a.initialize(f.state);
  await b.initialize(f.state);
  await Promise.all([a.rename('file', 'file', 'First'), b.rename('file', 'file', 'Second')]);
  await a.refresh();
  expect((await a.file('file')).name).toBe('Second');
  const heads = await f.repository().catalog(),
    object = await driveObject('file', 'file');
  expect(heads.filter((h) => h.object_id === object)).toHaveLength(1);
  await a.close();
  await b.close();
  await f.close();
});

test('a resumed metadata intent rejects altered raw bytes, phase and device before publishing', async () => {
  for (const corrupt of [
    async (intent: Record<string, unknown>) => {
      const raw = bytes(
        new TextDecoder()
          .decode(storageJSONBytes(intent.raw as string))
          .replace('"name":"Wanted"', '"name":"Injected"'),
      );
      intent.raw = encodeStorageJSON(raw);
      const item = intent.item as Record<string, unknown>;
      item.name = 'Injected';
      item.raw_sha256 = await digest(raw);
      raw.fill(0);
    },
    (intent: Record<string, unknown>) => {
      intent.catalog = { room_id: room, event_id: '$fake' };
    },
    (intent: Record<string, unknown>) => {
      intent.target = { room_id: room, device_id: 'FOREIGN' };
    },
  ]) {
    const f = await fixture(),
      drive = f.open();
    await drive.initialize(f.state);
    f.hooks.recordAck = () => Promise.reject(new Error('accepted without ACK'));
    await expect(drive.rename('file', 'file', 'Wanted')).rejects.toThrow();
    const keys = (await f.cache.listIDs()).filter((k) => k.startsWith('drive_metadata_'));
    expect(keys).toHaveLength(1);
    const raw = (await f.cache.read(keys[0]!))!,
      intent = JSON.parse(new TextDecoder().decode(raw)) as Record<string, unknown>;
    raw.fill(0);
    await corrupt(intent);
    const changed = bytes(JSON.stringify(intent));
    await f.cache.write(keys[0]!, changed);
    changed.fill(0);
    delete f.hooks.recordAck;
    const sends = f.sends.length;
    await expect(drive.rename('file', 'file', 'Wanted')).rejects.toThrow();
    expect(f.sends).toHaveLength(sends);
    expect((await drive.file('file')).name).toBe('Private file');
    await drive.close();
    await f.close();
  }
});
