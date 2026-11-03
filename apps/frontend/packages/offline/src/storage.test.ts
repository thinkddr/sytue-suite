/* SPDX-License-Identifier: Apache-2.0 */
import { beforeEach, expect, test } from 'bun:test';
import {
  clearOfflineStorage,
  deleteOfflineValue,
  exclusiveOfflineRecord,
  isOfflineStorageEncrypted,
  listOfflineValues,
  mutateOfflineValue,
  readOfflineValue,
  writeOfflineValue,
} from './storage.js';

interface MemoryStore {
  keyPath: string;
  rows: Map<string, Record<string, unknown>>;
  indexes: Map<string, string>;
}

function installMemoryIndexedDB() {
  const databases = new Map<string, { version: number; stores: Map<string, MemoryStore> }>();

  function cloneRow(value: Record<string, unknown>): Record<string, unknown> {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value)) {
      if (v instanceof Uint8Array) {
        const copy = new Uint8Array(v.byteLength);
        copy.set(v);
        out[k] = copy;
      } else if (v && typeof v === 'object' && !('usages' in v) && !Array.isArray(v)) {
        out[k] = cloneRow(v as Record<string, unknown>);
      } else {
        out[k] = v;
      }
    }
    return out;
  }

  function makeRequest<T>(compute: () => T): IDBRequest<T> {
    const req = {
      result: undefined as unknown as T,
      error: null,
      onsuccess: null as ((ev: Event) => unknown) | null,
      onerror: null as ((ev: Event) => unknown) | null,
    };
    queueMicrotask(() => {
      try {
        req.result = compute();
        req.onsuccess?.(new Event('success'));
      } catch (err) {
        (req as { error: unknown }).error = err;
        req.onerror?.(new Event('error'));
      }
    });
    return req as unknown as IDBRequest<T>;
  }

  const factory = {
    open(name: string, version = 1) {
      const req = {
        result: undefined as unknown as IDBDatabase,
        error: null,
        onsuccess: null as ((ev: Event) => unknown) | null,
        onerror: null as ((ev: Event) => unknown) | null,
        onupgradeneeded: null as ((ev: IDBVersionChangeEvent) => unknown) | null,
      };
      queueMicrotask(() => {
        let entry = databases.get(name);
        const upgrading = !entry || entry.version < version;
        if (!entry) {
          entry = { version, stores: new Map() };
          databases.set(name, entry);
        } else if (upgrading) {
          entry.version = version;
        }
        const current = entry;
        const db = {
          onversionchange: null as (() => void) | null,
          objectStoreNames: {
            contains: (storeName: string) => current.stores.has(storeName),
          },
          createObjectStore(storeName: string, options: { keyPath: string }) {
            const store: MemoryStore = {
              keyPath: options.keyPath,
              rows: new Map(),
              indexes: new Map(),
            };
            current.stores.set(storeName, store);
            return {
              createIndex(indexName: string, keyPath: string) {
                store.indexes.set(indexName, keyPath);
              },
            };
          },
          transaction(storeName: string) {
            const store = current.stores.get(storeName);
            if (!store) throw new Error(`missing object store ${storeName}`);
            // Como en IndexedDB real, «complete» llega después de las peticiones y
            // aunque el manejador se asigne tras esperar una de ellas: se dispara en
            // un macrotask al asignarlo, no en un microtask al crear la transacción.
            let alCompletar: ((ev: Event) => unknown) | null = null;
            const tx = {
              error: null,
              get oncomplete() {
                return alCompletar;
              },
              set oncomplete(fn: ((ev: Event) => unknown) | null) {
                alCompletar = fn;
                if (fn) setTimeout(() => fn(new Event('complete')), 0);
              },
              onerror: null as ((ev: Event) => unknown) | null,
              onabort: null as ((ev: Event) => unknown) | null,
              abort() {
                queueMicrotask(() => tx.onabort?.(new Event('abort')));
              },
              objectStore() {
                return {
                  get(key: string) {
                    return makeRequest(() => {
                      const found = store.rows.get(key);
                      return found ? cloneRow(found) : undefined;
                    });
                  },
                  put(value: Record<string, unknown>) {
                    const key = String(value[store.keyPath]);
                    store.rows.set(key, cloneRow(value));
                    return makeRequest(() => key);
                  },
                  delete(key: string) {
                    store.rows.delete(key);
                    return makeRequest(() => undefined);
                  },
                  index(indexName: string) {
                    const field = store.indexes.get(indexName)!;
                    return {
                      getAll(expected: string) {
                        return makeRequest(() =>
                          [...store.rows.values()]
                            .filter((row) => row[field] === expected)
                            .map((row) => cloneRow(row)),
                        );
                      },
                    };
                  },
                };
              },
            };
            return tx as unknown as IDBTransaction;
          },
          close() {},
        } as unknown as IDBDatabase;
        req.result = db;
        if (upgrading) req.onupgradeneeded?.(new Event('upgradeneeded') as IDBVersionChangeEvent);
        req.onsuccess?.(new Event('success'));
      });
      return req as unknown as IDBOpenDBRequest;
    },
    deleteDatabase(name: string) {
      databases.delete(name);
      const req = {
        onsuccess: null as ((ev: Event) => unknown) | null,
        onerror: null as ((ev: Event) => unknown) | null,
        onblocked: null as ((ev: Event) => unknown) | null,
      };
      queueMicrotask(() => req.onsuccess?.(new Event('success')));
      return req as unknown as IDBOpenDBRequest;
    },
  };

  Object.defineProperty(globalThis, 'indexedDB', {
    value: factory,
    configurable: true,
    writable: true,
  });
  return databases;
}

