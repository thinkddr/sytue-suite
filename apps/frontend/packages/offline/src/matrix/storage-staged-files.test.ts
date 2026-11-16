/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { storageWriteFixture } from './storage-write-fixture.js';
import { storageSourceBlockBytes } from './storage-file-upload.js';
import { digest } from './storage-file-codec.js';
import { encodeStorageJSON } from './storage.js';
const signal = () => new AbortController().signal;
const pattern = new TextEncoder().encode('ARTIFICIAL PRIVATE STAGED INPUT ñ 漢字');
function original(size = 2 * storageSourceBlockBytes + 31) {
  const bytes = new Uint8Array(size);
  for (let n = 0; n < size; n++) bytes[n] = pattern[n % pattern.length]!;
  return { bytes, blob: new Blob([bytes]) };
}
function request(size: number) {
  return {
    object_id: 'object',
    revision_id: 'revision',
    size_bytes: size,
    metadata: encodeStorageJSON(new TextEncoder().encode('{}')),
  };
}
test.each([0, 2 * storageSourceBlockBytes + 31])(
  'captured %s-byte sources reopen and decrypt ranges without retaining plaintext or the original Blob',
  async (size) => {
    const f = storageWriteFixture(),
      source = original(size),
      staged = await f.repository().stagedFiles().capture(source.blob);
    expect(staged.sha256).toBe(await digest(source.bytes));
    const reader = await f.repository().stagedFiles().open(staged),
      start = Math.max(0, storageSourceBlockBytes - 4),
      length = size ? 12 : 0;
    const offset = size ? start : 0,
      range = await reader.read(offset, length, signal());
    expect(range).toEqual(source.bytes.slice(offset, offset + length));
    range.fill(0);
    for (const record of f.memory.records.values()) {
      if (record.namespace !== 'local-crypto-value-v1') continue;
      const value = record.value as { v: number; ciphertext: Uint8Array; iv: Uint8Array };
      expect(value.v).toBe(1);
      expect(value.iv).toHaveLength(12);
      expect(new TextDecoder().decode(value.ciphertext)).not.toContain(
        'ARTIFICIAL PRIVATE STAGED INPUT',
      );
    }
    const ref = await f.repository().uploads().uploadSource('captured', request(size), reader);
    expect(await f.repository().files().readRange(ref, 'object', 'revision', 0, size)).toEqual(
      source.bytes,
    );
    expect(await f.cache.listIDs()).toContainEqual(expect.stringContaining('stage_'));
    await f.repository().stagedFiles().discard(staged);
    await f.repository().stagedFiles().discard(staged);
    expect((await f.cache.listIDs()).filter((k) => k.startsWith('stage_'))).toHaveLength(0);
    expect(new Uint8Array(await source.blob.arrayBuffer())).toEqual(source.bytes);
    await f.close();
  },
);
test('source changes, missing blocks, and foreign descriptors fail before publishing any ciphertext', async () => {
  for (const corruption of ['changed', 'missing', 'foreign']) {
    const f = storageWriteFixture(),
      source = original(),
      stager = f.repository().stagedFiles(),
      file = await stager.capture(source.blob),
      key = (await f.cache.listIDs()).find((k) => k.startsWith('stage_') && k.endsWith('_0'))!;
    if (corruption === 'foreign') {
      await expect(
        Promise.resolve().then(() =>
          stager.open({ ...file, scope: { ...file.scope, subject: 'foreign' } }),
        ),
      ).rejects.toThrow();
    } else {
      if (corruption === 'missing') await f.cache.delete(key);
      else await f.cache.write(key, new Uint8Array(storageSourceBlockBytes).fill(7));
      const reader = await stager.open(file);
      await expect(
        f.repository().uploads().uploadSource('captured', request(source.bytes.length), reader),
      ).rejects.toThrow();
    }
    expect(f.writes).toHaveLength(0);
    expect(f.sends).toHaveLength(0);
    await f.close();
  }
});
test('a capture interrupted by quota or an ambiguous marker write clears every attempted block', async () => {
  for (const kind of ['quota', 'marker']) {
    const f = storageWriteFixture(),
      source = original(),
      write = f.cache.write.bind(f.cache);
    let calls = 0;
    f.cache.write = async (key, bytes) => {
      if (!key.startsWith('stage_')) return write(key, bytes);
      calls++;
      if (kind === 'quota' && calls === 2) throw new Error('quota fixture');
      await write(key, bytes);
      if (kind === 'marker' && !/_\d+$/.test(key)) throw new Error('marker accepted without ACK');
    };
    await expect(f.repository().stagedFiles().capture(source.blob)).rejects.toThrow();
    expect((await f.cache.listIDs()).filter((k) => k.startsWith('stage_'))).toHaveLength(0);
    expect(f.writes).toHaveLength(0);
    expect(f.sends).toHaveLength(0);
    await f.close();
  }
});
test('shutdown drains an in-flight capture before cleanup and no cancelled capture succeeds', async () => {
  const f = storageWriteFixture(),
    source = original(),
    write = f.cache.write.bind(f.cache);
  let release!: () => void, arrived!: () => void;
  const waiting = new Promise<void>((resolve) => {
      arrived = resolve;
    }),
    gate = new Promise<void>((resolve) => {
      release = resolve;
    });
  let once = false;
  f.cache.write = async (key, bytes) => {
    await write(key, bytes);
    if (!once && key.startsWith('stage_')) {
      once = true;
      arrived();
      await gate;
    }
  };
  const capturing = f.repository().stagedFiles().capture(source.blob),
    caught = Promise.allSettled([capturing]);
  await waiting;
  let closed = false;
  const closing = f.close().then(() => {
    closed = true;
  });
  expect(closed).toBe(false);
  release();
  await closing;
  expect((await caught)[0]!.status).toBe('rejected');
  expect((await f.cache.listIDs()).filter((k) => k.startsWith('stage_'))).toHaveLength(0);
  expect(f.writes).toHaveLength(0);
});
test('descriptor cleanup rejects altered size before deleting the retained source', async () => {
  const f = storageWriteFixture(),
    stager = f.repository().stagedFiles(),
    file = await stager.capture(original().blob),
    before = await f.cache.listIDs();
  await expect(stager.discard({ ...file, size_bytes: file.size_bytes + 1 })).rejects.toThrow();
  expect(await f.cache.listIDs()).toEqual(before);
  await stager.discard(file);
  await f.close();
});

