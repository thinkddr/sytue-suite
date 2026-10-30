/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers PUROS del conector de datos externos (dataquery.ts):
 *  - cellText / resultToGrid: mapeo {columns, rows} → rejilla de texto.
 *  - buildConnectionBody / buildQuerySpec / buildExecuteBody: cuerpos al BFF.
 *  - querySpecToForm: inversa de buildQuerySpec (reutilizar una consulta guardada).
 *  - parseConnections / parseExecuteResult / parseSavedQueries: frontera de confianza.
 *  - sanitizeQuerySpec / sanitizeExternalTable: saneado de lo persistido.
 *  - errorMessageFrom / dataQueryErrorMessage: mensajes legibles.
 *  - queryErrorAdvice: fallo de una consulta traducido a ACCION concreta.
 * Sin DOM (bun corre sin navegador). Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/dataquery.test.ts
 */
import { expect, test } from 'bun:test';
import {
  buildConnectionBody,
  buildExecuteBody,
  buildQuerySpec,
  canEditConnection,
  cellText,
  dataQueryErrorMessage,
  errorMessageFrom,
  parseConnection,
  parseConnections,
  parseExecuteResult,
  parseSavedQueries,
  queryErrorAdvice,
  querySpecToForm,
  resultToGrid,
  sanitizeExternalTable,
  sanitizeQuerySpec,
  type ConnectionForm,
  type QueryForm,
  type RestConnConfig,
} from '../src/editor/dataquery';

/* ---------- cellText / resultToGrid ---------- */

test('cellText: primitivos, nulos y anidados', () => {
  expect(cellText(null)).toBe('');
  expect(cellText(undefined)).toBe('');
  expect(cellText('hola')).toBe('hola');
  expect(cellText(42)).toBe('42');
  expect(cellText(3.5)).toBe('3.5');
  expect(cellText(NaN)).toBe(''); // no finito → vacío, no "NaN"
  expect(cellText(true)).toBe('VERDADERO');
  expect(cellText(false)).toBe('FALSO');
  expect(cellText({ a: 1 })).toBe('{"a":1}');
  expect(cellText([1, 'x'])).toBe('[1,"x"]');
});

test('resultToGrid: cabecera + filas, huecos de filas irregulares a vacío', () => {
  const grid = resultToGrid({
    columns: [
      { name: 'id', type: 'number' },
      { name: 'nombre', type: 'text' },
    ],
    rows: [
      [1, 'Ada'],
      [2], // fila corta: la 2.ª columna se rellena con ''
    ],
  });
  expect(grid).toEqual([
    ['id', 'nombre'],
    ['1', 'Ada'],
    ['2', ''],
  ]);
});

/* ---------- buildConnectionBody ---------- */

const restForm = (over: Partial<ConnectionForm> = {}): ConnectionForm => ({
  name: 'Mi API',
  kind: 'rest',
  instanceId: '',
  baseUrl: 'https://api.example.com',
  authType: 'none',
  apiKeyIn: 'header',
  apiKeyName: '',
  username: '',
  tokenUrl: '',
  clientId: '',
  scope: '',
  audience: '',
  host: '',
  port: '',
  database: '',
  user: '',
  sslmode: 'prefer',
  secret: '',
  ...over,
});

test('buildConnectionBody REST: api_key incluye in/name y el secreto', () => {
  const body = buildConnectionBody(
    restForm({ authType: 'api_key', apiKeyIn: 'query', apiKeyName: 'token', secret: 'sk-123' }),
  );
  expect(body).toEqual({
    name: 'Mi API',
    kind: 'rest',
    config: {
      base_url: 'https://api.example.com',
      auth: { type: 'api_key', in: 'query', name: 'token' },
    },
    credentials: { secret: 'sk-123' },
  });
});

test('buildConnectionBody REST: auth none no manda credentials aunque haya secreto', () => {
  const body = buildConnectionBody(restForm({ authType: 'none', secret: 'x' }));
  expect(body.credentials).toBeUndefined();
  expect((body.config as { auth: { type: string } }).auth.type).toBe('none');
});

