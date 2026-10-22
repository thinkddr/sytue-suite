/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El proxy del BFF es la frontera entre el navegador y los servicios: lo único que añade
 * es el token de LA SESIÓN, y no deja que quien llama elija otra identidad ni otra ruta.
 * La autorización la decide el servicio; aquí se comprueba que el proxy no la esquiva.
 * Ejecutar: `bun test`.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';
import type { Bff } from './bff.js';
import { crearProxyDeServicio } from './proxy.js';

const real = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = real;
});

function espia() {
  const f = mock(() => Promise.resolve(new Response('{}', { status: 200 })));
  globalThis.fetch = f as unknown as typeof fetch;
  return f;
}

const bff = (token: string | null) =>
  ({
    getSession: () => Promise.resolve(token ? { tokens: { accessToken: token } } : null),
  }) as unknown as Bff;

function ctx(ruta: string, init: RequestInit = {}): APIContext {
  const request = new Request('https://bases.sytue.test/bases/api/x?limit=5', init);
  return { params: { ruta }, request, url: new URL(request.url), cookies: {} } as APIContext;
}

const proxy = (token: string | null) =>
  crearProxyDeServicio({
    destino: 'http://servicio:8102',
    bff: bff(token),
    prefijo: '/api/v1/',
    denegar: (ruta) => ruta.includes('/internal'),
  });

test('sin sesión no se llama al servicio', async () => {
  const f = espia();
  expect((await proxy(null)(ctx('bases'))).status).toBe(401);
  expect(f).not.toHaveBeenCalled();
});

test('un «..» no saca la petición del prefijo: las rutas sin sesión del servicio no se alcanzan', async () => {
  const f = espia();
  for (const ruta of ['../../_bases/v1/connector/read', 'bases/../../../_bases/v1/forms/x', '..'])
    expect((await proxy('tok')(ctx(ruta))).status).toBe(400);
  expect(f).not.toHaveBeenCalled();
});

test('un «..» camuflado con tab o barra invertida tampoco sale del prefijo (lo que `fetch` reinterpreta)', async () => {
  const f = espia();
  // El parser de URL de fetch borra \t \n \r y trata \\ como / en http(s), y lo
  // hace DESPUÉS del guardián de `..`: antes del arreglo estos lo esquivaban.
  for (const ruta of ['.\t./.\t./.\t./healthz', 'x\\..\\..\\..\\healthz', 'bases\n/../../_bases'])
    expect((await proxy('tok')(ctx(ruta))).status).toBe(400);
  expect(f).not.toHaveBeenCalled();
});

test('tampoco disfrazado: ni con barras invertidas, que fetch lee como «/», ni con %2F', async () => {
  const f = espia();
  // Así llegan `%5C` y `%2F` tras el `decodeURI` de Astro: la barra invertida ya en claro
  // y la barra codificada sin tocar. `%E0%A4%A` es un escape roto.
  for (const ruta of [
    'bases\\..\\..\\..\\_bases\\v1\\forms\\x',
    'bases%2F..%2F..%2F..%2F_bases%2Fv1%2Fforms%2Fx',
    '%E0%A4%A',
  ])
    expect((await proxy('tok')(ctx(ruta))).status).toBe(400);
  expect(f).not.toHaveBeenCalled();
});

test('una ruta denegada camuflada con tab no se reenvía (bypass de `denegar`)', async () => {
  const f = espia();
  // `inter<TAB>nal` no casa `/internal` en denegar, pero fetch borraría el tab.
  expect((await proxy('tok')(ctx('inter\tnal/usage'))).status).toBe(400);
  expect(f).not.toHaveBeenCalled();
});

test('una ruta denegada no se reenvía', async () => {
  const f = espia();
  expect((await proxy('tok')(ctx('internal/usage'))).status).toBe(403);
  expect(f).not.toHaveBeenCalled();
});

