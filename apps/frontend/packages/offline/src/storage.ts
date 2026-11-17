/*
 * Almacén local compartido de la suite.
 *
 * Cada origen mantiene su propia base IndexedDB y, dentro de ella, cada registro
 * queda acotado por ownerId + namespace y sellado en reposo con AES-256-GCM bajo
 * una CryptoKey de WebCrypto no exportable (`extractable: false`) custodiada por
 * clonación estructurada en IndexedDB. El AAD vincula cuenta, espacio e id para
 * rechazar traslados entre cuentas o espacios, y `clearOfflineStorage()` destruye
 * tanto los criptogramas como la clave del dispositivo al cerrar sesión.
 */

import { OFFLINE_CAS_DATABASE, OFFLINE_DATABASE, OFFLINE_DATABASES } from './database-names.js';

const DATABASE = OFFLINE_DATABASE;
// Old clients never open this database, so their blind puts cannot bypass CAS.
const CAS_DATABASE = OFFLINE_CAS_DATABASE;
const VERSION = 2;
const RECORDS = 'records';
const KEYS = 'keys';
const MASTER_KEY_ID = 'master-v1';
const SCOPE_INDEX = 'scope';
const SEAL_AAD_PREFIX = 'sytue-offline-v1';

export interface OfflineRecord<T = unknown> {
  key: string;
  scope: string;
  ownerId: string;
  namespace: string;
  id: string;
  value: T;
  updatedAt: number;
}

interface SealedOfflinePayload {
  v: 1;
  iv: Uint8Array<ArrayBuffer>;
  ciphertext: Uint8Array<ArrayBuffer>;
}

interface StoredOfflineRow<T = unknown> {
  key: string;
  scope: string;
  ownerId: string;
  namespace: string;
  id: string;
  sealed?: SealedOfflinePayload;
  /** Formato heredado v1; se acepta al leer para migrar sin perder colas previas. */
  value?: T;
  updatedAt: number;
}

interface StoredKeyRow {
  id: string;
  key: CryptoKey;
  createdAt: number;
}

const encoder = new TextEncoder();
const decoder = new TextDecoder('utf-8', { fatal: true });
const inProcessLocks = new Map<string, Promise<void>>();

let database: Promise<IDBDatabase> | null = null;
let casDatabase: Promise<IDBDatabase> | null = null;
let epoch = 0;
let clearing: Promise<void> | null = null;
// A failed deletion may leave either old data or an old root key. A new
// explicit clear is required before this runtime can open either database.
let clearFailed = false;

/**
 * Invalidates asynchronous encrypted writes, including clients created before
 * logout. An epoch error can be observed after an IndexedDB transaction has
 * committed: callers must reconcile the outcome, not assume rollback or replay.
 * This fence only describes this runtime and observed versionchange events.
 */
export function offlineStorageEpoch(): number {
  return epoch;
}

// This is a local runtime fence, not proof of another tab's session or logout.
function checkOfflineEpoch(expected: number): void {
  if (expected !== epoch || clearing) throw new Error('indexeddb cleared');
  if (clearFailed) throw new Error('indexeddb cleanup failed');
}

let masterKey: Promise<CryptoKey> | null = null;

/** Indica si el entorno dispone de WebCrypto + IndexedDB para sellar en reposo. */
export function isOfflineStorageEncrypted(): boolean {
  return typeof globalThis.crypto?.subtle !== 'undefined' && typeof indexedDB !== 'undefined';
}

function abortReason(signal: AbortSignal): Error {
  return signal.reason instanceof Error ? signal.reason : new DOMException('Aborted', 'AbortError');
}

async function runInProcessLock<R>(
  name: string,
  task: () => Promise<R>,
  signal?: AbortSignal,
): Promise<R> {
  signal?.throwIfAborted();
  const previous = inProcessLocks.get(name) ?? Promise.resolve();
  let release!: () => void;
  const current = new Promise<void>((resolve) => {
    release = resolve;
  });
  const tail = previous.then(() => current);
  inProcessLocks.set(name, tail);

  try {
    if (signal) {
      await Promise.race([
        previous,
        new Promise<never>((_, reject) => {
          if (signal.aborted) {
            reject(abortReason(signal));
            return;
          }
          signal.addEventListener('abort', () => reject(abortReason(signal)), { once: true });
        }),
      ]);
    } else {
      await previous;
    }
    signal?.throwIfAborted();
    return await task();
  } finally {
    release();
    if (inProcessLocks.get(name) === tail) {
      inProcessLocks.delete(name);
    }
  }
}