test('buildConnectionBody REST: basic lleva username en config y contraseña en credentials', () => {
  const body = buildConnectionBody(restForm({ authType: 'basic', username: 'ada', secret: 'pw' }));
  const cfg = body.config as { auth: { type: string; username?: string } };
  expect(cfg.auth).toEqual({ type: 'basic', username: 'ada' });
  expect(body.credentials).toEqual({ secret: 'pw' });
});

test('buildConnectionBody REST: oauth2 pone token_url/client_id/scope en config y client_secret en credentials', () => {
  const body = buildConnectionBody(
    restForm({
      authType: 'oauth2_client_credentials',
      tokenUrl: ' https://auth.example.com/token ',
      clientId: ' cid ',
      scope: ' read:data ',
      secret: 'cs-xyz',
    }),
  );
  const cfg = body.config as RestConnConfig;
  expect(cfg.auth).toEqual({
    type: 'oauth2_client_credentials',
    token_url: 'https://auth.example.com/token',
    client_id: 'cid',
    scope: 'read:data',
  });
  // El client_secret viaja SOLO en credentials (cifrado en el backend), nunca en config.
  expect(body.credentials).toEqual({ secret: 'cs-xyz' });
});

test('buildConnectionBody REST: oauth2 omite scope/audience vacíos', () => {
  const body = buildConnectionBody(
    restForm({
      authType: 'oauth2_client_credentials',
      tokenUrl: 'https://auth.example.com/token',
      clientId: 'cid',
      secret: 'cs',
    }),
  );
  const cfg = body.config as RestConnConfig;
  expect(cfg.auth.scope).toBeUndefined();
  expect(cfg.auth.audience).toBeUndefined();
});

test('buildConnectionBody Postgres: puerto numérico opcional + sslmode', () => {
  const body = buildConnectionBody({
    ...restForm(),
    kind: 'postgres',
    host: 'db.example.com',
    port: '5433',
    database: 'ventas',
    user: 'lector',
    sslmode: 'require',
    secret: 'pw',
  });
  expect(body).toEqual({
    name: 'Mi API',
    kind: 'postgres',
    config: {
      host: 'db.example.com',
      port: 5433,
      database: 'ventas',
      user: 'lector',
      sslmode: 'require',
    },
    credentials: { secret: 'pw' },
  });
});

test('buildConnectionBody Postgres: puerto vacío/invalido se omite', () => {
  const body = buildConnectionBody({
    ...restForm(),
    kind: 'postgres',
    host: 'h',
    port: '',
    database: 'd',
    user: 'u',
    secret: '',
  });
  expect((body.config as { port?: number }).port).toBeUndefined();
  expect(body.credentials).toBeUndefined();
});

test('buildConnectionBody pasarela: solo la instancia elegida, jamás una credencial', () => {
  const body = buildConnectionBody(
    restForm({ kind: 'gateway', instanceId: ' inst-1 ', secret: 'no-debe-viajar' }),
  );
  expect(body).toEqual({ name: 'Mi API', kind: 'gateway', config: { instance_id: 'inst-1' } });
});

/* ---------- buildQuerySpec / buildExecuteBody ---------- */

const queryForm = (over: Partial<QueryForm> = {}): QueryForm => ({
  kind: 'rest',
  method: 'GET',
  path: '',
  params: [],
  rootPath: '',
  sql: '',
  dataset: '',
  gatewayQuery: '',
  ...over,
});

test('buildQuerySpec REST: método/ruta/params/root_path, omitiendo vacíos', () => {
  const spec = buildQuerySpec(
    queryForm({
      method: 'POST',
      path: '/v1/items',
      params: [
        { key: 'limit', value: '50' },
        { key: '', value: 'ignorado' },
      ],
      rootPath: 'data.items',
    }),
  );
  expect(spec).toEqual({
    rest: { method: 'POST', path: '/v1/items', query: { limit: '50' }, root_path: 'data.items' },
  });
});

