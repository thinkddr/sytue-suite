/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { StorageFiles } from './storage-files.js';
import { decodeStorageRecord, storageReference, storageScope } from './storage.js';
const directory = process.env.SYTUE_STORAGE_FILE_VECTOR_DIR;

test.skipIf(!directory)(
  'independent Go file producer is decrypted and verified by the maintained Rust SDK in web',
  async () => {
    const input: unknown = JSON.parse(await readFile(path.join(directory!, 'vector.json'), 'utf8'));
    if (!input || typeof input !== 'object') throw new Error('artificial vector required');
    const vector = input as Record<string, unknown>;
    if (vector.artificial !== true || !Array.isArray(vector.entries))
      throw new Error('artificial vector required');
    const scope = storageScope(vector.scope),
      reference = storageReference(vector.reference);
    const records = new Map(
      vector.entries.map((entry: unknown) => {
        if (!entry || typeof entry !== 'object') throw new Error('vector entry');
        const row = entry as Record<string, unknown>,
          ref = storageReference(row.reference);
        return [JSON.stringify(ref), decodeStorageRecord(row.record, scope)] as const;
      }),
    );
    const files = new StorageFiles(
      scope,
      {
        assertOpen: () => {},
        readRecord: async (ref) => {
          const record = records.get(JSON.stringify(ref));
          if (!record) throw new Error('missing fixture record');
          return record;
        },
      },
      { read: async (id) => new Uint8Array(await readFile(path.join(directory!, id))) },
    );
    let bytes = 0,
      committed = false,
      aborted = false;
    const info = await files.download(reference, 'object', 'revision', {
      write: async (data) => {
        bytes += data.length;
      },
      commit: async () => {
        committed = true;
      },
      abort: async () => {
        aborted = true;
      },
    });
    expect(info).toEqual(vector.request);
    expect(bytes).toBe(16 * (8 << 20) + 123);
    expect(committed).toBe(true);
    expect(aborted).toBe(false);
    const selected = await files.readRange(reference, 'object', 'revision', (8 << 20) - 7, 35);
    const pattern = new TextEncoder().encode('ARTIFICIAL native file <bytes> ñ 漢字');
    expect(selected).toEqual(
      Uint8Array.from(
        { length: 35 },
        (_, index) => pattern[((8 << 20) - 7 + index) % pattern.length]!,
      ),
    );
  },
  60_000,
);
