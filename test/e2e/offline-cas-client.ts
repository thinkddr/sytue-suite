// SPDX-License-Identifier: Apache-2.0
// Synthetic-only browser fixture; all shared state is scoped to this origin.
import {
  clearOfflineStorage,
  deleteOfflineCAS,
  offlineStorageEpoch,
  readOfflineCAS,
  readOfflineValue,
  writeOfflineCAS,
  writeOfflineValue,
  type OfflineCASWitness,
} from "../../apps/frontend/packages/offline/src/storage";

const owner = "cas-artificial",
  namespace = "capture",
  id = "record";
let witness: OfflineCASWitness | null = null;
let operation: Promise<{ ok: true; value: boolean } | { ok: false }> | undefined;
let queuedTransactions = 0;
let pause = false;
let entered = false;
let releaseEncryption: (() => void) | undefined;
let releaseGate: (() => void) | undefined;
let gateDone: Promise<void> | undefined;
let heldDatabase: IDBDatabase | undefined;
let versionchangeSeen = false;

const encrypt = crypto.subtle.encrypt.bind(crypto.subtle);
const transaction = IDBDatabase.prototype.transaction;
IDBDatabase.prototype.transaction = function (
  this: IDBDatabase,
  storeNames: string | string[],
  mode?: IDBTransactionMode,
  options?: IDBTransactionOptions,
): IDBTransaction {
  const result = transaction.call(this, storeNames, mode, options);
  if (this.name === "sytue-offline-cas-v2" && mode === "readwrite") queuedTransactions++;
  return result;
};
Object.defineProperty(crypto.subtle, "encrypt", {
  configurable: true,
  value: async (...args: Parameters<SubtleCrypto["encrypt"]>) => {
    if (pause) {
      entered = true;
      await new Promise<void>((resolve) => {
        releaseEncryption = resolve;
      });
    }
    return encrypt(...args);
  },
});

async function gateTransactions(): Promise<void> {
  const db = await new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open("sytue-offline-cas-v2");
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
  const tx = db.transaction("records", "readwrite");
  const store = tx.objectStore("records");
  let released = false;
  releaseGate = () => {
    released = true;
  };
  gateDone = new Promise<void>((resolve, reject) => {
    tx.oncomplete = () => {
      db.close();
      resolve();
    };
    tx.onabort = () => {
      db.close();
      reject(tx.error);
    };
    tx.onerror = () => reject(tx.error);
  });
  await new Promise<void>((resolve, reject) => {
    let first = true;
    const keepAlive = () => {
      const request = store.get("__cas_test_gate__");
      request.onerror = () => reject(request.error);
      request.onsuccess = () => {
        if (first) {
          first = false;
          resolve();
        }
        if (!released) keepAlive();
      };
    };
    keepAlive();
  });
}

const probe = {
  async clear() {
    await clearOfflineStorage();
  },
  async create(value: string) {
    return writeOfflineCAS(owner, namespace, id, null, value);
  },
  async read() {
    const result = await readOfflineCAS<string>(owner, namespace, id);
    if (!result) return { present: null, value: null };
    return { present: result.present, value: result.present ? result.value : null };
  },
  async capture() {
    const result = await readOfflineCAS<string>(owner, namespace, id);
    if (!result) throw new Error("missing witness");
    witness = result.witness;
  },
  begin(action: "replace" | "delete" | "recreate") {
    if (!witness) throw new Error("missing witness");
    const current = witness;
    const pending =
      action === "delete"
        ? deleteOfflineCAS(owner, namespace, id, current)
        : writeOfflineCAS(
            owner,
            namespace,
            id,
            current,
            action === "replace" ? "replacement" : "recreated",
          );
    operation = pending.then(
      (value) => ({ ok: true as const, value }),
      () => ({ ok: false as const }),
    );
  },
  async outcome() {
    if (!operation) throw new Error("missing operation");
    return operation;
  },
  queuedTransactions() {
    return queuedTransactions;
  },
  gateTransactions,
  async releaseGate() {
    releaseGate?.();
    await gateDone;
  },
  pauseEncryption() {
    pause = true;
    entered = false;
  },
  encryptionEntered() {
    return entered;
  },
  releaseEncryption() {
    pause = false;
    releaseEncryption?.();
  },
  epoch() {
    return offlineStorageEpoch();
  },
  async createVirgin() {
    return writeOfflineCAS(owner, namespace, id, null, "stale virgin");
  },
  async legacyWrite() {
    await writeOfflineValue(owner, namespace, id, "legacy");
  },
  async legacyRead() {
    return readOfflineValue<string>(owner, namespace, id);
  },
  async holdDatabaseAcrossLogout() {
    versionchangeSeen = false;
    heldDatabase = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open("sytue-offline-cas-v2");
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
    heldDatabase.onversionchange = () => {
      versionchangeSeen = true;
    };
  },
  versionchangeSeen() {
    return versionchangeSeen;
  },
  releaseHeldDatabase() {
    heldDatabase?.close();
    heldDatabase = undefined;
  },
};
Object.assign(window, { offlineCASProbe: probe });
