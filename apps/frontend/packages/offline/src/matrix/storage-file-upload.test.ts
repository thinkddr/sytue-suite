/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { storageWriteFixture as fixture } from './storage-write-fixture.js';
import { encodeStorageJSON, storageJSONBytes, storageObjectKind } from './storage.js';
import { maxCipherBlockBytes } from '@sytue/config/matrix';
import { type StorageFileRequest } from './storage-file-codec.js';
const target = { room_id: '!private:matrix.test', device_id: 'DEVICE' };
const metadata = encodeStorageJSON(
  new TextEncoder().encode('{"title":"private source","large":900719925474099512345}'),
);
const request: StorageFileRequest = {
  object_id: 'object',
  revision_id: 'revision',
  size_bytes: 73,
  metadata,
};
function source(size = 73): Blob {
  const parts = [];
  for (let index = 0; index < Math.max(1, Math.ceil(size / maxCipherBlockBytes)); index++)
    parts.push(
      new Uint8Array(
        Math.min(maxCipherBlockBytes, Math.max(0, size - index * maxCipherBlockBytes)),
      ).fill(index + 1),
    );
  return new Blob(parts);
}

test('real Rust writer produces 17-block native manifests; verified receipts survive caller restart and retain the original', async () => {
  const f = fixture(),
    size = 16 * maxCipherBlockBytes + 73,
    original = source(size),
    wanted = { ...request, size_bytes: size };
  const ref = await f.repository().uploads().upload('job', wanted, original);
  expect(f.blobs.size).toBe(17);
  expect(f.records.size).toBe(3);
  expect(
    await f
      .repository()
      .files()
      .readRange(ref, 'object', 'revision', maxCipherBlockBytes - 4, 8),
  ).toEqual(new Uint8Array([1, 1, 1, 1, 2, 2, 2, 2]));
  expect((await f.repository().files().info(ref, 'object', 'revision')).metadata).toBe(metadata);
  expect(await f.repository().uploads().upload('job', wanted, original)).toEqual(ref);
  expect(f.writes).toHaveLength(17);
  expect(f.records.size).toBe(3);
  expect((await f.journal())?.published).toEqual(ref);
  expect(new Uint8Array(await original.slice(0, 73).arrayBuffer())).toEqual(
    new Uint8Array(73).fill(1),
  );
}, 60_000);

test.each([0, 73])(
  'writer verifies a %s-byte source, including an empty authenticated block',
  async (size) => {
    const f = fixture(),
      ref = await f
        .repository()
        .uploads()
        .upload('job', { ...request, size_bytes: size }, source(size));
    expect(await f.repository().files().readRange(ref, 'object', 'revision', 0, size)).toEqual(
      new Uint8Array(size).fill(1),
    );
    expect(f.blobs.size).toBe(1);
  },
);

test('ambiguous provider PUT reuses exact cached ciphertext, ID, key and transaction after resume', async () => {
  const f = fixture();
  f.hooks.blobAck = async () => {
    throw new Error('lost acknowledgement');
  };
  await expect(f.repository().uploads().upload('job', request, source())).rejects.toThrow(
    'lost acknowledgement',
  );
  const pending = (await f.journal())?.pending;
  f.hooks.blobAck = undefined;
  const ref = await f.repository().uploads().upload('job', request, source());
  expect(f.writes).toHaveLength(2);
  expect(f.writes[1]).toEqual(f.writes[0]);
  expect(f.blobs.size).toBe(1);
  const document = JSON.parse(
    new TextDecoder().decode(storageJSONBytes(f.records.get(ref.event_id)!.payload)),
  ) as { root: { nodes: { block: unknown }[] } };
  expect(document.root.nodes[0]!.block).toEqual(pending);
});

