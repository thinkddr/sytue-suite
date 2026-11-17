/**
 * «Detalles» de un fichero o carpeta: lo que reúne el BFF `/api/details`
 * (`detalles.ts`). Ejecutar: `bun test`.
 *
 * El servicio de Drive se simula sustituyendo `fetch`: `pedir` hace lo mismo que
 * `driveFetch` (la URL del servicio + la ruta), así que lo que se prueba es qué se le
 * pide y cómo se combina lo que contesta. El directorio se sustituye entero.
 */
import { afterEach, expect, test } from 'bun:test';
import { contarAcceso, reunirDetalles, type DependenciasDetalles } from './detalles';

const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

const ANA = 'sub-ana';
const LUIS = 'sub-luis';
const AHORA = Date.parse('2026-11-15T12:00:00Z');

/** Simula el servicio: cada ruta, su respuesta; lo demás, 404. */
function servicio(rutas: Record<string, { status?: number; body: unknown }>): string[] {
  const pedidas: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    pedidas.push(url.pathname);
    const r = rutas[url.pathname];
    if (!r) return new Response('{}', { status: 404 });
    return new Response(JSON.stringify(r.body), { status: r.status ?? 200 });
  }) as typeof fetch;
  return pedidas;
}

function deps(over: Partial<DependenciasDetalles> = {}): DependenciasDetalles {
  return {
    pedir: (ruta) => fetch(`https://drive.test${ruta}`),
    resolver: async (subs) =>
      new Map(
        subs.flatMap((s) =>
          s === ANA ? [[s, { email: 'ana@org.eu', name: 'Ana Ruiz' }] as const] : [],
        ),
      ),
    ahora: () => AHORA,
    ...over,
  };
}

const FICHERO = {
  id: 'f1',
  name: 'informe.pdf',
  path: '/Proyectos/informe.pdf',
  size_bytes: 2048,
  content_type: 'application/pdf',
  owner_sub: ANA,
  modified_by: LUIS,
  version: 4,
  created_at: '2026-01-02T10:00:00Z',
  updated_at: '2026-11-01T09:30:00Z',
  label: { id: 'l1', name: 'Confidencial', rank: 2, block_external_share: true },
};

test('un fichero junta metadato, personas, versión, clasificación y acceso', async () => {
  const pedidas = servicio({
    '/api/v1/files/f1': { body: FICHERO },
    '/api/v1/files/f1/shares': {
      body: {
        shares: [
          { kind: 'link' },
          { kind: 'link', expires_at: '2026-01-01T00:00:00Z' }, // caducado: no cuenta
          { kind: 'user', subject: LUIS },
          { kind: 'user', subject: LUIS, permission: 'write' }, // la misma persona
          { kind: 'user', subject: 'sub-eva' },
          { kind: 'group', subject: 'ventas' },
        ],
      },
    },
  });
  const r = await reunirDetalles('f1', 'file', deps());
  expect(r.status).toBe(200);
  expect(pedidas.sort()).toEqual(['/api/v1/files/f1', '/api/v1/files/f1/shares']);
  expect(r.body).toEqual({
    details: {
      kind: 'file',
      name: 'informe.pdf',
      path: '/Proyectos/informe.pdf',
      contentType: 'application/pdf',
      size: 2048,
      owner: { name: 'Ana Ruiz', email: 'ana@org.eu' },
      createdAt: '2026-01-02T10:00:00Z',
      modifiedAt: '2026-11-01T09:30:00Z',
      // Luis no está en el directorio: persona vacía, la UI dice «Alguien».
      modifiedBy: {},
      version: 4,
      label: { name: 'Confidencial', blockExternalShare: true },
      access: { links: 1, people: 2, groups: 1 },
    },
  });
});

test('una carpeta va por /folders y no trae versión, tamaño ni modificador', async () => {
  const pedidas = servicio({
    '/api/v1/folders/c1': {
      body: {
        id: 'c1',
        name: 'Proyectos',
        owner_sub: ANA,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-02-01T00:00:00Z',
      },
    },
    '/api/v1/folders/c1/shares': { body: { shares: [] } },
  });
  const r = await reunirDetalles('c1', 'folder', deps());
  expect(pedidas.sort()).toEqual(['/api/v1/folders/c1', '/api/v1/folders/c1/shares']);
  expect(r.status).toBe(200);
  const d = (r.body as unknown as { details: Record<string, unknown> }).details;
  expect(d.kind).toBe('folder');
  expect(d.modifiedBy).toBeNull();
  expect(d).not.toHaveProperty('version');
  expect(d).not.toHaveProperty('size');
  expect(d.access).toEqual({ links: 0, people: 0, groups: 0 });
});

test('sin poder leer las comparticiones el panel sigue, pero no dice «0»', async () => {
  servicio({
    '/api/v1/files/f1': { body: FICHERO },
    '/api/v1/files/f1/shares': { status: 403, body: {} },
  });
  const r = await reunirDetalles('f1', 'file', deps());
  expect(r.status).toBe(200);
  expect((r.body as { details: { access: unknown } }).details.access).toBeNull();
});

test('con el directorio caído las personas salen sin cara, no con su sub', async () => {
  servicio({
    '/api/v1/files/f1': { body: FICHERO },
    '/api/v1/files/f1/shares': { body: { shares: [] } },
  });
  const r = await reunirDetalles(
    'f1',
    'file',
    deps({
      resolver: async () => {
        throw new Error('directory:resolve no disponible');
      },
    }),
  );
  const d = (r.body as { details: { owner: unknown; modifiedBy: unknown } }).details;
  expect(d.owner).toEqual({});
  expect(d.modifiedBy).toEqual({});
  expect(JSON.stringify(r.body)).not.toContain(ANA);
});

test('los fallos del servicio se traducen; sin sesión, 401', async () => {
  servicio({ '/api/v1/files/f1': { status: 500, body: {} } });
  expect(await reunirDetalles('f1', 'file', deps())).toEqual({
    status: 502,
    body: { error: 'details_failed' },
  });
  servicio({});
  expect((await reunirDetalles('f1', 'file', deps())).status).toBe(404);
  const sinSesion = await reunirDetalles('f1', 'file', deps({ pedir: async () => null }));
  expect(sinSesion).toEqual({ status: 401, body: { error: 'unauthenticated' } });
});

test('el id viaja escapado: no puede colarse otra ruta del servicio', async () => {
  const pedidas = servicio({});
  await reunirDetalles('../quota', 'file', deps());
  expect(pedidas).toContain('/api/v1/files/..%2Fquota');
});

test('contarAcceso: enlaces vivos, personas y grupos distintos', () => {
  expect(
    contarAcceso(
      [
        { kind: 'link', expires_at: '2027-01-01T00:00:00Z' },
        { kind: 'link', expires_at: '2026-11-15T11:59:59Z' },
        { kind: 'group', subject: 'a' },
        { kind: 'group', subject: 'a' },
      ],
      AHORA,
    ),
  ).toEqual({ links: 1, people: 0, groups: 1 });
});
