/* SPDX-License-Identifier: Apache-2.0 */
import { beforeEach, expect, test } from 'bun:test';
import { EncryptedOfflineValues } from './encrypted-storage.js';
import {
  clearOfflineStorage,
  deleteOfflineCAS,
  deleteOfflineValue,
  exclusiveOfflineRecord,
  isOfflineStorageEncrypted,
  listOfflineValues,
  mutateOfflineValue,
  readOfflineValue,
  readOfflineCAS,
  writeOfflineCAS,
  writeOfflineValue,
} from './storage.js';

interface MemoryStore {
  keyPath: string;
  rows: Map<string, Record<string, unknown>>;
  indexes: Map<string, string>;
}

let afterMemoryCommit: ((database: string, store: string) => void) | undefined;

async function withinTestDeadline<T>(promise: Promise<T>, label: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      promise,
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${label} timed out`)), 2000);
      }),
    ]);
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
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

  function makeRequest<T>(compute: () => T, onSettled?: () => void): IDBRequest<T> {
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
      onSettled?.();
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
            // A real IndexedDB transaction completes once every request issued
            // inside it has settled — never at creation. storage.ts awaits
            // requests before assigning `oncomplete`, so an eagerly fired event
            // would leave `transactionDone` pending forever.
            let pending = 0;
            let finished = false;
            let complete: ((ev: Event) => unknown) | null = null;
            // Complete in a task after pending requests and their await
            // continuations have settled, including a late-installed handler.
            const arm = () => {
              setTimeout(() => {
                if (!finished && pending === 0 && complete) {
                  finished = true;
                  afterMemoryCommit?.(name, storeName);
                  complete(new Event('complete'));
                }
              }, 0);
            };
            const settle = () => {
              pending -= 1;
              if (pending <= 0) arm();
            };
            const tx = {
              error: null,
              get oncomplete() {
                return complete;
              },
              set oncomplete(handler: ((ev: Event) => unknown) | null) {
                complete = handler;
                arm();
              },
              onerror: null as ((ev: Event) => unknown) | null,
              onabort: null as ((ev: Event) => unknown) | null,
              abort() {
                queueMicrotask(() => tx.onabort?.(new Event('abort')));
              },
              objectStore() {
                return {
                  get(key: string) {
                    pending += 1;
                    return makeRequest(() => {
                      const found = store.rows.get(key);
                      return found ? cloneRow(found) : undefined;
                    }, settle);
                  },
                  put(value: Record<string, unknown>) {
                    pending += 1;
                    const key = String(value[store.keyPath]);
                    store.rows.set(key, cloneRow(value));
                    return makeRequest(() => key, settle);
                  },
                  delete(key: string) {
                    pending += 1;
                    store.rows.delete(key);
                    return makeRequest(() => undefined, settle);
                  },
                  index(indexName: string) {
                    const field = store.indexes.get(indexName)!;
                    return {
                      getAllKeys(expected: string, count = Infinity) {
                        pending += 1;
                        return makeRequest(
                          () =>
                            [...store.rows.values()]
                              .filter((row) => row[field] === expected)
                              .map((row) => row[store.keyPath])
                              .slice(0, count),
                          settle,
                        );
                      },
                      openKeyCursor(expected: string) {
                        pending += 1;
                        const keys = [...store.rows.values()]
                          .filter((row) => row[field] === expected)
                          .map((row) => row[store.keyPath]);
                        let index = 0;
                        const request = {
                          result: null as null | { primaryKey: unknown; continue: () => void },
                          onsuccess: null as null | ((event: Event) => unknown),
                        };
                        const next = () =>
                          queueMicrotask(() => {
                            request.result =
                              index < keys.length
                                ? { primaryKey: keys[index++], continue: next }
                                : null;
                            request.onsuccess?.(new Event('success'));
                            if (!request.result) settle();
                          });
                        next();
                        return request;
                      },
                      getAll(expected: string) {
                        pending += 1;
                        return makeRequest(
                          () =>
                            [...store.rows.values()]
                              .filter((row) => row[field] === expected)
                              .map((row) => cloneRow(row)),
                          settle,
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
  afterMemoryCommit = undefined;
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

  expect(
    await readOfflineValue<{ subject: string; amount: string }>(
      'actor-1',
      'mail-drafts',
      'draft-42',
    ),
  ).toEqual({
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
  sealed.ciphertext[0] = sealed.ciphertext[0]! ^ 0xff;
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

  expect(await readOfflineValue<{ totalCents: number }>('alice', 'erp-tpv', 'cobro')).toEqual({
    totalCents: 1850,
  });
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

test('the production encrypted-value adapter retains non-extractable keys and ciphertext bytes across reopen', async () => {
  const original = new TextEncoder().encode('private real adapter recovery data');
  const first = new EncryptedOfflineValues('device-account', 'owner-cache');
  await first.write('owner', original);
  first.close();
  const second = new EncryptedOfflineValues('device-account', 'owner-cache');
  expect(await second.read('owner')).toEqual(original);
  const rows = [...databases.get('sytue-offline')!.stores.get('records')!.rows.values()];
  const root = rows.find((row) => row.namespace === 'local-crypto-key-v1')!.value as {
    key: CryptoKey;
  };
  expect(root.key).toBeInstanceOf(CryptoKey);
  expect(root.key.extractable).toBe(false);
  await expect(crypto.subtle.exportKey('raw', root.key)).rejects.toThrow();
  const sealed = rows.find((row) => row.namespace === 'local-crypto-value-v1')!.value as {
    iv: Uint8Array;
    ciphertext: Uint8Array;
  };
  expect(sealed.iv).toBeInstanceOf(Uint8Array);
  expect(sealed.ciphertext).toBeInstanceOf(Uint8Array);
  expect(JSON.stringify(rows)).not.toContain('private real adapter recovery data');
  await second.destroy();
  expect(databases.get('sytue-offline')!.stores.get('records')!.rows.size).toBe(0);
});

test('protected local namespaces reject plaintext and extractable keys', async () => {
  await expect(
    writeOfflineValue('device-account', 'local-crypto-value-v1', 'data', { body: 'private data' }),
  ).rejects.toThrow('offline_protected_value');
  const key = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, true, [
    'encrypt',
    'decrypt',
  ]);
  await expect(
    writeOfflineValue('device-account', 'local-crypto-key-v1', 'root', { v: 1, key }),
  ).rejects.toThrow('offline_protected_key');
});

test('CAS preserves newer ciphertext and tombstones across stale deletion and ABA', async () => {
  expect(await readOfflineCAS('alice', 'capture-v2', 'one')).toBeNull();
  expect(await writeOfflineCAS('alice', 'capture-v2', 'one', null, { body: 'first secret' })).toBe(
    true,
  );
  const first = await readOfflineCAS<{ body: string }>('alice', 'capture-v2', 'one');
  if (!first?.present) throw new Error('missing record');
  expect(first.value.body).toBe('first secret');
  expect(
    await writeOfflineCAS('alice', 'capture-v2', 'one', first.witness, { body: 'second secret' }),
  ).toBe(true);
  expect(await deleteOfflineCAS('alice', 'capture-v2', 'one', first.witness)).toBe(false);
  const second = await readOfflineCAS<{ body: string }>('alice', 'capture-v2', 'one');
  if (!second?.present) throw new Error('missing record');
  expect(second.value.body).toBe('second secret');
  expect(await deleteOfflineCAS('alice', 'capture-v2', 'one', second.witness)).toBe(true);
  const tombstone = await readOfflineCAS('alice', 'capture-v2', 'one');
  if (!tombstone || tombstone.present) throw new Error('missing tombstone');
  expect(await writeOfflineCAS('alice', 'capture-v2', 'one', null, { body: 'stale create' })).toBe(
    false,
  );
  expect(
    await writeOfflineCAS('alice', 'capture-v2', 'one', tombstone.witness, { body: 'recreated' }),
  ).toBe(true);
  expect(await deleteOfflineCAS('alice', 'capture-v2', 'one', second.witness)).toBe(false);
  const current = await readOfflineCAS<{ body: string }>('alice', 'capture-v2', 'one');
  if (!current?.present) throw new Error('missing recreated record');
  expect(current.value.body).toBe('recreated');
  expect(
    new Set([
      first.witness.revision,
      second.witness.revision,
      tombstone.witness.revision,
      current.witness.revision,
    ]).size,
  ).toBe(4);
  const raw = [...databases.get('sytue-offline-cas-v2')!.stores.get('records')!.rows.values()];
  expect(JSON.stringify(raw)).not.toContain('secret');
  expect(JSON.stringify(raw)).not.toContain('recreated');
});

test('CAS rejects altered ciphertext and a witness from another scope', async () => {
  expect(await writeOfflineCAS('alice', 'capture-v2', 'one', null, 'private')).toBe(true);
  const record = await readOfflineCAS<string>('alice', 'capture-v2', 'one');
  if (!record?.present) throw new Error('missing record');
  const altered = { ...record.witness, ciphertext: new Uint8Array(record.witness.ciphertext) };
  altered.ciphertext[0] = altered.ciphertext[0]! ^ 1;
  expect(await deleteOfflineCAS('alice', 'capture-v2', 'one', altered)).toBe(false);
  await expect(deleteOfflineCAS('bob', 'capture-v2', 'one', record.witness)).rejects.toThrow();
  expect((await readOfflineCAS<string>('alice', 'capture-v2', 'one'))?.present).toBe(true);
  await clearOfflineStorage();
  expect(databases.has('sytue-offline-cas-v2')).toBe(false);
  expect(databases.has('sytue-offline')).toBe(false);
});

test('CAS rejects ill-formed scope strings and authenticates distinct Unicode scopes', async () => {
  // TextEncoder maps each isolated surrogate to the same replacement bytes.
  expect(new TextEncoder().encode('\ud800')).toEqual(new TextEncoder().encode('\ud801'));
  for (const bad of ['\ud800', '\ud801', '\udc00', 'x\ud800x']) {
    await expect(writeOfflineCAS(bad, 'scope', 'id', null, 'private')).rejects.toThrow(
      'offline_cas_key',
    );
    await expect(writeOfflineCAS('owner', bad, 'id', null, 'private')).rejects.toThrow(
      'offline_cas_key',
    );
    await expect(writeOfflineCAS('owner', 'scope', bad, null, 'private')).rejects.toThrow(
      'offline_cas_key',
    );
  }
  const composed = 'café';
  const decomposed = 'cafe\u0301';
  expect(await writeOfflineCAS(composed, 'scope', 'id', null, 'private')).toBe(true);
  const rows = databases.get('sytue-offline-cas-v2')!.stores.get('records')!.rows;
  const original = rows.get(`${composed}\u001fscope\u001fid`)!;
  rows.set(`${decomposed}\u001fscope\u001fid`, {
    ...original,
    key: `${decomposed}\u001fscope\u001fid`,
    scope: `${decomposed}\u001fscope`,
    ownerId: decomposed,
  });
  await expect(readOfflineCAS(decomposed, 'scope', 'id')).rejects.toThrow();
  expect((await readOfflineCAS(composed, 'scope', 'id'))?.present).toBe(true);
  expect(await writeOfflineCAS('🗝️', 'scope', 'id', null, 'valid pair')).toBe(true);
});

test('logout invalidates an in-flight CAS write before it can create a new store', async () => {
  const result = writeOfflineCAS('alice', 'capture-v2', 'one', null, 'private').then(
    () => 'written',
    () => 'cancelled',
  );
  await clearOfflineStorage();
  expect(await result).toBe('cancelled');
  expect(databases.has('sytue-offline-cas-v2')).toBe(false);
});

test('CAS snapshots mutable input before the first asynchronous operation', async () => {
  const value = { body: 'first' };
  const pending = writeOfflineCAS('alice', 'capture-v2', 'one', null, value);
  value.body = 'changed while opening';
  expect(await pending).toBe(true);
  const stored = await readOfflineCAS<typeof value>('alice', 'capture-v2', 'one');
  if (!stored?.present) throw new Error('missing record');
  expect(stored.value.body).toBe('first');
});

test('a failed delete keeps clearing active until the other database delete settles', async () => {
  const original = indexedDB.deleteDatabase;
  const pending: { finishSecond?: () => void } = {};
  indexedDB.deleteDatabase = ((name: string) => {
    const request = {
      error: name === 'sytue-offline' ? new Error('delete failed') : null,
      onsuccess: null as ((event: Event) => void) | null,
      onerror: null as ((event: Event) => void) | null,
      onblocked: null as ((event: Event) => void) | null,
    };
    if (name === 'sytue-offline') queueMicrotask(() => request.onerror?.(new Event('error')));
    else pending.finishSecond = () => request.onsuccess?.(new Event('success'));
    return request as unknown as IDBOpenDBRequest;
  }) as typeof indexedDB.deleteDatabase;
  try {
    const done = clearOfflineStorage().then(
      () => 'success',
      () => 'failed',
    );
    await Promise.resolve();
    await expect(readOfflineCAS('alice', 'capture-v2', 'one')).rejects.toThrow('clearing');
    if (!pending.finishSecond) throw new Error('second deletion did not start');
    pending.finishSecond();
    expect(await done).toBe('failed');
  } finally {
    indexedDB.deleteDatabase = original;
  }
});

for (const failedName of ['sytue-offline', 'sytue-offline-cas-v2']) {
  test(`a failed ${failedName} deletion blocks both stores until a successful retry`, async () => {
    await writeOfflineValue('alice', 'draft', 'old', 'legacy source');
    expect(await writeOfflineCAS('alice', 'capture-v2', 'old', null, 'cas source')).toBe(true);
    const original = indexedDB.deleteDatabase;
    indexedDB.deleteDatabase = ((name: string) => {
      if (name !== failedName) return original.call(indexedDB, name);
      const request = {
        error: new Error('synthetic deletion failure'),
        onsuccess: null as ((event: Event) => void) | null,
        onerror: null as ((event: Event) => void) | null,
        onblocked: null as ((event: Event) => void) | null,
      };
      queueMicrotask(() => request.onerror?.(new Event('error')));
      return request as unknown as IDBOpenDBRequest;
    }) as typeof indexedDB.deleteDatabase;
    try {
      await expect(withinTestDeadline(clearOfflineStorage(), 'failed cleanup')).rejects.toThrow(
        'synthetic deletion failure',
      );
      expect(databases.has(failedName)).toBe(true);
      expect(
        databases.has(failedName === 'sytue-offline' ? 'sytue-offline-cas-v2' : 'sytue-offline'),
      ).toBe(false);
      await expect(
        withinTestDeadline(readOfflineValue('alice', 'draft', 'old'), 'blocked read'),
      ).rejects.toThrow('cleanup failed');
      await expect(
        withinTestDeadline(
          writeOfflineValue('alice', 'draft', 'new', 'do not persist'),
          'blocked write',
        ),
      ).rejects.toThrow('cleanup failed');
      await expect(
        withinTestDeadline(readOfflineCAS('alice', 'capture-v2', 'old'), 'blocked CAS read'),
      ).rejects.toThrow('cleanup failed');
      await expect(
        withinTestDeadline(
          writeOfflineCAS('alice', 'capture-v2', 'new', null, 'do not persist'),
          'blocked CAS write',
        ),
      ).rejects.toThrow('cleanup failed');
    } finally {
      indexedDB.deleteDatabase = original;
      // Retry is explicit; both deletions must complete before the gate opens.
      await withinTestDeadline(clearOfflineStorage(), 'cleanup retry');
    }
    expect(databases.has('sytue-offline')).toBe(false);
    expect(databases.has('sytue-offline-cas-v2')).toBe(false);
    expect(await readOfflineValue('alice', 'draft', 'old')).toBeNull();
    expect(await readOfflineCAS('alice', 'capture-v2', 'old')).toBeNull();
    await writeOfflineValue('alice', 'draft', 'fresh', 'new session');
    expect(await readOfflineValue('alice', 'draft', 'fresh')).toBe('new session');
    expect(await writeOfflineCAS('alice', 'capture-v2', 'fresh', null, 'new session')).toBe(true);
    expect((await readOfflineCAS('alice', 'capture-v2', 'fresh'))?.present).toBe(true);
  }, 10000);
}

for (const operation of ['write', 'read'] as const) {
  test(`a legacy ${operation} started before logout cannot resume after failed clear and retry`, async () => {
    await writeOfflineValue('alice', 'draft', 'old', 'private source');
    const method = operation === 'write' ? 'encrypt' : 'decrypt';
    const previousDescriptor = Object.getOwnPropertyDescriptor(crypto.subtle, method);
    const originalCrypto = crypto.subtle[method].bind(crypto.subtle);
    let entered!: () => void;
    let release!: () => void;
    const started = new Promise<void>((resolve) => {
      entered = resolve;
    });
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    Object.defineProperty(crypto.subtle, method, {
      configurable: true,
      value: async (...args: Parameters<SubtleCrypto['encrypt']>) => {
        entered();
        await gate;
        return originalCrypto(...args);
      },
    });
    const originalDelete = indexedDB.deleteDatabase;
    const pending =
      operation === 'write'
        ? writeOfflineValue('alice', 'draft', 'late', 'stale private content')
        : readOfflineValue('alice', 'draft', 'old');
    const outcome = pending.then(
      () => null,
      (error: unknown) => error,
    );
    try {
      await withinTestDeadline(started, 'crypto barrier');
      indexedDB.deleteDatabase = ((name: string) => {
        if (name !== 'sytue-offline') return originalDelete.call(indexedDB, name);
        const request = {
          error: new Error('synthetic deletion failure'),
          onerror: null as ((event: Event) => void) | null,
        };
        queueMicrotask(() => request.onerror?.(new Event('error')));
        return request as unknown as IDBOpenDBRequest;
      }) as typeof indexedDB.deleteDatabase;
      await expect(withinTestDeadline(clearOfflineStorage(), 'failed cleanup')).rejects.toThrow(
        'synthetic deletion failure',
      );
      indexedDB.deleteDatabase = originalDelete;
      await withinTestDeadline(clearOfflineStorage(), 'cleanup retry');
      release();
      const result = await withinTestDeadline(outcome, 'old operation');
      expect(result).toBeInstanceOf(Error);
      expect((result as Error).message).toContain('indexeddb cleared');
      expect(
        await withinTestDeadline(readOfflineValue('alice', 'draft', 'late'), 'read late'),
      ).toBeNull();
      expect(
        await withinTestDeadline(readOfflineValue('alice', 'draft', 'old'), 'read old'),
      ).toBeNull();
    } finally {
      release();
      indexedDB.deleteDatabase = originalDelete;
      if (previousDescriptor) Object.defineProperty(crypto.subtle, method, previousDescriptor);
      else delete (crypto.subtle as unknown as Record<string, unknown>)[method];
      try {
        await withinTestDeadline(outcome, 'drain old operation');
      } finally {
        await withinTestDeadline(clearOfflineStorage(), 'final cleanup');
      }
    }
  }, 10000);
}

test('a cancelled delete preserves its original row', async () => {
  await writeOfflineValue('alice', 'draft', 'one', 'private source');
  const controller = new AbortController();
  controller.abort();
  await expect(
    deleteOfflineValue('alice', 'draft', 'one', () => controller.signal.throwIfAborted()),
  ).rejects.toMatchObject({ name: 'AbortError' });
  expect(await readOfflineValue('alice', 'draft', 'one')).toBe('private source');
});

test('an epoch change after a simulated commit reports an uncertain outcome', async () => {
  await writeOfflineValue('alice', 'draft', 'seed', 'source');
  const state: { committed: boolean; cleanup?: Promise<void> } = { committed: false };
  afterMemoryCommit = (database, store) => {
    if (database !== 'sytue-offline' || store !== 'records') return;
    afterMemoryCommit = undefined;
    state.committed =
      databases.get(database)?.stores.get(store)?.rows.has('alice\u001fdraft\u001flate') ?? false;
    // The test double has reached transaction completion before the epoch moves.
    state.cleanup = clearOfflineStorage();
  };
  try {
    await expect(
      withinTestDeadline(writeOfflineValue('alice', 'draft', 'late', 'private'), 'write outcome'),
    ).rejects.toThrow('indexeddb cleared');
    expect(state.committed).toBe(true);
    if (!state.cleanup) throw new Error('cleanup did not start');
    await withinTestDeadline(state.cleanup, 'post-commit cleanup');
  } finally {
    afterMemoryCommit = undefined;
    if (state.cleanup) await withinTestDeadline(state.cleanup, 'drain cleanup');
  }
}, 10000);

test('a root-key request waiting for a lock cannot recreate a key after logout', async () => {
  const navigatorDescriptor = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
  const originalDelete = indexedDB.deleteDatabase;
  let entered!: () => void;
  let release!: () => void;
  const started = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  Object.defineProperty(globalThis, 'navigator', {
    configurable: true,
    value: {
      locks: {
        request: async (_name: string, _options: unknown, task: () => Promise<unknown>) => {
          entered();
          await gate;
          return task();
        },
      },
    },
  });
  const outcome = writeOfflineValue('alice', 'draft', 'late', 'private').then(
    () => null,
    (error: unknown) => error,
  );
  try {
    await withinTestDeadline(started, 'root lock barrier');
    indexedDB.deleteDatabase = ((name: string) => {
      if (name !== 'sytue-offline') return originalDelete.call(indexedDB, name);
      const request = {
        error: new Error('synthetic deletion failure'),
        onerror: null as ((event: Event) => void) | null,
      };
      queueMicrotask(() => request.onerror?.(new Event('error')));
      return request as unknown as IDBOpenDBRequest;
    }) as typeof indexedDB.deleteDatabase;
    await expect(withinTestDeadline(clearOfflineStorage(), 'failed cleanup')).rejects.toThrow(
      'synthetic deletion failure',
    );
    indexedDB.deleteDatabase = originalDelete;
    await withinTestDeadline(clearOfflineStorage(), 'cleanup retry');
    release();
    const error = await withinTestDeadline(outcome, 'old root operation');
    expect(error).toBeInstanceOf(Error);
    expect((error as Error).message).toContain('indexeddb cleared');
    expect(databases.has('sytue-offline')).toBe(false);
    expect(databases.has('sytue-offline-cas-v2')).toBe(false);
  } finally {
    release();
    indexedDB.deleteDatabase = originalDelete;
    if (navigatorDescriptor) Object.defineProperty(globalThis, 'navigator', navigatorDescriptor);
    else delete (globalThis as unknown as Record<string, unknown>).navigator;
    try {
      await withinTestDeadline(outcome, 'drain old root operation');
    } finally {
      await withinTestDeadline(clearOfflineStorage(), 'final cleanup');
    }
  }
}, 10000);
