/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { createClient } from 'matrix-js-sdk';
import { calculateKeyCheck } from 'matrix-js-sdk/lib/secret-storage';
import { encodeRecoveryKey } from 'matrix-js-sdk/lib/crypto-api/recovery-key';
import { MatrixOwnerKeyCache } from './owner-key-cache.js';
import { ownerProofAlgorithm } from './owner-proof.js';
function values() {
  let data: Uint8Array<ArrayBuffer> | null = null;
  return {
    read: async () => data?.slice() ?? null,
    write: async (_id: string, value: Uint8Array) => {
      data = value.slice();
    },
    close: () => {},
    delete: async () => {
      data?.fill(0);
      data = null;
    },
    destroy: async () => {
      data?.fill(0);
      data = null;
    },
    data: () => data?.slice() ?? null,
  };
}
async function fixture() {
  const key = crypto.getRandomValues(new Uint8Array(32));
  const description = { algorithm: ownerProofAlgorithm, ...(await calculateKeyCheck(key)) };
  const storage = values(),
    keys = new Map<string, Uint8Array<ArrayBuffer>>();
  const sdk = createClient({ baseUrl: 'https://matrix.test' }).secretStorage;
  let active = true,
    asked = 0;
  const make = (
    read = async () => {
      asked++;
      return encodeRecoveryKey(key)!;
    },
  ) => new MatrixOwnerKeyCache(storage, keys, sdk.checkKey.bind(sdk), read, () => active);
  return {
    key,
    description,
    storage,
    keys,
    make,
    asked: () => asked,
    close: () => {
      active = false;
    },
  };
}
test('private owner keys survive restart without another recovery prompt and validate current Matrix metadata', async () => {
  const f = await fixture(),
    first = f.make();
  expect((await first.get({ owner: f.description }))[1]).toEqual(f.key);
  const retained = f.keys.get('owner')!;
  first.close();
  expect(retained.every((value) => value === 0)).toBe(true);
  const second = f.make(async () => {
    throw new Error('must use encrypted cache');
  });
  expect((await second.get({ owner: f.description }))[1]).toEqual(f.key);
  expect(f.asked()).toBe(1);
  await expect(second.get({ owner: { algorithm: ownerProofAlgorithm } })).rejects.toThrow(
    'matrix_recovery',
  );
  const otherDescription = {
    algorithm: ownerProofAlgorithm,
    ...(await calculateKeyCheck(crypto.getRandomValues(new Uint8Array(32)))),
  };
  await expect(second.get({ owner: otherDescription })).rejects.toThrow('matrix_recovery');
  expect(f.asked()).toBe(1);
  await second.destroy();
  expect(f.storage.data()).toBeNull();
});

test('SDK synchronous cache callbacks copy input and flush persistence before reporting readiness', async () => {
  const f = await fixture(),
    cache = f.make();
  const input = f.key.slice();
  cache.remember('owner', f.description, input);
  input.fill(0);
  await cache.flush();
  const first = await cache.get({ owner: f.description });
  expect(first[1]).toEqual(f.key);
  expect((await cache.get({ owner: f.description }))[1]).toBe(first[1]);
  expect(first[1]).toEqual(f.key); // Reuse never zeroes a buffer the SDK is still using.
  expect(f.asked()).toBe(0);
});

test('implicit key or identifier rotation never overwrites the historical authentication secret', async () => {
  const f = await fixture(),
    cache = f.make();
  await cache.get({ owner: f.description });
  const original = f.storage.data();
  const replacement = crypto.getRandomValues(new Uint8Array(32));
  const description = { algorithm: ownerProofAlgorithm, ...(await calculateKeyCheck(replacement)) };
  cache.remember('new-owner', description, replacement);
  await expect(cache.flush()).rejects.toThrow('matrix_recovery');
  expect(f.storage.data()).toEqual(original);
  const reopened = f.make();
  await expect(reopened.get({ 'new-owner': description })).rejects.toThrow('matrix_recovery');
  expect(f.storage.data()).toEqual(original);
});

test('an unavailable encrypted cache fails closed before prompting or retaining an owner key', async () => {
  const f = await fixture(),
    cache = f.make();
  f.storage.read = async () => {
    throw new Error('artificial disk locked');
  };
  await expect(cache.get({ owner: f.description })).rejects.toThrow('artificial disk locked');
  expect(f.asked()).toBe(0);
  expect(f.keys.size).toBe(0);
});

test('a failed synchronous-callback persistence is surfaced and cannot mark the cache ready', async () => {
  const f = await fixture(),
    cache = f.make();
  f.storage.write = async () => {
    throw new Error('artificial quota');
  };
  cache.remember('owner', f.description, f.key);
  await expect(cache.flush()).rejects.toThrow('artificial quota');
  expect(f.keys.size).toBe(0);
  expect(f.storage.data()).toBeNull();
});

