/*
 * Almacén local compartido de la suite.
 *
 * Cada origen mantiene su propia base IndexedDB y, dentro de ella, cada registro
 * queda acotado por ownerId + namespace. Cambiar de cuenta en un dispositivo no
 * mezcla Mail, Calendar o Docs ni permite que una cuenta enumere datos de otra.
 */

const DATABASE = 'sytue-offline';
const VERSION = 1;
const RECORDS = 'records';
const SCOPE_INDEX = 'scope';

export interface OfflineRecord<T = unknown> {
  key: string;
  scope: string;
  ownerId: string;
  namespace: string;
  id: string;
  value: T;
  updatedAt: number;
}

let database: Promise<IDBDatabase> | null = null;

/**
 * Cierra la conexión de esta ventana y elimina toda la base del origen. El
 * service worker lo solicita a TODAS las ventanas antes de borrar al cerrar
 * sesión; sin este cierre, deleteDatabase puede quedarse bloqueado y dejar los
 * datos de la cuenta anterior en el dispositivo.
 */
export async function clearOfflineStorage(): Promise<void> {
  const pending = database;
  database = null;
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

function openDatabase(): Promise<IDBDatabase> {
  if (database !== null) return database;
  database = new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE, VERSION);
    request.onerror = () => {
      database = null;
      reject(request.error ?? new Error('indexeddb open'));
    };
    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(RECORDS)) {
        const store = db.createObjectStore(RECORDS, { keyPath: 'key' });
        store.createIndex(SCOPE_INDEX, 'scope', { unique: false });
      }
    };
    request.onsuccess = () => {
      const db = request.result;
      db.onversionchange = () => {
        db.close();
        database = null;
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

export async function readOfflineValue<T>(
  ownerId: string,
  namespace: string,
  id: string,
): Promise<T | null> {
  const db = await openDatabase();
  const transaction = db.transaction(RECORDS, 'readonly');
  const record = (await requestResult(
    transaction.objectStore(RECORDS).get(recordKey(ownerId, namespace, id)),
  )) as OfflineRecord<T> | undefined;
  return record?.value ?? null;
}

export async function writeOfflineValue<T>(
  ownerId: string,
  namespace: string,
  id: string,
  value: T,
): Promise<void> {
  const db = await openDatabase();
  const transaction = db.transaction(RECORDS, 'readwrite');
  transaction.objectStore(RECORDS).put({
    key: recordKey(ownerId, namespace, id),
    scope: scopeKey(ownerId, namespace),
    ownerId,
    namespace,
    id,
    value,
    updatedAt: Date.now(),
  } satisfies OfflineRecord<T>);
  await transactionDone(transaction);
}

/**
 * Mutación atómica. El put/delete se programa dentro del callback onsuccess
 * del GET, antes de que IndexedDB pueda cerrar la transacción por inactividad.
 */
export async function mutateOfflineValue<T>(
  ownerId: string,
  namespace: string,
  id: string,
  update: (previous: T | null) => T | null,
): Promise<void> {
  const db = await openDatabase();
  const transaction = db.transaction(RECORDS, 'readwrite');
  const store = transaction.objectStore(RECORDS);
  const key = recordKey(ownerId, namespace, id);
  await new Promise<void>((resolve, reject) => {
    const get = store.get(key);
    get.onerror = () => reject(get.error ?? new Error('indexeddb get'));
    get.onsuccess = () => {
      try {
        const previous = (get.result as OfflineRecord<T> | undefined)?.value ?? null;
        const next = update(previous);
        if (next === null) {
          store.delete(key);
        } else {
          store.put({
            key,
            scope: scopeKey(ownerId, namespace),
            ownerId,
            namespace,
            id,
            value: next,
            updatedAt: Date.now(),
          } satisfies OfflineRecord<T>);
        }
        resolve();
      } catch (error) {
        transaction.abort();
        reject(error instanceof Error ? error : new Error('offline mutation'));
      }
    };
  });
  await transactionDone(transaction);
}

export async function listOfflineValues<T>(
  ownerId: string,
  namespace: string,
): Promise<OfflineRecord<T>[]> {
  const db = await openDatabase();
  const transaction = db.transaction(RECORDS, 'readonly');
  const records = (await requestResult(
    transaction.objectStore(RECORDS).index(SCOPE_INDEX).getAll(scopeKey(ownerId, namespace)),
  )) as OfflineRecord<T>[];
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