test('buildQuerySpec REST: formulario vacío = GET a la URL base (spec mínima válida)', () => {
  expect(buildQuerySpec(queryForm())).toEqual({ rest: {} });
});

test('buildQuerySpec Postgres: SQL requerido (null si vacío)', () => {
  expect(buildQuerySpec(queryForm({ kind: 'postgres', sql: '  SELECT 1  ' }))).toEqual({
    postgres: { sql: 'SELECT 1' },
  });
  expect(buildQuerySpec(queryForm({ kind: 'postgres', sql: '   ' }))).toBeNull();
});

test('buildQuerySpec pasarela: cada driver pide lo suyo y viaja con SU clave', () => {
  // OData: la colección es obligatoria; el filtro, opcional y con clave propia.
  expect(
    buildQuerySpec(
      queryForm({ kind: 'gateway', dataset: ' Products ', gatewayQuery: " Country eq 'ES' " }),
      'odata',
    ),
  ).toEqual({ gateway: { spec: { dataset: 'Products', filter: "Country eq 'ES'" } } });
  expect(buildQuerySpec(queryForm({ kind: 'gateway' }), 'odata')).toBeNull();

  // Jira no tiene «conjunto de datos»: solo JQL, y obligatorio.
  expect(
    buildQuerySpec(queryForm({ kind: 'gateway', gatewayQuery: 'project = ACME' }), 'jira'),
  ).toEqual({ gateway: { spec: { jql: 'project = ACME' } } });
  expect(buildQuerySpec(queryForm({ kind: 'gateway', dataset: 'x' }), 'jira')).toBeNull();

  // Power BI necesita las dos cosas (dataset + DAX).
  expect(buildQuerySpec(queryForm({ kind: 'gateway', dataset: 'ds-1' }), 'powerbi')).toBeNull();

  // Driver desconocido: par genérico, nada obligatorio, pero spec no vacía.
  expect(buildQuerySpec(queryForm({ kind: 'gateway', dataset: 'cosas' }), 'nuevo')).toEqual({
    gateway: { spec: { dataset: 'cosas' } },
  });
  expect(buildQuerySpec(queryForm({ kind: 'gateway' }), 'nuevo')).toBeNull();
});

test('sanitizeQuerySpec pasarela: conserva la spec opaca, descarta lo que no es objeto', () => {
  expect(sanitizeQuerySpec({ gateway: { spec: { jql: 'project = ACME' } } })).toEqual({
    gateway: { spec: { jql: 'project = ACME' } },
  });
  expect(sanitizeQuerySpec({ gateway: {} })).toBeNull();
  expect(sanitizeQuerySpec({ gateway: { spec: 'texto' } })).toBeNull();
});

test('queryErrorAdvice pasarela: no manda revisar credenciales que el usuario no puso', () => {
  const gw = queryErrorAdvice({ error: { code: 'upstream_error' } }, 502, 'gateway');
  expect(gw.fix).toBe('admin');
  expect(gw.action).not.toContain('URL base');
  expect(queryErrorAdvice({ error: { code: 'not_found' } }, 404, 'gateway').fix).toBe('connection');
  expect(queryErrorAdvice({ error: { code: 'timeout' } }, 504, 'gateway').fix).toBe('query');
});

test('buildExecuteBody: modo inline conexión + consulta', () => {
  expect(buildExecuteBody('conn-1', { rest: {} })).toEqual({
    connection_id: 'conn-1',
    query: { rest: {} },
  });
});

/* ---------- querySpecToForm (inversa: consulta guardada → formulario) ---------- */

