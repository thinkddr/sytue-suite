/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import type { Bff } from './bff.js';
import { rutasRecuperacionMatrix } from './matrix-recovery.js';

const actor = '11111111-1111-4111-8111-111111111111';
const wrapper = {
  v: 1,
  kind: 'password',
  key_id: 'KEY1',
  credential_id: '',
  kdf: { alg: 'argon2id', m: 65536, t: 3, p: 1, salt: 'EBESExQVFhcYGRobHB0eHw==' },
  nonce: 'cHFyc3R1dnd4eXp7',
  ciphertext: 'GWRz0ee1TKLgjzg3',
};

function fixture() {
  const state = {
    session: {
      user: { id: actor, tenant: 'acme' },
      tokens: { accessToken: 'artificial-sytue-oidc-token' },
    } as { user: { id: string; tenant?: string }; tokens: { accessToken: string } } | null,
    requests: [] as Request[],
    upstream: { wrappers: [{ ...wrapper, id: 'w1', extra: 'dropped' }], legacy_escrow: true },
  };
  const bff = { getSession: async () => state.session } as unknown as Bff;
  const fetchImpl = (async (input, init) => {
    const request = new Request(input, init);
    state.requests.push(request);
    if (request.url.includes('/recovery-key'))
      return Response.json({
        recovery_key: 'EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP',
      });
    if (request.method === 'PUT') return Response.json({ id: 'w2' });
    return Response.json(state.upstream);
  }) as typeof fetch;
  const routes = rutasRecuperacionMatrix({
    bff,
    product: 'drive',
    env: { SYTUE_ROOMS_INTERNAL_URL: 'https://rooms.sytue.invalid' },
    fetchImpl,
  });
  const call = (route: (ctx: APIContext) => unknown, url: string, init?: RequestInit) =>
    route({
      request: new Request(url, init),
      cookies: {},
      url: new URL(url),
    } as unknown as APIContext) as Promise<Response>;
  return { state, routes, call };
}

test('without identity_version it is the Rooms account, and the retired copy is reported', async () => {
  const f = fixture();
  const response = await f.call(
    f.routes.wrappers.GET,
    'https://drive.sytue.invalid/drive/api/matrix/recovery-wrappers',
  );
  expect(response.status).toBe(200);
  const body = (await response.json()) as { wrappers: unknown[]; legacy_escrow: boolean };
  expect(body.legacy_escrow).toBe(true);
  expect(body.wrappers).toEqual([{ ...wrapper, id: 'w1' }]);
  const upstream = new URL(f.state.requests[0]!.url);
  expect(upstream.pathname).toBe('/api/v1/matrix/recovery-wrappers');
  expect(upstream.search).toBe('');
  expect(f.state.requests[0]!.headers.get('authorization')).toBe(
    'Bearer artificial-sytue-oidc-token',
  );
});

test('identity_version=2 is the app’s isolated account; the product is never the browser’s', async () => {
  const f = fixture();
  const response = await f.call(
    f.routes.wrappers.GET,
    'https://drive.sytue.invalid/drive/api/matrix/recovery-wrappers?identity_version=2&product=mail',
  );
  expect(response.status).toBe(200);
  expect(((await response.json()) as { legacy_escrow: boolean }).legacy_escrow).toBe(false);
  const upstream = new URL(f.state.requests[0]!.url);
  expect(upstream.searchParams.get('product')).toBe('drive');
  expect(upstream.searchParams.get('identity_version')).toBe('2');
  for (const bad of ['1', '3', '2&identity_version=2']) {
    const refused = await f.call(
      f.routes.wrappers.GET,
      'https://drive.sytue.invalid/drive/api/matrix/recovery-wrappers?identity_version=' + bad,
    );
    expect(refused.status).toBe(400);
  }
});

test('PUT passes only a well-formed v1 wrapper, without extra fields', async () => {
  const f = fixture();
  const url = 'https://drive.sytue.invalid/drive/api/matrix/recovery-wrappers?identity_version=2';
  const ok = await f.call(f.routes.wrappers.PUT, url, {
    method: 'PUT',
    body: JSON.stringify({ ...wrapper, recovery_key: 'leak', id: 'forged' }),
  });
  expect(ok.status).toBe(200);
  expect(await f.state.requests[0]!.json()).toEqual(wrapper);
  for (const body of [
    { ...wrapper, v: 2 },
    { ...wrapper, kind: 'passkey' },
    { ...wrapper, credential_id: 'x' },
    { ...wrapper, nonce: 'not base64!' },
    'not json',
  ]) {
    const refused = await f.call(f.routes.wrappers.PUT, url, {
      method: 'PUT',
      body: typeof body === 'string' ? body : JSON.stringify(body),
    });
    expect(refused.status).toBe(400);
  }
  expect(f.state.requests).toHaveLength(1);
});

test('no session, no access', async () => {
  const f = fixture();
  f.state.session = null;
  for (const route of [f.routes.wrappers.GET, f.routes.legacyKey.GET]) {
    expect((await f.call(route, 'https://drive.sytue.invalid/drive/api/matrix/x')).status).toBe(
      401,
    );
  }
  expect(f.state.requests).toHaveLength(0);
});
