/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { privateDriveWriteFixture } from './private-drive-fixture.js';
import { driveCursor, encodeDriveCursor } from './private-drive-cursor.js';

test('private recent pages timestamp ties after its previous cursor item has been trashed', async () => {
  const f = await privateDriveWriteFixture();
  for (let i = 1; i <= 101; i++)
    await f.addFile(
      'file_' + String(i).padStart(3, '0'),
      'root',
      'Reunión ' + i + '.txt',
      '2026-10-29T10:00:00.123456789Z',
    );
  const drive = f.open();
  await drive.initialize(f.state);
  try {
    const first = await drive.recent();
    expect(first.items).toHaveLength(50);
    expect(first.items[0]?.row.id).toBe('file_101');
    expect(first.items[49]?.row.id).toBe('file_052');
    expect(first.items[0]?.path).toBe('/Reunión 101.txt');
    const seen = new Set(first.items.map(({ row }) => row.id));
    let reads = 0;
    f.hooks.read = async () => {
      reads++;
    };
    const valid = driveCursor(first.next_cursor);
    for (const patch of [
      { account: 'f'.repeat(64) },
      { v: 2 },
      { before: 'invalid' },
      { kind: 'mail_recent' },
    ])
      await expect(drive.recent(encodeDriveCursor({ ...valid, ...patch }))).rejects.toThrow();
    expect(reads).toBe(0);
    delete f.hooks.read;
    await drive.delete('file', 'file_052');
    let cursor = first.next_cursor;
    while (cursor) {
      const page = await drive.recent(cursor);
      for (const { row } of page.items) {
        expect(seen.has(row.id)).toBe(false);
        seen.add(row.id);
      }
      cursor = page.next_cursor;
    }
    expect(seen.size).toBe(102); // 101 new rows plus the pre-existing private fixture file.
  } finally {
    await drive.close();
    await f.close();
  }
});

test('private recent uses nanoseconds, hides descendants and validates its owner before reading content', async () => {
  const f = await privateDriveWriteFixture();
  await f.addFolder('parent', 'root', 'Parent');
  await f.addFolder('child', 'parent', 'Child');
  await f.addFile('newer', 'child', 'Newer', '2026-10-29T10:00:00.123456789Z');
  await f.addFile('older', 'root', 'Older', '2026-10-29T10:00:00.123456788Z');
  const drive = f.open();
  await drive.initialize(f.state);
  try {
    const page = await drive.recent();
    expect(page.items[0]?.row.id).toBe('newer');
    expect(page.items[1]?.row.id).toBe('older');
    expect(page.items[0]?.path).toBe('/Parent/Child/Newer');
    await drive.delete('folder', 'parent');
    expect((await drive.recent()).items.map(({ row }) => row.id)).not.toContain('newer');
    await drive.restore('folder', 'parent');
    expect((await drive.recent()).items[0]?.row.id).toBe('newer');
    await expect(drive.recent('x'.repeat(4097))).rejects.toThrow();
    expect(() => driveCursor('not/base64')).toThrow();
  } finally {
    await drive.close();
    await f.close();
  }
});

test('private trash returns deleted roots once, preserves their subtree, and restores original paths', async () => {
  const f = await privateDriveWriteFixture();
  await f.addFolder('parent', 'root', 'Parent');
  await f.addFolder('child', 'parent', 'Child');
  await f.addFile('nested', 'child', 'Nested', '2026-10-29T10:00:00Z');
  const drive = f.open();
  await drive.initialize(f.state);
  try {
    await drive.delete('file', 'nested');
    await drive.delete('folder', 'parent');
    let trash = await drive.trash();
    expect(trash.map(({ row }) => row.id)).toEqual(['parent']);
    expect(trash[0]?.kind).toBe('folder');
    expect(trash[0]?.path).toBe('/Parent');
    await drive.restore('folder', 'parent');
    trash = await drive.trash();
    expect(trash.map(({ row }) => row.id)).toEqual(['nested']);
    expect(trash[0]?.path).toBe('/Parent/Child/Nested');
    await drive.restore('file', 'nested');
    expect(await drive.trash()).toEqual([]);
    expect((await drive.file('nested')).name).toBe('Nested');
  } finally {
    await drive.close();
    await f.close();
  }
});