// Lo que de verdad importa de la inversa es que dé la vuelta completa: si al
// recargar una consulta guardada el formulario no reprodujera la MISMA spec, el
// usuario la reutilizaría creyendo que pide una cosa y pediría otra, en silencio.
test('querySpecToForm: ida y vuelta REST sin perder nada de lo que el formulario pinta', () => {
  const spec = buildQuerySpec(
    queryForm({
      method: 'POST',
      path: '/v1/items',
      params: [
        { key: 'limit', value: '50' },
        { key: 'q', value: 'ñ y espacios' },
      ],
      rootPath: 'data.items',
    }),
  );
  expect(spec).not.toBeNull();
  const form = querySpecToForm(spec!, 'rest');
  expect(form).toEqual(
    queryForm({
      method: 'POST',
      path: '/v1/items',
      params: [
        { key: 'limit', value: '50' },
        { key: 'q', value: 'ñ y espacios' },
      ],
      rootPath: 'data.items',
    }),
  );
  expect(buildQuerySpec(form)).toEqual(spec);
});

test('querySpecToForm: ida y vuelta PostgreSQL y pasarela (claves del driver)', () => {
  const sql = querySpecToForm({ postgres: { sql: 'SELECT 1' } }, 'postgres');
  expect(sql.sql).toBe('SELECT 1');
  expect(buildQuerySpec(sql)).toEqual({ postgres: { sql: 'SELECT 1' } });

  // odata: dataset viaja como `dataset` y la consulta como `filter`.
  const gw = querySpecToForm(
    { gateway: { spec: { dataset: 'Products', filter: "Country eq 'ES'" } } },
    'gateway',
    'odata',
  );
  expect(gw.dataset).toBe('Products');
  expect(gw.gatewayQuery).toBe("Country eq 'ES'");
  expect(buildQuerySpec(gw, 'odata')).toEqual({
    gateway: { spec: { dataset: 'Products', filter: "Country eq 'ES'" } },
  });
});

test('querySpecToForm: una spec corrupta da formulario vacío, no campos con basura', () => {
  const form = querySpecToForm(
    { rest: { method: 42, path: null, query: { ok: 'sí', malo: 7 } } } as never,
    'rest',
  );
  expect(form.method).toBe('GET');
  expect(form.path).toBe('');
  expect(form.params).toEqual([{ key: 'ok', value: 'sí' }]);
});

/* ---------- parseConnections / parseExecuteResult / parseSavedQueries ---------- */

test('parseSavedQueries: descarta lo que no trae id, nombre y spec', () => {
  expect(
    parseSavedQueries({
      queries: [
        { id: 'q1', name: 'Ventas', spec: { rest: { path: '/v1/sales' } } },
        { id: 'q2', name: 'Sin spec' },
        { name: 'Sin id', spec: {} },
        'basura',
      ],
    }),
  ).toEqual([{ id: 'q1', name: 'Ventas', spec: { rest: { path: '/v1/sales' } } }]);
  expect(parseSavedQueries(null)).toEqual([]);
  expect(parseSavedQueries({ queries: 'no es lista' })).toEqual([]);
});

test('parseConnections: extrae y sanea; descarta lo inválido', () => {
  const conns = parseConnections({
    connections: [
      { id: 'a', name: 'API', kind: 'rest', config: { base_url: 'x' }, has_credentials: true },
      { id: 'b', name: 'DB', kind: 'postgres', config: {}, has_credentials: false },
      { id: 'c', name: 'Malo', kind: 'ftp' }, // kind no soportado → fuera
      { name: 'sin-id' }, // sin id → fuera
    ],
  });
  expect(conns.map((c) => c.id)).toEqual(['a', 'b']);
  expect(conns[0]!.has_credentials).toBe(true);
});

test('parseConnections: cuerpo no-lista → []', () => {
  expect(parseConnections(null)).toEqual([]);
  expect(parseConnections({})).toEqual([]);
});

