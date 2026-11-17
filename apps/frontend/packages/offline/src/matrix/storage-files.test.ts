/* SPDX-License-Identifier: Apache-2.0 */
import { readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { expect, test } from 'bun:test';
import { Attachment, initAsync } from '@matrix-org/matrix-sdk-crypto-wasm';
import { createSHA256 } from 'hash-wasm';
import { maxCipherBlockBytes, type CipherBlobReceipt } from '@sytue/config/matrix';
import { StorageRepository } from './storage-repository.js';
import { StorageJobs, type StorageJobRunner } from './storage-jobs.js';
import { StorageFiles, maxStorageFileBytes, type StorageFileSink } from './storage-files.js';
import {
  createStorageRecord,
  encodeStorageJSON,
  storageJSONBytes,
  type StorageRecord,
} from './storage.js';
import type { MatrixReference, MatrixStorageScope } from './types.js';
const scope: MatrixStorageScope = { tenant: 'acme', subject: 'person', product: 'drive' };
const ref = (name: string): MatrixReference => ({
  room_id: '!private:matrix.test',
  event_id: '$' + name,
});
const metadata = encodeStorageJSON(
  new TextEncoder().encode('{"title":"private <file>","amount":900719925474099512345}'),
);
async function digest(data: Uint8Array<ArrayBuffer>): Promise<string> {
  return [...new Uint8Array(await crypto.subtle.digest('SHA-256', data))]
    .map((value) => value.toString(16).padStart(2, '0'))
    .join('');
}
async function fixture(size = 73, run?: StorageJobRunner) {
  await initAsync();
  const blobs = new Map<string, Uint8Array<ArrayBuffer>>(),
    records = new Map<string, StorageRecord>();
  const leaves: {
    block: { blob: CipherBlobReceipt; matrix_file: unknown; plain_sha256: string };
  }[] = [];
  const all = await createSHA256();
  for (let index = 0; index < Math.max(1, Math.ceil(size / maxCipherBlockBytes)); index++) {
    const data = new Uint8Array(
      Math.min(maxCipherBlockBytes, Math.max(0, size - index * maxCipherBlockBytes)),
    ).fill(index + 1);
    all.update(data);
    const encrypted = Attachment.encrypt(data);
    try {
      const ciphertext = encrypted.encryptedData as Uint8Array<ArrayBuffer>;
      const id = 'A'.repeat(26) + String.fromCharCode(65 + index);
      blobs.set(id, ciphertext);
      leaves.push({
        block: {
          blob: { id, product: 'drive', size_bytes: data.length, sha256: await digest(ciphertext) },
          matrix_file: JSON.parse(encrypted.mediaEncryptionInfo!) as unknown,
          plain_sha256: await digest(data),
        },
      });
    } finally {
      data.fill(0);
      encrypted.free();
    }
  }
  let root: unknown = { kind: 'org.sytue.file.page.v1', level: 0, index: 0, nodes: leaves };
  if (leaves.length > 16) {
    const nodes = [];
    for (let index = 0; index < Math.ceil(leaves.length / 16); index++) {
      const id = 'leaf-' + index;
      records.set(
        '$' + id,
        createStorageRecord(
          scope,
          'object',
          'revision',
          'manifest',
          new TextEncoder().encode(
            JSON.stringify({
              kind: 'org.sytue.file.page.v1',
              level: 0,
              index,
              nodes: leaves.slice(index * 16, index * 16 + 16),
            }),
          ),
        ),
      );
      nodes.push({ event: ref(id) });
    }
    root = { kind: 'org.sytue.file.page.v1', level: 1, index: 0, nodes };
  }
  const document = {
    kind: 'org.sytue.file.v1',
    request: { object_id: 'object', revision_id: 'revision', size_bytes: size, metadata },
    sha256: all.digest('hex'),
    root,
  };
  const save = () =>
    records.set(
      '$root',
      createStorageRecord(
        scope,
        'object',
        'revision',
        'manifest',
        new TextEncoder().encode(JSON.stringify(document)),
      ),
    );
  save();
  const requests: string[] = [];
  let closed = false;
  const files = new StorageFiles(
    scope,
    {
      ...(run ? { run } : {}),
      assertOpen: () => {
        if (closed) throw new Error('closed artificial device');
      },
      readRecord: async (reference) => {
        const value = records.get(reference.event_id);
        if (!value || reference.room_id !== '!private:matrix.test')
          throw new Error('missing artificial record');
        return structuredClone(value);
      },
    },
    {
      read: async (id) => {
        requests.push(id);
        const bytes = blobs.get(id);
        if (!bytes) throw new Error('missing artificial block');
        return bytes.slice();
      },
    },
  );
  return {
    files,
    document,
    blobs,
    records,
    requests,
    save,
    close: () => {
      closed = true;
    },
  };
}
function staging() {
  const result = { bytes: 0, committed: false, aborted: false, chunks: 0 };
  const sink: StorageFileSink = {
    write: async (bytes) => {
      result.bytes += bytes.length;
      result.chunks++;
    },
    commit: async () => {
      result.committed = true;
    },
    abort: async () => {
      result.aborted = true;
    },
  };
  return { result, sink };
}

test('native-format pages verify 17 bounded Matrix blocks, exact private metadata and ranges across a boundary', async () => {
  const size = 16 * maxCipherBlockBytes + 73,
    f = await fixture(size),
    staged = staging();
  expect((await f.files.info(ref('root'), 'object', 'revision')).metadata).toBe(metadata);
  expect(f.requests).toHaveLength(0);
  const range = await f.files.readRange(
    ref('root'),
    'object',
    'revision',
    maxCipherBlockBytes - 4,
    8,
  );
  expect(range).toEqual(new Uint8Array([1, 1, 1, 1, 2, 2, 2, 2]));
  const result = await f.files.download(ref('root'), 'object', 'revision', staged.sink);
  expect(result.size_bytes).toBe(size);
  expect(staged.result).toEqual({ bytes: size, chunks: 17, committed: true, aborted: false });
  const directory = process.env.SYTUE_STORAGE_WEB_FILE_VECTOR_DIR;
  if (directory) {
    if (!path.isAbsolute(directory) || (await readdir(directory)).length)
      throw new Error('empty absolute fixture directory required');
    for (const [id, data] of f.blobs)
      await writeFile(path.join(directory, id), data, { mode: 0o600 });
    await writeFile(
      path.join(directory, 'vector.json'),
      JSON.stringify({
        artificial: true,
        selected: {
          offset: maxCipherBlockBytes - 4,
          bytes: btoa(String.fromCharCode(1, 1, 1, 1, 2, 2, 2, 2)),
        },
        scope,
        reference: ref('root'),
        request: f.document.request,
        entries: [...f.records].map(([event, record]) => ({
          reference: ref(event.slice(1)),
          record,
        })),
      }),
      { mode: 0o600 },
    );
  }
}, 60_000);

test('an empty native-format file still authenticates its single empty Matrix block', async () => {
  const f = await fixture(0),
    staged = staging();
  await f.files.download(ref('root'), 'object', 'revision', staged.sink);
  expect(staged.result).toEqual({ bytes: 0, chunks: 1, committed: true, aborted: false });
  expect(await f.files.readRange(ref('root'), 'object', 'revision', 0, 0)).toEqual(
    new Uint8Array(),
  );
});

test.each([
  'whole-hash',
  'plain-hash',
  'ciphertext',
  'blob-size',
  'cipher-hash',
  'blob-hash',
  'key',
  'iv',
  'object',
  'revision',
  'scope',
  'parents',
  'metadata',
  'page-index',
  'page-kind',
])('%s tampering never commits a replacement and aborts staging', async (kind) => {
  const f = await fixture(),
    staged = staging();
  const root = f.document.root as {
    index: number;
    kind: string;
    nodes: {
      block: {
        blob: CipherBlobReceipt;
        matrix_file: { key: { k: string }; iv: string; hashes: { sha256: string } };
        plain_sha256: string;
      };
    }[];
  };
  const leaf = root.nodes[0]!.block;
  if (kind === 'whole-hash') f.document.sha256 = '0'.repeat(64);
  if (kind === 'plain-hash') leaf.plain_sha256 = '0'.repeat(64);
  if (kind === 'ciphertext') f.blobs.get(leaf.blob.id)![0]! ^= 1;
  if (kind === 'blob-size') leaf.blob.size_bytes++;
  if (kind === 'blob-hash') leaf.blob.sha256 = '0'.repeat(64);
  if (kind === 'cipher-hash') leaf.matrix_file.hashes.sha256 = 'A'.repeat(43);
  if (kind === 'key') leaf.matrix_file.key.k = 'A'.repeat(43);
  if (kind === 'iv') leaf.matrix_file.iv = 'A'.repeat(22);
  if (kind === 'object') f.document.request.object_id = 'other';
  if (kind === 'revision') f.document.request.revision_id = 'other';
  if (kind === 'parents') Object.assign(f.document.request, { previous: ['$other'] });
  if (kind === 'metadata') f.document.request.metadata = btoa('not JSON');
  if (kind === 'page-index') root.index = 1;
  if (kind === 'page-kind') root.kind = 'other';
  f.save();
  if (kind === 'scope') f.records.get('$root')!.scope.product = 'mail';
  await expect(f.files.download(ref('root'), 'object', 'revision', staged.sink)).rejects.toThrow();
  expect(staged.result.committed).toBe(false);
  expect(staged.result.aborted).toBe(true);
});

test('maximum root geometry is bounded without downloading an entire multi-terabyte file', async () => {
  const f = await fixture();
  f.document.request.size_bytes = maxStorageFileBytes;
  f.document.root = {
    kind: 'org.sytue.file.page.v1',
    level: 5,
    index: 0,
    nodes: Array.from({ length: 16 }, (_, index) => ({ event: ref('child-' + index) })),
  };
  f.save();
  expect((await f.files.info(ref('root'), 'object', 'revision')).size_bytes).toBe(
    maxStorageFileBytes,
  );
  expect(f.requests).toHaveLength(0);
  f.document.request.size_bytes++;
  f.save();
  await expect(f.files.info(ref('root'), 'object', 'revision')).rejects.toThrow();
});

test('abort and logout after asynchronous reads cannot publish plaintext or commit staging', async () => {
  for (const reason of ['signal', 'logout']) {
    const f = await fixture(),
      staged = staging(),
      signal = new AbortController();
    staged.sink.write = async () => {
      if (reason === 'signal') signal.abort();
      else f.close();
    };
    await expect(
      f.files.download(ref('root'), 'object', 'revision', staged.sink, signal.signal),
    ).rejects.toThrow();
    expect(staged.result.committed).toBe(false);
    expect(staged.result.aborted).toBe(true);
  }
});

test('range limits and object/revision context fail before fetching ciphertext', async () => {
  const f = await fixture();
  for (const [offset, length] of [
    [-1, 1],
    [0, -1],
    [0, maxCipherBlockBytes + 1],
    [73, 1],
    [74, 0],
    [0.5, 1],
  ])
    await expect(
      f.files.readRange(ref('root'), 'object', 'revision', offset!, length!),
    ).rejects.toThrow();
  await expect(f.files.readRange(ref('root'), 'object', 'other', 0, 1)).rejects.toThrow();
  expect(f.requests).toHaveLength(0);
  const original = JSON.parse(
    new TextDecoder().decode(storageJSONBytes(f.records.get('$root')!.payload)),
  ) as { request: { metadata: string } };
  expect(original.request.metadata).toBe(metadata);
});

test('file catalog publication verifies every stored block before writing an index revision', async () => {
  const { StorageRepository } = await import('./storage-repository.js');
  const f = await fixture();
  let published = 0;
  const repository = new StorageRepository(
    scope,
    {
      rooms: ['!private:matrix.test'],
      assertOpen: () => {},
      put: async (_transaction, record) => {
        const reference = ref('published-' + ++published);
        f.records.set(reference.event_id, record);
        return reference;
      },
      read: async (reference) => {
        const record = f.records.get(reference.event_id);
        if (!record) throw new Error('missing artificial record');
        return record;
      },
      history: async () => ({ entries: [], scanned: 0, next: null }),
    },
    {
      read: async (id) => {
        const data = f.blobs.get(id);
        if (!data) throw new Error('missing artificial block');
        return data.slice();
      },
    },
  );
  const commit = {
    kind: 'org.sytue.object.v1' as const,
    state: 'live' as const,
    metadata,
    content: { kind: 'file' as const, revision_id: 'revision', reference: ref('root') },
  };
  const first = await repository.putObject('first-file', 'object', 'published', commit);
  expect(first.commit.content).toEqual(commit.content);
  expect(published).toBe(1);
  f.document.sha256 = '0'.repeat(64);
  f.save();
  await expect(
    repository.putObject('bad-file', 'object', 'bad-published', commit),
  ).rejects.toThrow();
  expect(published).toBe(1);
});

test('file shutdown waits for asynchronous staged plaintext disposal and never commits a cancelled stream', async () => {
  const jobs = new StorageJobs();
  const f = await fixture(73);
  const repository = new StorageRepository(
    scope,
    {
      rooms: ['!private:matrix.test'],
      assertOpen: () => {},
      run: (name, action, parent) => jobs.run(name, action, parent),
      read: async (reference) => {
        const record = f.records.get(reference.event_id);
        if (!record) throw new Error('missing artificial record');
        return structuredClone(record);
      },
      put: async () => {
        throw new Error('unexpected fixture write');
      },
      history: async () => {
        throw new Error('unexpected fixture history');
      },
    },
    {
      read: async (id) => {
        const data = f.blobs.get(id);
        if (!data) throw new Error('missing artificial block');
        return data.slice();
      },
    },
  );
  let began!: () => void, release!: () => void, aborting!: () => void, cleaned!: () => void;
  const entered = new Promise<void>((resolve) => {
      began = resolve;
    }),
    gate = new Promise<void>((resolve) => {
      release = resolve;
    });
  const cleanupStarted = new Promise<void>((resolve) => {
      aborting = resolve;
    }),
    cleanupGate = new Promise<void>((resolve) => {
      cleaned = resolve;
    });
  let committed = false,
    stopped = false,
    aborted = false;
  const pending = repository.files().download(ref('root'), 'object', 'revision', {
    write: async () => {
      began();
      await gate;
    },
    commit: async () => {
      committed = true;
    },
    abort: async () => {
      aborted = true;
      aborting();
      await cleanupGate;
    },
  });
  const caught = Promise.allSettled([pending]);
  await entered;
  const closing = jobs.close().then(() => {
    stopped = true;
  });
  try {
    expect(jobs.empty).toBe(false);
    expect(stopped).toBe(false);
    release();
    await cleanupStarted;
    expect(stopped).toBe(false);
    expect(committed).toBe(false);
  } finally {
    release();
    cleaned();
    await caught;
    await closing;
  }
  expect(aborted).toBe(true);
  expect(committed).toBe(false);
  expect(stopped).toBe(true);
});

test('late job cancellation zeroes a plaintext range that has not been handed to its caller', async () => {
  let discarded: Uint8Array | undefined;
  const f = await fixture(73, async (_name, action, parent) => {
    const value = await action(parent ?? new AbortController().signal);
    if (value instanceof Uint8Array) discarded = value;
    throw new Error('late cancellation');
  });
  await expect(f.files.readRange(ref('root'), 'object', 'revision', 0, 12)).rejects.toThrow(
    'late cancellation',
  );
  expect(discarded?.every((byte) => byte === 0)).toBe(true);
});
