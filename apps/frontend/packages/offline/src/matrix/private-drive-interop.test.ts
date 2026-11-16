/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { StorageRepository } from './storage-repository.js';
import { PrivateDriveRepository } from './private-drive.js';
import {
  storageScope,
  storageReference,
  decodeStorageRecord,
  type StorageRecordEntry,
} from './storage.js';
const directory = process.env.SYTUE_DRIVE_PRIVATE_BROWSER_VECTOR;
test.skipIf(!directory)(
  'native PostgreSQL/Synapse Drive data is read by web with maintained Rust file decryption',
  async () => {
    const vector = JSON.parse(await readFile(path.join(directory!, 'vector.json'), 'utf8')) as {
      artificial: boolean;
      scope: unknown;
      state: unknown;
      rooms: string[];
      entries: { reference: unknown; record: unknown }[];
      objects: {
        kind: 'file' | 'folder';
        id: string;
        raw: string;
        name: string;
        block_external: boolean;
        binary?: string;
      }[];
    };
    expect(vector.artificial).toBe(true);
    const scope = storageScope(vector.scope);
    const entries = vector.entries.map((e) => ({
      reference: storageReference(e.reference),
      record: decodeStorageRecord(e.record, scope),
    }));
    const records = new Map(entries.map((e) => [JSON.stringify(e.reference), e.record]));
    const indexes = new Map(
      vector.rooms.map((room) => [
        room,
        entries.filter((e) => e.reference.room_id === room && e.record.role === 'index'),
      ]),
    );
    const repository = new StorageRepository(
      scope,
      {
        rooms: vector.rooms,
        assertOpen: () => {},
        put: async () => {
          throw new Error('fixture must stay read-only');
        },
        read: async (ref) => {
          const record = records.get(JSON.stringify(ref));
          if (!record) throw new Error('missing native record');
          return record;
        },
        history: async (room, cursor, limit) => {
          const all = indexes.get(room)!,
            start = Number(cursor ?? 0),
            page: StorageRecordEntry[] = all.slice(start, start + limit);
          return {
            entries: page,
            scanned: page.length,
            next: start + page.length < all.length ? String(start + page.length) : null,
          };
        },
      },
      { read: async (id) => new Uint8Array(await readFile(path.join(directory!, id))) },
    );
    const drive = new PrivateDriveRepository(repository);
    await drive.initialize(vector.state);
    for (const object of vector.objects) {
      const raw = await drive.readRaw(object.kind, object.id);
      expect(Buffer.from(raw).toString('base64')).toBe(object.raw);
      const view =
        object.kind === 'file' ? await drive.file(object.id) : await drive.folder(object.id);
      expect(view.name).toBe(object.name);
      if (object.kind === 'file') {
        expect(
          Boolean(
            (view.label as { block_external_share?: boolean } | undefined)?.block_external_share,
          ),
        ).toBe(object.block_external);
        const chunks: Uint8Array[] = [];
        let committed = false,
          aborted = false;
        await drive.download(object.id, {
          write: async (chunk) => {
            chunks.push(chunk.slice());
          },
          commit: async () => {
            committed = true;
          },
          abort: async () => {
            aborted = true;
          },
        });
        expect(Buffer.concat(chunks).toString('base64')).toBe(object.binary ?? '');
        expect(committed).toBe(true);
        expect(aborted).toBe(false);
      }
    }
    await drive.close();
    expect(() => drive.root()).toThrow('closed');
  },
  120_000,
);
