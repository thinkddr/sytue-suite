// El selector de organización de la shell se alimenta de `/auth/organizations`,
// que vive en el BFF común porque el menú de cuenta lo montan las diez apps.
//
// Dos cosas no puede equivocar: ofrecer una organización SUSPENDIDA —Sytue ID la
// rechaza con 403, así que sería un botón que solo sabe fallar— y tumbar el menú
// entero cuando el IdP no contesta. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { SytueSession } from '../types.js';
import { resolveOidcConfig } from '../config.js';
import { createBff } from './bff.js';
import { sealSession } from './cookies.js';

const SECRET = 'x'.repeat(32);

function bffDePrueba() {
  return createBff({
    config: resolveOidcConfig({
      SYTUE_OIDC_ISSUER: 'https://id.sytue.eu',
      SYTUE_OIDC_INTERNAL_BASE_URL: 'http://sytue-one-id',
      SYTUE_OIDC_CLIENT_ID: 'sytue-console',
      SYTUE_OIDC_CLIENT_SECRET: 'secret',
      SYTUE_OIDC_REDIRECT_URI: 'https://admin.sytue.eu/auth/callback',
    }),
    sessionSecret: SECRET,
  });
}

const sesion: SytueSession = {
  clientId: 'sytue-console',
  user: {
    id: 'u1',
    email: 'quien@ejemplo.eu',
    emailVerified: true,
    name: 'Quien',
    preferredUsername: 'quien',
    tenant: 'vieja',
  } as SytueSession['user'],
  tokens: {
    accessToken: 'tok',
    // Lejos de caducar: si no, getSession intenta refrescar contra el IdP.
    expiresAt: Date.now() + 3_600_000,
    tokenType: 'Bearer',
  },
};

async function pedirOrganizaciones(
  respuesta: () => Promise<Response>,
  method = 'GET',
  body?: string,
) {
  const cookie = await sealSession(sesion, SECRET);
  const url = new URL('https://admin.sytue.eu/auth/organizations');
  const real = globalThis.fetch;
  globalThis.fetch = (() => respuesta()) as unknown as typeof fetch;
  try {
    const init: RequestInit = { method };
    if (body !== undefined) init.body = body;
    return await bffDePrueba().organizations({
      request: new Request(url, init),
      url,
      cookies: { get: () => ({ value: cookie }), set: () => {}, delete: () => {} },
    });
  } finally {
    globalThis.fetch = real;
  }
}

test('lista solo las activas y marca la del claim tenant', async () => {
  const res = await pedirOrganizaciones(() =>
    Promise.resolve(
      new Response(
        JSON.stringify({
          active_org: 'nueva',
          memberships: [
            { org_id: 'a', slug: 'vieja', name: 'Vieja SL', status: 'active' },
            { org_id: 'b', slug: 'nueva', name: 'Nueva SL', status: 'active' },
            { org_id: 'c', slug: 'cortada', name: 'Cortada SL', status: 'suspended' },
          ],
        }),
        { status: 200, headers: { 'content-type': 'application/json' } },
      ),
    ),
  );
  expect(res.status).toBe(200);
  const body = (await res.json()) as { organizations: { slug: string; active: boolean }[] };
  expect(body.organizations.map((o) => o.slug)).toEqual(['vieja', 'nueva']);
  expect(body.organizations.find((o) => o.active)?.slug).toBe('nueva');
});

test('si el IdP no contesta, el menú de cuenta sigue en pie', async () => {
  const res = await pedirOrganizaciones(() => Promise.reject(new Error('sin red')));
  expect(res.status).toBe(200);
  expect(await res.json()).toEqual({ organizations: [] });
});

test('el cambio sin org_id no viaja al IdP', async () => {
  let llamado = false;
  const res = await pedirOrganizaciones(
    () => {
      llamado = true;
      return Promise.resolve(new Response('{}', { status: 200 }));
    },
    'PUT',
    JSON.stringify({}),
  );
  expect(res.status).toBe(400);
  expect(llamado).toBe(false);
});

test('el cambio con éxito pide rehacer el login SSO', async () => {
  const res = await pedirOrganizaciones(
    () =>
      Promise.resolve(
        new Response(JSON.stringify({ slug: 'nueva' }), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        }),
      ),
    'PUT',
    JSON.stringify({ org_id: 'b' }),
  );
  expect(res.status).toBe(200);
  // Sin esto el cliente recargaría con el `tenant` anterior —va firmado dentro
  // del token— y el cambio parecería no haber funcionado.
  expect(await res.json()).toEqual({ slug: 'nueva', reauthorize: true });
});
