/* SPDX-License-Identifier: Apache-2.0 */
/** Synthetic structured-clone port; yields before transactions like the real IDB adapter. */
import { EncryptedOfflineValues, type EncryptedOfflinePort } from './encrypted-storage.js';
import type { OfflineRecord } from './storage.js';
export function encryptedStorageMemory() {
  const records = new Map<string, OfflineRecord>();
  let generation = 0;
  const location = (owner: string, namespace: string, id: string) =>
    JSON.stringify([owner, namespace, id]);
  const port: EncryptedOfflinePort = {
    read: async <T>(owner: string, namespace: string, id: string): Promise<T | null> => {
      await Promise.resolve();
      const value = records.get(location(owner, namespace, id));
      return value ? (structuredClone(value.value) as T) : null;
    },
    mutate: async <T>(
      ownerId: string,
      namespace: string,
      id: string,
      update: (previous: T | null) => T | null,
    ) => {
      await Promise.resolve();
      const key = location(ownerId, namespace, id);
      const previous = records.get(key);
      const value = update(previous ? (structuredClone(previous.value) as T) : null);
      if (value === null) records.delete(key);
      else
        records.set(key, {
          key,
          ownerId,
          namespace,
          id,
          scope: JSON.stringify([ownerId, namespace]),
          value: structuredClone(value),
          updatedAt: 0,
        });
    },
    ids: async (ownerId, namespace, maximum, prefix = '') => {
      await Promise.resolve();
      const ids = [...records.values()]
        .filter(
          (row) =>
            row.ownerId === ownerId && row.namespace === namespace && row.id.startsWith(prefix),
        )
        .map((row) => row.id);
      if (!Number.isSafeInteger(maximum) || maximum < 1 || maximum > 10000 || ids.length > maximum)
        throw new Error('offline_records_limit');
      return ids;
    },
    pageIDs: async (ownerId, namespace, maximumArg, prefixArg, afterArg) => {
      // The real port defaults omitted arguments; the fixture mirrors it.
      const maximum = maximumArg ?? 128,
        prefix = prefixArg ?? '',
        after = afterArg ?? '';
      await Promise.resolve();
      if (!Number.isSafeInteger(maximum) || maximum < 1 || maximum > 10000)
        throw new Error('offline_records_limit');
      const ids = [...records.values()]
        .filter(
          (row) =>
            row.ownerId === ownerId &&
            row.namespace === namespace &&
            row.id.startsWith(prefix) &&
            (!after || row.id > after),
        )
        .map((row) => row.id)
        .sort();
      return { ids: ids.slice(0, maximum), next: ids.length > maximum ? ids[maximum - 1]! : '' };
    },
    deleteScope: async (ownerId, namespace) => {
      await Promise.resolve();
      for (const [key, row] of records)
        if (row.ownerId === ownerId && row.namespace === namespace) records.delete(key);
    },
    delete: async (owner, namespace, id, beforeDelete) => {
      await Promise.resolve();
      beforeDelete?.();
      records.delete(location(owner, namespace, id));
    },
    epoch: () => generation,
  };
  return {
    port,
    records,
    invalidate: () => {
      generation++;
    },
    create: (owner = 'person', namespace = 'records', active = () => true) =>
      new EncryptedOfflineValues(owner, namespace, active, port),
  };
}