async function exclusiveLock<R>(
  name: string,
  task: () => Promise<R>,
  signal?: AbortSignal,
): Promise<R> {
  signal?.throwIfAborted();
  const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined;
  if (locks?.request) {
    return locks.request(name, signal ? { signal } : {}, async () => {
      signal?.throwIfAborted();
      return runInProcessLock(name, task, signal);
    });
  }
  return runInProcessLock(name, task, signal);
}

/**
 * Ejecuta `task` en exclusión mutua para un registro concreto (entre pestañas con
 * Web Locks e intraproceso en cualquier entorno). Necesario para diarios de subida
 * reanudables y mutaciones atómicas sobre valores cifrados.
 */
export async function exclusiveOfflineRecord<R>(
  ownerId: string,
  namespace: string,
  id: string,
  task: () => Promise<R>,
  signal?: AbortSignal,
): Promise<R> {
  return exclusiveLock(`sytue-offline:record:${recordKey(ownerId, namespace, id)}`, task, signal);
}

/**
 * Cierra la conexión de esta ventana y elimina toda la base del origen (registros
 * cifrados y CryptoKey no exportable). El service worker lo solicita a TODAS las
 * ventanas antes de borrar al cerrar sesión; sin este cierre, deleteDatabase puede
 * quedarse bloqueado y dejar los datos de la cuenta anterior en el dispositivo.
 * Si falla una eliminación, el resultado puede ser parcial y esta instancia no
 * reabre ninguna de las dos bases hasta que una limpieza explícita tenga éxito.
 */
export function clearOfflineStorage(): Promise<void> {
  if (clearing) return clearing;
  epoch++;
  // Keep the gate closed on any failure, including failures before deletion
  // requests have both been created. Only a complete retry reopens it.
  clearFailed = true;
  const pending = database;
  const pendingCAS = casDatabase;
  database = null;
  casDatabase = null;
  clearing = (async () => {
    masterKey = null;
    if (pending !== null) {
      try {
        (await pending).close();
      } catch {
        /* A failed open has no connection. */
      }
    }
    if (pendingCAS !== null) {
      try {
        (await pendingCAS).close();
      } catch {
        /* A failed open has no connection. */
      }
    }
    const results = await Promise.allSettled(
      OFFLINE_DATABASES.map(
        (name) =>
          new Promise<void>((resolve, reject) => {
            const request = indexedDB.deleteDatabase(name);
            request.onsuccess = () => resolve();
            request.onerror = () => reject(request.error ?? new Error('indexeddb delete'));
            request.onblocked = () => undefined;
          }),
      ),
    );
    const failed = results.find((result) => result.status === 'rejected');
    if (failed?.status === 'rejected') throw failed.reason;
    clearFailed = false;
  })().finally(() => {
    clearing = null;
  });
  return clearing;
}

function openCASDatabase(): Promise<IDBDatabase> {
  if (clearing) return Promise.reject(new Error('indexeddb clearing'));
  if (clearFailed) return Promise.reject(new Error('indexeddb cleanup failed'));
  const openingEpoch = epoch;
  if (casDatabase !== null) return casDatabase;
  const pending = new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open(CAS_DATABASE, 1);
    request.onerror = () => reject(request.error ?? new Error('indexeddb open'));
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains(RECORDS)) {
        request.result.createObjectStore(RECORDS, { keyPath: 'key' });
      }
    };
    request.onsuccess = () => {
      const db = request.result;
      if (openingEpoch !== epoch || clearing) {
        db.close();
        reject(new Error('indexeddb cleared'));
        return;
      }
      db.onversionchange = () => {
        epoch++;
        db.close();
        if (casDatabase === pending) casDatabase = null;
      };
      resolve(db);
    };
  }).catch((error: unknown) => {
    if (casDatabase === pending) casDatabase = null;
    throw error;
  });
  casDatabase = pending;
  return pending;
}

function scopeKey(ownerId: string, namespace: string): string {
  return `${ownerId}\u001f${namespace}`;
}

function recordKey(ownerId: string, namespace: string, id: string): string {
  return `${scopeKey(ownerId, namespace)}\u001f${id}`;
}

function recordAAD(ownerId: string, namespace: string, id: string): Uint8Array<ArrayBuffer> {
  return encoder.encode(`${SEAL_AAD_PREFIX}\u0000${ownerId}\u0000${namespace}\u0000${id}`);
}

