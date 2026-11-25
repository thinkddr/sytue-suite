/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El POST de la BFF de SP SAML: lo que vale llega a Sytue ID en su forma, lo que no
 * vale se queda en la BFF con el campo que falla, y un «no» de Sytue ID (409: ese
 * entity ID ya existe) vuelve con su status y su motivo. Se prueba la ruta de Astro
 * entera —guarda incluida— con la sesión simulada y `fetch` capturando lo que llega a
 * Sytue ID. Ejecutar: `bun test`.
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

let roles = ['admin'];
mock.module('../server/bff.js', () => ({
  getBff: () => ({
    basePath: '/admin',
    getSession: () =>
      Promise.resolve({
        tokens: { accessToken: 'x.e30.y' },
        user: { id: 'u1', roles, orgKind: 'org' },
      }),
  }),
  toBffContext: (ctx: APIContext) => ({ request: ctx.request, cookies: ctx.cookies, url: ctx.url }),
}));

const { POST } = await import('../pages/api/admin/apps/saml-sps.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
  roles = ['admin'];
});

interface Llamada {
  url: string;
  method: string;
  body: unknown;
}

async function post(
  cuerpo: unknown,
  upstream: Response,
): Promise<{ res: Response; llamadas: Llamada[] }> {
  const llamadas: Llamada[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    const raw = init?.body ?? (input instanceof Request ? await input.text() : undefined);
    const method = init?.method ?? (input instanceof Request ? input.method : 'GET');
    llamadas.push({ url, method, body: raw ? JSON.parse(String(raw)) : undefined });
    return upstream;
  }) as unknown as typeof fetch;
  const url = new URL('https://admin.sytue.eu/admin/api/admin/apps/saml-sps');
  const request = new Request(url, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(cuerpo),
  });
  // `get`: la página de «sin permiso» lee la cookie de idioma.
  const cookies = { get: () => undefined };
  const res = await POST({ request, url, cookies } as unknown as APIContext);
  return { res: res as Response, llamadas };
}

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

const SP_CREADO = {
  id: '6f1c7a52-6f0e-4d0a-9d55-1b4c9a1f0e11',
  name: 'Gastos',
  entity_id: 'urn:gastos',
  acs_url: 'https://gastos.acme.es/acs',
  name_id_format: 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
  idp_initiated: false,
  enabled: true,
  configured: true,
  created_at: '2026-11-25T10:00:00Z',
  idp_entity_id: 'https://id.ejemplo/saml/idp/acme',
  idp_metadata_url: 'https://id.ejemplo/saml/idp/acme',
  idp_sso_url: 'https://id.ejemplo/saml/idp/sso',
  idp_slo_url: 'https://id.ejemplo/saml/idp/slo',
  initiate_url: 'https://id.ejemplo/saml/idp/initiate/6f1c',
};

test('un alta válida llega a Sytue ID en snake_case y vuelve 201 con los datos del IdP', async () => {
  const { res, llamadas } = await post(
    {
      name: ' Gastos ',
      entity_id: 'urn:gastos',
      acs_url: 'https://gastos.acme.es/acs',
      attribute_mapping: { email: 'mail' },
    },
    json(201, SP_CREADO),
  );
  expect(res.status).toBe(201);
  expect(llamadas).toHaveLength(1);
  expect(llamadas[0]?.method).toBe('POST');
  expect(llamadas[0]?.url).toEndWith('/api/v1/admin/apps/saml-sps');
  expect(llamadas[0]?.body).toEqual({
    name: 'Gastos',
    entity_id: 'urn:gastos',
    acs_url: 'https://gastos.acme.es/acs',
    cert_pem: '',
    name_id_format: 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
    attribute_mapping: { email: 'mail' },
    idp_initiated: false,
    relay_state_default: '',
    enabled: true,
  });
  const sp = (await res.json()) as Record<string, unknown>;
  expect(sp.entityId).toBe('urn:gastos');
  expect(sp.idpMetadataUrl).toBe('https://id.ejemplo/saml/idp/acme');
});

test('lo que no vale se queda en la BFF con el campo que falla', async () => {
  const { res, llamadas } = await post(
    {
      name: 'Gastos',
      entity_id: 'urn:gastos',
      acs_url: 'http://gastos.acme.es/acs',
    },
    json(201, SP_CREADO),
  );
  expect(res.status).toBe(400);
  expect(await res.json()).toEqual({ error: 'bad_request', code: 'acsInsecure' });
  expect(llamadas).toHaveLength(0);
});

test('una clave privada en el campo del certificado no sale de la BFF', async () => {
  const { res, llamadas } = await post(
    {
      name: 'Gastos',
      entity_id: 'urn:gastos',
      acs_url: 'https://gastos.acme.es/acs',
      cert_pem: '-----BEGIN PRIVATE KEY-----\nMIGH\n-----END PRIVATE KEY-----',
    },
    json(201, SP_CREADO),
  );
  expect(res.status).toBe(400);
  expect(await res.json()).toEqual({ error: 'bad_request', code: 'certPrivateKey' });
  expect(llamadas).toHaveLength(0);
});

test('el 409 de Sytue ID vuelve con su status y su motivo', async () => {
  const { res } = await post(
    { name: 'Gastos', entity_id: 'urn:gastos', acs_url: 'https://gastos.acme.es/acs' },
    json(409, { error: { code: 'conflict', message: 'Resource already exists.' } }),
  );
  expect(res.status).toBe(409);
  expect(await res.json()).toEqual({
    error: 'upstream',
    code: 'conflict',
    message: 'Resource already exists.',
  });
});

test('sin rol de administración no se da de alta nada', async () => {
  roles = ['member'];
  const { res, llamadas } = await post(
    { name: 'Gastos', entity_id: 'urn:gastos', acs_url: 'https://gastos.acme.es/acs' },
    json(201, SP_CREADO),
  );
  expect(res.status).toBe(403);
  expect(llamadas).toHaveLength(0);
});