test('la identidad es la de la sesión: ni el Authorization ni las cabeceras de quien llama pasan', async () => {
  const f = espia();
  const res = await proxy('token-de-la-sesion')(
    ctx('bases', {
      headers: {
        authorization: 'Bearer token-de-otra-persona',
        cookie: 'sesion=x',
        'x-sytue-tenant': 'otra-organizacion',
        'x-forwarded-for': '10.0.0.1',
        'if-none-match': '"abc"',
      },
    }),
  );
  expect(res.status).toBe(200);
  const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
  expect(url).toBe('http://servicio:8102/api/v1/bases?limit=5');
  expect(init.headers).toEqual({
    authorization: 'Bearer token-de-la-sesion',
    'if-none-match': '"abc"',
  });
});

// ── Escrituras sin sesión ────────────────────────────────────────────────────

const conDenuncias = (token: string | null) =>
  crearProxyDeServicio({
    destino: 'http://servicio:8099',
    bff: bff(token),
    prefijo: '/api/v1/',
    rutasPublicas: ['/api/v1/public/'],
    escriturasPublicas: [/^\/api\/v1\/public\/videos\/[0-9a-f-]{36}\/reports$/],
  });
const VIDEO = '0b7f0d0e-7a43-4c57-9a5e-6d1f0c2b8a11';
const post = (cuerpo: string, extra: Record<string, string> = {}): RequestInit => ({
  method: 'POST',
  body: cuerpo,
  headers: {
    'content-type': 'application/json',
    'content-length': String(cuerpo.length),
    ...extra,
  },
});

test('una escritura sin sesión declarada pasa sin token, con la IP de quien llama', async () => {
  const f = espia();
  // Con sesión abierta tampoco lleva token: por esa ruta nadie tiene identidad.
  for (const token of [null, 'token-de-la-sesion']) {
    const res = await conDenuncias(token)(
      ctx(
        `public/videos/${VIDEO}/reports`,
        post('{"kind":"video"}', { 'x-forwarded-for': '203.0.113.7' }),
      ),
    );
    expect(res.status).toBe(200);
  }
  expect(f).toHaveBeenCalledTimes(2);
  for (const llamada of f.mock.calls as unknown as [string, RequestInit][]) {
    expect(llamada[0]).toBe(`http://servicio:8099/api/v1/public/videos/${VIDEO}/reports?limit=5`);
    const h = llamada[1].headers as Record<string, string>;
    expect(h['authorization']).toBeUndefined();
    expect(h['x-forwarded-for']).toBe('203.0.113.7');
  }
});

test('solo la ruta declarada: otra escritura bajo lo público sigue pidiendo sesión', async () => {
  const f = espia();
  for (const ruta of [
    `public/videos/${VIDEO}/comments`,
    `public/videos/${VIDEO}/reports/extra`,
    `videos/${VIDEO}/reports`,
  ])
    expect((await conDenuncias(null)(ctx(ruta, post('{}')))).status).toBe(401);
  // Y con otro método tampoco.
  expect(
    (await conDenuncias(null)(ctx(`public/videos/${VIDEO}/reports`, { method: 'DELETE' }))).status,
  ).toBe(401);
  expect(f).not.toHaveBeenCalled();
});

test('una escritura sin sesión va con el cuerpo acotado y la longitud declarada', async () => {
  const f = espia();
  const ruta = `public/videos/${VIDEO}/reports`;
  const grande = post('{}', { 'content-length': String(16 * 1024 + 1) });
  expect((await conDenuncias(null)(ctx(ruta, grande))).status).toBe(413);
  const sinLargo = new Request('https://cast.sytue.test/cast/api/x', { method: 'POST' });
  const res = await conDenuncias(null)({
    params: { ruta },
    request: sinLargo,
    url: new URL(sinLargo.url),
    cookies: {},
  } as APIContext);
  expect(res.status).toBe(411);
  expect(f).not.toHaveBeenCalled();
});

test('las lecturas con sesión no llevan la IP de quien llama', async () => {
  const f = espia();
  await conDenuncias('tok')(ctx('videos', { headers: { 'x-forwarded-for': '203.0.113.7' } }));
  const h = (f.mock.calls as unknown as [string, RequestInit][])[0]![1].headers as Record<
    string,
    string
  >;
  expect(h['x-forwarded-for']).toBeUndefined();
});
