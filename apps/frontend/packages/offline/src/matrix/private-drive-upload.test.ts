/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { privateDriveWriteFixture as fixture, bytes, room } from './private-drive-fixture.js';
import { driveItemKind, json } from './private-drive-codec.js';
import { storageJSONBytes } from './storage.js';
import type { PrivateDriveRepository } from './private-drive.js';
async function download(drive: PrivateDriveRepository, id: string) {
  const chunks: Uint8Array<ArrayBuffer>[] = [];
  let committed = false;
  await drive.download(id, {
    write: (chunk) => {
      chunks.push(chunk.slice());
      return Promise.resolve();
    },
    commit: () => {
      committed = true;
      return Promise.resolve();
    },
    abort: () => Promise.reject(new Error('unexpected download abort')),
  });
  expect(committed).toBe(true);
  const out = new Uint8Array(chunks.reduce((n, c) => n + c.length, 0));
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.length;
    chunk.fill(0);
  }
  return out;
}
const source = () =>
  new Blob([bytes('ARTIFICIAL PRIVATE DRIVE SOURCE ' + 'ñ 漢字 '.repeat(11000))]);
test('new private uploads replace by name, retain old ciphertext and clean captures only after catalog verification', async () => {
  const f = await fixture(),
    drive = f.open(),
    input = source();
  await drive.initialize(f.state);
  const first = await drive.upload('root', ' New.bin ', 'application/octet-stream', input),
    id = first.id as string;
  expect(first.version).toBe(1);
  expect(first.name).toBe('New.bin');
  expect(await download(drive, id)).toEqual(new Uint8Array(await input.arrayBuffer()));
  const prior = f
    .entries()
    .find(
      (e) =>
        e.record.role === 'manifest' &&
        json(storageJSONBytes(e.record.payload)).kind === 'org.sytue.file.v1',
    )!;
  const priorRequest = json(storageJSONBytes(prior.record.payload)).request as {
    object_id: string;
    revision_id: string;
    size_bytes: number;
  };
  const second = await drive.upload('root', 'New.bin', 'text/plain', new Blob());
  expect(second.id).toBe(id);
  expect(second.version).toBe(2);
  expect(await download(drive, id)).toHaveLength(0);
  expect(
    await f
      .repository()
      .files()
      .readRange(
        prior.reference,
        priorRequest.object_id,
        priorRequest.revision_id,
        0,
        priorRequest.size_bytes,
      ),
  ).toEqual(new Uint8Array(await input.arrayBuffer()));
  expect(await f.cache.listIDs()).toHaveLength(0);
  for (const cipher of f.blobs.values())
    expect(new TextDecoder().decode(cipher)).not.toContain('ARTIFICIAL PRIVATE DRIVE SOURCE');
  await drive.close();
  await f.close();
});
test('replacing an imported file preserves lineage, sensitivity and unknown numeric lexemes', async () => {
  const f = await fixture(),
    drive = f.open();
  await drive.initialize(f.state);
  const file = await drive.upload('root', 'Private file', 'text/plain', source());
  expect(file.id).toBe('file');
  expect(file.version).toBe(2);
  expect((file.label as { block_external_share: boolean }).block_external_share).toBe(true);
  expect(new TextDecoder().decode(await drive.readRaw('file', 'file'))).toContain(
    '900719925474099512345',
  );
  expect(await download(drive, 'file')).toEqual(new Uint8Array(await source().arrayBuffer()));
  await drive.close();
  await f.close();
});
test.each(['blob', 'body', 'content', 'index'])(
  'a lost %s ACK resumes from the encrypted source after restart and room rotation without selecting the file again',
  async (stage) => {
    const f = await fixture(),
      first = f.open();
    await first.initialize(f.state);
    let lost = false;
    f.hooks.blobAck = () => {
      if (stage === 'blob' && !lost) {
        lost = true;
        return Promise.reject(new Error('accepted block without ACK'));
      }
      return Promise.resolve();
    };
    f.hooks.recordAck = (record) => {
      const data = json(storageJSONBytes(record.payload));
      let match = false;
      if (stage === 'content')
        match = record.role === 'content' && data.kind === 'org.sytue.drive.entry.v1';
      if (record.role === 'index') {
        const meta = json(storageJSONBytes(data.metadata));
        match =
          (stage === 'index' && meta.kind === driveItemKind) ||
          (stage === 'body' &&
            meta.kind === 'org.sytue.drive.body.v1' &&
            meta.purpose === 'binary');
      }
      if (match && !lost) {
        lost = true;
        return Promise.reject(new Error('accepted record without ACK'));
      }
      return Promise.resolve();
    };
    await expect(
      first.upload('root', 'Pending.bin', 'application/octet-stream', source()),
    ).rejects.toThrow();
    expect(lost).toBe(true);
    await first.close();
    const drive = f.open({ room_id: '!rotated:matrix.test', device_id: 'DEVICE' });
    await drive.initialize(f.state);
    const result = await drive.resumeUploads();
    expect(result.failed).toHaveLength(0);
    expect(result.completed).toHaveLength(1);
    expect(await download(drive, result.completed[0]!.id as string)).toEqual(
      new Uint8Array(await source().arrayBuffer()),
    );
    expect(f.sends.every((s) => s.target.room_id === room)).toBe(true);
    expect(await f.cache.listIDs()).toHaveLength(0);
    await drive.close();
    await f.close();
  },
);
test('an explicit retry with changed source cannot use retained keys or publish another block', async () => {
  const f = await fixture(),
    drive = f.open();
  await drive.initialize(f.state);
  let lost = false;
  f.hooks.blobAck = () => {
    if (!lost) {
      lost = true;
      return Promise.reject(new Error('lost block ACK'));
    }
    return Promise.resolve();
  };
  await expect(
    drive.upload('root', 'Pending', 'text/plain', new Blob([bytes('first')])),
  ).rejects.toThrow();
  const writes = f.writes.length,
    sends = f.sends.length;
  await expect(
    drive.upload('root', 'Pending', 'text/plain', new Blob([bytes('other')])),
  ).rejects.toThrow();
  expect(f.writes).toHaveLength(writes);
  expect(f.sends).toHaveLength(sends);
  const resumed = await drive.resumeUploads();
  expect(resumed.failed).toHaveLength(0);
  expect(await download(drive, resumed.completed[0]!.id as string)).toEqual(bytes('first'));
  expect(await f.cache.listIDs()).toHaveLength(0);
  await drive.close();
  await f.close();
});
test('a trashed upload destination leaves a recoverable failure without publishing until it is restored', async () => {
  const f = await fixture();
  await f.addFolder('parent', 'root', 'Parent');
  const drive = f.open();
  await drive.initialize(f.state);
  let lost = false;
  f.hooks.blobAck = () => {
    if (!lost) {
      lost = true;
      return Promise.reject(new Error('lost ACK'));
    }
    return Promise.resolve();
  };
  await expect(drive.upload('parent', 'Pending', 'text/plain', source())).rejects.toThrow();
  await drive.delete('folder', 'parent');
  const sends = f.sends.length,
    writes = f.writes.length;
  const failed = await drive.resumeUploads();
  expect(failed.completed).toHaveLength(0);
  expect(failed.failed).toHaveLength(1);
  expect(f.sends).toHaveLength(sends);
  expect(f.writes).toHaveLength(writes);
  await drive.restore('folder', 'parent');
  expect((await drive.resumeUploads()).completed).toHaveLength(1);
  await drive.close();
  await f.close();
});

