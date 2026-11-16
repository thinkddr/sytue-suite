/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import { sessionOwnerHeader, sessionChangedHeader } from '@sytue/config/session-owner';
import { createBff, type CookieJar } from './bff.js';
import { seal, SESSION_COOKIE } from './cookies.js';
import { rutaMailPrivado, rutaCorteIdentidadMatrixPrivada } from './mail-private-storage.js';
import { rutaMetadataMailPrivado } from './mail-private-metadata.js';
import type { SytueSession } from '../types.js';
import { readPrivateStorageBody } from './matrix-storage-scope.js';
const scope = { tenant: 'acme', subject: 'alice', product: 'mail' } as const;
const epoch = '018f3340-7b12-7000-8000-000000000001';
const secret = 'mail-private-route-fixture-only-32bytes',
  generation = 'a'.repeat(64);
async function fixture(
  patch: {
    path?: string;
    method?: string;
    scope?: string;
    generation?: string | null;
    origin?: string;
    body?: string;
    type?: string;
    length?: string | null;
    submission?: string;
    query?: string;
    cutover?: boolean;
    metadata?: boolean;
    duringRead?: (values: Map<string, string>) => void;
    duringUpstream?: (values: Map<string, string>) => void;
  } = {},
) {
  const values = new Map<string, string>();
  const session: SytueSession = {
    user: {
      id: scope.subject,
      tenant: scope.tenant,
      email: 'alice@example.test',
      emailVerified: true,
      name: 'Alice',
      preferredUsername: 'alice',
      roles: ['member'],
      groups: [],
    },
    tokens: {
      accessToken: 'route-fixture-access',
      refreshToken: 'route-fixture-refresh',
      expiresAt: Date.now() + 3600000,
      tokenType: 'Bearer',
    },
    clientId: 'mail',
    generation,
  };
  values.set(SESSION_COOKIE, await seal(session, secret));
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
      clientId: 'mail',
      clientSecret: 'fixture-only',
      redirectUri: 'https://mail.test/mail/auth/callback',
      postLogoutRedirectUri: 'https://mail.test/',
    },
    sessionSecret: secret,
    basePath: '/mail',
    secureCookies: false,
  });
  const calls: { url: string; init: RequestInit | undefined; body: string }[] = [];
  const upstream = async (input: Parameters<typeof fetch>[0], init?: RequestInit) => {
    const body = init?.body ? await new Response(init.body).text() : '';
    calls.push({ url: String(input), init, body });
    patch.duringUpstream?.(values);
    return new Response('exact-response', {
      headers: {
        'content-type': 'application/octet-stream',
        'x-sytue-source-sha256': 'b'.repeat(64),
      },
    });
  };
  const route = patch.cutover
    ? rutaCorteIdentidadMatrixPrivada({
        bff,
        product: 'mail',
        env: { SYTUE_ROOMS_INTERNAL_URL: 'https://rooms.internal' },
        fetchImpl: upstream as typeof fetch,
      })
    : patch.metadata
      ? rutaMetadataMailPrivado({
          bff,
          env: {
            SYTUE_MAIL_INTERNAL_URL: 'https://mail.internal',
            SYTUE_DRIVE_INTERNAL_URL: 'https://drive.internal',
          },
          fetchImpl: upstream as typeof fetch,
        })
      : rutaMailPrivado({
          bff,
          env: { SYTUE_MAIL_INTERNAL_URL: 'https://mail.internal' },
          fetchImpl: upstream as typeof fetch,
        });
  const path = patch.path ?? 'migration',
    method = patch.method ?? 'GET',
    body = patch.body;
  const headers = new Headers({
    origin: patch.origin ?? 'https://mail.test',
    [storageScopeHeader]: patch.scope ?? encodeStorageScopeHeader(scope),
    authorization: 'Bearer injected-worker-value',
  });
  if (patch.generation !== null) headers.set(sessionOwnerHeader, patch.generation ?? generation);
  if (body !== undefined) {
    headers.set('content-type', patch.type ?? 'application/json');
    if (patch.length !== null)
      headers.set('content-length', patch.length ?? String(new TextEncoder().encode(body).length));
  }
  if (patch.submission !== undefined) headers.set('x-sytue-mail-submission', patch.submission);
  const request = new Request(
    'https://mail.test/mail/api/' +
      (patch.cutover
        ? 'matrix/storage/migration/'
        : patch.metadata
          ? 'private-metadata/'
          : 'private-storage/') +
      path +
      (patch.query ?? ''),
    {
      method,
      headers,
      ...(body === undefined ? {} : { body }),
    },
  );
  Object.defineProperty(request, 'headers', { value: headers });
  if (patch.duringRead)
    Object.defineProperty(request, 'body', {
      value: new ReadableStream<Uint8Array>({
        pull(controller) {
          patch.duringRead?.(values);
          controller.enqueue(new TextEncoder().encode(body ?? ''));
          controller.close();
        },
      }),
    });
  const context = {
    request,
    url: new URL(request.url),
    params: { ruta: patch.cutover ? (patch.path ?? '') : path },
    cookies,
  } as unknown as APIContext;
  return { calls, route, context, values };
}
test('Mail source/control/submission routes stream exact bytes with real sealed owner and fixed destinations', async () => {
  for (const patch of [
    {},
    { method: 'POST', body: '{"epoch":"' + epoch + '","provision":true}' },
    { path: 'migration/' + epoch + '/rows', query: '?after=YWJj' },
    { path: 'migration/' + epoch + '/source/mime_message/message' },
    { path: 'ingress/enrollment', method: 'POST', body: '{"epoch":"' + epoch + '"}' },
    { path: 'ingress/body', method: 'POST', body: '{"proof":"public transcript"}' },
    { path: 'submissions/register', method: 'POST', body: '{"registration":"public receipt"}' },
    { path: 'submissions/control', method: 'POST', body: '{"control":"public receipt"}' },
    { path: 'submissions/' + epoch + '/' + epoch },
    {
      path: 'submissions',
      method: 'POST',
      body: 'From: alice@example.test\r\n\r\ntransient MIME',
      type: 'message/rfc822',
      length: null,
      submission: 'cHVibGljLXRyYW5zY3JpcHQ',
    },
  ]) {
    const f = await fixture(patch),
      response = await f.route(f.context);
    expect(response.status).toBe(200);
    expect(await response.text()).toBe('exact-response');
    expect(response.headers.get('x-sytue-source-sha256')).toBe('b'.repeat(64));
    expect(response.headers.get('cache-control')).toBe('no-store');
    expect(f.calls).toHaveLength(1);
    const request = f.calls[0]!;
    expect(new URL(request.url).origin).toBe('https://mail.internal');
    expect(new URL(request.url).pathname).toBe(
      '/api/v1/private-storage/' + (patch.path ?? 'migration'),
    );
    expect(new Headers(request.init?.headers).get('authorization')).toBe(
      'Bearer route-fixture-access',
    );
    expect(new Headers(request.init?.headers).get(storageScopeHeader)).toBe(
      encodeStorageScopeHeader(scope),
    );
    expect(request.init?.redirect).toBe('error');
    expect(request.body).toBe(patch.body ?? '');
    expect(new Headers(request.init?.headers).has(sessionOwnerHeader)).toBe(false);
  }
});
test('Mail routes deny changed generation, account, cross-site and malformed transport before any upstream IO', async () => {
  for (const patch of [
    { generation: 'b'.repeat(64) },
    { generation: null },
    { scope: encodeStorageScopeHeader({ ...scope, subject: 'bob' }) },
    { scope: encodeStorageScopeHeader({ ...scope, product: 'drive' }) },
    { origin: 'https://outside.test' },
    { path: 'accounts' },
    { path: 'migration/../../accounts' },
    { path: 'migration/' + epoch + '/rows', query: '?after=YWJj&extra=1' },
    { path: 'ingress/body', method: 'GET' },
    { path: 'ingress/enrollment', method: 'GET' },
    { path: 'ingress/enrollment/extra', method: 'POST', body: '{}' },
    { method: 'POST', body: '{}', length: '257' },
    { path: 'submissions/register', method: 'POST', body: '{}', length: '65537' },
    {
      path: 'submissions',
      method: 'POST',
      body: 'raw',
      type: 'message/rfc822',
      submission: 'bad header',
    },
    {
      path: 'submissions',
      method: 'POST',
      body: 'raw',
      type: 'application/json',
      submission: 'cHVibGlj',
    },
  ]) {
    const f = await fixture(patch),
      response = await f.route(f.context);
    if (response.status < 400) throw new Error('accepted transport case ' + JSON.stringify(patch));
    expect(response.status).toBeGreaterThanOrEqual(400);
    expect(f.calls).toHaveLength(0);
    if (patch.generation === 'b'.repeat(64))
      expect(response.headers.get(sessionChangedHeader)).toBe('1');
  }
});
test('Matrix identity cutover uses its own fixed routes and keeps epochs distinct from SQL migration', async () => {
  for (const patch of [
    { cutover: true, path: '', method: 'GET' },
    { cutover: true, path: '', method: 'POST', body: '{"epoch":"' + epoch + '"}' },
    {
      cutover: true,
      path: 'complete',
      method: 'POST',
      body: '{"epoch":"' + epoch + '","receipt":{"room_id":"!room:test","event_id":"$event"}}',
    },
  ]) {
    const f = await fixture(patch),
      response = await f.route(f.context);
    expect(response.status).toBe(200);
    expect(f.calls).toHaveLength(1);
    expect(f.calls[0]!.url).toBe(
      'https://rooms.internal/api/v1/matrix/storage/mail/migration' +
        (patch.path ? '/' + patch.path : ''),
    );
    expect(f.calls[0]!.init?.redirect).toBe('error');
  }
  for (const patch of [
    { cutover: true, path: 'authority' },
    { cutover: true, path: 'complete', method: 'GET' },
    { cutover: true, path: '', method: 'POST', body: '{}', generation: 'b'.repeat(64) },
  ]) {
    const f = await fixture(patch),
      response = await f.route(f.context);
    expect(response.status).toBeGreaterThanOrEqual(400);
    expect(f.calls).toHaveLength(0);
  }
});

