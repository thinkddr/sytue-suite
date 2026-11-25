/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import {
  importEndpoint,
  latestImports,
  listImportOrigins,
  listMyImports,
  unifyOrigins,
  type ImportJob,
  type ImportOrigin,
} from './importData.js';

const BASE = 'https://integrations.test';

/** Un gateway de mentira: cada ruta, su respuesta; lo que no esté, 404. */
function gateway(rutas: Record<string, () => Response>, llamadas: string[] = []): typeof fetch {
  return (async (url: string | URL | Request, init?: RequestInit) => {
    const u = new URL(String(url));
    const clave = `${init?.method ?? 'GET'} ${u.pathname}`;
    llamadas.push(clave);
    const r = rutas[clave];
    return r ? r() : new Response('{}', { status: 404 });
  }) as typeof fetch;
}

function origen(o: Partial<ImportOrigin> & Pick<ImportOrigin, 'instanceId'>): ImportOrigin {
  return {
    instanceIds: [o.instanceId],
    name: 'Google',
    source: 'google',
    accountKey: '',
    accountLabel: '',
    state: 'ready',
    ...o,
  };
}

function trabajo(j: Partial<ImportJob> & Pick<ImportJob, 'id'>): ImportJob {
  return {
    instanceId: 'a',
    domain: 'mail',
    source: 'google',
    status: 'done',
    migrated: 0,
    skipped: 0,
    errors: 0,
    lastError: '',
    errorCode: '',
    accountKey: 'google:acme.com',
    accountLabel: 'acme.com',
    createdAt: '2026-11-01T10:00:00Z',
    ...j,
  };
}

test('una cuenta conectada varias veces es UN origen, con la que puede leer delante', () => {
  const got = unifyOrigins([
    origen({ instanceId: 'rota', accountKey: 'google:acme.com', state: 'unavailable' }),
    origen({ instanceId: 'buena', accountKey: 'google:acme.com', accountLabel: 'acme.com' }),
    origen({ instanceId: 'sin-cuenta' }),
    origen({ instanceId: 'm', source: 'm365', accountKey: 'm365:contoso.onmicrosoft.com' }),
  ]);
  expect(got).toHaveLength(2);
  expect(got[0]).toMatchObject({
    instanceId: 'buena',
    instanceIds: ['buena', 'rota', 'sin-cuenta'],
    accountLabel: 'acme.com',
    state: 'ready',
  });
  expect(got[1]?.instanceId).toBe('m');
});

test('con dos cuentas conocidas del mismo proveedor no se adivina de quién es la desconocida', () => {
  const got = unifyOrigins([
    origen({ instanceId: 'a', accountKey: 'google:acme.com' }),
    origen({ instanceId: 'b', accountKey: 'google:beta.com' }),
    origen({ instanceId: 'x' }),
  ]);
  expect(got.map((o) => o.instanceIds)).toEqual([['a'], ['b'], ['x']]);
});

test('listImportOrigins pide al gateway los orígenes y deduplica por su cuenta', async () => {
  const fetchImpl = gateway({
    'GET /api/v1/migrations/self/origins': () =>
      Response.json({
        origins: [
          {
            instance_id: 'b',
            instance_ids: ['b', 'a'],
            name: 'Google',
            source: 'google',
            account_key: 'google:acme.com',
            account_label: 'acme.com',
            state: 'ready',
          },
          // Un gateway con un fallo que repitiera la cuenta no la enseña dos veces.
          { instance_id: 'a', name: 'Google', source: 'google', account_key: 'Google:Acme.com' },
        ],
      }),
  });
  const got = await listImportOrigins(BASE, 'tok', fetchImpl);
  expect(got).toEqual([
    {
      instanceId: 'b',
      instanceIds: ['b', 'a'],
      name: 'Google',
      source: 'google',
      accountKey: 'google:acme.com',
      accountLabel: 'acme.com',
      state: 'ready',
    },
  ]);
});

test('con un gateway anterior se deduce de /instances, solo con las plantillas que migran', async () => {
  const llamadas: string[] = [];
  const fetchImpl = gateway(
    {
      'GET /api/v1/instances': () =>
        Response.json({
          instances: [
            // La misma cuenta para Drive, Meet y el correo de Outlook: no son orígenes.
            { id: 'd', name: 'Drive', template_key: 'google-drive', status: 'active' },
            { id: 'mt', name: 'Meet', template_key: 'google-meet', status: 'active' },
            { id: 'o', name: 'Outlook', template_key: 'microsoft-mail', status: 'active' },
            { id: 't', name: 'Teams', template_key: 'microsoft-teams', status: 'active' },
            { id: 'b', name: 'Borrador', template_key: 'google-workspace', status: 'draft' },
            {
              id: 'g1',
              name: 'Google',
              template_key: 'google-workspace',
              status: 'active',
              has_secrets: true,
              config: { domain: 'Acme.com' },
            },
            { id: 'g2', name: 'Google 2', template_key: 'google-workspace', status: 'active' },
            { id: 'm', name: 'M365', template_key: 'microsoft365', status: 'error' },
          ],
        }),
    },
    llamadas,
  );
  const got = await listImportOrigins(BASE, 'tok', fetchImpl);
  expect(llamadas).toEqual(['GET /api/v1/migrations/self/origins', 'GET /api/v1/instances']);
  expect(got.map((o) => [o.instanceIds, o.accountLabel, o.state])).toEqual([
    [['g1', 'g2'], 'acme.com', 'ready'],
    [['m'], '', 'unavailable'],
  ]);
});