function openDatabase(): Promise<IDBDatabase> {
  if (clearing) return Promise.reject(new Error('indexeddb clearing'));
  if (clearFailed) return Promise.reject(new Error('indexeddb cleanup failed'));
  const openingEpoch = epoch;
  if (database !== null) return database;
  database = new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE, VERSION);
    request.onerror = () => {
      database = null;
      masterKey = null;
      reject(request.error ?? new Error('indexeddb open'));
    };
    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(RECORDS)) {
        const store = db.createObjectStore(RECORDS, { keyPath: 'key' });
        store.createIndex(SCOPE_INDEX, 'scope', { unique: false });
      }
      if (!db.objectStoreNames.contains(KEYS)) {
        db.createObjectStore(KEYS, { keyPath: 'id' });
      }
    };
    request.onsuccess = () => {
      const db = request.result;
      if (openingEpoch !== epoch || clearing) {
        db.close();
        reject(new Error('indexeddb cleared'));
        return;
      }
      db.onversionchange = () => {
        epoch++;
        db.close();
        database = null;
        masterKey = null;
      };
      resolve(db);
    };
  });
  return database;
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error('indexeddb request'));
  });
}

function transactionDone(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve();
    transaction.onerror = () => reject(transaction.error ?? new Error('indexeddb transaction'));
    transaction.onabort = () => reject(transaction.error ?? new Error('indexeddb abort'));
  });
}

function ensureMasterKey(): Promise<CryptoKey> {
  const keyEpoch = epoch;
  try {
    checkOfflineEpoch(keyEpoch);
  } catch (error) {
    return Promise.reject(error instanceof Error ? error : new Error(String(error)));
  }
  if (!isOfflineStorageEncrypted()) {
    return Promise.reject(new Error('offline_encryption_unavailable'));
  }
  if (masterKey !== null)
    return masterKey.then((key) => {
      checkOfflineEpoch(keyEpoch);
      return key;
    });
  const pending = exclusiveLock('sytue-offline:master-key', async () => {
    checkOfflineEpoch(keyEpoch);
    const db = await openDatabase();
    checkOfflineEpoch(keyEpoch);
    const readTx = db.transaction(KEYS, 'readonly');
    const existing = (await requestResult(readTx.objectStore(KEYS).get(MASTER_KEY_ID))) as
      StoredKeyRow | undefined;
    checkOfflineEpoch(keyEpoch);
    if (existing?.key) return existing.key;

    const generated = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, false, [
      'encrypt',
      'decrypt',
    ]);
    checkOfflineEpoch(keyEpoch);
    const writeTx = db.transaction(KEYS, 'readwrite');
    const writeDone = transactionDone(writeTx);
    // The request and transaction can fail independently; observe both even
    // when an epoch change aborts before the transaction completion is awaited.
    void writeDone.catch(() => undefined);
    try {
      const check = (await requestResult(writeTx.objectStore(KEYS).get(MASTER_KEY_ID))) as
        StoredKeyRow | undefined;
      checkOfflineEpoch(keyEpoch);
      if (check?.key) {
        await writeDone;
        checkOfflineEpoch(keyEpoch);
        return check.key;
      }
      writeTx.objectStore(KEYS).put({
        id: MASTER_KEY_ID,
        key: generated,
        createdAt: Date.now(),
      } satisfies StoredKeyRow);
      await writeDone;
      checkOfflineEpoch(keyEpoch);
      return generated;
    } catch (error) {
      try {
        writeTx.abort();
      } catch {
        /* Already complete or aborted. */
      }
      await writeDone.catch(() => undefined);
      throw error;
    }
  }).catch((error: unknown) => {
    if (masterKey === pending) masterKey = null;
    throw error;
  });
  masterKey = pending;
  return pending;
}

function toBytes(input: unknown): Uint8Array<ArrayBuffer> | null {
  if (input instanceof Uint8Array) {
    const copy = new Uint8Array(input.byteLength);
    copy.set(input);
    return copy;
  }
  if (input instanceof ArrayBuffer) {
    return new Uint8Array(input.slice(0));
  }
  return null;
}

async function sealValue<T>(
  ownerId: string,
  namespace: string,
  id: string,
  value: T,
  additionalData = recordAAD(ownerId, namespace, id),
): Promise<SealedOfflinePayload> {
  const plain = encoder.encode(JSON.stringify({ v: value }));
  try {
    return await sealPlain(plain, additionalData);
  } finally {
    plain.fill(0);
  }
}

async function sealPlain(
  plain: Uint8Array<ArrayBuffer>,
  additionalData: Uint8Array<ArrayBuffer>,
): Promise<SealedOfflinePayload> {
  const key = await ensureMasterKey();
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const encrypted = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv, additionalData },
    key,
    plain,
  );
  return { v: 1, iv, ciphertext: new Uint8Array(encrypted) };
}

