/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { encodeRecoveryKey } from 'matrix-js-sdk/lib/crypto-api';
import { escrowRecoveryKey, readEscrowedRecoveryKey, recoveryKeyShape } from './recovery.js';

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

test('la forma admitida es la de las claves que genera el SDK de Matrix', () => {
  for (let i = 0; i < 200; i++) {
    const key = encodeRecoveryKey(crypto.getRandomValues(new Uint8Array(32)));
    expect(key).toBeDefined();
    expect(recoveryKeyShape.test(key!)).toBe(true);
  }
  // Ni texto libre ni algo que no sea base58 entran en la custodia.
  expect(recoveryKeyShape.test('hola')).toBe(false);
  expect(recoveryKeyShape.test('EsT0 abcd abcd abcd abcd abcd abcd abcd abcd abcd')).toBe(false);
  expect(recoveryKeyShape.test('')).toBe(false);
});

test('sin clave custodiada (404) o con una respuesta rara, se pide a mano', async () => {
  globalThis.fetch = (async () => new Response('{}', { status: 404 })) as unknown as typeof fetch;
  expect(await readEscrowedRecoveryKey()).toBeNull();
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ recovery_key: 'no es una clave' }))) as unknown as typeof fetch;
  expect(await readEscrowedRecoveryKey()).toBeNull();
  globalThis.fetch = (async () => {
    throw new TypeError('Failed to fetch');
  }) as unknown as typeof fetch;
  expect(await readEscrowedRecoveryKey()).toBeNull();
});

test('la clave custodiada se lee y se guarda contra la ruta del BFF', async () => {
  const key = encodeRecoveryKey(new Uint8Array(32).fill(7))!;
  const calls: { url: string; init?: RequestInit }[] = [];
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    calls.push({ url, ...(init ? { init } : {}) });
    return init?.method === 'POST'
      ? new Response(null, { status: 204 })
      : new Response(JSON.stringify({ recovery_key: key }));
  }) as unknown as typeof fetch;
  expect(await readEscrowedRecoveryKey()).toBe(key);
  expect(await escrowRecoveryKey(key)).toBe(true);
  expect(calls.map((c) => c.url)).toEqual([
    '/rooms/api/matrix/recovery-key',
    '/rooms/api/matrix/recovery-key',
  ]);
  expect(JSON.parse(String(calls[1]!.init?.body))).toEqual({ recovery_key: key });
  globalThis.fetch = (async () => new Response(null, { status: 503 })) as unknown as typeof fetch;
  expect(await escrowRecoveryKey(key)).toBe(false);
});