test('un fallo del gateway es «no hay orígenes», no un error', async () => {
  expect(
    await listImportOrigins(
      BASE,
      'tok',
      gateway({ 'GET /api/v1/migrations/self/origins': () => new Response('', { status: 500 }) }),
    ),
  ).toEqual([]);
  const roto = (async () => {
    throw new TypeError('sin red');
  }) as unknown as typeof fetch;
  expect(await listImportOrigins(BASE, 'tok', roto)).toEqual([]);
});

test('el resumen cuenta UNA vez cada cuenta y contenido: la importación vigente', () => {
  const got = latestImports([
    trabajo({ id: 'viejo', migrated: 1200, createdAt: '2026-10-01T10:00:00Z' }),
    trabajo({ id: 'nuevo', instanceId: 'b', migrated: 1250, createdAt: '2026-11-01T10:00:00Z' }),
    trabajo({ id: 'contactos', domain: 'contacts', migrated: 30 }),
    trabajo({ id: 'otra-cuenta', accountKey: 'google:beta.com', migrated: 5 }),
    trabajo({ id: 'sin-cuenta-1', accountKey: '', instanceId: 'z', migrated: 7 }),
    trabajo({
      id: 'sin-cuenta-2',
      accountKey: '',
      instanceId: 'z',
      migrated: 9,
      createdAt: '2026-12-01T10:00:00Z',
    }),
  ]);
  expect(got.map((j) => j.id).sort()).toEqual([
    'contactos',
    'nuevo',
    'otra-cuenta',
    'sin-cuenta-2',
  ]);
  expect(got.reduce((n, j) => n + j.migrated, 0)).toBe(1250 + 30 + 5 + 9);
});

test('listMyImports filtra al contenido de la app y se queda con la vigente', async () => {
  const fetchImpl = gateway({
    'GET /api/v1/migrations/self': () =>
      Response.json({
        migrations: [
          {
            id: '2',
            instance_id: 'b',
            domain: 'mail',
            source: 'google',
            status: 'error',
            error_code: 'connection_expired',
            account_key: 'google:acme.com',
            account_label: 'acme.com',
            counts: { migrated: 10 },
            created_at: '2026-11-02T10:00:00Z',
          },
          {
            id: '1',
            instance_id: 'a',
            domain: 'mail',
            source: 'google',
            status: 'done',
            account_key: 'google:acme.com',
            created_at: '2026-11-01T10:00:00Z',
          },
          { id: '3', domain: 'drive', source: 'google', status: 'done', created_at: 'x' },
        ],
      }),
  });
  const got = await listMyImports(BASE, 'tok', ['mail'], fetchImpl);
  expect(got).toHaveLength(1);
  expect(got[0]).toMatchObject({
    id: '2',
    errorCode: 'connection_expired',
    accountLabel: 'acme.com',
  });
});

test('importEndpoint acota el contenido al del producto y devuelve el código del rechazo', async () => {
  let enviado: unknown;
  const fetchImpl = (async (url: string | URL | Request, init?: RequestInit) => {
    enviado = JSON.parse(String(init?.body));
    expect(String(url)).toBe(`${BASE}/api/v1/migrations/self`);
    return Response.json(
      { error: { code: 'import_connection_unavailable', message: 'x' } },
      { status: 409 },
    );
  }) as typeof fetch;
  const req = new Request('https://mail.test/api/import', {
    method: 'POST',
    body: JSON.stringify({ instanceId: 'a', source: 'google', domains: ['mail', 'drive'] }),
  });
  const res = await importEndpoint(req, {
    baseUrl: BASE,
    accessToken: 'tok',
    domains: ['mail', 'contacts'],
    fetchImpl,
  });
  expect(enviado).toEqual({ instance_id: 'a', source: 'google', domains: ['mail'] });
  expect(res.status).toBe(409);
  expect(await res.json()).toEqual({
    error: 'import_failed',
    code: 'import_connection_unavailable',
  });
});

test('importEndpoint sin sesión no llega al gateway', async () => {
  const llamadas: string[] = [];
  const res = await importEndpoint(new Request('https://mail.test/api/import'), {
    baseUrl: BASE,
    accessToken: null,
    domains: ['mail'],
    fetchImpl: gateway({}, llamadas),
  });
  expect(res.status).toBe(401);
  expect(llamadas).toEqual([]);
});

test('importEndpoint GET junta orígenes y trabajos del contenido pedido', async () => {
  const fetchImpl = gateway({
    'GET /api/v1/migrations/self/origins': () => Response.json({ origins: [] }),
    'GET /api/v1/migrations/self': () =>
      Response.json({
        migrations: [
          { id: '1', domain: 'contacts', source: 'google', status: 'done', created_at: 'a' },
          { id: '2', domain: 'mail', source: 'google', status: 'done', created_at: 'b' },
        ],
      }),
  });
  const res = await importEndpoint(new Request('https://mail.test/api/import?domains=contacts'), {
    baseUrl: BASE,
    accessToken: 'tok',
    domains: ['mail', 'contacts'],
    fetchImpl,
  });
  const body = (await res.json()) as { origins: unknown[]; jobs: ImportJob[] };
  expect(body.origins).toEqual([]);
  expect(body.jobs.map((j) => j.id)).toEqual(['1']);
});
