/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { StorageRepository, type StorageRecordTransport } from './storage-repository.js';
import {
  StorageRecordError,
  encodeStorageJSON,
  storageJSONBytes,
  storageObjectKind,
  type StorageRecord,
  type StorageRecordEntry,
} from './storage.js';
import {
  StorageSourceChangedError,
  maxCipherBlockBytes,
  sha256Hex,
  type BlobReceipt,
  type CipherBlobGateway,
} from './storage-blocks.js';
import {
  StreamingSha256,
  bytesStorageFileSource,
  filePageKind,
  fileRootLevel,
  fileBlocks,
  pendingUploadsNamespace,
  validateStorageFilePage,
  type StorageUploadJournal,
} from './storage-files.js';
import type { MatrixReference, MatrixStorageProduct, MatrixStorageScope } from './types.js';

const scope: MatrixStorageScope = { tenant: 'acme', subject: 'person', product: 'drive' };
const room = '!private:matrix.test';
const encoder = new TextEncoder();

class MemoryTransport implements StorageRecordTransport {
  readonly rooms = [room];
  readonly records = new Map<string, StorageRecord>();
  readonly transactions = new Map<string, MatrixReference>();
  closed = false;
  posts = 0;
  failOnPutCount = -1;

  assertOpen = (): void => {
    if (this.closed) throw new Error('closed');
  };

  put = async (transaction: string, record: StorageRecord): Promise<MatrixReference> => {
    this.posts++;
    if (this.posts === this.failOnPutCount) {
      throw new Error('simulated_manifest_network_drop');
    }
    let ref = this.transactions.get(transaction);
    if (!ref) {
      ref = { room_id: room, event_id: `$evt_${this.records.size + 1}` };
      this.transactions.set(transaction, ref);
      this.records.set(ref.event_id, structuredClone(record));
    }
    return ref;
  };

  read = async (reference: MatrixReference): Promise<StorageRecord> => {
    const found = this.records.get(reference.event_id);
    if (!found) throw new Error('missing_record');
    return structuredClone(found);
  };

  history = async (): Promise<{
    entries: StorageRecordEntry[];
    scanned: number;
    next: string | null;
  }> => {
    const entries = [...this.records.entries()].map(([event_id, record]) => ({
      reference: { room_id: room, event_id },
      record: structuredClone(record),
    }));
    return { entries, scanned: entries.length, next: null };
  };
}

class MemoryBlobGateway implements CipherBlobGateway {
  readonly blobs = new Map<
    string,
    { product: MatrixStorageProduct; bytes: Uint8Array<ArrayBuffer> }
  >();
  puts = 0;
  failOnPutNumber = -1;

  put = async (
    product: MatrixStorageProduct,
    id: string,
    ciphertext: Uint8Array<ArrayBuffer>,
  ): Promise<BlobReceipt> => {
    this.puts++;
    if (this.puts === this.failOnPutNumber) {
      throw new Error('simulated_blob_network_drop');
    }
    const copy = new Uint8Array(ciphertext.byteLength);
    copy.set(ciphertext);
    this.blobs.set(id, { product, bytes: copy });
    return {
      id,
      product,
      size_bytes: copy.byteLength,
      sha256: await sha256Hex(copy),
    };
  };

  read = async (product: MatrixStorageProduct, id: string): Promise<Uint8Array<ArrayBuffer>> => {
    const entry = this.blobs.get(id);
    if (!entry || entry.product !== product) throw new Error('missing_blob');
    const copy = new Uint8Array(entry.bytes.byteLength);
    copy.set(entry.bytes);
    return copy;
  };
}

function createMemoryJournal(encrypted = true): {
  journal: StorageUploadJournal;
  rows: Map<string, string>;
} {
  const rows = new Map<string, string>();
  const locks = new Map<string, Promise<void>>();
  const keyOf = (owner: string, ns: string, id: string) => `${owner}\u001f${ns}\u001f${id}`;
  return {
    rows,
    journal: {
      isEncrypted: () => encrypted,
      exclusive: async (owner, ns, id, task) => {
        const key = keyOf(owner, ns, id);
        const prev = locks.get(key) ?? Promise.resolve();
        let release!: () => void;
        const curr = new Promise<void>((r) => {
          release = r;
        });
        locks.set(
          key,
          prev.then(() => curr),
        );
        await prev;
        try {
          return await task();
        } finally {
          release();
        }
      },
      read: async <T>(owner: string, ns: string, id: string): Promise<T | null> => {
        const raw = rows.get(keyOf(owner, ns, id));
        return raw ? (JSON.parse(raw) as T) : null;
      },
      write: async (owner, ns, id, value) => {
        rows.set(keyOf(owner, ns, id), JSON.stringify(value));
      },
      delete: async (owner, ns, id) => {
        rows.delete(keyOf(owner, ns, id));
      },
    },
  };
}