test('Private Mail metadata keeps real sealed owner and exact public-only destinations', async () => {
  for (const patch of [
    { path: 'aliases' },
    { path: 'aliases', method: 'POST', body: '{"address":"alias@example.test"}' },
    { path: 'aliases/' + epoch, method: 'DELETE' },
    { path: 'quota' },
    { path: 'usage' },
  ]) {
    const f = await fixture({ ...patch, metadata: true }),
      response = await f.route(f.context);
    expect(response.status).toBe(200);
    expect(f.calls).toHaveLength(1);
    const call = f.calls[0]!;
    expect(call.url).toBe(
      patch.path === 'usage'
        ? 'https://drive.internal/api/v1/cipher/usage/mail'
        : 'https://mail.internal/api/v1/' + patch.path,
    );
    expect(call.init?.redirect).toBe('error');
    expect(new Headers(call.init?.headers).get(storageScopeHeader)).toBe(
      encodeStorageScopeHeader(scope),
    );
    expect(new Headers(call.init?.headers).get('authorization')).toBe(
      'Bearer route-fixture-access',
    );
    expect(response.headers.get('cache-control')).toBe('no-store');
  }
  for (const patch of [
    { path: 'messages' },
    { path: 'aliases/../../messages', method: 'DELETE' },
    { path: 'aliases/bad', method: 'DELETE' },
    { path: 'aliases/' + epoch, method: 'POST', body: '{}' },
    { path: 'aliases', method: 'POST', body: '{}', length: '2049' },
    { path: 'usage', method: 'POST', body: '{}' },
    { path: 'quota', query: '?owner=bob' },
    { path: 'quota', generation: 'b'.repeat(64) },
    { path: 'usage', scope: encodeStorageScopeHeader({ ...scope, subject: 'bob' }) },
    { path: 'aliases', generation: null },
  ]) {
    const f = await fixture({ ...patch, metadata: true }),
      response = await f.route(f.context);
    expect(response.status).toBeGreaterThanOrEqual(400);
    expect(f.calls).toHaveLength(0);
  }
});

