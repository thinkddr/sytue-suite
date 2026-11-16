/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import { DriveClient } from '@sytue/api-clients';
import { sessionOwnerHeader } from '@sytue/config/session-owner';
import { createBff, type Bff, type CookieJar } from './bff.js';
import { seal, SESSION_COOKIE } from './cookies.js';
import type { SytueSession } from '../types.js';
import { rutaMigracionMatrixPrivada } from './matrix-storage-migration.js';
import { rutaCorteIdentidadMatrixPrivada } from './mail-private-storage.js';
const scope = { tenant: 'acme', subject: 'alice', product: 'drive' } as const;
const epoch = '12345678-1234-1234-1234-123456789abc';
const id = '87654321-1234-1234-1234-123456789abc';
const snapshot = {
  scope,
  mode: 'frozen',
  epoch,
  started_at: '2026-10-27T12:00:00Z',
  rows: 1,
  blobs: 1,
};
function fixture(
  product: 'drive' | 'contacts' = 'drive',
  responseFor?: (request: Request) => Response,
) {
  const calls: Request[] = [];
  let tenant = scope.tenant as string;
  const activeScope = { ...scope, product };
  const bff = {
    sessionGuard: async ({ request }: { request: Request }) =>
      request.headers.get(sessionOwnerHeader) === 'a'.repeat(64)
        ? null
        : new Response(null, { status: 409 }),
    getSession: async () => ({
      user: { id: scope.subject, tenant },
      tokens: { accessToken: 'synthetic-server-token' },
    }),
  } as unknown as Bff;
  const route = rutaMigracionMatrixPrivada({
    bff,
    env: {
      SYTUE_DRIVE_INTERNAL_URL: 'https://internal.test',
      SYTUE_CONTACTS_INTERNAL_URL: 'https://contacts.internal.test',
    },
    product,
    fetchImpl: (async (url, init) => {
      const request = new Request(url, init);
      calls.push(request);
      if (responseFor) return responseFor(request);
      if (request.url.endsWith('/rows'))
        return Response.json({
          scope,
          epoch,
          rows: [{ kind: 'file', id, payload: btoa('{"integer":900719925474099512345}') }],
        });
      if (request.url.includes('/blobs/'))
        return new Response('original', {
          headers: { 'content-length': '8', 'content-type': 'application/octet-stream' },
        });
      return Response.json(snapshot);
    }) as typeof fetch,
  });
  const send = async (
    path: string,
    method = 'GET',
    extra: Record<string, string> = {},
    body?: string,
  ) => {
    const request = new Request(
      'https://drive.test/drive/api/matrix/storage/migration' + (path ? '/' + path : ''),
      {
        method,
        headers: {
          [storageScopeHeader]: encodeStorageScopeHeader(activeScope),
          ...(product === 'contacts' ? { [sessionOwnerHeader]: 'a'.repeat(64) } : {}),
          authorization: 'Bearer synthetic-browser-injection',
          ...extra,
        },
        ...(body ? { body } : {}),
      },
    );
    const url = new URL(request.url);
    return route({
      request,
      url,
      params: { ruta: path.split('?')[0] },
      cookies: {},
    } as unknown as APIContext);
  };
  return {
    route,
    calls,
    send,
    switchTenant: () => {
      tenant = 'other';
    },
  };
}
test('migration BFF preserves its exact backend root, uses only the server token and streams retained source bytes', async () => {
  const f = fixture();
  const body = JSON.stringify({ epoch });
  for (const response of [
    await f.send(''),
    await f.send(
      '',
      'POST',
      { 'content-type': 'application/json', 'content-length': String(body.length) },
      body,
    ),
    await f.send(epoch + '/rows'),
    await f.send(epoch + '/blobs/file/' + id),
  ]) {
    expect(response.status).toBe(200);
    expect(response.headers.get('cache-control')).toBe('no-store');
    expect(response.headers.get('vary')).toBe('Cookie');
    await response.body?.cancel();
  }
  expect(f.calls).toHaveLength(4);
  expect(f.calls[0]!.url).toBe('https://internal.test/api/v1/private-storage/migration');
  expect(await f.calls[1]!.text()).toBe(body);
  expect(f.calls[2]!.url).toBe(
    'https://internal.test/api/v1/private-storage/migration/' + epoch + '/rows',
  );
  for (const request of f.calls) {
    expect(request.headers.get('authorization')).toBe('Bearer synthetic-server-token');
    expect(request.headers.get('accept-encoding')).toBeNull();
  }
});
test('Contacts source routes require the active owner and never accept Drive paths or other scopes', async () => {
  const f = fixture('contacts');
  const body = JSON.stringify({ epoch });
  for (const response of [
    await f.send(''),
    await f.send(
      '',
      'POST',
      { 'content-type': 'application/json', 'content-length': String(body.length) },
      body,
    ),
    await f.send(epoch + '/rows?after=YWJj'),
  ]) {
    expect(response.status).toBe(200);
    await response.body?.cancel();
  }
  for (const kind of ['book', 'card', 'contact', 'email', 'phone', 'group', 'member']) {
    const response = await f.send(epoch + '/blobs/' + kind + '/' + id);
    expect(response.status).toBe(200);
    await response.body?.cancel();
  }
  expect(f.calls).toHaveLength(10);
  for (const request of f.calls) {
    expect(new URL(request.url).origin).toBe('https://contacts.internal.test');
    expect(request.headers.get(storageScopeHeader)).toBe(
      encodeStorageScopeHeader({ ...scope, product: 'contacts' }),
    );
    expect(request.headers.get('authorization')).toBe('Bearer synthetic-server-token');
    expect(request.headers.get('accept-encoding')).toBe('identity');
    expect(request.redirect).toBe('error');
  }
  const before = f.calls.length;
  for (const [path, method, headers] of [
    [epoch + '/blobs/file/' + id, 'GET', {}],
    [epoch + '/blobs/card/not-a-uuid', 'GET', {}],
    [epoch + '/rows?after=a&after=b', 'GET', {}],
    [epoch + '/rows', 'POST', {}],
    ['', 'DELETE', {}],
    ['', 'GET', { [sessionOwnerHeader]: 'b'.repeat(64) }],
    ['', 'GET', { [storageScopeHeader]: encodeStorageScopeHeader(scope) }],
    [
      '',
      'GET',
      {
        [storageScopeHeader]: encodeStorageScopeHeader({
          ...scope,
          subject: 'bob',
          product: 'contacts',
        }),
      },
    ],
  ] as [string, string, Record<string, string>][]) {
    expect((await f.send(path, method, headers)).status).toBeGreaterThanOrEqual(400);
  }
  f.switchTenant();
  expect((await f.send('')).status).toBe(409);
  expect(f.calls).toHaveLength(before);
});
test('Contacts preserves source bytes and metadata and rejects encoded upstream responses', async () => {
  const digest = '0682c5f2076f099c34cfdd15a9e063849ed437a49677e6fcc5b4198c76575be5';
  const path = epoch + '/blobs/card/' + id;
  const good = fixture(
    'contacts',
    () =>
      new Response('original', {
        headers: {
          'content-type': 'application/octet-stream',
          'content-length': '8',
          'x-sytue-source-sha256': digest,
        },
      }),
  );
  const response = await good.send(path);
  expect(good.calls[0]!.headers.get('accept-encoding')).toBe('identity');
  expect(response.status).toBe(200);
  expect(response.headers.get('content-length')).toBe('8');
  expect(response.headers.get('x-sytue-source-sha256')).toBe(digest);
  expect(await response.text()).toBe('original');

  let cancelled = false;
  const compressed = fixture(
    'contacts',
    () =>
      new Response(
        new ReadableStream({
          cancel() {
            cancelled = true;
          },
        }),
        { headers: { 'content-encoding': 'gzip', 'content-length': '4' } },
      ),
  );
  const failure = await compressed.send(epoch + '/rows');
  expect(compressed.calls[0]!.headers.get('accept-encoding')).toBe('identity');
  expect(failure.status).toBe(503);
  expect(failure.headers.get('cache-control')).toBe('no-store');
  expect(cancelled).toBe(true);
});
test('Contacts source rejects a missing or replaced sealed session generation', async () => {
  const secret = 'contacts-private-route-fixture-only-32bytes';
  const generation = 'a'.repeat(64);
  const session: SytueSession = {
    user: {
      id: 'alice',
      tenant: 'acme',
      email: 'alice@example.test',
      emailVerified: true,
      name: 'Alice',
      preferredUsername: 'alice',
      roles: ['member'],
      groups: [],
    },
    tokens: {
      accessToken: 'fixture-access',
      refreshToken: 'fixture-refresh',
      expiresAt: Date.now() + 3600000,
      tokenType: 'Bearer',
    },
    clientId: 'contacts',
    generation,
  };
  const values = new Map<string, string>([[SESSION_COOKIE, await seal(session, secret)]]);
  const cookies: CookieJar = {
    get: (name) => {
      const value = values.get(name);
      return value === undefined ? undefined : { value };
    },
    set: (name, value) => {
      values.set(name, value);
    },
    delete: (name) => {
      values.delete(name);
    },
  };
  const bff = createBff({
    config: {
      issuer: 'https://identity.fixture.invalid',
      clientId: 'contacts',
      clientSecret: 'fixture-only',
      redirectUri: 'https://contacts.test/contacts/auth/callback',
      postLogoutRedirectUri: 'https://contacts.test/',
    },
    sessionSecret: secret,
    basePath: '/contacts',
    secureCookies: false,
  });
  let calls = 0;
  const route = rutaMigracionMatrixPrivada({
    bff,
    product: 'contacts',
    env: { SYTUE_CONTACTS_INTERNAL_URL: 'https://contacts.internal.test' },
    fetchImpl: (async () => {
      calls++;
      return new Response('ok');
    }) as typeof fetch,
  });
  for (const [header, expected] of [
    [generation, 200],
    ['', 409],
    ['b'.repeat(64), 409],
  ] as const) {
    const headers = new Headers({
      [storageScopeHeader]: encodeStorageScopeHeader({ ...scope, product: 'contacts' }),
    });
    if (header) headers.set(sessionOwnerHeader, header);
    const request = new Request('https://contacts.test/contacts/api/private-storage/migration', {
      headers,
    });
    const response = await route({
      request,
      url: new URL(request.url),
      params: { ruta: '' },
      cookies,
    } as unknown as APIContext);
    expect(response.status).toBe(expected);
  }
  expect(calls).toBe(1);
  const cutover = rutaCorteIdentidadMatrixPrivada({
    bff,
    product: 'contacts',
    env: { SYTUE_ROOMS_INTERNAL_URL: 'https://rooms.internal.test' },
    fetchImpl: (async (input) => {
      expect(String(input)).toBe(
        'https://rooms.internal.test/api/v1/matrix/storage/contacts/migration',
      );
      calls++;
      return new Response('ok');
    }) as typeof fetch,
  });
  for (const [product, expected] of [
    ['contacts', 200],
    ['drive', 409],
  ] as const) {
    const headers = new Headers({
      [storageScopeHeader]: encodeStorageScopeHeader({ ...scope, product }),
      [sessionOwnerHeader]: generation,
    });
    const request = new Request('https://contacts.test/contacts/api/matrix/storage/migration', {
      headers,
    });
    const response = await cutover({
      request,
      url: new URL(request.url),
      params: { ruta: '' },
      cookies,
    } as unknown as APIContext);
    expect(response.status).toBe(expected);
  }
  expect(calls).toBe(2);
});
test('Contacts migration checks the actual POST bytes before contacting the service', async () => {
  const f = fixture('contacts');
  for (const [declared, body] of [
    ['2', '{}more'],
    ['8', '{}'],
    ['128', 'x'.repeat(129)],
    ['2', 'éx'],
  ]) {
    const response = await f.send(
      '',
      'POST',
      { 'content-type': 'application/json', 'content-length': declared! },
      body,
    );
    expect(response.status).toBe(400);
  }
  expect(f.calls).toHaveLength(0);
  const body = JSON.stringify({ epoch });
  const response = await f.send(
    '',
    'POST',
    { 'content-type': 'application/json', 'content-length': String(body.length) },
    body,
  );
  expect(response.status).toBe(200);
  expect(f.calls).toHaveLength(1);
  expect(await f.calls[0]!.text()).toBe(body);
});
test('foreign sessions, extra routes, unsafe methods and unbounded bodies are rejected before upstream access', async () => {
  const f = fixture();
  for (const [path, method, headers, body] of [
    ['elsewhere', 'GET', {}],
    [epoch + '/rows?after=a&after=b', 'GET', {}],
    [epoch + '/rows?limit=100000', 'GET', {}],
    [epoch + '/blobs/file/' + id + '?download=1', 'GET', {}],
    ['', 'DELETE', {}],
    [epoch + '/rows', 'POST', {}],
    ['', 'POST', { 'content-type': 'application/json', 'content-length': '129' }, 'x'],
    ['', 'POST', { 'content-type': 'text/plain', 'content-length': '2' }, '{}'],
    ['', 'GET', { [storageScopeHeader]: encodeStorageScopeHeader({ ...scope, product: 'mail' }) }],
    ['', 'GET', { [storageScopeHeader]: '' }],
    ['', 'GET', { 'sec-fetch-site': 'cross-site' }],
  ] as [string, string, Record<string, string>, string?][]) {
    expect((await f.send(path, method, headers, body)).status).toBeGreaterThanOrEqual(400);
  }
  f.switchTenant();
  expect((await f.send('')).status).toBe(409);
  expect(f.calls).toHaveLength(0);
});
test('cookie SDK walks the mounted BFF without exposing its token or materializing a source blob', async () => {
  const f = fixture();
  const sdk = new DriveClient({
    baseUrl: 'https://drive.test/drive',
    fetchImpl: (async (url, init) => {
      const request = new Request(url, init);
      expect(request.headers.get('authorization')).toBeNull();
      const parsed = new URL(request.url),
        path = parsed.pathname.split('/migration')[1]?.replace(/^\//, '') ?? '';
      // Fetch sets this forbidden header at the network boundary, not in app JS.
      if (request.method === 'POST')
        request.headers.set(
          'content-length',
          String(new TextEncoder().encode(await request.clone().text()).length),
        );
      return f.route({
        request,
        url: parsed,
        params: { ruta: path },
        cookies: {},
      } as unknown as APIContext);
    }) as typeof fetch,
  });
  expect(await sdk.beginPrivateStorageMigration(scope, epoch)).toEqual(snapshot);
  expect((await sdk.privateStorageSourcePage(scope, epoch)).rows).toHaveLength(1);
  expect(await (await sdk.openPrivateStorageSource(scope, epoch, 'file', id, 8)).text()).toBe(
    'original',
  );
  f.switchTenant();
  await expect(sdk.privateStorageState(scope)).rejects.toThrow('HTTP 409');
  expect(f.calls).toHaveLength(3);
});
