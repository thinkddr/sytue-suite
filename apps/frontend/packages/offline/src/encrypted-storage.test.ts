/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { encryptedStorageMemory as memory } from './encrypted-storage-fixture.js';
const bytes = new TextEncoder().encode('private artificial message');

test('encrypted local values persist across instances with a non-extractable device key and random IVs', async () => {
  const f = memory(),
    first = f.create();
  await first.write('a', bytes);
  await first.write('b', bytes);
  first.close();
  const second = f.create();
  expect(await second.read('a')).toEqual(bytes);
  const root = [...f.records.values()].find((row) => row.namespace === 'local-crypto-key-v1')!
    .value as { key: CryptoKey };
  expect(root.key.extractable).toBe(false);
  await expect(crypto.subtle.exportKey('raw', root.key)).rejects.toThrow();
  const values = [...f.records.values()]
    .filter((row) => row.namespace === 'local-crypto-value-v1')
    .map((row) => row.value as { iv: Uint8Array; ciphertext: Uint8Array });
  expect(values).toHaveLength(2);
  expect(values[0]!.iv).not.toEqual(values[1]!.iv);
  expect(values[0]!.ciphertext).not.toEqual(bytes);
  expect(new TextDecoder().decode(values[0]!.ciphertext)).not.toContain('private artificial');
});

test.each(['id', 'owner', 'namespace', 'ciphertext', 'iv', 'missing-key', 'extractable-key'])(
  'encrypted local cache rejects %s substitution without replacing retained bytes or falling back to plaintext',
  async (kind) => {
    const f = memory();
    await f.create().write('a', bytes);
    const row = [...f.records.values()].find(
      (value) => value.namespace === 'local-crypto-value-v1',
    )!;
    const sealed = row.value as { iv: Uint8Array; ciphertext: Uint8Array };
    let target = f.create(),
      id = 'a';
    if (kind === 'id' || kind === 'owner' || kind === 'namespace') {
      if (kind === 'owner') target = f.create('other');
      if (kind === 'namespace') target = f.create('person', 'other');
      if (kind === 'id') id = 'b';
      const owner = JSON.stringify([
        kind === 'owner' ? 'other' : 'person',
        kind === 'namespace' ? 'other' : 'records',
      ]);
      // Copy both the root and ciphertext: account isolation must also be cryptographically authenticated.
      const copies = [...f.records.values()].map((original) => ({
        ...structuredClone(original),
        ownerId: owner,
        id: original.id === 'a' ? id : original.id,
      }));
      for (const copy of copies) {
        f.records.set(JSON.stringify([owner, copy.namespace, copy.id]), copy);
      }
    } else if (kind === 'ciphertext') sealed.ciphertext[0]! ^= 1;
    else if (kind === 'iv') sealed.iv[0]! ^= 1;
    else {
      const root = [...f.records.entries()].find(
        ([, value]) => value.namespace === 'local-crypto-key-v1',
      )!;
      if (kind === 'missing-key') f.records.delete(root[0]);
      else {
        const key = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, true, [
          'encrypt',
          'decrypt',
        ]);
        root[1].value = { v: 1, key };
        // Matching ciphertext isolates the non-extractability guard from the GCM integrity check.
        sealed.ciphertext = new Uint8Array(
          await crypto.subtle.encrypt(
            {
              name: 'AES-GCM',
              iv: sealed.iv.slice(),
              additionalData: new TextEncoder().encode(
                JSON.stringify(['sytue-local-encryption-v1', 'person', 'records', 'a']),
              ),
              tagLength: 128,
            },
            key,
            bytes,
          ),
        );
      }
    }
    await expect(target.read(id)).rejects.toThrow('encrypted_offline_unavailable');
    if (kind === 'missing-key') {
      await expect(target.write('new', bytes)).rejects.toThrow('encrypted_offline_unavailable');
      expect(
        [...f.records.values()].some((value) => value.namespace === 'local-crypto-key-v1'),
      ).toBe(false);
    }
    expect(
      [...f.records.values()].some((value) => value.namespace === 'local-crypto-value-v1'),
    ).toBe(true);
  },
);

test('concurrent encrypted writers keep the same root and preserve caller bytes before awaiting storage', async () => {
  const f = memory(),
    a = f.create(),
    b = f.create();
  const value = bytes.slice(),
    original = value.slice();
  const pending = a.write('a', value);
  value.fill(0);
  await Promise.all([pending, b.write('b', bytes)]);
  expect(await a.read('a')).toEqual(original);
  expect(await a.read('b')).toEqual(bytes);
  expect(
    [...f.records.values()].filter((row) => row.namespace === 'local-crypto-key-v1'),
  ).toHaveLength(1);
});

