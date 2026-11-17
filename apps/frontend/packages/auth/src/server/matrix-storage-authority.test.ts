/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import type { Bff } from './bff.js';
import {
  rutaAutoridadMatrixPrivada,
  rutaPublicacionesMatrixPrivadas,
} from './matrix-storage-authority.js';
import { sessionOwnerHeader, sessionChangedHeader } from '@sytue/config/session-owner';

const scope = { tenant: 'acme', subject: 'alice', product: 'mail' } as const;
function fixture(
  patch: {
    area?: 'authority' | 'publications';
    sessionChanged?: boolean;
    encoding?: string;
    path?: string;
    method?: string;
    header?: string;
    session?: boolean;
    origin?: string;
    query?: string;
    body?: string;
    length?: string;
  } = {},
) {
  const calls: Request[] = [];
  const bff = {
    sessionGuard: async () =>
      patch.sessionChanged
        ? new Response(null, { status: 409, headers: { [sessionChangedHeader]: '1' } })
        : null,
    getSession: async () =>
      patch.session === false
        ? null
        : {
            user: { id: scope.subject, tenant: scope.tenant },
            tokens: { accessToken: 'dummy-token' },
          },
  } as unknown as Bff;
  const area = patch.area ?? 'authority';
  const route = (
    area === 'authority' ? rutaAutoridadMatrixPrivada : rutaPublicacionesMatrixPrivadas
  )({
    bff,
    product: 'mail',
    env: { SYTUE_ROOMS_INTERNAL_URL: 'https://internal.test' },
    fetchImpl: (async (input, init) => {
      calls.push(new Request(input, init));
      return Response.json({ generation: 1 });
    }) as typeof fetch,
  });
  const method = patch.method ?? 'GET',
    body = patch.body;
  const request = new Request(
    'https://mail.test/mail/api/matrix/storage/' +
      area +
      (patch.path ? '/' + patch.path : '') +
      (patch.query ?? ''),
    {
      method,
      headers: {
        origin: patch.origin ?? 'https://mail.test',
        [storageScopeHeader]: patch.header ?? encodeStorageScopeHeader(scope),
        authorization: 'Bearer dummy-injected',
        ...(body !== undefined
          ? {
              'content-type': 'application/json',
              'content-length': patch.length ?? String(new TextEncoder().encode(body).length),
            }
          : {}),
      },
      ...(body !== undefined ? { body } : {}),
    },
  );
  // Astro receives these headers from the server. happy-dom's browser Request
  // strips forbidden headers when another UI test registers it globally.
  const serverHeaders = new Headers({
    origin: patch.origin ?? 'https://mail.test',
    [storageScopeHeader]: patch.header ?? encodeStorageScopeHeader(scope),
    authorization: 'Bearer dummy-injected',
    ...(body !== undefined
      ? {
          'content-type': 'application/json',
          'content-length': patch.length ?? String(new TextEncoder().encode(body).length),
        }
      : {}),
  });
  Object.defineProperty(request, 'headers', { value: serverHeaders });
  if (patch.encoding) serverHeaders.set('content-encoding', patch.encoding);
  if (patch.sessionChanged) serverHeaders.set(sessionOwnerHeader, 'bound-generation');
  const context = {
    request,
    url: new URL(request.url),
    params: { ruta: patch.path ?? '' },
    cookies: { get: () => undefined, set: () => {}, delete: () => {} },
  } as unknown as APIContext;
  return { calls, route, context };
}

test('storage authority proxy fixes the product and forwards only its exact BFF session and scope', async () => {
  for (const patch of [
    {},
    { method: 'POST', path: 'challenge', body: '{"epoch":"synthetic"}' },
    { method: 'POST', path: 'confirm', body: '{"v":1}' },
  ]) {
    const f = fixture(patch),
      response = await f.route(f.context);
    if (response.status !== 200)
      throw new Error(
        'control case ' +
          JSON.stringify(patch) +
          ' status ' +
          response.status +
          ' ' +
          (await response.text()),
      );
    expect(response.status).toBe(200);
    expect(response.headers.get('cache-control')).toBe('no-store');
    expect(response.headers.get('vary')).toBe('Cookie');
    const request = f.calls[0]!;
    expect(new URL(request.url).pathname).toBe(
      '/api/v1/matrix/storage/mail/authority' + (patch.path ? '/' + patch.path : ''),
    );
    expect(request.headers.get('authorization')).toBe('Bearer dummy-token');
    expect(request.headers.get(storageScopeHeader)).toBe(encodeStorageScopeHeader(scope));
  }
});