async function openRow<T>(
  row: StoredOfflineRow<T>,
  expectedOwnerId: string,
  expectedNamespace?: string,
  expectedId?: string,
  additionalData?: Uint8Array<ArrayBuffer>,
): Promise<{ record: OfflineRecord<T>; legacy: boolean }> {
  const namespace = expectedNamespace ?? row.namespace;
  const id = expectedId ?? row.id;
  if (
    typeof row.ownerId !== 'string' ||
    typeof row.namespace !== 'string' ||
    typeof row.id !== 'string' ||
    typeof row.updatedAt !== 'number' ||
    row.ownerId !== expectedOwnerId ||
    row.namespace !== namespace ||
    row.id !== id ||
    row.scope !== scopeKey(expectedOwnerId, namespace) ||
    row.key !== recordKey(expectedOwnerId, namespace, id)
  ) {
    throw new Error('offline_record_tampered');
  }
  if (row.sealed !== undefined) {
    if ('value' in row && row.value !== undefined) {
      throw new Error('offline_record_tampered');
    }
    const iv = toBytes(row.sealed.iv);
    const ciphertext = toBytes(row.sealed.ciphertext);
    if (
      row.sealed.v !== 1 ||
      !iv ||
      iv.byteLength !== 12 ||
      !ciphertext ||
      ciphertext.byteLength < 16
    ) {
      throw new Error('offline_record_tampered');
    }
    const key = await ensureMasterKey();
    const clearBuffer = await crypto.subtle.decrypt(
      {
        name: 'AES-GCM',
        iv,
        additionalData: additionalData ?? recordAAD(expectedOwnerId, namespace, id),
      },
      key,
      ciphertext,
    );
    const clearBytes = new Uint8Array(clearBuffer);
    try {
      const parsed = JSON.parse(decoder.decode(clearBytes)) as { v?: T } | null;
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('offline_record_tampered');
      }
      return {
        record: {
          key: row.key,
          scope: row.scope,
          ownerId: row.ownerId,
          namespace: row.namespace,
          id: row.id,
          value: parsed.v as T,
          updatedAt: row.updatedAt,
        },
        legacy: false,
      };
    } finally {
      clearBytes.fill(0);
    }
  }
  if (!('value' in row)) {
    throw new Error('offline_record_tampered');
  }
  return {
    record: {
      key: row.key,
      scope: row.scope,
      ownerId: row.ownerId,
      namespace: row.namespace,
      id: row.id,
      value: row.value,
      updatedAt: row.updatedAt,
    },
    legacy: !protectedLocalValue(namespace, row.value),
  };
}

// These two internal namespaces already contain AES-GCM ciphertext or a
// non-extractable AES key. JSON serialization destroys CryptoKey/typed arrays;
// IndexedDB must retain them by structured clone, as it retains the master key.
// Reject ordinary application values at this boundary: it is not a raw-data API.
function protectedLocalValue(namespace: string, value: unknown): boolean {
  if (namespace !== 'local-crypto-key-v1' && namespace !== 'local-crypto-value-v1') return false;
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new Error('offline_protected_value');
  const record = value as Record<string, unknown>;
  if (record.v !== 1) throw new Error('offline_protected_value');
  if (namespace === 'local-crypto-key-v1') {
    const key = record.key;
    if (
      !(key instanceof CryptoKey) ||
      key.type !== 'secret' ||
      key.extractable ||
      key.algorithm.name !== 'AES-GCM' ||
      (key.algorithm as AesKeyAlgorithm).length !== 256 ||
      !key.usages.includes('encrypt') ||
      !key.usages.includes('decrypt') ||
      Object.keys(record).some((key) => !['v', 'key'].includes(key))
    )
      throw new Error('offline_protected_key');
  } else if (
    !(record.iv instanceof Uint8Array) ||
    record.iv.byteLength !== 12 ||
    !(record.ciphertext instanceof Uint8Array) ||
    record.ciphertext.byteLength < 16 ||
    record.ciphertext.byteLength > (8 << 20) + 16 ||
    Object.keys(record).some((key) => !['v', 'iv', 'ciphertext'].includes(key))
  )
    throw new Error('offline_protected_value');
  return true;
}

