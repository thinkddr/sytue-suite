// Lo que el selector de organización enseña sale de `GET /api/v1/me`, y hay dos
// cosas que no puede equivocar: cuál está activa y cuáles se pueden elegir.
//
// Una organización SUSPENDIDA no se puede activar —Sytue ID responde 403—, así
// que ofrecerla es pintar un botón que solo sabe fallar. Y la activa se decide
// por el claim `tenant` del token, no por el orden de la lista: cuando el
// usuario cambie, el orden seguirá siendo el mismo y la marca tiene que
// moverse. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { SytueSession } from '@sytue/auth';

// `admin.ts` arrastra el wiring del BFF, que exige su configuración OIDC al
// cargarse —es lo que impide desplegar una superficie a medio configurar—. Se
// da aquí y se importa después, como en platform.test.ts.
process.env.SYTUE_OIDC_ISSUER = 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID = 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET = 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI = 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI = 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET = 'x'.repeat(32);

const { loadMyOrganizations } = await import('./admin.js');

const sesion = { tokens: { accessToken: 'tok' } } as SytueSession;

function conRespuesta<T>(res: Response, fn: () => Promise<T>): Promise<T> {
  const real = globalThis.fetch;
  globalThis.fetch = (() => Promise.resolve(res)) as unknown as typeof fetch;
  return fn().finally(() => {
    globalThis.fetch = real;
  });
}

function meJSON(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}

test('marca la activa por el tenant y descarta las suspendidas', async () => {
  const { live, data } = await conRespuesta(
    meJSON({
      active_org: 'nueva',
      memberships: [
        { org_id: 'a', slug: 'vieja', name: 'Vieja SL', role: 'member', status: 'active' },
        { org_id: 'b', slug: 'nueva', name: 'Nueva SL', role: 'owner', status: 'active' },
        { org_id: 'c', slug: 'cortada', name: 'Cortada SL', role: 'owner', status: 'suspended' },
      ],
    }),
    () => loadMyOrganizations(sesion),
  );
  expect(live).toBe(true);
  expect(data.map((o) => o.slug)).toEqual(['vieja', 'nueva']);
  expect(data.find((o) => o.active)?.slug).toBe('nueva');
  expect(data.find((o) => o.slug === 'nueva')?.role).toBe('owner');
});

test('sin backend vivo devuelve lista vacía, no una excepción', async () => {
  // El selector se pinta en el pie del sidebar de TODAS las páginas: que una
  // caída del IdP tumbe la consola entera sería cambiar un problema por otro
  // mayor. Con la lista vacía, el componente no se pinta y ya está.
  const { live, data } = await conRespuesta(new Response('<html>502</html>', { status: 502 }), () =>
    loadMyOrganizations(sesion),
  );
  expect(live).toBe(false);
  expect(data).toEqual([]);
});