test('a local checkpoint failure after provider acceptance retains the exact retry ciphertext', async () => {
  const f = fixture(),
    write = f.cache.write.bind(f.cache);
  let interrupted = false;
  f.cache.write = async (id, bytes) => {
    if (
      id === 'j_job' &&
      !interrupted &&
      (JSON.parse(new TextDecoder().decode(bytes)) as { next: number }).next === 1
    ) {
      interrupted = true;
      throw new Error('local checkpoint unavailable');
    }
    await write(id, bytes);
  };
  await expect(f.repository().uploads().upload('job', request, source())).rejects.toThrow(
    'local checkpoint unavailable',
  );
  expect((await f.journal())?.pending).toBeDefined();
  await f.repository().uploads().upload('job', request, source());
  expect(f.writes).toHaveLength(2);
  expect(f.writes[1]).toEqual(f.writes[0]);
  expect(f.blobs.size).toBe(1);
});

test.each(['page', 'root'])(
  'lost %s ACK retries the pinned room and endpoint transaction across room rotation',
  async (kind) => {
    const f = fixture(),
      size = kind === 'page' ? 16 * maxCipherBlockBytes + 73 : 73;
    let dropped = false;
    f.hooks.recordAck = async (record) => {
      const payload = JSON.parse(new TextDecoder().decode(storageJSONBytes(record.payload))) as {
        kind: string;
      };
      if (
        !dropped &&
        payload.kind === (kind === 'page' ? 'org.sytue.file.page.v1' : 'org.sytue.file.v1')
      ) {
        dropped = true;
        throw new Error('lost Matrix acknowledgement');
      }
    };
    const wanted = { ...request, size_bytes: size };
    await expect(f.repository().uploads().upload('job', wanted, source(size))).rejects.toThrow(
      'lost Matrix acknowledgement',
    );
    expect(dropped).toBe(true);
    const first = f.sends[0]!;
    const ref = await f
      .repository({ ...target, room_id: '!rotated:matrix.test' })
      .uploads()
      .upload('job', wanted, source(size));
    expect(ref.room_id).toBe(target.room_id);
    expect(f.sends[1]).toEqual(first);
    expect(f.records.size).toBe(kind === 'page' ? 3 : 1);
  },
  60_000,
);

test.each(['changed-bytes', 'metadata', 'revision', 'device', 'missing-room'])(
  'resumption rejects %s before any new upstream effect',
  async (kind) => {
    const f = fixture();
    f.hooks.blobAck = async () => {
      throw new Error('interrupted');
    };
    await expect(f.repository().uploads().upload('job', request, source())).rejects.toThrow();
    f.hooks.blobAck = undefined;
    const wanted = { ...request },
      publication = { ...target };
    let original = source();
    if (kind === 'changed-bytes') original = new Blob([new Uint8Array(73).fill(2)]);
    if (kind === 'metadata') wanted.metadata = encodeStorageJSON(new TextEncoder().encode('{}'));
    if (kind === 'revision') wanted.revision_id = 'changed';
    if (kind === 'device') publication.device_id = 'ANOTHER';
    const repo = f.repository(
      publication,
      kind === 'missing-room' ? ['!rotated:matrix.test'] : undefined,
    );
    await expect(
      Promise.resolve().then(() => repo.uploads().upload('job', wanted, original)),
    ).rejects.toThrow();
    expect(f.writes).toHaveLength(1);
    expect(f.sends).toHaveLength(0);
    expect((await f.journal())?.pending).toBeDefined();
  },
);

test.each(['corrupt-cache', 'missing-cache', 'missing-cache-and-provider'])(
  'pending %s cannot silently replace a descriptor',
  async (kind) => {
    const f = fixture();
    f.hooks.blobAck = async () => {
      throw new Error('interrupted');
    };
    await expect(f.repository().uploads().upload('job', request, source())).rejects.toThrow();
    f.hooks.blobAck = undefined;
    if (kind === 'corrupt-cache') await f.cache.write('b_job', new Uint8Array(73));
    else await f.cache.delete('b_job');
    if (kind === 'missing-cache-and-provider') f.blobs.clear();
    if (kind === 'missing-cache') {
      await f.repository().uploads().upload('job', request, source());
      expect(f.writes[1]).toEqual(f.writes[0]);
    } else {
      await expect(f.repository().uploads().upload('job', request, source())).rejects.toThrow();
      expect(f.writes).toHaveLength(1);
      expect(f.records.size).toBe(0);
    }
  },
);