test('logout epoch invalidates old instances permanently, including a write inside the atomic mutation', async () => {
  const f = memory(),
    old = f.create();
  const mutate = f.port.mutate;
  f.port.mutate = async (owner, namespace, id, update) => {
    if (namespace === 'local-crypto-value-v1') f.invalidate();
    return mutate(owner, namespace, id, update);
  };
  await expect(old.write('a', bytes)).rejects.toThrow('encrypted_offline_unavailable');
  expect([...f.records.values()].some((row) => row.namespace === 'local-crypto-value-v1')).toBe(
    false,
  );
  await expect(old.read('a')).rejects.toThrow('encrypted_offline_unavailable');
  f.port.mutate = mutate;
  await f.create().write('fresh', bytes);
  await expect(old.write('late', bytes)).rejects.toThrow('encrypted_offline_unavailable');
});

test('late decrypted plaintext is zeroed when the account closes and erasure removes only the selected scope key first', async () => {
  const f = memory();
  let active = true;
  const own = f.create('person', 'records', () => active),
    other = f.create('person', 'other');
  await own.write('a', bytes);
  await other.write('b', bytes);
  const read = f.port.read;
  f.port.read = async <T>(owner: string, namespace: string, id: string): Promise<T | null> => {
    const value = await read<T>(owner, namespace, id);
    active = false;
    return value;
  };
  await expect(own.read('a')).rejects.toThrow('encrypted_offline_unavailable');
  f.port.read = read;
  const deleted: string[] = [],
    remove = f.port.delete;
  f.port.delete = async (owner, namespace, id) => {
    deleted.push(namespace);
    await remove(owner, namespace, id);
  };
  const eraseScope = f.port.deleteScope;
  f.port.deleteScope = async (owner, namespace) => {
    deleted.push(namespace);
    await eraseScope(owner, namespace);
  };
  await own.destroy();
  expect(deleted).toEqual(['local-crypto-key-v1', 'local-crypto-value-v1']);
  expect(await other.read('b')).toEqual(bytes);
  await expect(own.write('late', bytes)).rejects.toThrow('encrypted_offline_unavailable');
});

test('encrypted journals enumerate opaque IDs without reading values and deletion never crosses a scope', async () => {
  const f = memory(),
    own = f.create(),
    other = f.create('other', 'records');
  await own.write('job-a', bytes);
  await own.write('job-b', bytes);
  await other.write('job-a', bytes);
  const read = f.port.read;
  f.port.read = async () => {
    throw new Error('payload read while enumerating');
  };
  expect((await own.listIDs()).sort()).toEqual(['job-a', 'job-b']);
  expect(await own.listIDs(1, 'job-b')).toEqual(['job-b']);
  await expect(own.listIDs(1)).rejects.toThrow('offline_records_limit');
  f.port.read = read;
  await own.delete('job-a');
  expect(await own.read('job-a')).toBeNull();
  expect(await other.read('job-a')).toEqual(bytes);
  expect(await own.listIDs()).toEqual(['job-b']);
  const remove = f.port.delete;
  f.port.delete = async (owner, namespace, id, check) => {
    f.invalidate();
    return remove(owner, namespace, id, check);
  };
  await expect(own.delete('job-b')).rejects.toThrow('encrypted_offline_unavailable');
  f.port.delete = remove;
  expect(await f.create().read('job-b')).toEqual(bytes);
  await expect(own.listIDs()).rejects.toThrow('encrypted_offline_unavailable');
});

test('a JSON-destroyed empty key descriptor is repaired only when it protects no retained ciphertext', async () => {
  const empty = memory();
  await empty.port.mutate(
    JSON.stringify(['person', 'records']),
    'local-crypto-key-v1',
    'root',
    () => ({ v: 1, key: {} }),
  );
  await empty.create().write('first', bytes);
  expect(await empty.create().read('first')).toEqual(bytes);
  const retained = memory();
  await retained.create().write('old', bytes);
  const root = [...retained.records.values()].find(
    (row) => row.namespace === 'local-crypto-key-v1',
  )!;
  root.value = { v: 1, key: {} };
  await expect(retained.create().write('new', bytes)).rejects.toThrow(
    'encrypted_offline_unavailable',
  );
  expect((root.value as { key: unknown }).key).toEqual({});
  expect(
    [...retained.records.values()].filter((row) => row.namespace === 'local-crypto-value-v1'),
  ).toHaveLength(1);
});

test('encrypted metadata pages preserve exclusive cursors and fence late epochs', async () => {
  const f = memory(),
    own = f.create();
  for (const id of ['a/03', 'a/01', 'a/02', 'b/01']) await own.write(id, bytes);
  await f.create('other').write('a/02', bytes);
  expect(await own.listIDPage(2, 'a/')).toEqual({ ids: ['a/01', 'a/02'], next: 'a/02' });
  expect(await own.listIDPage(2, 'a/', 'a/02')).toEqual({ ids: ['a/03'], next: '' });
  await expect(own.listIDPage(2, 'a/', 'b/01')).rejects.toThrow('encrypted_offline_unavailable');
  await expect(own.listIDPage(10001)).rejects.toThrow('encrypted_offline_unavailable');
  const original = f.port.pageIDs;
  f.port.pageIDs = async (...args) => {
    const page = await original(...args);
    f.invalidate();
    return page;
  };
  await expect(own.listIDPage(2, 'a/')).rejects.toThrow('encrypted_offline_unavailable');
});