async function putSealedRow<T>(
  ownerId: string,
  namespace: string,
  id: string,
  value: T,
  updatedAt = Date.now(),
  writeEpoch = epoch,
): Promise<void> {
  checkOfflineEpoch(writeEpoch);
  const protectedValue = protectedLocalValue(namespace, value);
  const sealed = protectedValue ? undefined : await sealValue(ownerId, namespace, id, value);
  checkOfflineEpoch(writeEpoch);
  const db = await openDatabase();
  checkOfflineEpoch(writeEpoch);
  const transaction = db.transaction(RECORDS, 'readwrite');
  transaction.objectStore(RECORDS).put({
    key: recordKey(ownerId, namespace, id),
    scope: scopeKey(ownerId, namespace),
    ownerId,
    namespace,
    id,
    ...(protectedValue ? { value } : { sealed: sealed! }),
    updatedAt,
  } satisfies StoredOfflineRow<T>);
  await transactionDone(transaction);
  checkOfflineEpoch(writeEpoch);
}

export async function readOfflineValue<T>(
  ownerId: string,
  namespace: string,
  id: string,
): Promise<T | null> {
  const readEpoch = epoch;
  checkOfflineEpoch(readEpoch);
  const db = await openDatabase();
  checkOfflineEpoch(readEpoch);
  const transaction = db.transaction(RECORDS, 'readonly');
  const row = (await requestResult(
    transaction.objectStore(RECORDS).get(recordKey(ownerId, namespace, id)),
  )) as StoredOfflineRow<T> | undefined;
  checkOfflineEpoch(readEpoch);
  if (!row) return null;
  const { record, legacy } = await openRow(row, ownerId, namespace, id);
  checkOfflineEpoch(readEpoch);
  if (legacy) {
    await putSealedRow(ownerId, namespace, id, record.value, record.updatedAt, readEpoch).catch(
      () => undefined,
    );
  }
  checkOfflineEpoch(readEpoch);
  return record.value;
}

export async function writeOfflineValue<T>(
  ownerId: string,
  namespace: string,
  id: string,
  value: T,
): Promise<void> {
  const writeEpoch = epoch;
  checkOfflineEpoch(writeEpoch);
  await putSealedRow(ownerId, namespace, id, value, undefined, writeEpoch);
  checkOfflineEpoch(writeEpoch);
}

/**
 * Mutación atómica serializada por registro. Descifra, aplica `update` y vuelve a
 * sellar o borrar bajo `exclusiveOfflineRecord`, evitando que WebCrypto cierre la
 * transacción de IndexedDB por inactividad entre el GET y el PUT.
 */
export async function mutateOfflineValue<T>(
  ownerId: string,
  namespace: string,
  id: string,
  update: (previous: T | null) => T | null,
): Promise<void> {
  const mutationEpoch = epoch;
  checkOfflineEpoch(mutationEpoch);
  await exclusiveOfflineRecord(ownerId, namespace, id, async () => {
    checkOfflineEpoch(mutationEpoch);
    const db = await openDatabase();
    checkOfflineEpoch(mutationEpoch);
    const readTx = db.transaction(RECORDS, 'readonly');
    const row = (await requestResult(
      readTx.objectStore(RECORDS).get(recordKey(ownerId, namespace, id)),
    )) as StoredOfflineRow<T> | undefined;
    checkOfflineEpoch(mutationEpoch);
    const previous = row ? (await openRow(row, ownerId, namespace, id)).record.value : null;
    checkOfflineEpoch(mutationEpoch);
    const next = update(previous);
    checkOfflineEpoch(mutationEpoch);
    if (next === null) {
      await deleteOfflineValue(ownerId, namespace, id, () => checkOfflineEpoch(mutationEpoch));
      checkOfflineEpoch(mutationEpoch);
      return;
    }
    await putSealedRow(ownerId, namespace, id, next, undefined, mutationEpoch);
  });
  checkOfflineEpoch(mutationEpoch);
}

export async function listOfflineValues<T>(
  ownerId: string,
  namespace: string,
): Promise<OfflineRecord<T>[]> {
  const readEpoch = epoch;
  checkOfflineEpoch(readEpoch);
  const db = await openDatabase();
  checkOfflineEpoch(readEpoch);
  const transaction = db.transaction(RECORDS, 'readonly');
  const rows = (await requestResult(
    transaction.objectStore(RECORDS).index(SCOPE_INDEX).getAll(scopeKey(ownerId, namespace)),
  )) as StoredOfflineRow<T>[];
  checkOfflineEpoch(readEpoch);
  const records: OfflineRecord<T>[] = [];
  for (const row of rows) {
    const { record, legacy } = await openRow(row, ownerId, namespace);
    checkOfflineEpoch(readEpoch);
    if (legacy) {
      await putSealedRow(
        ownerId,
        namespace,
        record.id,
        record.value,
        record.updatedAt,
        readEpoch,
      ).catch(() => undefined);
    }
    checkOfflineEpoch(readEpoch);
    records.push(record);
  }
  return records.sort((a, b) => b.updatedAt - a.updatedAt);
}