test('cache quota failure before journal intent produces no upstream side effect and keeps the original', async () => {
  const f = fixture(),
    write = f.cache.write.bind(f.cache),
    original = source();
  f.cache.write = async (id, bytes) => {
    if (id.startsWith('b_')) throw new Error('quota');
    await write(id, bytes);
  };
  await expect(f.repository().uploads().upload('job', request, original)).rejects.toThrow('quota');
  expect(f.writes).toHaveLength(0);
  expect(f.records.size).toBe(0);
  expect((await f.journal())?.pending).toBeUndefined();
  expect(original.size).toBe(73);
});

test('a failed final readback retains the published receipt for repair and verifies it on the next attempt', async () => {
  const f = fixture();
  f.hooks.read = async () => {
    throw new Error('provider temporarily unavailable');
  };
  await expect(f.repository().uploads().upload('job', request, source())).rejects.toThrow(
    'provider temporarily unavailable',
  );
  const ref = (await f.journal())?.published;
  expect(ref).toBeDefined();
  f.hooks.read = undefined;
  expect(await f.repository().uploads().upload('job', request, source())).toEqual(ref);
  expect(f.writes).toHaveLength(1);
  expect(f.sends).toHaveLength(1);
});

test('device cancellation drains a pending PUT; concurrent repositories cannot resume the same job', async () => {
  const f = fixture();
  let started!: () => void, finish!: () => void;
  const entered = new Promise<void>((resolve) => {
      started = resolve;
    }),
    gate = new Promise<void>((resolve) => {
      finish = resolve;
    });
  f.hooks.blobAck = async () => {
    started();
    await gate;
  };
  const upload = f.repository().uploads().upload('job', request, source());
  const failure = upload.then(
    () => {
      throw new Error('cancelled upload succeeded');
    },
    () => undefined,
  );
  await entered;
  await expect(f.repository().uploads().upload('job', request, source())).rejects.toThrow(
    'matrix_locked',
  );
  let drained = false;
  const closing = f.close().then(() => {
    drained = true;
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(drained).toBe(false);
  finish();
  await closing;
  await failure;
  expect(drained).toBe(true);
  expect(f.records.size).toBe(0);
  expect((await f.journal())?.pending).toBeDefined();
});

test('upload acknowledgement preserves retry material until the exact live catalog is authenticated', async () => {
  const f = fixture(),
    repository = f.repository(),
    uploads = repository.uploads();
  const published = await uploads.upload('job', request, source());
  const commit = {
    kind: storageObjectKind,
    state: 'live' as const,
    metadata,
    content: { kind: 'file' as const, reference: published, revision_id: request.revision_id },
    parents: [],
  };
  const wrong = await repository.putObject('wrong', 'object', 'wrong', {
    ...commit,
    state: 'trash',
  });
  await expect(uploads.acknowledge('job', request, wrong.reference)).rejects.toThrow();
  expect(await f.journal()).not.toBeNull();
  const correct = await repository.putObject('correct', 'object', 'correct', commit);
  await expect(
    uploads.acknowledge(
      'job',
      { ...request, metadata: encodeStorageJSON(new TextEncoder().encode('{}')) },
      correct.reference,
    ),
  ).rejects.toThrow();
  expect(await f.journal()).not.toBeNull();
  expect(await uploads.acknowledge('job', request, correct.reference)).toBe(true);
  expect(await f.journal()).toBeNull();
  expect(await uploads.acknowledge('job', request, correct.reference)).toBe(false);
  expect((await f.cache.listIDs()).filter((k) => k === 'j_job' || k === 'b_job')).toHaveLength(0);
  await f.close();
});