test('a descriptor delivered after cancellation is cleared before the reader is rejected', async () => {
  const f = storageWriteFixture(),
    stager = f.repository().stagedFiles(),
    file = await stager.capture(original().blob),
    read = f.cache.read.bind(f.cache),
    parent = new AbortController();
  let delivered: Uint8Array<ArrayBuffer> | undefined;
  f.cache.read = async (key) => {
    const bytes = await read(key);
    if (key.endsWith(file.id) && bytes) {
      delivered = bytes;
      parent.abort(new Error('cancel after descriptor read'));
    }
    return bytes;
  };
  await expect(stager.open(file, parent.signal)).rejects.toThrow();
  expect(delivered).toBeDefined();
  expect(delivered!.every((value) => value === 0)).toBe(true);
  await f.close();
});

function httpSource(data: Uint8Array<ArrayBuffer>, parts = [65537, 17, 90001]) {
  let offset = 0,
    pulls = 0,
    cancelled = 0;
  const yielded: Uint8Array<ArrayBuffer>[] = [];
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>(
    {
      pull(controller) {
        pulls++;
        if (offset === data.length) {
          controller.close();
          return;
        }
        const next = data.slice(
          offset,
          Math.min(data.length, offset + parts[(pulls - 1) % parts.length]!),
        );
        offset += next.length;
        yielded.push(next);
        controller.enqueue(next);
      },
      cancel() {
        cancelled++;
      },
    },
    { highWaterMark: 0 },
  );
  return {
    body,
    yielded,
    get pulls() {
      return pulls;
    },
    get received() {
      return offset;
    },
    get cancelled() {
      return cancelled;
    },
  };
}
test.each([0, 2 * storageSourceBlockBytes + 31])(
  'HTTP capture verifies %s bytes and EOF while clearing consumed input chunks',
  async (size) => {
    const f = storageWriteFixture(),
      data = original(size),
      http = httpSource(data.bytes),
      stager = f.repository().stagedFiles(),
      file = await stager.captureStream(http.body, size, await digest(data.bytes));
    expect(file.sha256).toBe(await digest(data.bytes));
    expect(http.received).toBe(size);
    expect(http.cancelled).toBe(0);
    expect(http.body.locked).toBe(false);
    expect(http.yielded.every((chunk) => chunk.every((byte) => byte === 0))).toBe(true);
    const source = await stager.open(file);
    expect(await source.read(0, size, signal())).toEqual(data.bytes);
    await stager.discard(file);
    await f.close();
  },
);
test.each(['short', 'long', 'hash'])(
  'an HTTP %s mismatch never leaves a completed source or reaches the provider',
  async (kind) => {
    const f = storageWriteFixture(),
      data = original(),
      http = httpSource(data.bytes),
      size = data.bytes.length + (kind === 'short' ? 1 : kind === 'long' ? -1 : 0),
      expected = kind === 'hash' ? '0'.repeat(64) : undefined;
    await expect(
      f.repository().stagedFiles().captureStream(http.body, size, expected),
    ).rejects.toThrow();
    expect(await f.cache.listIDs()).toHaveLength(0);
    expect(f.writes).toHaveLength(0);
    expect(f.sends).toHaveLength(0);
    expect(http.body.locked).toBe(false);
    expect(http.yielded.every((chunk) => chunk.every((byte) => byte === 0))).toBe(true);
    await f.close();
  },
);
test('HTTP capture rejects a trailing chunk after the exact advertised bytes', async () => {
  const f = storageWriteFixture(),
    chunks = [new Uint8Array(17).fill(3), new Uint8Array(1).fill(9)];
  const body = new ReadableStream<Uint8Array<ArrayBuffer>>({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(chunk);
      controller.close();
    },
  });
  await expect(f.repository().stagedFiles().captureStream(body, 17, undefined)).rejects.toThrow();
  expect(await f.cache.listIDs()).toHaveLength(0);
  expect(f.writes).toHaveLength(0);
  expect(chunks.every((chunk) => chunk.every((byte) => byte === 0))).toBe(true);
  expect(body.locked).toBe(false);
  await f.close();
});
test('HTTP capture pauses upstream while encrypted cache writes are pending and drains cancellation', async () => {
  const f = storageWriteFixture(),
    data = original(),
    http = httpSource(data.bytes, [65536]),
    write = f.cache.write.bind(f.cache);
  let release!: () => void, arrived!: () => void;
  const gate = new Promise<void>((resolve) => {
      release = resolve;
    }),
    waiting = new Promise<void>((resolve) => {
      arrived = resolve;
    });
  let once = false;
  f.cache.write = async (key, bytes) => {
    await write(key, bytes);
    if (!once && key.startsWith('stage_')) {
      once = true;
      arrived();
      await gate;
    }
  };
  const pending = f
      .repository()
      .stagedFiles()
      .captureStream(http.body, data.bytes.length, undefined),
    caught = Promise.allSettled([pending]);
  await waiting;
  expect(http.received).toBe(storageSourceBlockBytes);
  const closing = f.close();
  release();
  await closing;
  expect((await caught)[0]!.status).toBe('rejected');
  expect(http.cancelled).toBe(1);
  expect(http.body.locked).toBe(false);
  expect(await f.cache.listIDs()).toHaveLength(0);
});
test('cancelling a pending HTTP read waits for the underlying cancellation and does not acknowledge a capture', async () => {
  const f = storageWriteFixture();
  let arrived!: () => void,
    finish!: () => void,
    cancelled = false,
    closed = false;
  const waiting = new Promise<void>((resolve) => {
      arrived = resolve;
    }),
    gate = new Promise<void>((resolve) => {
      finish = resolve;
    }),
    body = new ReadableStream<Uint8Array<ArrayBuffer>>(
      {
        pull() {
          arrived();
        },
        async cancel() {
          cancelled = true;
          await gate;
        },
      },
      { highWaterMark: 0 },
    );
  const capture = f.repository().stagedFiles().captureStream(body, 10, undefined),
    caught = Promise.allSettled([capture]);
  await waiting;
  const closing = f.close().then(() => {
    closed = true;
  });
  await Promise.resolve();
  expect(cancelled).toBe(true);
  expect(closed).toBe(false);
  finish();
  await closing;
  expect((await caught)[0]!.status).toBe('rejected');
  expect(body.locked).toBe(false);
  expect(await f.cache.listIDs()).toHaveLength(0);
});
test('a pre-cancelled capture closes its unclaimed HTTP body without reading it', async () => {
  const f = storageWriteFixture(),
    http = httpSource(original().bytes),
    parent = new AbortController();
  parent.abort();
  await expect(
    f.repository().stagedFiles().captureStream(http.body, 10, undefined, parent.signal),
  ).rejects.toThrow();
  expect(http.pulls).toBe(0);
  expect(http.cancelled).toBe(1);
  expect(http.body.locked).toBe(false);
  expect(await f.cache.listIDs()).toHaveLength(0);
  await f.close();
});