let databases = installMemoryIndexedDB();

beforeEach(async () => {
  databases = installMemoryIndexedDB();
  await clearOfflineStorage();
});

test('seals values with a non-extractable AES-GCM key and leaves no plaintext in IndexedDB', async () => {
  expect(isOfflineStorageEncrypted()).toBe(true);
  await writeOfflineValue('actor-1', 'mail-drafts', 'draft-42', {
    subject: 'Informe confidencial Q4',
    amount: '900719925474099312345',
  });

  const db = databases.get('sytue-offline')!;
  const keyRow = db.stores.get('keys')?.rows.get('master-v1');
  expect(keyRow).toBeDefined();
  expect((keyRow!.key as CryptoKey).extractable).toBe(false);

  const rawRecords = [...(db.stores.get('records')?.rows.values() ?? [])];
  expect(rawRecords).toHaveLength(1);
  expect('value' in rawRecords[0]!).toBe(false);
  const serialized = JSON.stringify(rawRecords, (_k, v) =>
    v instanceof Uint8Array ? Array.from(v) : v,
  );
  expect(serialized).not.toContain('Informe confidencial Q4');
  expect(serialized).not.toContain('900719925474099312345');

  expect(await readOfflineValue('actor-1', 'mail-drafts', 'draft-42')).toEqual({
    subject: 'Informe confidencial Q4',
    amount: '900719925474099312345',
  });
});

test('rejects ciphertext tampering and cross-account AAD substitution', async () => {
  await writeOfflineValue('alice', 'notes', 'secret-note', { body: 'solo para alice' });
  const records = databases.get('sytue-offline')!.stores.get('records')!.rows;
  const originalKey = 'alice\u001fnotes\u001fsecret-note';
  const original = records.get(originalKey)!;

  // Attempt to move Alice's sealed payload into Bob's scope.
  const bobKey = 'bob\u001fnotes\u001fsecret-note';
  records.set(bobKey, {
    ...original,
    key: bobKey,
    scope: 'bob\u001fnotes',
    ownerId: 'bob',
  });
  await expect(readOfflineValue('bob', 'notes', 'secret-note')).rejects.toThrow();

  // Attempt to flip a bit of Alice's ciphertext.
  const sealed = original.sealed as { v: 1; iv: Uint8Array; ciphertext: Uint8Array };
  sealed.ciphertext[0] ^= 0xff;
  await expect(readOfflineValue('alice', 'notes', 'secret-note')).rejects.toThrow();
});

test('migrates legacy v1 unsealed records transparently on read and clears keys on logout', async () => {
  await writeOfflineValue('alice', 'erp-tpv', 'init', { ok: true });
  const records = databases.get('sytue-offline')!.stores.get('records')!.rows;
  const legacyKey = 'alice\u001ferp-tpv\u001fcobro';
  records.set(legacyKey, {
    key: legacyKey,
    scope: 'alice\u001ferp-tpv',
    ownerId: 'alice',
    namespace: 'erp-tpv',
    id: 'cobro',
    value: { totalCents: 1850 },
    updatedAt: 1700000000000,
  });

  expect(await readOfflineValue('alice', 'erp-tpv', 'cobro')).toEqual({ totalCents: 1850 });
  const upgraded = records.get(legacyKey)!;
  expect('value' in upgraded).toBe(false);
  expect('sealed' in upgraded).toBe(true);

  await deleteOfflineValue('alice', 'erp-tpv', 'init');
  const listed = await listOfflineValues<{ totalCents: number }>('alice', 'erp-tpv');
  expect(listed.map((r) => r.value)).toEqual([{ totalCents: 1850 }]);

  await clearOfflineStorage();
  expect(databases.has('sytue-offline')).toBe(false);
});

test('serializes concurrent mutations and exclusive record locks without losing updates', async () => {
  await Promise.all(
    Array.from({ length: 8 }, (_, index) =>
      mutateOfflineValue<number[]>('alice', 'queue', 'items', (prev) => [...(prev ?? []), index]),
    ),
  );
  const items = await readOfflineValue<number[]>('alice', 'queue', 'items');
  expect(items?.slice().sort((a, b) => a - b)).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);

  const order: string[] = [];
  const first = exclusiveOfflineRecord('alice', 'uploads', 'job-1', async () => {
    order.push('first:start');
    await new Promise((resolve) => setTimeout(resolve, 15));
    order.push('first:end');
    return 1;
  });
  const second = exclusiveOfflineRecord('alice', 'uploads', 'job-1', async () => {
    order.push('second:start');
    return 2;
  });
  expect(await Promise.all([first, second])).toEqual([1, 2]);
  expect(order).toEqual(['first:start', 'first:end', 'second:start']);
});
