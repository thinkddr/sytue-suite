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

const DATABASE = 'sytue-offline';
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
 */
export async function clearOfflineStorage(): Promise<void> {
  const pending = database;
  database = null;
  masterKey = null;
  if (pending !== null) {
    try {
      (await pending).close();
    } catch {
      // Si open falló no existe una conexión que cerrar; se intenta borrar igual.
    }
  }
  await new Promise<void>((resolve) => {
    const request = indexedDB.deleteDatabase(DATABASE);
    request.onsuccess = () => resolve();
    request.onerror = () => resolve();
    // Otra pestaña puede tardar en recibir el mensaje del worker. No se resuelve
    // aquí: IndexedDB volverá a emitir success cuando esa conexión se cierre.
    request.onblocked = () => undefined;
  });
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
      db.onversionchange = () => {
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
  if (!isOfflineStorageEncrypted()) {
    return Promise.reject(new Error('offline_encryption_unavailable'));
  }
  if (masterKey !== null) return masterKey;
  const pending = exclusiveLock('sytue-offline:master-key', async () => {
    const db = await openDatabase();
    const readTx = db.transaction(KEYS, 'readonly');
    const existing = (await requestResult(readTx.objectStore(KEYS).get(MASTER_KEY_ID))) as
      StoredKeyRow | undefined;
    if (existing?.key) return existing.key;

    const generated = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, false, [
      'encrypt',
      'decrypt',
    ]);
    const writeTx = db.transaction(KEYS, 'readwrite');
    const check = (await requestResult(writeTx.objectStore(KEYS).get(MASTER_KEY_ID))) as
      StoredKeyRow | undefined;
    if (check?.key) {
      await transactionDone(writeTx);
      return check.key;
    }
    writeTx.objectStore(KEYS).put({
      id: MASTER_KEY_ID,
      key: generated,
      createdAt: Date.now(),
    } satisfies StoredKeyRow);
    await transactionDone(writeTx);
    return generated;
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
): Promise<SealedOfflinePayload> {
  const key = await ensureMasterKey();
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const plain = encoder.encode(JSON.stringify({ v: value }));
  try {
    const encrypted = await crypto.subtle.encrypt(
      { name: 'AES-GCM', iv, additionalData: recordAAD(ownerId, namespace, id) },
      key,
      plain,
    );
    return { v: 1, iv, ciphertext: new Uint8Array(encrypted) };
  } finally {
    plain.fill(0);
  }
}

async function openRow<T>(
  row: StoredOfflineRow<T>,
  expectedOwnerId: string,
  expectedNamespace?: string,
  expectedId?: string,
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
      { name: 'AES-GCM', iv, additionalData: recordAAD(expectedOwnerId, namespace, id) },
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
    legacy: true,
  };
}

async function putSealedRow<T>(
  ownerId: string,
  namespace: string,
  id: string,
  value: T,
  updatedAt = Date.now(),
): Promise<void> {
  const sealed = await sealValue(ownerId, namespace, id, value);
  const db = await openDatabase();
  const transaction = db.transaction(RECORDS, 'readwrite');
  transaction.objectStore(RECORDS).put({
    key: recordKey(ownerId, namespace, id),
    scope: scopeKey(ownerId, namespace),
    ownerId,
    namespace,
    id,
    sealed,
    updatedAt,
  } satisfies StoredOfflineRow<T>);
  await transactionDone(transaction);
}

export async function readOfflineValue<T>(
  ownerId: string,
  namespace: string,
  id: string,
): Promise<T | null> {
  const db = await openDatabase();
  const transaction = db.transaction(RECORDS, 'readonly');
  const row = (await requestResult(
    transaction.objectStore(RECORDS).get(recordKey(ownerId, namespace, id)),
  )) as StoredOfflineRow<T> | undefined;
  if (!row) return null;
  const { record, legacy } = await openRow(row, ownerId, namespace, id);
  if (legacy) {
    await putSealedRow(ownerId, namespace, id, record.value, record.updatedAt).catch(
      () => undefined,
    );
  }
  return record.value;
}

export async function writeOfflineValue<T>(
  ownerId: string,
  namespace: string,
  id: string,
  value: T,
): Promise<void> {
  await putSealedRow(ownerId, namespace, id, value);
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
  await exclusiveOfflineRecord(ownerId, namespace, id, async () => {
    const db = await openDatabase();
    const readTx = db.transaction(RECORDS, 'readonly');
    const row = (await requestResult(
      readTx.objectStore(RECORDS).get(recordKey(ownerId, namespace, id)),
    )) as StoredOfflineRow<T> | undefined;
    const previous = row ? (await openRow(row, ownerId, namespace, id)).record.value : null;
    const next = update(previous);
    if (next === null) {
      await deleteOfflineValue(ownerId, namespace, id);
      return;
    }
    await putSealedRow(ownerId, namespace, id, next);
  });
}

export async function listOfflineValues<T>(
  ownerId: string,
  namespace: string,
): Promise<OfflineRecord<T>[]> {
  const db = await openDatabase();
  const transaction = db.transaction(RECORDS, 'readonly');
  const rows = (await requestResult(
    transaction.objectStore(RECORDS).index(SCOPE_INDEX).getAll(scopeKey(ownerId, namespace)),
  )) as StoredOfflineRow<T>[];
  const records: OfflineRecord<T>[] = [];
  for (const row of rows) {
    const { record, legacy } = await openRow(row, ownerId, namespace);
    if (legacy) {
      await putSealedRow(ownerId, namespace, record.id, record.value, record.updatedAt).catch(
        () => undefined,
      );
    }
    records.push(record);
  }
  return records.sort((a, b) => b.updatedAt - a.updatedAt);
}

export async function deleteOfflineValue(
  ownerId: string,
  namespace: string,
  id: string,
): Promise<void> {
  const db = await openDatabase();
  const transaction = db.transaction(RECORDS, 'readwrite');
  transaction.objectStore(RECORDS).delete(recordKey(ownerId, namespace, id));
  await transactionDone(transaction);
}
