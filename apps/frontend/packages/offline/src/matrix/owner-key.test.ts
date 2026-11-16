/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { createClient } from 'matrix-js-sdk';
import { calculateKeyCheck } from 'matrix-js-sdk/lib/secret-storage';
import { encodeRecoveryKey } from 'matrix-js-sdk/lib/crypto-api/recovery-key';
import { SuiteMatrix } from './client.js';
import { hasSecretStorageCheck, ownerProofAlgorithm, type StorageOwnerKey } from './owner-proof.js';
interface KeyReader {
  storageAuthKey: (id?: string) => Promise<StorageOwnerKey>;
}
async function fixture(read: () => Promise<string>) {
  const key = crypto.getRandomValues(new Uint8Array(32));
  const description = { algorithm: ownerProofAlgorithm, ...(await calculateKeyCheck(key)) };
  const sdk = createClient({ baseUrl: 'https://matrix.test' }).secretStorage;
  const keys = new Map<string, Uint8Array<ArrayBuffer>>();
  const state = {
    closed: false,
    temporaryKeys: keys,
    ui: { readRecoveryKey: read },
    client: {
      secretStorage: {
        getKey: async () => ['artificial-owner', description] as const,
        checkKey: sdk.checkKey.bind(sdk),
      },
    },
  };
  const reader = Object.assign(Object.create(SuiteMatrix.prototype), state) as KeyReader;
  return { reader, state, key, description, keys };
}

test('a late recovery-key response cannot restore a closed private client', async () => {
  let supply!: (key: string) => void, requested!: () => void;
  const seen = new Promise<void>((resolve) => {
    requested = resolve;
  });
  const pendingKey = new Promise<string>((resolve) => {
    supply = resolve;
  });
  const f = await fixture(async () => {
    requested();
    return pendingKey;
  });
  const pending = f.reader.storageAuthKey('artificial-owner');
  await seen;
  Object.assign(f.reader, { closed: true });
  supply(encodeRecoveryKey(f.key)!);
  await expect(pending).rejects.toThrow('matrix_device');
  expect(f.keys.size).toBe(0);
});

test('owner key reuse checks maintained SSSS metadata and does not ask for a key already cached', async () => {
  let asked = false;
  const f = await fixture(async () => {
    asked = true;
    throw new Error('must use verified cache');
  });
  f.keys.set('artificial-owner', f.key);
  expect(hasSecretStorageCheck(f.description)).toBe(true);
  expect((await f.reader.storageAuthKey('artificial-owner')).key).toEqual(f.key);
  expect(asked).toBe(false);
  for (const bad of [
    { algorithm: ownerProofAlgorithm },
    { ...f.description, mac: 'AA' },
    { ...f.description, iv: 'AA' },
    { ...f.description, algorithm: 'other' },
  ])
    expect(hasSecretStorageCheck(bad)).toBe(false);
  Object.assign(f.state.client.secretStorage, {
    getKey: async () => ['artificial-owner', { algorithm: ownerProofAlgorithm }],
  });
  await expect(f.reader.storageAuthKey('artificial-owner')).rejects.toThrow('matrix_recovery');
  expect(asked).toBe(false);
});

test('an incorrect owner key is never cached', async () => {
  const wrong = encodeRecoveryKey(crypto.getRandomValues(new Uint8Array(32)))!;
  const f = await fixture(async () => wrong);
  await expect(f.reader.storageAuthKey('artificial-owner')).rejects.toThrow('matrix_recovery');
  expect(f.keys.size).toBe(0);
});