test('Mail counts actual JSON bytes and refuses short Content-Length before upstream', async () => {
  for (const patch of [
    { path: 'migration', method: 'POST', body: '{}x', length: '2' },
    { path: 'ingress/body', method: 'POST', body: 'x'.repeat((256 << 10) + 1), length: '2' },
    { path: 'submissions/control', method: 'POST', body: 'x'.repeat(4097), length: '2' },
    {
      cutover: true,
      path: 'complete',
      method: 'POST',
      body: 'x'.repeat((64 << 10) + 1),
      length: '2',
    },
  ]) {
    const f = await fixture(patch);
    const result = await f.route(f.context);
    expect(result.status).toBe(400);
    expect(f.calls).toHaveLength(0);
  }
  const valid = await fixture({
    path: 'submissions/control',
    method: 'POST',
    body: '{"control":"original public transcript"}',
  });
  expect((await valid.route(valid.context)).status).toBe(200);
  expect(valid.calls[0]?.body).toBe('{"control":"original public transcript"}');
  const retained = valid.calls[0]?.init?.body;
  expect(retained instanceof Uint8Array).toBe(true);
  expect((retained as Uint8Array).every((value) => value === 0)).toBe(true);
});

test('Mail cannot forward a buffered control or reply after the captured BFF session retires', async () => {
  const duringRead = await fixture({
    path: 'submissions/control',
    method: 'POST',
    body: '{}',
    duringRead: (values) => values.delete(SESSION_COOKIE),
  });
  expect((await duringRead.route(duringRead.context)).status).not.toBe(200);
  expect(duringRead.calls).toHaveLength(0);
  const duringUpstream = await fixture({
    path: 'submissions/control',
    method: 'POST',
    body: '{}',
    duringUpstream: (values) => values.delete(SESSION_COOKIE),
  });
  expect((await duringUpstream.route(duringUpstream.context)).status).not.toBe(200);
  expect(duringUpstream.calls).toHaveLength(1);
});