test('one invalid pending job is isolated while another upload completes', async () => {
  const f = await fixture(),
    drive = f.open();
  await drive.initialize(f.state);
  f.hooks.blobAck = () => Promise.reject(new Error('lost ACK'));
  for (const name of ['Invalid', 'Valid'])
    await expect(drive.upload('root', name, 'text/plain', source())).rejects.toThrow();
  delete f.hooks.blobAck;
  const ids = await f.cache.listIDs(10, 'drive_upload_');
  expect(ids).toHaveLength(2);
  for (const id of ids) {
    const raw = (await f.cache.read(id))!;
    try {
      const intent = JSON.parse(new TextDecoder().decode(raw)) as {
        request: { name: string };
        target: { device_id: string };
      };
      if (intent.request.name === 'Invalid') {
        intent.target.device_id = 'FOREIGN';
        const changed = bytes(JSON.stringify(intent));
        await f.cache.write(id, changed);
        changed.fill(0);
      }
    } finally {
      raw.fill(0);
    }
  }
  const result = await drive.resumeUploads();
  expect(result.failed).toHaveLength(1);
  expect(result.completed).toHaveLength(1);
  expect(result.completed[0]!.name).toBe('Valid');
  expect(await f.cache.listIDs(10, 'drive_upload_')).toHaveLength(1);
  await drive.close();
  await f.close();
});
test('an ambiguous source cleanup resumes an already confirmed catalog without reading removed source blocks', async () => {
  const f = await fixture(),
    drive = f.open();
  await drive.initialize(f.state);
  const discard = f.cache.delete.bind(f.cache);
  let lost = false;
  f.cache.delete = async (id) => {
    await discard(id);
    if (!lost && id.startsWith('stage_') && !/_\d+$/.test(id)) {
      lost = true;
      throw new Error('source marker delete accepted without ACK');
    }
  };
  await expect(drive.upload('root', 'Cleanup', 'text/plain', source())).rejects.toThrow();
  expect(lost).toBe(true);
  const sends = f.sends.length,
    writes = f.writes.length;
  const result = await drive.resumeUploads();
  expect(result.failed).toHaveLength(0);
  expect(result.completed).toHaveLength(1);
  expect(f.sends).toHaveLength(sends);
  expect(f.writes).toHaveLength(writes);
  expect(await f.cache.listIDs()).toHaveLength(0);
  await drive.close();
  await f.close();
});
test('a concurrent metadata edit cannot be overwritten by resuming an older replacement', async () => {
  const f = await fixture(),
    drive = f.open();
  await drive.initialize(f.state);
  f.hooks.blobAck = () => Promise.reject(new Error('lost ACK'));
  await expect(drive.upload('root', 'Private file', 'text/plain', source())).rejects.toThrow();
  delete f.hooks.blobAck;
  await drive.rename('file', 'file', 'Renamed while pending');
  const sends = f.sends.length,
    writes = f.writes.length,
    result = await drive.resumeUploads();
  expect(result.failed).toHaveLength(1);
  expect(result.completed).toHaveLength(0);
  expect(f.sends).toHaveLength(sends);
  expect(f.writes).toHaveLength(writes);
  expect((await drive.file('file')).name).toBe('Renamed while pending');
  await drive.close();
  await f.close();
});
