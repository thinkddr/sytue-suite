/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El BFF de invitaciones acepta una TANDA (`emails`) además de una sola
 * dirección (`email`), sin cambiar la forma de esta. Se prueba la ruta de Astro
 * entera —guarda incluida— con la sesión simulada y `fetch` capturando lo que
 * llega a Sytue ID.
 *
 * Ejecutar: `bun test`.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

// lib/auth.ts monta su BFF al importarse y exige la configuración OIDC.
process.env.SYTUE_OIDC_ISSUER = 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID = 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET = 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI = 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI = 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET = 'x'.repeat(32);

mock.module('../server/bff.js', () => ({
  getBff: () => ({
    basePath: '/admin',
    getSession: () =>
      Promise.resolve({
        tokens: { accessToken: 'x.e30.y' },
        user: { id: 'u1', roles: ['admin'], orgKind: 'org' },
      }),
  }),
  toBffContext: (ctx: APIContext) => ({ request: ctx.request, cookies: ctx.cookies, url: ctx.url }),
}));

const { POST } = await import('../pages/api/admin/invitations.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

interface Llamada {
  url: string;
  body: unknown;
}

/** Hace el POST al BFF con Sytue ID contestando `upstream`. */
async function post(
  cuerpo: unknown,
  upstream: Response,
): Promise<{ res: Response; llamadas: Llamada[] }> {
  const llamadas: Llamada[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    const raw = init?.body ?? (input instanceof Request ? await input.text() : undefined);
    llamadas.push({ url, body: raw ? JSON.parse(String(raw)) : undefined });
    return upstream;
  }) as unknown as typeof fetch;
  const url = new URL('https://admin.sytue.eu/admin/api/admin/invitations');
  const request = new Request(url, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(cuerpo),
  });
  const res = await POST({ request, url, cookies: {} } as unknown as APIContext);
  return { res: res as Response, llamadas };
}

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

test('una tanda llega a Sytue ID como `emails` y vuelve con el resultado de cada dirección', async () => {
  const resultado = {
    sent: 1,
    results: [
      { email: 'ana@acme.es', status: 'sent', expires_at: '2026-11-17T10:00:00Z' },
      { email: 'x', status: 'invalid' },
    ],
  };
  const { res, llamadas } = await post(
    { emails: [' ana@acme.es ', 'x', ''], role: 'member' },
    json(200, resultado),
  );
  expect(res.status).toBe(200);
  expect(await res.json()).toEqual(resultado);
  expect(llamadas).toHaveLength(1);
  expect(llamadas[0]!.url).toEndWith('/api/v1/admin/invitations');
  expect(llamadas[0]!.body).toEqual({ emails: ['ana@acme.es', 'x'], role: 'member' });
});

test('el rechazo de Sytue ID a una tanda conserva su status y su mensaje', async () => {
  const { res } = await post(
    { emails: ['a@acme.es'], role: 'owner' },
    json(400, { error: { code: 'bad_request', message: 'Invalid role for a batch.' } }),
  );
  expect(res.status).toBe(400);
  expect(await res.json()).toMatchObject({ message: 'Invalid role for a batch.' });
});

test('una tanda sin correos, con algo que no es texto o con rol inventado no sale del BFF', async () => {
  for (const cuerpo of [
    { emails: [], role: 'member' },
    { emails: ['  '], role: 'member' },
    { emails: 'a@acme.es', role: 'member' },
    { emails: ['a@acme.es', 7], role: 'member' },
    { emails: ['a@acme.es'], role: 'dios' },
  ]) {
    const { res, llamadas } = await post(cuerpo, json(200, {}));
    expect(res.status).toBe(400);
    expect(llamadas).toHaveLength(0);
  }
});

test('un cuerpo JSON que no es un objeto contesta 400, no revienta', async () => {
  for (const cuerpo of ['hola', 7, true]) {
    const { res, llamadas } = await post(cuerpo, json(200, {}));
    expect(res.status).toBe(400);
    expect(llamadas).toHaveLength(0);
  }
});

test('una sola dirección conserva su forma: `email` y 201', async () => {
  const { res, llamadas } = await post(
    { email: 'ana@acme.es', role: 'admin' },
    json(201, { email: 'ana@acme.es', role: 'admin', expires_at: '2026-11-17T10:00:00Z' }),
  );
  expect(res.status).toBe(201);
  expect(llamadas[0]!.body).toEqual({ email: 'ana@acme.es', role: 'admin', mailbox_local: '' });
});