test.each(['success', 'limit', 'read-error', 'cancel'] as const)(
  'Mail erases original JSON fragments after %s',
  async (mode) => {
    const first = new TextEncoder().encode('{"proof":'),
      second = new TextEncoder().encode('"private"}');
    const expected = new TextDecoder().decode(first) + new TextDecoder().decode(second),
      delivered: Uint8Array[] = [];
    const abort = new AbortController();
    let index = 0;
    const body = new ReadableStream<Uint8Array>(
      {
        pull(controller) {
          if (index++ === 0) {
            delivered.push(first);
            controller.enqueue(first);
            return;
          }
          if (mode === 'read-error') {
            controller.error(new Error('fixture reader failed'));
            return;
          }
          delivered.push(second);
          controller.enqueue(second);
          if (mode === 'cancel') abort.abort();
          else controller.close();
        },
      },
      { highWaterMark: 0 },
    );
    const request = new Request('https://mail.test/api/private-storage/submissions/control', {
      method: 'POST',
      body,
      signal: abort.signal,
      headers: {
        'content-type': 'application/json',
        'content-length': String(first.length + second.length),
      },
      duplex: 'half',
    } as RequestInit);
    const result = await readPrivateStorageBody(request, mode === 'limit' ? first.length : 256);
    if (mode === 'success') {
      expect(result instanceof Uint8Array).toBe(true);
      if (!(result instanceof Uint8Array)) throw new Error('expected owned buffered body');
      expect(new TextDecoder().decode(result)).toBe(expected);
      result.fill(0);
    } else {
      expect(result instanceof Response).toBe(true);
      if (result instanceof Response) expect(result.status).toBe(400);
    }
    expect(delivered.length).toBe(mode === 'read-error' ? 1 : 2);
    for (const fragment of delivered) expect(fragment.every((value) => value === 0)).toBe(true);
  },
);