test('logout while UI or a queued SDK callback is pending cannot restore keys', async () => {
  const f = await fixture();
  let supplied!: (value: string) => void, requested!: () => void;
  const seen = new Promise<void>((done) => {
    requested = done;
  });
  const cache = f.make(async () => {
    requested();
    return new Promise<string>((done) => {
      supplied = done;
    });
  });
  const pending = cache.get({ owner: f.description });
  await seen;
  f.close();
  cache.close();
  supplied(encodeRecoveryKey(f.key)!);
  await expect(pending).rejects.toThrow('matrix_device');
  await cache.destroy();
  expect(f.keys.size).toBe(0);
  expect(f.storage.data()).toBeNull();
  const other = await fixture(),
    queued = other.make();
  queued.remember('owner', other.description, other.key);
  queued.close();
  await expect(queued.flush()).rejects.toThrow('matrix_device');
  await queued.destroy();
  expect(other.keys.size).toBe(0);
  expect(other.storage.data()).toBeNull();
});

test('logout waits for a commit already in flight before erasing the cached owner secret', async () => {
  const f = await fixture(),
    cache = f.make();
  let release!: () => void, entered!: () => void;
  const gate = new Promise<void>((done) => {
    release = done;
  });
  const seen = new Promise<void>((done) => {
    entered = done;
  });
  const write = f.storage.write;
  f.storage.write = async (id, value) => {
    entered();
    await gate;
    await write(id, value);
  };
  cache.remember('owner', f.description, f.key);
  await seen;
  const cleanup = cache.destroy();
  release();
  await cleanup;
  expect(f.keys.size).toBe(0);
  expect(f.storage.data()).toBeNull();
});

test('parallel owner-key requests share validation and persistence while each verifies its own metadata', async () => {
  const f = await fixture();
  let release!: () => void,
    asked = 0;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const cache = f.make(async () => {
    asked++;
    await gate;
    return encodeRecoveryKey(f.key)!;
  });
  const first = cache.get({ owner: f.description }),
    second = cache.get({ owner: { ...f.description } }),
    foreign = cache.get({ owner: { ...f.description, mac: 'invalid' } });
  const all = Promise.allSettled([first, second, foreign]);
  await new Promise((resolve) => setTimeout(resolve, 0));
  try {
    expect(asked).toBe(1);
  } finally {
    release();
    await all;
  }
  expect((await first)[1]).toEqual(f.key);
  expect((await second)[1]).toEqual(f.key);
  await expect(foreign).rejects.toThrow('matrix_recovery');
});

test('owner-key requests snapshot Matrix metadata before asynchronous validation', async () => {
  const f = await fixture(),
    cache = f.make(),
    available = { owner: { ...f.description } };
  const pending = cache.get(available);
  available.owner.mac = 'invalid';
  expect((await pending)[1]).toEqual(f.key);
});

test('the key a device already holds is handed out only from its own cache, checked, and never asked for', async () => {
  const f = await fixture(),
    first = f.make();
  // Nothing cached yet: nothing held, and nobody is asked.
  expect(await first.held('owner', f.description)).toBeNull();
  expect(f.asked()).toBe(0);
  await first.get({ owner: f.description });
  first.close();
  const second = f.make(async () => {
    throw new Error('held never asks');
  });
  const held = await second.held('owner', f.description);
  expect(held).toEqual(f.key);
  // A copy: wiping it does not touch the cache.
  held!.fill(0);
  expect(await second.held('owner', f.description)).toEqual(f.key);
  // Another key ID, or a description the cached key does not match: nothing.
  expect(await second.held('other', f.description)).toBeNull();
  const otherDescription = {
    algorithm: ownerProofAlgorithm,
    ...(await calculateKeyCheck(crypto.getRandomValues(new Uint8Array(32)))),
  };
  expect(await second.held('owner', otherDescription)).toBeNull();
  expect(f.asked()).toBe(1);
  await second.destroy();
});

test('a key that never protected anything can be forgotten, so a new one can be kept', async () => {
  // Una cuenta a medias de la v3.20: se configura de nuevo con otra clave, que la caché
  // rechazaría mientras guarde la vieja (rotar exige migrar el historial).
  const f = await fixture(),
    first = f.make();
  expect(await first.cachedID()).toBeNull();
  await first.get({ owner: f.description });
  expect(await first.cachedID()).toBe('owner');
  const otherKey = crypto.getRandomValues(new Uint8Array(32));
  const otherDescription = {
    algorithm: ownerProofAlgorithm,
    ...(await calculateKeyCheck(otherKey)),
  };
  first.remember('other', otherDescription, otherKey);
  await expect(first.flush()).rejects.toThrow('matrix_recovery');
  await first.discard();
  expect(f.storage.data()).toBeNull();
  expect(f.keys.size).toBe(0);
  expect(await first.cachedID()).toBeNull();
  first.remember('other', otherDescription, otherKey);
  await first.flush();
  expect(await first.cachedID()).toBe('other');
  expect(await first.held('other', otherDescription)).toEqual(otherKey);
  await first.destroy();
});
