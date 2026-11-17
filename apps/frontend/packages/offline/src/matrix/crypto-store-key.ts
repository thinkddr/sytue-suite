/* SPDX-License-Identifier: Apache-2.0 */
/**
 * The key of the Matrix SDK's crypto store in this browser (ADR 0085).
 *
 * matrix-sdk-crypto keeps this device's Olm account, its Megolm sessions and the
 * cross-signing private keys in IndexedDB. Opened without a key it stores them as they
 * are, readable by anything that reads the browser profile. Each new store is opened with
 * a random 32-byte key instead, kept only sealed (AES-256-GCM) under a non-extractable
 * CryptoKey of this origin (`EncryptedOfflineValues`, like the owner-key cache): nothing
 * of it is readable at rest, and the person does nothing for it.
 *
 * A store the SDK already wrote without a key (a device from before ADR 0085) keeps
 * opening without one: the Olm account cannot be moved into a new store, and dropping it
 * would lose this device's keys. It is replaced by an encrypted one with the device
 * itself, at the next sign-in on this browser (ADR 0085 says so).
 */
import { EncryptedOfflineValues, type EncryptedOfflinePort } from '../encrypted-storage.js';
import { MatrixSecurityError } from './types.js';

const namespace = 'matrix-crypto-store-key-v1';
const recordID = 'store';

/** Where the sealed key lives: this device's `EncryptedOfflineValues` (or a test double). */
export type CryptoStoreKeyValues = Pick<EncryptedOfflineValues, 'read' | 'write' | 'close'>;

/** Whether the SDK already has a crypto store under a prefix. */
export type CryptoStoreExists = (prefix: string) => Promise<boolean>;

/** The SDK's database name for a crypto store prefix (matrix-sdk-indexeddb). */
export function cryptoStoreDatabase(prefix: string): string {
  return `${prefix}::matrix-sdk-crypto`;
}

/**
 * Whether an IndexedDB database exists, without creating it: opening a missing one
 * fires `upgradeneeded`, which is aborted so that nothing is left behind.
 */
export function indexedDBExists(name: string): Promise<boolean> {
  return new Promise((resolve) => {
    let created = false;
    let request: IDBOpenDBRequest;
    try {
      request = indexedDB.open(name);
    } catch {
      resolve(false);
      return;
    }
    request.onupgradeneeded = () => {
      created = true;
      request.transaction?.abort();
    };
    request.onsuccess = () => {
      request.result.close();
      resolve(!created);
    };
    request.onerror = () => resolve(false);
    request.onblocked = () => resolve(true);
  });
}

/** The sealed key's home for a device: closed with the device's network lifetime. */
export function cryptoStoreKeyValues(
  prefix: string,
  active: () => boolean,
  port?: EncryptedOfflinePort,
): EncryptedOfflineValues {
  return port
    ? new EncryptedOfflineValues(prefix, namespace, active, port)
    : new EncryptedOfflineValues(prefix, namespace, active);
}

/**
 * The key to open this device's crypto store with: the one it was created with, a new
 * one for a store that does not exist yet, or null for a store from before ADR 0085.
 * The caller zeroes the returned bytes once the SDK has them.
 */
export async function matrixCryptoStoreKey(
  sealed: CryptoStoreKeyValues,
  prefix: string,
  exists: CryptoStoreExists = (p) => indexedDBExists(cryptoStoreDatabase(p)),
): Promise<Uint8Array<ArrayBuffer> | null> {
  let existing: Uint8Array<ArrayBuffer> | null;
  try {
    existing = await sealed.read(recordID);
  } catch {
    // The key is there but cannot be opened: the store it protects cannot be read.
    throw new MatrixSecurityError('device');
  }
  if (existing) {
    if (existing.byteLength !== 32) {
      existing.fill(0);
      throw new MatrixSecurityError('device');
    }
    return existing;
  }
  if (await exists(prefix)) return null;
  const key = crypto.getRandomValues(new Uint8Array(32));
  try {
    await sealed.write(recordID, key);
  } catch (error) {
    key.fill(0);
    throw error;
  }
  return key;
}

/** Opens the SDK's crypto store with its key; the key bytes are zeroed afterwards. */
export async function initCryptoStore(
  client: {
    initRustCrypto: (args: {
      useIndexedDB: boolean;
      cryptoDatabasePrefix: string;
      storageKey?: Uint8Array;
    }) => Promise<void>;
  },
  prefix: string,
  sealed: CryptoStoreKeyValues,
  exists?: CryptoStoreExists,
): Promise<void> {
  const storageKey = await matrixCryptoStoreKey(sealed, prefix, exists);
  try {
    await client.initRustCrypto({
      useIndexedDB: true,
      cryptoDatabasePrefix: prefix,
      ...(storageKey ? { storageKey } : {}),
    });
  } finally {
    // The SDK copied it into its own (WebAssembly) memory: this copy is not needed.
    storageKey?.fill(0);
  }
}
