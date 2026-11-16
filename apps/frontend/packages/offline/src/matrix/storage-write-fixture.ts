/* SPDX-License-Identifier: Apache-2.0 */
/** Artificial device/gateway plumbing shared by file and domain conformance tests. */
import type { EncryptedOfflineValues } from '../encrypted-storage.js';
import { encryptedStorageMemory } from '../encrypted-storage-fixture.js';
import { StorageJobs } from './storage-jobs.js';
import { StorageRepository } from './storage-repository.js';
import {
  sameStorageRecord,
  storageJSONBytes,
  type StorageRecord,
  type StorageRecordEntry,
  type StoragePublicationTarget,
} from './storage.js';
import { digest } from './storage-file-codec.js';
import type { MatrixReference, MatrixStorageScope } from './types.js';
import type { StorageBlobPort } from './storage-files.js';
export function storageWriteFixture(
  scope: MatrixStorageScope = { tenant: 'acme', subject: 'person', product: 'drive' },
  target: StoragePublicationTarget = { room_id: '!private:matrix.test', device_id: 'DEVICE' },
  retainedCache?: EncryptedOfflineValues,
) {
  const memory = encryptedStorageMemory(),
    cache = retainedCache ?? memory.create('leased-device', 'uploads');
  const blobs = new Map<string, Uint8Array<ArrayBuffer>>(),
    records = new Map<string, StorageRecord>(),
    transactions = new Map<string, MatrixReference>();
  const references = new Map<string, MatrixReference>();
  const writes: { id: string; hash: string }[] = [],
    sends: { transaction: string; target: StoragePublicationTarget; payload: unknown }[] = [];
  const jobs = new StorageJobs();
  const hooks: {
    prepare?: () => Promise<void>;
    history?: () => void;
    blobAck?: () => Promise<void>;
    recordAck?: (record: StorageRecord) => Promise<void>;
    read?: (id: string) => Promise<void>;
  } = {};
  let open = true;
  const repository = (
    publication = target,
    rooms = [target.room_id, '!rotated:matrix.test'],
    externalBlobs?: StorageBlobPort,
  ) =>
    new StorageRepository(
      scope,
      {
        rooms,
        publication,
        prepare: async () => {
          await hooks.prepare?.();
        },
        run: (name, action, signal) => jobs.run(name, action, signal),
        exclusive: (action, signal) => jobs.exclusive('catalog', action, signal),
        assertOpen: () => {
          if (!open) throw new Error('closed artificial device');
        },
        put: async (transaction, record, destination) => {
          const selected = destination ?? publication;
          const key = JSON.stringify([selected.device_id, selected.room_id, transaction]);
          sends.push({
            transaction,
            target: structuredClone(selected),
            payload: JSON.parse(
              new TextDecoder().decode(storageJSONBytes(record.payload)),
            ) as unknown,
          });
          let reference = transactions.get(key);
          if (reference) {
            if (!sameStorageRecord(records.get(reference.event_id)!, record))
              throw new Error('transaction reused with changed content');
          } else {
            reference = { room_id: selected.room_id, event_id: '$' + (records.size + 1) };
            records.set(reference.event_id, structuredClone(record));
            transactions.set(key, reference);
            references.set(reference.event_id, reference);
          }
          await hooks.recordAck?.(record);
          return structuredClone(reference);
        },
        read: (ref) => Promise.resolve(structuredClone(records.get(ref.event_id)!)),
        history: (room, cursor, limit) => {
          hooks.history?.();
          const all = [...references.values()].filter((ref) => ref.room_id === room).reverse(),
            offset = Number(cursor ?? 0),
            page = all.slice(offset, offset + limit);
          return Promise.resolve({
            entries: page.map((reference) => ({
              reference,
              record: structuredClone(records.get(reference.event_id)!),
            })),
            scanned: page.length,
            next: offset + page.length < all.length ? String(offset + page.length) : null,
          });
        },
      },
      externalBlobs ?? {
        read: async (id) => {
          await hooks.read?.(id);
          const data = blobs.get(id);
          if (!data) throw new Error('missing artificial block');
          return data.slice();
        },
        write: async (id, data) => {
          const hash = await digest(data);
          writes.push({ id, hash });
          const existing = blobs.get(id);
          if (existing && (await digest(existing)) !== hash)
            throw new Error('immutable provider conflict');
          blobs.set(id, data.slice());
          await hooks.blobAck?.();
          return { id, product: scope.product, size_bytes: data.length, sha256: hash };
        },
      },
      cache,
    );
  const journal = async () => {
    const bytes = await cache.read('j_job');
    try {
      return bytes
        ? (JSON.parse(new TextDecoder().decode(bytes)) as Record<string, unknown>)
        : null;
    } finally {
      bytes?.fill(0);
    }
  };
  return {
    entries: () =>
      [...references.values()].map((reference) => ({
        reference: structuredClone(reference),
        record: structuredClone(records.get(reference.event_id)!),
      })),
    seed: (entry: StorageRecordEntry) => {
      if (records.has(entry.reference.event_id)) throw new Error('duplicate fixture event');
      records.set(entry.reference.event_id, structuredClone(entry.record));
      references.set(entry.reference.event_id, structuredClone(entry.reference));
    },
    memory,
    cache,
    blobs,
    records,
    transactions,
    writes,
    sends,
    jobs,
    hooks,
    repository,
    journal,
    close: () => {
      open = false;
      return jobs.close();
    },
  };
}
