// El SDK de Sytue Bases habla con la raíz de la API tal cual (sin añadir /api/v1) y
// codifica cuerpos y consultas como espera el servicio Go. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { BasesClient } from './index.js';

function cliente() {
  const llamadas: { url: string; init?: RequestInit }[] = [];
  const fetchImpl = (async (url: string | URL | Request, init?: RequestInit) => {
    llamadas.push({ url: String(url), init });
    return new Response(init?.method === 'DELETE' ? null : '[]', {
      status: init?.method === 'DELETE' ? 204 : 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  return {
    llamadas,
    c: new BasesClient({
      baseUrl: 'https://bases.test/api/v1/',
      auth: { getAccessToken: () => 'tok' },
      fetchImpl,
    }),
  };
}

test('listBases pide /bases con el Bearer y sin barra doble', async () => {
  const { c, llamadas } = cliente();
  await c.listBases();
  expect(llamadas[0]!.url).toBe('https://bases.test/api/v1/bases');
  expect(new Headers(llamadas[0]!.init?.headers).get('authorization')).toBe('Bearer tok');
});

test('updateRecord manda la versión vista y los enlaces', async () => {
  const { c, llamadas } = cliente();
  await c.updateRecord('r1', 3, { f: 'x' }, { l: ['a'] }).catch(() => undefined);
  const l = llamadas[0]!;
  expect(l.init?.method).toBe('PATCH');
  expect(JSON.parse(String(l.init?.body))).toEqual({
    rev: 3,
    data: { f: 'x' },
    links: { l: ['a'] },
  });
});

test('listRecords lleva la vista y la página en la consulta; deleteRecord fuerza solo si se pide', async () => {
  const { c, llamadas } = cliente();
  await c.listRecords('t1', { view: 'v1', limit: 50, cursor: 'abc' }).catch(() => undefined);
  expect(llamadas[0]!.url).toContain('/tables/t1/records?');
  expect(llamadas[0]!.url).toContain('view=v1');
  expect(llamadas[0]!.url).toContain('limit=50');
  expect(llamadas[0]!.url).toContain('cursor=abc');
  expect(llamadas[0]!.url).not.toContain('offset');
  await c.deleteRecord('r1');
  await c.deleteRecord('r2', true);
  expect(llamadas[1]!.url.endsWith('/records/r1')).toBe(true);
  expect(llamadas[2]!.url.endsWith('/records/r2?force=true')).toBe(true);
});

test('importCsv envía el CSV tal cual con su tipo y el nombre en la consulta', async () => {
  const { c, llamadas } = cliente();
  await c.importCsv('b1', 'Clientes', 'a,b\n1,2\n').catch(() => undefined);
  expect(llamadas[0]!.init?.body).toBe('a,b\n1,2\n');
  expect(new Headers(llamadas[0]!.init?.headers).get('content-type')).toBe('text/csv');
  expect(llamadas[0]!.url).toContain('name=Clientes');
});

test('los enlaces de formulario cuelgan de la vista y se revocan por su id', async () => {
  const { c, llamadas } = cliente();
  await c.createFormLink('v1', { max_responses: 20 }).catch(() => undefined);
  await c.listFormLinks('v1');
  await c.revokeFormLink('e1');
  expect(llamadas[0]!.url.endsWith('/views/v1/form-links')).toBe(true);
  expect(JSON.parse(String(llamadas[0]!.init?.body))).toEqual({ max_responses: 20 });
  expect(llamadas[1]!.init?.method).toBe('GET');
  expect(llamadas[2]!.url.endsWith('/form-links/e1')).toBe(true);
  expect(llamadas[2]!.init?.method).toBe('DELETE');
});

test('las conexiones se conceden por tabla, se listan por base y se rotan o revocan por id', async () => {
  const { c, llamadas } = cliente();
  await c
    .createConnection('t1', { name: 'Calcus', view_id: 'v1', days: 30 })
    .catch(() => undefined);
  await c.listConnections('b1');
  await c.rotateConnection('c1').catch(() => undefined);
  await c.revokeConnection('c1');
  expect(llamadas[0]!.url.endsWith('/tables/t1/connections')).toBe(true);
  expect(JSON.parse(String(llamadas[0]!.init?.body))).toEqual({
    name: 'Calcus',
    view_id: 'v1',
    days: 30,
  });
  expect(llamadas[1]!.url.endsWith('/bases/b1/connections')).toBe(true);
  expect(llamadas[2]!.url.endsWith('/connections/c1/rotate')).toBe(true);
  expect(llamadas[2]!.init?.method).toBe('POST');
  expect(llamadas[3]!.init?.method).toBe('DELETE');
});

test('los comentarios cuelgan del registro y se borran por su id', async () => {
  const { c, llamadas } = cliente();
  await c.listComments('r1');
  await c.createComment('r1', 'hola').catch(() => undefined);
  await c.createComment('r1', 'respuesta', 'c1').catch(() => undefined);
  await c.deleteComment('c2');
  expect(llamadas[0]!.url.endsWith('/records/r1/comments')).toBe(true);
  expect(llamadas[0]!.init?.method).toBe('GET');
  expect(JSON.parse(String(llamadas[1]!.init?.body))).toEqual({ body: 'hola' });
  expect(JSON.parse(String(llamadas[2]!.init?.body))).toEqual({
    body: 'respuesta',
    parent_id: 'c1',
  });
  expect(llamadas[3]!.url.endsWith('/comments/c2')).toBe(true);
  expect(llamadas[3]!.init?.method).toBe('DELETE');
});

test('compartir con una persona va por su dirección, y transferir también', async () => {
  const { c, llamadas } = cliente();
  await c
    .grantPermission('b1', { principal_kind: 'user', email: 'bea@acme.es', role: 'viewer' })
    .catch(() => undefined);
  await c
    .grantPermission('b1', { principal_kind: 'group', principal_id: 'ventas', role: 'editor' })
    .catch(() => undefined);
  await c.transferBase('b1', 'bea@acme.es').catch(() => undefined);
  await c.myGroups().catch(() => undefined);
  expect(JSON.parse(String(llamadas[0]!.init?.body))).toEqual({
    principal_kind: 'user',
    email: 'bea@acme.es',
    role: 'viewer',
  });
  expect(JSON.parse(String(llamadas[1]!.init?.body))).toEqual({
    principal_kind: 'group',
    principal_id: 'ventas',
    role: 'editor',
  });
  expect(llamadas[2]!.url.endsWith('/bases/b1/transfer')).toBe(true);
  expect(JSON.parse(String(llamadas[2]!.init?.body))).toEqual({ email: 'bea@acme.es' });
  expect(llamadas[3]!.url.endsWith('/groups')).toBe(true);
});

test('a alguien de otra organización se le invita por su @usuario o su dirección', async () => {
  const { c, llamadas } = cliente();
  await c
    .grantPermission('b1', { principal_kind: 'external', query: '@gus', role: 'commenter' })
    .catch(() => undefined);
  expect(JSON.parse(String(llamadas[0]!.init?.body))).toEqual({
    principal_kind: 'external',
    query: '@gus',
    role: 'commenter',
  });
});

test('una operación de celda lleva su clave de idempotencia y la versión del contrato', async () => {
  const { c, llamadas } = cliente();
  await c.writeCell('r1', 'f1', { value: 'x', expected: null }, 'op-1').catch(() => undefined);
  await c.getRecord('r1').catch(() => undefined);
  await c.heartbeat('b1', { conn: 'pestana-1', table_id: 't1' });
  expect(llamadas[0]!.url).toContain('/records/r1/cells/f1');
  expect(llamadas[0]!.url).toContain('v=1');
  expect(llamadas[0]!.init?.method).toBe('PUT');
  expect(new Headers(llamadas[0]!.init?.headers).get('idempotency-key')).toBe('op-1');
  expect(JSON.parse(String(llamadas[0]!.init?.body))).toEqual({ value: 'x', expected: null });
  expect(llamadas[1]!.url.endsWith('/records/r1')).toBe(true);
  expect(llamadas[2]!.url.endsWith('/bases/b1/presence')).toBe(true);
  expect(llamadas[2]!.init?.method).toBe('PUT');
});

test('los scripts cuelgan de la base: versiones, revisión con el hash, ejecución y registro', async () => {
  const { c, llamadas } = cliente();
  await c.listScripts('b1').catch(() => undefined);
  await c
    .createScript('b1', { name: 'G', scope: 'shared', code: 'x', manifest: { v: 1 } })
    .catch(() => undefined);
  await c.proposeScriptVersion('b1', 's1', 2);
  await c.reviewScriptVersion('b1', 's1', 2, 'approve', 'abc');
  await c.authorizeScriptRun('b1', 's1', 2).catch(() => undefined);
  await c.closeScriptRun('b1', 41, { outcome: 'ok', error_code: '', duration_ms: 5, ops: 1 });
  await c.listScriptRuns('b1', { before: 9 }).catch(() => undefined);
  await c.deleteScript('b1', 's1');
  const vistas = llamadas.map((l) => `${l.init?.method ?? 'GET'} ${new URL(l.url).pathname}`);
  expect(vistas).toEqual([
    'GET /api/v1/bases/b1/scripts',
    'POST /api/v1/bases/b1/scripts',
    'POST /api/v1/bases/b1/scripts/s1/versions/2/propose',
    'POST /api/v1/bases/b1/scripts/s1/versions/2/review',
    'POST /api/v1/bases/b1/scripts/s1/versions/2/runs',
    'PUT /api/v1/bases/b1/script-runs/41',
    'GET /api/v1/bases/b1/script-runs',
    'DELETE /api/v1/bases/b1/scripts/s1',
  ]);
  expect(JSON.parse(String(llamadas[3]!.init?.body))).toEqual({
    decision: 'approve',
    code_hash: 'abc',
  });
  expect(llamadas[6]!.url).toContain('before=9');
});