test('parseExecuteResult: valida columns/rows; null si no encaja', () => {
  const ok = parseExecuteResult({
    columns: [{ name: 'x', type: 'number' }],
    rows: [[1], [2]],
  });
  expect(ok).toEqual({ columns: [{ name: 'x', type: 'number' }], rows: [[1], [2]] });
  expect(parseExecuteResult({ columns: 'x', rows: [] })).toBeNull();
  expect(parseExecuteResult({ columns: [{ type: 't' }], rows: [] })).toBeNull(); // columna sin name
  expect(parseExecuteResult({ columns: [], rows: [42] })).toBeNull(); // fila no-array
  expect(parseExecuteResult(null)).toBeNull();
});

test('parseExecuteResult: tipo de columna ausente → cadena vacía', () => {
  const res = parseExecuteResult({ columns: [{ name: 'c' }], rows: [] });
  expect(res?.columns[0]).toEqual({ name: 'c', type: '' });
});

/* ---------- sanitizeQuerySpec ---------- */

test('sanitizeQuerySpec: REST conserva campos conocidos y descarta basura', () => {
  const spec = sanitizeQuerySpec({
    rest: {
      method: 'POST',
      path: '/x',
      query: { a: '1', bad: 2 },
      headers: { 'X-H': 'v' },
      root_path: 'd',
      body: { q: 1 },
      extra: 'fuera',
    },
  });
  expect(spec).toEqual({
    rest: {
      method: 'POST',
      path: '/x',
      query: { a: '1' },
      headers: { 'X-H': 'v' },
      root_path: 'd',
      body: { q: 1 },
    },
  });
});

test('sanitizeQuerySpec: método distinto de POST se descarta (GET implícito)', () => {
  expect(sanitizeQuerySpec({ rest: { method: 'DELETE', path: '/x' } })).toEqual({
    rest: { path: '/x' },
  });
});

test('sanitizeQuerySpec: Postgres exige sql; params solo escalares', () => {
  expect(
    sanitizeQuerySpec({ postgres: { sql: 'SELECT 1', params: [1, 'a', true, { x: 1 }] } }),
  ).toEqual({ postgres: { sql: 'SELECT 1', params: [1, 'a', true] } });
  expect(sanitizeQuerySpec({ postgres: { sql: '' } })).toBeNull();
  expect(sanitizeQuerySpec({})).toBeNull();
  expect(sanitizeQuerySpec(null)).toBeNull();
});

/* ---------- sanitizeExternalTable ---------- */

test('sanitizeExternalTable: entrada válida se conserva', () => {
  const et = sanitizeExternalTable({
    table: 'Tabla1',
    connectionId: 'conn-1',
    anchor: 'B2',
    spec: { postgres: { sql: 'SELECT 1' } },
    refreshedAt: '2026-07-20T10:00:00Z',
  });
  expect(et).toEqual({
    table: 'Tabla1',
    connectionId: 'conn-1',
    anchor: 'B2',
    spec: { postgres: { sql: 'SELECT 1' } },
    refreshedAt: '2026-07-20T10:00:00Z',
  });
});

test('sanitizeExternalTable: rechaza ancla no-A1, tabla/conn vacías y spec inválida', () => {
  const base = {
    table: 'T',
    connectionId: 'c',
    anchor: 'A1',
    spec: { rest: {} },
  };
  expect(sanitizeExternalTable({ ...base, anchor: 'ZZ' })).toBeNull();
  expect(sanitizeExternalTable({ ...base, anchor: 'a1' })).toBeNull(); // minúsculas no
  expect(sanitizeExternalTable({ ...base, table: '  ' })).toBeNull();
  expect(sanitizeExternalTable({ ...base, connectionId: '' })).toBeNull();
  expect(sanitizeExternalTable({ ...base, spec: { nope: 1 } })).toBeNull();
  expect(sanitizeExternalTable(null)).toBeNull();
});

test('sanitizeExternalTable: refreshedAt ausente no se inventa', () => {
  const et = sanitizeExternalTable({
    table: 'T',
    connectionId: 'c',
    anchor: 'A1',
    spec: { rest: {} },
  });
  expect(et?.refreshedAt).toBeUndefined();
});

/* ---------- errores ---------- */