export interface OfflineIDPage {
  ids: string[];
  next: string;
}

/** One bounded key-only range. No encrypted record bodies are cloned. */
async function offlineIDWindow(
  ownerId: string,
  namespace: string,
  maximum: number,
  filter: string,
  after = '',
): Promise<string[]> {
  if (!Number.isSafeInteger(maximum) || maximum < 1 || maximum > 10000)
    throw new Error('offline_records_limit');
  if (
    typeof filter !== 'string' ||
    filter.length > 4096 ||
    !/^[ -~]*$/.test(filter) ||
    typeof after !== 'string' ||
    after.length > 4096 ||
    !/^[ -~]*$/.test(after) ||
    (after && !after.startsWith(filter))
  )
    throw new Error('offline_record_key');
  const readEpoch = epoch;
  checkOfflineEpoch(readEpoch);
  const db = await openDatabase();
  checkOfflineEpoch(readEpoch);
  const transaction = db.transaction(RECORDS, 'readonly');
  const prefix = recordKey(ownerId, namespace, '');
  const start = prefix + filter;
  const upper = start.slice(0, -1) + String.fromCharCode(start.charCodeAt(start.length - 1) + 1);
  const store = transaction.objectStore(RECORDS);
  const keys = await requestResult(
    !filter && !after
      ? store.index(SCOPE_INDEX).getAllKeys(scopeKey(ownerId, namespace), maximum + 1)
      : store.getAllKeys(
          IDBKeyRange.bound(after ? prefix + after : start, upper, !!after, true),
          maximum + 1,
        ),
  );
  checkOfflineEpoch(readEpoch);
  return keys.map((key) => {
    if (typeof key !== 'string' || !key.startsWith(start) || key.length === prefix.length)
      throw new Error('offline_record_key');
    const id = key.slice(prefix.length);
    if (after && id <= after) throw new Error('offline_record_key');
    return id;
  });
}

/** Bounded complete enumeration; overflow fails rather than silently truncating. */
export async function listOfflineIDs(
  ownerId: string,
  namespace: string,
  maximum = 1000,
  filter = '',
): Promise<string[]> {
  const readEpoch = epoch;
  checkOfflineEpoch(readEpoch);
  const ids = await offlineIDWindow(ownerId, namespace, maximum, filter);
  checkOfflineEpoch(readEpoch);
  if (ids.length > maximum) throw new Error('offline_records_limit');
  return ids;
}

/** Exclusive keyset pagination, including one lookahead key to detect the next page. */
export async function pageOfflineIDs(
  ownerId: string,
  namespace: string,
  maximum = 128,
  filter = '',
  after = '',
): Promise<OfflineIDPage> {
  const readEpoch = epoch;
  checkOfflineEpoch(readEpoch);
  const ids = await offlineIDWindow(ownerId, namespace, maximum, filter, after);
  checkOfflineEpoch(readEpoch);
  return { ids: ids.slice(0, maximum), next: ids.length > maximum ? ids[maximum - 1]! : '' };
}

/** Delete one indexed scope with a key cursor, independent of total ciphertext size. */
export async function deleteOfflineScope(ownerId: string, namespace: string): Promise<void> {
  const deleteEpoch = epoch;
  checkOfflineEpoch(deleteEpoch);
  const db = await openDatabase();
  checkOfflineEpoch(deleteEpoch);
  const transaction = db.transaction(RECORDS, 'readwrite');
  const done = transactionDone(transaction);
  const store = transaction.objectStore(RECORDS),
    prefix = recordKey(ownerId, namespace, '');
  const request = store.index(SCOPE_INDEX).openKeyCursor(scopeKey(ownerId, namespace));
  request.onsuccess = () => {
    if (deleteEpoch !== epoch || clearing || clearFailed) {
      transaction.abort();
      return;
    }
    const cursor = request.result;
    if (!cursor) return;
    if (typeof cursor.primaryKey !== 'string' || !cursor.primaryKey.startsWith(prefix)) {
      transaction.abort();
      return;
    }
    store.delete(cursor.primaryKey);
    cursor.continue();
  };
  await done;
  checkOfflineEpoch(deleteEpoch);
}

