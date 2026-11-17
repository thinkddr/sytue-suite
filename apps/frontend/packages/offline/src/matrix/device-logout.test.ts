/* SPDX-License-Identifier: Apache-2.0 */
import { afterEach, expect, test } from 'bun:test';
import { SuiteMatrix } from './client.js';
import { logoutMatrixDevice } from './device-logout.js';

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

/** A live device of Mail's private storage, as MatrixDeviceGate's sign-out finds it. */
function device(events: string[]) {
  const erase = (part: string) => async () => {
    events.push(`erase ${part}`);
  };
  return Object.assign(Object.create(SuiteMatrix.prototype), {
    closed: false,
    network: { closed: false },
    clientModules: new Map(),
    temporaryKeys: new Map(),
    urls: new Set<string>(),
    release: () => {},
    bootstrap: { homeserver_url: 'https://matrix.sytue.test' },
    session: { user_id: '@ana:sytue.test', device_id: 'MAIL', access_token: 'token-mail-ana' },
    client: { stopClient: () => {}, clearStores: erase('sdk') },
    adapter: { forgetSession: erase('session') },
    ownerKeys: { close: () => {}, destroy: erase('owner-key') },
    uploadCache: { close: () => {}, destroy: erase('uploads') },
    onOfflineClear: () => {},
  }) as SuiteMatrix;
}

// VISTO EN ROJO sin la llamada en `forget()`: el homeserver no recibía nada y el
// dispositivo seguía vivo tras cada salida.
test('sign-out closes this device on the homeserver once, with its own token, before erasing it', async () => {
  const events: string[] = [];
  const requests: { url: string; init: RequestInit | undefined }[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    requests.push({ url: String(input), init });
    events.push('logout');
    return new Response('{}', { status: 200 });
  }) as typeof fetch;
  const instance = device(events);
  await Promise.all([instance.forget(), instance.forget()]);
  expect(requests).toHaveLength(1);
  expect(requests[0]!.url).toBe('https://matrix.sytue.test/_matrix/client/v3/logout');
  expect(requests[0]!.init?.method).toBe('POST');
  expect(new Headers(requests[0]!.init?.headers).get('authorization')).toBe(
    'Bearer token-mail-ana',
  );
  expect(requests[0]!.init?.credentials).toBe('omit');
  expect(events[0]).toBe('logout');
  expect(events.slice(1).sort()).toEqual([
    'erase owner-key',
    'erase sdk',
    'erase session',
    'erase uploads',
  ]);
});

test('sign-out still erases the device when the homeserver is down', async () => {
  const events: string[] = [];
  globalThis.fetch = (async () => {
    events.push('logout');
    throw new TypeError('Failed to fetch');
  }) as unknown as typeof fetch;
  await device(events).forget();
  expect(events).toContain('logout');
  expect(events).toContain('erase session');
  expect(events).toContain('erase sdk');
});

test('a hanging homeserver cannot hold the sign-out beyond its deadline', async () => {
  const started = Date.now();
  const closed = await logoutMatrixDevice('https://matrix.sytue.test', 't', {
    fetch: (() => new Promise<Response>(() => {})) as unknown as typeof fetch,
    timeoutMs: 20,
  });
  expect(closed).toBe(false);
  expect(Date.now() - started).toBeLessThan(1000);
});

test.each([
  [200, '{}', true],
  [401, '{"errcode":"M_UNKNOWN_TOKEN"}', true],
  [401, '{"errcode":"M_UNKNOWN_TOKEN","soft_logout":false}', true],
  [401, '{"errcode":"M_UNKNOWN_TOKEN","soft_logout":true}', false],
  [401, '{"errcode":"M_UNKNOWN_TOKEN","soft_logout":"false"}', false],
  [403, '{"errcode":"M_FORBIDDEN"}', false],
])('HTTP %i %s counts as closed: %p', async (status, body, closed) => {
  const result = await logoutMatrixDevice('https://matrix.sytue.test/', 't', {
    fetch: (async () => new Response(body, { status })) as unknown as typeof fetch,
  });
  expect(result).toBe(closed);
});