test('StreamingSha256 matches WebCrypto SHA-256 across all block padding boundaries', async () => {
  for (const len of [0, 1, 55, 56, 63, 64, 65, 119, 120, 128, 4099]) {
    const data = new Uint8Array(len);
    for (let i = 0; i < len; i++) data[i] = (i * 131 + 17) & 0xff;
    const stream = new StreamingSha256();
    for (let offset = 0; offset < len;) {
      const step = Math.min(13, len - offset);
      stream.update(data.subarray(offset, offset + step));
      offset += step;
    }
    expect(stream.digestHex()).toBe(await sha256Hex(data));
  }
});

test('cipher blocks authenticate scope, object, revision, part and ciphertext before returning plaintext', async () => {
  const transport = new MemoryTransport();
  const repo = new StorageRepository(scope, transport);
  const gateway = new MemoryBlobGateway();
  const secret = encoder.encode('Secreto empresarial privado E2EE: 900719925474099312345');

  const block = await repo.uploadBlock(gateway, 'doc-1', 'rev-1', 0, secret);
  const storedCipher = gateway.blobs.get(block.blob.id)!.bytes;
  expect(new TextDecoder().decode(storedCipher)).not.toContain('Secreto empresarial');

  expect(await repo.downloadBlock(gateway, block, 'doc-1', 'rev-1', 0)).toEqual(secret);
  await expect(repo.downloadBlock(gateway, block, 'other-doc', 'rev-1', 0)).rejects.toThrow(
    StorageRecordError,
  );
  await expect(repo.downloadBlock(gateway, block, 'doc-1', 'other-rev', 0)).rejects.toThrow(
    StorageRecordError,
  );
  await expect(repo.downloadBlock(gateway, block, 'doc-1', 'rev-1', 1)).rejects.toThrow(
    StorageRecordError,
  );

  storedCipher[0]! ^= 0x01;
  await expect(repo.downloadBlock(gateway, block, 'doc-1', 'rev-1', 0)).rejects.toThrow(
    'matrix_unverified',
  );
});