export async function deleteOfflineValue(
  ownerId: string,
  namespace: string,
  id: string,
  beforeDelete?: () => void,
): Promise<void> {
  const deleteEpoch = epoch;
  checkOfflineEpoch(deleteEpoch);
  const db = await openDatabase();
  checkOfflineEpoch(deleteEpoch);
  // Check after asynchronous opening, then enqueue deletion without another
  // await or reading/cloning the potentially large stored value.
  beforeDelete?.();
  checkOfflineEpoch(deleteEpoch);
  const transaction = db.transaction(RECORDS, 'readwrite');
  transaction.objectStore(RECORDS).delete(recordKey(ownerId, namespace, id));
  await transactionDone(transaction);
  checkOfflineEpoch(deleteEpoch);
}

/** Opaque comparison witness; retain it unchanged until the attempted write. */
export interface OfflineCASWitness {
  readonly ownerId: string;
  readonly namespace: string;
  readonly id: string;
  readonly revision: string;
  readonly deleted: boolean;
  readonly iv: Uint8Array;
  readonly ciphertext: Uint8Array;
}

export type OfflineCASRead<T> =
  | { readonly present: true; readonly value: T; readonly witness: OfflineCASWitness }
  | { readonly present: false; readonly witness: OfflineCASWitness };

interface CASRow extends StoredOfflineRow<never> {
  revision: string;
  deleted: boolean;
  sealed: SealedOfflinePayload;
}

function validateCASKey(ownerId: string, namespace: string, id: string): void {
  if (
    [ownerId, namespace, id].some(
      (part) =>
        typeof part !== 'string' ||
        !part ||
        part.includes('\u001f') ||
        part.includes('\u0000') ||
        !wellFormedUTF16(part),
    )
  ) {
    throw new Error('offline_cas_key');
  }
}

// TextEncoder replaces lone UTF-16 surrogates. Reject them before two distinct
// IndexedDB keys can collapse into the same authenticated bytes.
function wellFormedUTF16(value: string): boolean {
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(++i);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
    } else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}

function casRow(
  row: StoredOfflineRow & { revision?: unknown },
  ownerId: string,
  namespace: string,
  id: string,
): CASRow {
  if (
    row.key !== recordKey(ownerId, namespace, id) ||
    row.scope !== scopeKey(ownerId, namespace) ||
    row.ownerId !== ownerId ||
    row.namespace !== namespace ||
    row.id !== id ||
    typeof row.updatedAt !== 'number' ||
    !Number.isFinite(row.updatedAt) ||
    typeof row.revision !== 'string' ||
    !/^[0-9a-f]{32}$/.test(row.revision) ||
    typeof (row as CASRow).deleted !== 'boolean' ||
    'value' in row ||
    !row.sealed ||
    row.sealed.v !== 1 ||
    !(row.sealed.iv instanceof Uint8Array) ||
    row.sealed.iv.byteLength !== 12 ||
    !(row.sealed.ciphertext instanceof Uint8Array) ||
    row.sealed.ciphertext.byteLength < 16
  )
    throw new Error('offline_cas_tampered');
  return row as CASRow;
}

function casWitness(row: CASRow): OfflineCASWitness {
  return {
    ownerId: row.ownerId,
    namespace: row.namespace,
    id: row.id,
    revision: row.revision,
    deleted: row.deleted,
    iv: new Uint8Array(row.sealed.iv),
    ciphertext: new Uint8Array(row.sealed.ciphertext),
  };
}

function copyWitness(
  witness: OfflineCASWitness,
  ownerId: string,
  namespace: string,
  id: string,
): OfflineCASWitness {
  if (
    witness.ownerId !== ownerId ||
    witness.namespace !== namespace ||
    witness.id !== id ||
    !/^[0-9a-f]{32}$/.test(witness.revision) ||
    typeof witness.deleted !== 'boolean' ||
    !(witness.iv instanceof Uint8Array) ||
    witness.iv.byteLength !== 12 ||
    !(witness.ciphertext instanceof Uint8Array) ||
    witness.ciphertext.byteLength < 16
  )
    throw new Error('offline_cas_witness');
  return {
    ...witness,
    iv: new Uint8Array(witness.iv),
    ciphertext: new Uint8Array(witness.ciphertext),
  };
}

function sameBytes(a: Uint8Array, b: Uint8Array): boolean {
  return a.byteLength === b.byteLength && a.every((value, i) => value === b[i]);
}

function casAAD(
  ownerId: string,
  namespace: string,
  id: string,
  revision: string,
  deleted: boolean,
): Uint8Array<ArrayBuffer> {
  return encoder.encode(
    `sytue-offline-cas-v2\u0000${ownerId}\u0000${namespace}\u0000${id}\u0000${revision}\u0000${deleted ? 'deleted' : 'present'}`,
  );
}

