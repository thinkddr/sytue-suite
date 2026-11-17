/* SPDX-License-Identifier: Apache-2.0 */
/**
 * The Matrix SDK's crypto store is opened with a key (ADR 0085), and that key is kept in
 * this browser only sealed under a non-extractable CryptoKey: nothing of it is readable
 * at rest, and the SDK gets it without the person doing anything.
 */
import { expect, test } from 'bun:test';
import { encryptedStorageMemory } from '../encrypted-storage-fixture.js';
import {
  cryptoStoreDatabase,
  cryptoStoreKeyValues,
  initCryptoStore,
  matrixCryptoStoreKey,
} from './crypto-store-key.js';
import { MatrixSecurityError } from './types.js';

const prefix = 'sytue-rooms-matrix-https%3A%2F%2Fmatrix.example-%40sytue_ana%3Amatrix.example-DEV';

/** Everything the offline store keeps, walked: keys are non-extractable, bytes are ciphertext. */
function expectNothingReadable(records: Map<string, { value: unknown }>, secret: Uint8Array) {
  const hex = Buffer.from(secret).toString('hex');
  let keys = 0;
  const walk = (value: unknown): void => {
    if (value instanceof CryptoKey) {
      keys++;
      expect(value.extractable).toBe(false);
    } else if (value instanceof Uint8Array) {
      expect(Buffer.from(value).toString('hex').includes(hex)).toBe(false);
    } else if (typeof value === 'string') {
      expect(value.includes(hex)).toBe(false);
      expect(value.includes(Buffer.from(secret).toString('base64'))).toBe(false);
    } else if (value && typeof value === 'object') Object.values(value).forEach(walk);
  };
  for (const row of records.values()) walk(row.value);
  expect(keys).toBe(1);
}

function sdk() {
  const calls: { storageKey?: Uint8Array; seen?: string; prefix: string }[] = [];
  return {
    calls,
    client: {
      initRustCrypto: async (args: {
        useIndexedDB: boolean;
        cryptoDatabasePrefix: string;
        storageKey?: Uint8Array;
      }) => {
        expect(args.useIndexedDB).toBe(true);
        calls.push({
          prefix: args.cryptoDatabasePrefix,
          ...(args.storageKey
            ? { storageKey: args.storageKey, seen: Buffer.from(args.storageKey).toString('hex') }
            : {}),
        });
      },
    },
  };
}

test('a new crypto store is opened with a random key the SDK gets and nobody can read at rest', async () => {
  const memory = encryptedStorageMemory();
  const sealed = (p = prefix) => cryptoStoreKeyValues(p, () => true, memory.port);
  const first = sdk();
  await initCryptoStore(first.client, prefix, sealed(), async () => false);
  expect(first.calls).toHaveLength(1);
  const given = first.calls[0]!;
  expect(given.prefix).toBe(prefix);
  expect(given.seen).toMatch(/^[0-9a-f]{64}$/);
  // The page's copy is wiped once the SDK has it.
  expect(given.storageKey!.every((b) => b === 0)).toBe(true);
  expectNothingReadable(memory.records, Buffer.from(given.seen!, 'hex'));
  // The next open gets the same key: the store it encrypted stays readable to the SDK.
  const again = sdk();
  await initCryptoStore(again.client, prefix, sealed(), async () => true);
  expect(again.calls[0]!.seen).toBe(given.seen!);
  // Another device (another prefix) has its own.
  const other = sdk();
  await initCryptoStore(other.client, prefix + '2', sealed(prefix + '2'), async () => false);
  expect(other.calls[0]!.seen).not.toBe(given.seen!);
});

test('a store written before ADR 0085 keeps opening without a key instead of losing the device', async () => {
  const memory = encryptedStorageMemory();
  const asked: string[] = [];
  const legacy = sdk();
  await initCryptoStore(
    legacy.client,
    prefix,
    cryptoStoreKeyValues(prefix, () => true, memory.port),
    async (p) => {
      asked.push(p);
      return true;
    },
  );
  expect(asked).toEqual([prefix]);
  expect(legacy.calls[0]!.storageKey).toBeUndefined();
  expect(memory.records.size).toBe(0);
});

test('a key that cannot be opened stops the SDK instead of opening its store without it', async () => {
  const memory = encryptedStorageMemory();
  await matrixCryptoStoreKey(
    cryptoStoreKeyValues(prefix, () => true, memory.port),
    prefix,
    async () => false,
  );
  // The non-extractable root key is gone (another browser profile, a cleared key store).
  for (const [k, row] of memory.records)
    if ((row as { namespace: string }).namespace === 'local-crypto-key-v1')
      memory.records.delete(k);
  const blocked = sdk();
  await expect(
    initCryptoStore(
      blocked.client,
      prefix,
      cryptoStoreKeyValues(prefix, () => true, memory.port),
      async () => true,
    ),
  ).rejects.toEqual(new MatrixSecurityError('device'));
  expect(blocked.calls).toEqual([]);
});

test('forgetting the device erases its sealed key', async () => {
  const memory = encryptedStorageMemory();
  const sealed = cryptoStoreKeyValues(prefix, () => true, memory.port);
  await matrixCryptoStoreKey(sealed, prefix, async () => false);
  expect(memory.records.size).toBe(2);
  await sealed.destroy();
  expect(memory.records.size).toBe(0);
  expect(cryptoStoreDatabase(prefix)).toBe(`${prefix}::matrix-sdk-crypto`);
});

test('the SDK client opens its crypto store only through the key, and forgets the key with the device', async () => {
  const { readFileSync } = await import('node:fs');
  const source = readFileSync(new URL('./client.ts', import.meta.url), 'utf8');
  expect(source).not.toMatch(/\.initRustCrypto\(/);
  expect(source).toContain('initCryptoStore(client, prefix, cryptoStoreKeys)');
  expect(source).toContain('this.cryptoStoreKeys?.destroy()');
});