// This functional fixture must cross the real 8 MiB block boundary. Its repeated
// encryption, resume and full-file hashes need a bounded budget under CI CPU load.
test('multi-block resumable upload, range read across block boundary, source-change guard and kind=file catalog publication', async () => {
  const transport = new MemoryTransport();
  const repo = new StorageRepository(scope, transport);
  const gateway = new MemoryBlobGateway();
  const { journal, rows } = createMemoryJournal(true);

  // Two full 8 MiB blocks + trailing 321 bytes => 3 encrypted blocks.
  const totalSize = maxCipherBlockBytes * 2 + 321;
  const plain = new Uint8Array(totalSize);
  for (let i = 0; i < totalSize; i += 4096) {
    plain[i] = ((i / 4096) * 37 + 11) & 0xff;
  }
  plain.set(encoder.encode('BOUNDARY-MARKER-ACROSS-8MIB'), maxCipherBlockBytes - 12);

  const metaJson = '{"filename":"presupuesto-2027.xlsx","exact_cents":900719925474099312345}';
  const request = {
    object_id: 'sheet-budget',
    revision_id: 'rev-file-1',
    size_bytes: totalSize,
    metadata: encodeStorageJSON(encoder.encode(metaJson), 8192),
  };

  // Unencrypted journals are refused upfront.
  await expect(
    repo.uploadFile(gateway, 'job-budget', request, bytesStorageFileSource(plain), {
      journal: createMemoryJournal(false).journal,
    }),
  ).rejects.toThrow(StorageRecordError);

  // Simulate network failure on the 2nd block upload: pending block 1 stays checkpointed.
  gateway.failOnPutNumber = 2;
  await expect(
    repo.uploadFile(gateway, 'job-budget', request, bytesStorageFileSource(plain), { journal }),
  ).rejects.toThrow('simulated_blob_network_drop');
  expect(rows.size).toBe(1);
  expect([...rows.keys()][0]).toContain(pendingUploadsNamespace);

  // Mutating the source while a pending AES-CTR key/IV is checkpointed MUST fail without reusing keys.
  const mutated = new Uint8Array(plain);
  mutated[maxCipherBlockBytes + 5]! ^= 0xff;
  await expect(
    repo.uploadFile(gateway, 'job-budget', request, bytesStorageFileSource(mutated), { journal }),
  ).rejects.toThrow(StorageSourceChangedError);

  // Resume with the original immutable source; then simulate root manifest network drop.
  gateway.failOnPutNumber = -1;
  transport.failOnPutCount = 1;
  await expect(
    repo.uploadFile(gateway, 'job-budget', request, bytesStorageFileSource(plain), { journal }),
  ).rejects.toThrow('simulated_manifest_network_drop');
  expect(rows.size).toBe(1);

  // Final resume completes verification and removes the pending upload journal entry.
  transport.failOnPutCount = -1;
  const manifestRef = await repo.uploadFile(
    gateway,
    'job-budget',
    request,
    bytesStorageFileSource(plain),
    { journal },
  );
  expect(rows.size).toBe(0);

  const info = await repo.fileInfo(manifestRef, 'sheet-budget', 'rev-file-1');
  expect(info.size_bytes).toBe(totalSize);
  expect(new TextDecoder().decode(storageJSONBytes(info.metadata, 8192))).toBe(metaJson);

  // Range read spanning across the end of block 0 and start of block 1.
  const crossSlice = await repo.readFileRange(
    gateway,
    manifestRef,
    'sheet-budget',
    'rev-file-1',
    maxCipherBlockBytes - 12,
    27,
  );
  expect(new TextDecoder().decode(crossSlice)).toBe('BOUNDARY-MARKER-ACROSS-8MIB');

  // Full streaming download verifies whole-file SHA-256.
  const reassembled = new Uint8Array(totalSize);
  let cursor = 0;
  await repo.downloadFile(gateway, manifestRef, 'sheet-budget', 'rev-file-1', (chunk) => {
    reassembled.set(chunk, cursor);
    cursor += chunk.byteLength;
  });
  expect(cursor).toBe(totalSize);
  expect(await sha256Hex(reassembled)).toBe(await sha256Hex(plain));

  // Publish catalog commit with `kind: 'file'` and verify catalog head resolution!
  const version = await repo.putObject('commit-budget-1', 'sheet-budget', 'rev-obj-1', {
    kind: storageObjectKind,
    state: 'live',
    metadata: encodeStorageJSON(encoder.encode(metaJson), 8192),
    content: {
      kind: 'file',
      revision_id: 'rev-file-1',
      reference: manifestRef,
    },
  });
  expect(version.commit.content?.kind).toBe('file');
  const heads = await repo.catalog();
  expect(heads.map((h) => h.object_id)).toEqual(['sheet-budget']);
}, 20_000);

test('paged manifest tree geometry matches Go fanout-16 multi-level invariants', () => {
  expect(fileBlocks(0)).toBe(1);
  expect(fileRootLevel(fileBlocks(0))).toBe(0);
  expect(fileRootLevel(16)).toBe(0);
  expect(fileRootLevel(17)).toBe(1);
  expect(fileRootLevel(256)).toBe(1);
  expect(fileRootLevel(257)).toBe(2);

  const req17 = {
    object_id: 'video',
    revision_id: 'rev1',
    size_bytes: 16 * maxCipherBlockBytes + 100,
    metadata: encodeStorageJSON(encoder.encode('{}'), 8192),
  };
  expect(() =>
    validateStorageFilePage(
      {
        kind: filePageKind,
        level: 1,
        index: 0,
        nodes: [
          { event: { room_id: room, event_id: '$p0' } },
          { event: { room_id: room, event_id: '$p1' } },
        ],
      },
      scope,
      req17,
      1,
      0,
    ),
  ).not.toThrow();

  // Wrong node count at root level 1 (17 blocks requires 2 child pages, not 1).
  expect(() =>
    validateStorageFilePage(
      {
        kind: filePageKind,
        level: 1,
        index: 0,
        nodes: [{ event: { room_id: room, event_id: '$p0' } }],
      },
      scope,
      req17,
      1,
      0,
    ),
  ).toThrow(StorageRecordError);
});