test('storage authority proxy rejects cross-account, cross-site and malformed control routes before upstream IO', async () => {
  for (const patch of [
    { session: false },
    { header: encodeStorageScopeHeader({ ...scope, subject: 'bob' }) },
    { header: encodeStorageScopeHeader({ ...scope, product: 'drive' }) },
    { origin: 'https://foreign.test' },
    { path: 'confirm', method: 'GET' },
    { path: 'other' },
    { query: '?product=drive' },
    { path: 'challenge', method: 'POST', body: '{}', length: '65537' },
  ]) {
    const f = fixture(patch),
      response = await f.route(f.context);
    if (response.status < 400)
      throw new Error('accepted forbidden control case ' + JSON.stringify(patch));
    expect(response.status).toBeGreaterThanOrEqual(400);
    expect(f.calls).toHaveLength(0);
  }
});

test('private publication routes keep signed body and ACK query at the same product boundary', async () => {
  const area = 'publications' as const;
  for (const patch of [
    {
      method: 'POST',
      path: 'reserve',
      body: '{"v":1,"authority_generation":"9223372036854775807","signatures":{}}',
    },
    { method: 'POST', path: 'cancel', body: '{"v":1}' },
    { query: '?room_id=%21private%3Amatrix.test&transaction_id=original.1' },
    { path: 'recovery', query: '?room_id=%21private%3Amatrix.test&transaction_id=original.1' },
    { query: '?room_id=%21' + 'A'.repeat(43) + '&transaction_id=opaque.1' },
  ]) {
    const f = fixture({ area, ...patch });
    const response = await f.route(f.context);
    expect(response.status).toBe(200);
    expect(response.headers.get('cache-control')).toBe('no-store');
    expect(response.headers.get('vary')).toBe('Cookie');
    expect(f.calls).toHaveLength(1);
    const request = f.calls[0]!;
    expect(new URL(request.url).pathname).toBe(
      '/api/v1/matrix/storage/mail/publications' + ('path' in patch ? '/' + patch.path : ''),
    );
    expect(request.headers.get('authorization')).toBe('Bearer dummy-token');
    expect(request.headers.get(storageScopeHeader)).toBe(encodeStorageScopeHeader(scope));
    if ('body' in patch) expect(await request.text()).toBe(patch.body);
    else {
      expect(new URL(request.url).searchParams.get('room_id')).toBe(
        new URLSearchParams(patch.query).get('room_id'),
      );
      expect(new URL(request.url).searchParams.get('transaction_id')).toBe(
        new URLSearchParams(patch.query).get('transaction_id'),
      );
    }
  }
});

test('private publication routes reject mixed scope, generation, writes, queries and compression before IO', async () => {
  const area = 'publications' as const;
  for (const patch of [
    { session: false, query: '?room_id=%21r%3Atest&transaction_id=t' },
    { sessionChanged: true, query: '?room_id=%21r%3Atest&transaction_id=t' },
    {
      header: encodeStorageScopeHeader({ ...scope, subject: 'bob' }),
      query: '?room_id=%21r%3Atest&transaction_id=t',
    },
    {
      header: encodeStorageScopeHeader({ ...scope, product: 'drive' }),
      method: 'POST',
      path: 'reserve',
      body: '{}',
    },
    { origin: 'https://foreign.test', method: 'POST', path: 'reserve', body: '{}' },
    { path: 'reserve' },
    { path: 'recovery', method: 'POST', body: '{}' },
    { path: 'recovery', query: '?room_id=%21r%3Atest&transaction_id=t&transaction_id=other' },
    { path: 'recovery', query: '?room_id=invalid&transaction_id=t' },
    { path: 'other', method: 'POST', body: '{}' },
    { query: '?room_id=%21r%3Atest&transaction_id=t&transaction_id=other' },
    { query: '?room_id=invalid&transaction_id=t' },
    { query: '?room_id=%21r%3Atest&transaction_id=t&url=https%3A%2F%2Fevil.test' },
    { query: '?room_id=%21r%3Atest&transaction_id=bad%0Aintent' },
    { method: 'POST', path: 'reserve', body: '{}', length: '16385' },
    { method: 'POST', path: 'reserve', body: '{}', encoding: 'gzip' },
    { method: 'POST', path: 'cancel', body: '{}', query: '?room_id=%21r%3Atest' },
  ]) {
    const f = fixture({ area, ...patch });
    const response = await f.route(f.context);
    expect(response.status).toBeGreaterThanOrEqual(400);
    expect(f.calls).toHaveLength(0);
    if ('sessionChanged' in patch) {
      expect(response.status).toBe(409);
      expect(response.headers.get(sessionChangedHeader)).toBe('1');
    }
  }
});
