/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import type { Bff } from './bff.js';
import { rutaBootstrapMatrixPrivado } from './matrix-storage.js';
const actor = '11111111-1111-4111-8111-111111111111';
function fixture() {
  const data = {
    actor,
    tenant: 'acme',
    homeserver_url: 'https://matrix.test',
    sso_provider_id: 'sytue',
    matrix_user_id: `@sytue_${actor}:matrix.test`,
    controller_user_id: '@rooms_controller:matrix.test',
    encryption: 'm.megolm.v1.aes-sha2',
    channels: [],
    storage_product: 'drive',
    storage_identity_version: 1,
    storage_room_id: '!private:matrix.test',
    storage_room_ids: ['!private:matrix.test'],
  };
  const state = {
    session: {
      user: { id: actor, tenant: 'acme' },
      tokens: { accessToken: 'artificial-sytue-oidc-token' },
    } as { user: { id: string; tenant?: string }; tokens: { accessToken: string } } | null,
    data: { ...data } as Record<string, unknown>,
    status: 200,
    requests: [] as Request[],
  };
  const bff = { getSession: async () => state.session } as unknown as Bff;
  const fetchImpl = (async (input, init) => {
    state.requests.push(new Request(input, init));
    return Response.json(state.data, { status: state.status });
  }) as typeof fetch;
  const route = rutaBootstrapMatrixPrivado({
    bff,
    product: 'drive',
    env: { SYTUE_ROOMS_INTERNAL_URL: 'https://rooms.sytue.invalid' },
    fetchImpl,
  });
  const request = new Request(
    'https://suite.test/drive/api/matrix/storage/bootstrap?product=mail&actor=another',
    {
      headers: {
        Authorization: 'Bearer artificial-browser-injection',
        'X-Sytue-Tenant': 'foreign',
      },
    },
  );
  const context = {
    request,
    url: new URL(request.url),
    cookies: { get: () => undefined, set: () => {}, delete: () => {} },
  } as unknown as APIContext;
  return { state, route, context, data };
}

test('private bootstrap uses only its own BFF identity and a product fixed by the app', async () => {
  const f = fixture();
  const response = await f.route(f.context);
  expect(response.status).toBe(200);
  expect(response.headers.get('cache-control')).toBe('no-store');
  const request = f.state.requests[0]!;
  expect(new URL(request.url).pathname).toBe('/api/v1/matrix/storage/bootstrap');
  expect(new URL(request.url).searchParams.get('product')).toBe('drive');
  expect(request.headers.get('authorization')).toBe('Bearer artificial-sytue-oidc-token');
  expect(request.headers.get('x-sytue-tenant')).toBeNull();
  expect(await response.json()).toEqual(f.data);
});

test('bootstrap omits unexpected upstream credentials and requires a session before making a request', async () => {
  const f = fixture();
  f.state.data.access_token = 'artificial-should-never-escape';
  f.state.data.recovery_key = 'artificial-private';
  expect(await (await f.route(f.context)).json()).toEqual(f.data);
  f.state.requests.length = 0;
  f.state.session = null;
  expect((await f.route(f.context)).status).toBe(401);
  expect(f.state.requests).toHaveLength(0);
});

test('foreign scope and mixed Rooms authority cannot become private app bootstrap metadata', async () => {
  for (const patch of [
    { actor: 'another' },
    { tenant: 'foreign' },
    { storage_product: 'mail' },
    { channels: [{ channel_id: 'channel', matrix_room_id: '!channel:matrix.test' }] },
    { draft_room_id: '!draft:matrix.test' },
    { storage_room_ids: [] },
    { storage_room_ids: ['!prior:matrix.test'] },
  ]) {
    const f = fixture();
    Object.assign(f.state.data, patch);
    const response = await f.route(f.context);
    expect(response.status).toBe(502);
    expect(await response.json()).toEqual({ error: 'matrix_identity' });
  }
});

test('upstream failures use the shared gateway status without returning their body or credentials', async () => {
  const f = fixture();
  f.state.status = 503;
  f.state.data = { error: 'UPSTREAM_PRIVATE_DETAIL', access_token: 'artificial-private-token' };
  const response = await f.route(f.context);
  expect(response.status).toBe(502);
  const text = await response.text();
  expect(text).toBe('{"error":"matrix_unavailable"}');
  expect(text).not.toContain('UPSTREAM_PRIVATE_DETAIL');
});

test('bootstrap selects only explicit supported identity versions and preserves the authenticated version DTO', async () => {
  const f = fixture();
  f.context.request = new Request(
    'https://suite.test/drive/api/matrix/storage/bootstrap?identity_version=2',
  );
  f.state.data.storage_identity_version = 2;
  f.state.data.matrix_user_id = `@sytue_storage_drive_acme_${actor}:matrix.test`;
  const response = await f.route(f.context);
  expect(response.status).toBe(200);
  expect(((await response.json()) as Record<string, unknown>).storage_identity_version).toBe(2);
  expect(new URL(f.state.requests[0]!.url).searchParams.getAll('identity_version')).toEqual(['2']);
  for (const suffix of ['0', '3', 'two', '2&identity_version=1', '']) {
    const invalid = fixture();
    invalid.context.request = new Request(
      'https://suite.test/drive/api/matrix/storage/bootstrap?identity_version=' + suffix,
    );
    expect((await invalid.route(invalid.context)).status).toBe(400);
    expect(invalid.state.requests).toHaveLength(0);
  }
  for (const version of [undefined, 0, 1, '2', false]) {
    const wrong = fixture();
    wrong.context.request = new Request(
      'https://suite.test/drive/api/matrix/storage/bootstrap?identity_version=2',
    );
    wrong.state.data.storage_identity_version = version;
    expect((await wrong.route(wrong.context)).status).toBe(502);
  }
});

test('sin el complemento de cifrado E2E responde «desactivado» sin llamar a Rooms, y solo con sesión', async () => {
  const requests: Request[] = [];
  let session: unknown = { user: { id: actor, tenant: 'acme' }, tokens: { accessToken: 'token' } };
  const route = rutaBootstrapMatrixPrivado({
    bff: { getSession: async () => session } as unknown as Bff,
    product: 'notes',
    env: {
      SYTUE_ROOMS_INTERNAL_URL: 'https://rooms.sytue.invalid',
      SYTUE_CIFRADO_E2E: 'desactivado',
    },
    fetchImpl: (async (input, init) => {
      requests.push(new Request(input, init));
      return Response.json({});
    }) as typeof fetch,
  });
  const contexto = () => {
    const request = new Request('https://suite.test/notes/api/matrix/storage/bootstrap');
    return {
      request,
      url: new URL(request.url),
      cookies: { get: () => undefined, set: () => {}, delete: () => {} },
    } as unknown as APIContext;
  };
  const res = await route(contexto());
  expect(res.status).toBe(200);
  expect(await res.json()).toEqual({ cifrado: 'desactivado' });
  expect(requests).toHaveLength(0);
  session = null;
  expect((await route(contexto())).status).toBe(401);
});