test('errorMessageFrom: prefiere message del backend; cae al código; genérico', () => {
  expect(errorMessageFrom({ error: { code: 'invalid', message: 'SQL no válido' } })).toBe(
    'SQL no válido',
  );
  expect(errorMessageFrom({ error: { code: 'not_found' } })).toBe(
    dataQueryErrorMessage('not_found'),
  );
  expect(errorMessageFrom({ error: 'upstream_unreachable' })).toBe(
    dataQueryErrorMessage('upstream_unreachable'),
  );
  expect(errorMessageFrom(null, 'timeout')).toBe(dataQueryErrorMessage('timeout'));
  // En inglés: sin `document`, `editorT()` cae al idioma base.
  expect(errorMessageFrom('basura')).toBe('The operation could not be completed.');
});

test('queryErrorAdvice: el fallo se traduce a una acción, y la acción depende del origen', () => {
  // upstream_error no dice qué pasó (el backend no filtra la respuesta ajena):
  // se nombran las causas reales y se lleva al usuario a la credencial.
  // Los textos salen en inglés: sin `document`, `editorT()` cae al idioma base.
  const rest = queryErrorAdvice({ error: { code: 'upstream_error' } }, 502, 'rest');
  expect(rest.fix).toBe('credential');
  expect(rest.action).toContain('credential');
  const pg = queryErrorAdvice({ error: { code: 'upstream_error' } }, 502, 'postgres');
  expect(pg.action).toContain('read permission');
  expect(pg.title).not.toBe(rest.title);

  // Códigos con salida propia.
  expect(queryErrorAdvice({ error: 'unauthenticated' }, 401, 'rest').fix).toBe('session');
  expect(queryErrorAdvice({ error: { code: 'not_found' } }, 404, 'rest').fix).toBe('connection');
  expect(queryErrorAdvice({ error: { code: 'timeout' } }, 504, 'postgres').fix).toBe('query');
  expect(queryErrorAdvice({ error: { code: 'encryption_unavailable' } }, 400, 'rest').fix).toBe(
    'admin',
  );

  // 400 sin código: se respeta el mensaje del backend (ya es específico y en español).
  expect(queryErrorAdvice({ error: { message: 'SQL no válido' } }, 400, 'postgres').title).toBe(
    'SQL no válido',
  );

  // Sin cuerpo (500 de PostgreSQL): no se inventa la causa, se nombran las probables.
  const unknown = queryErrorAdvice(null, 500, 'postgres');
  expect(unknown.action).toContain('Lo más habitual');
});

test('connection_owner_only y can_edit: traduce el código e informa si la conexión es editable', () => {
  expect(
    errorMessageFrom({
      error: {
        code: 'connection_owner_only',
        message:
          'Only whoever created this connection or an organisation administrator can change or delete it.',
      },
    }),
  ).toBe(
    'Only the person who created this connection or an organisation administrator can change or delete it.',
  );
  expect(errorMessageFrom({ error: 'connection_owner_only' })).toBe(
    dataQueryErrorMessage('connection_owner_only'),
  );

  const ajena = parseConnection({
    connection: {
      id: 'c1',
      name: 'ERP',
      kind: 'rest',
      config: { base_url: 'https://api.example.com/v1', auth: { type: 'bearer' } },
      has_credentials: true,
      can_edit: false,
    },
  });
  expect(ajena?.can_edit).toBe(false);
  expect(canEditConnection(ajena)).toBe(false);

  const propia = parseConnection({
    connection: {
      id: 'c2',
      name: 'Propia',
      kind: 'rest',
      config: { base_url: 'https://api.example.com/v1', auth: { type: 'bearer' } },
      has_credentials: true,
      can_edit: true,
    },
  });
  expect(propia?.can_edit).toBe(true);
  expect(canEditConnection(propia)).toBe(true);
  expect(canEditConnection(null)).toBe(false);
  expect(canEditConnection(undefined)).toBe(false);
});