/** Read and authenticate a v2 record; legacy records are never migrated implicitly. */
export async function readOfflineCAS<T>(
  ownerId: string,
  namespace: string,
  id: string,
): Promise<OfflineCASRead<T> | null> {
  validateCASKey(ownerId, namespace, id);
  const generation = epoch;
  const db = await openCASDatabase();
  if (generation !== epoch || clearing) throw new Error('indexeddb cleared');
  const tx = db.transaction(RECORDS, 'readonly');
  const row = (await requestResult(
    tx.objectStore(RECORDS).get(recordKey(ownerId, namespace, id)),
  )) as CASRow | undefined;
  if (generation !== epoch || clearing) throw new Error('indexeddb cleared');
  if (!row) return null;
  const checked = casRow(row, ownerId, namespace, id);
  const witness = casWitness(checked);
  const opened = await openRow<T | null>(
    checked,
    ownerId,
    namespace,
    id,
    casAAD(ownerId, namespace, id, checked.revision, checked.deleted),
  );
  if (generation !== epoch || clearing) throw new Error('indexeddb cleared');
  if (checked.deleted) {
    if (opened.record.value !== null) throw new Error('offline_cas_tampered');
    return { present: false, witness };
  }
  return { present: true, value: opened.record.value as T, witness };
}

/** Compare and swap/delete in one IDB transaction, with no asynchronous work in it. */
async function changeOfflineCAS(
  ownerId: string,
  namespace: string,
  id: string,
  expected: OfflineCASWitness | null,
  value: { present: true; value: unknown } | { present: false },
): Promise<boolean> {
  validateCASKey(ownerId, namespace, id);
  if (!value.present && expected === null) throw new Error('offline_cas_witness');
  const witness = expected === null ? null : copyWitness(expected, ownerId, namespace, id);
  const generation = epoch;
  const revision = Array.from(crypto.getRandomValues(new Uint8Array(16)), (n) =>
    n.toString(16).padStart(2, '0'),
  ).join('');
  const deleted = !value.present;
  const plain = encoder.encode(JSON.stringify({ v: value.present ? value.value : null }));
  // Keep a connection open during sealing so a v2 logout in another window can
  // deliver versionchange before this operation commits. Legacy windows do not
  // know this database and cannot provide that lifecycle fence.
  let db: IDBDatabase;
  let sealed: SealedOfflinePayload;
  try {
    db = await openCASDatabase();
    if (generation !== epoch || clearing) throw new Error('indexeddb cleared');
    sealed = await sealPlain(plain, casAAD(ownerId, namespace, id, revision, deleted));
  } finally {
    plain.fill(0);
  }
  if (generation !== epoch || clearing) throw new Error('indexeddb cleared');
  const tx = db.transaction(RECORDS, 'readwrite');
  const done = transactionDone(tx);
  const store = tx.objectStore(RECORDS);
  const request = store.get(recordKey(ownerId, namespace, id));
  let changed = false;
  let failure: unknown;
  request.onsuccess = () => {
    try {
      if (generation !== epoch || clearing) throw new Error('indexeddb cleared');
      const row = request.result as CASRow | undefined;
      const current = row ? casRow(row, ownerId, namespace, id) : null;
      const matches =
        witness === null
          ? current === null
          : current !== null &&
            current.revision === witness.revision &&
            current.deleted === witness.deleted &&
            sameBytes(current.sealed.iv, witness.iv) &&
            sameBytes(current.sealed.ciphertext, witness.ciphertext);
      if (!matches || (!value.present && current?.deleted)) return;
      store.put({
        key: recordKey(ownerId, namespace, id),
        scope: scopeKey(ownerId, namespace),
        ownerId,
        namespace,
        id,
        revision,
        deleted,
        sealed,
        updatedAt: Date.now(),
      } satisfies CASRow);
      changed = true;
    } catch (error) {
      failure = error;
      tx.abort();
    }
  };
  try {
    await done;
  } catch (error) {
    throw failure ?? error;
  }
  if (generation !== epoch || clearing) throw new Error('indexeddb cleared');
  return changed;
}

/** Create only when expected is null; every replacement requires its read witness. */
export function writeOfflineCAS<T>(
  ownerId: string,
  namespace: string,
  id: string,
  expected: OfflineCASWitness | null,
  value: T,
): Promise<boolean> {
  return changeOfflineCAS(ownerId, namespace, id, expected, { present: true, value });
}

export function deleteOfflineCAS(
  ownerId: string,
  namespace: string,
  id: string,
  expected: OfflineCASWitness,
): Promise<boolean> {
  return changeOfflineCAS(ownerId, namespace, id, expected, { present: false });
}
