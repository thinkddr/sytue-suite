/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { privateDriveWriteFixture } from './private-drive-fixture.js';
import { driveObject, json, type DriveBodyPointer } from './private-drive-codec.js';
import { storageJSONBytes } from './storage.js';

test('private history retains content revisions without inventing versions for metadata edits', async () => {
  const f = await privateDriveWriteFixture(),
    drive = f.open();
  await drive.initialize(f.state);
  try {
    const folder = await drive.createFolder('root', 'Versions'),
      first = await drive.upload(
        folder.id as string,
        'Version.txt',
        'text/plain',
        new Blob(['first private version']),
      ),
      id = first.id as string;
    await drive.upload(
      folder.id as string,
      'Version.txt',
      'text/plain',
      new Blob(['second private version']),
    );
    await drive.rename('file', id, 'Renamed.txt');
    f.hooks.read = async () => {
      throw new Error('listing must not fetch binary blocks');
    };
    const page = await drive.versions(id);
    expect(page.versions.map((v) => v.version)).toEqual([2, 1]);
    expect(page.versions.map((v) => v.is_current)).toEqual([true, false]);
    expect(page.next_cursor).toBe('');
    expect((await drive.versions(id, '2')).versions.map((v) => v.version)).toEqual([1]);
    delete f.hooks.read;
    const chunks: Uint8Array[] = [];
    let committed = 0,
      aborted = 0;
    const sink = {
      write: async (chunk: Uint8Array) => {
        chunks.push(chunk.slice());
      },
      commit: async () => {
        committed++;
      },
      abort: async () => {
        aborted++;
      },
    };
    await drive.downloadVersion(id, 1, sink);
    expect(new TextDecoder().decode(Buffer.concat(chunks))).toBe('first private version');
    expect(committed).toBe(1);
    expect(aborted).toBe(0);
    await expect(drive.downloadVersion(id, 2, sink)).rejects.toThrow('not_found');
    expect(aborted).toBe(1);
    await drive.delete('folder', folder.id as string);
    await expect(drive.versions(id)).rejects.toThrow('not_found');
    await expect(drive.downloadVersion(id, 1, sink)).rejects.toThrow('not_found');
    expect(aborted).toBe(2);
    await drive.restore('folder', folder.id as string);
    expect((await drive.versions(id)).versions).toHaveLength(2);
  } finally {
    await drive.close();
    await f.close();
  }
});
test.each([false, true])(
  'retained same-number archives are deduplicated or reject divergent content (%s)',
  async (divergent) => {
    const f = await privateDriveWriteFixture(),
      drive = f.open();
    await drive.initialize(f.state);
    try {
      const file = await drive.upload('root', 'History.txt', 'text/plain', new Blob(['A'])),
        id = file.id as string;
      await drive.upload('root', 'History.txt', 'text/plain', new Blob(['B']));
      const object = await driveObject('file', id),
        entries = f
          .entries()
          .filter((e) => e.record.object_id === object && e.record.role === 'content'),
        selected = json(storageJSONBytes(entries[divergent ? 1 : 0]!.record.payload)),
        binary = {
          ...(selected.binary as DriveBodyPointer),
          origin: { kind: 'file' as const, id, epoch: '' },
        };
      await f.addVersion(
        'legacy',
        {
          id: 'legacy',
          file_id: id,
          version: 1,
          size_bytes: 1,
          content_type: 'text/plain',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
        binary,
      );
      await drive.refresh();
      if (divergent) await expect(drive.versions(id)).rejects.toThrow('conflict');
      else {
        const page = await drive.versions(id);
        expect(page.versions).toHaveLength(2);
        expect(page.versions[1]?.created_at).toBe('2026-01-01T00:00:00Z');
        const chunks: Uint8Array[] = [];
        await drive.downloadVersion(id, 1, {
          write: async (chunk) => {
            chunks.push(chunk.slice());
          },
          commit: async () => {},
          abort: async () => {
            throw new Error('unexpected archive abort');
          },
        });
        expect(new TextDecoder().decode(Buffer.concat(chunks))).toBe('A');
      }
    } finally {
      await drive.close();
      await f.close();
    }
  },
);
