/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El origen «Sytue Bases» de los datos externos de Calcus (dataquery.ts): la conexión
 * solo lleva el token como credencial, la consulta está vacía, la procedencia que declara
 * Bases se valida y se guarda con la tabla, y una conexión revocada se cuenta como tal.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/dataquery-bases.test.ts
 */
import { readFileSync } from 'node:fs';
import { expect, test } from 'bun:test';
import {
  buildConnectionBody,
  buildQuerySpec,
  kindOfSpec,
  literalDeDato,
  parseConnections,
  parseExecuteResult,
  queryErrorAdvice,
  sanitizeExternalTable,
  sanitizeProvenance,
  sanitizeQuerySpec,
  type ConnectionForm,
  type QueryForm,
} from '../src/editor/dataquery';

const form = (patch: Partial<ConnectionForm>): ConnectionForm => ({
  name: '',
  kind: 'bases',
  instanceId: '',
  baseUrl: '',
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
  ...patch,
});

test('la conexión de Bases solo lleva el nombre y el token como credencial, sin host', () => {
  const body = buildConnectionBody(
    form({ name: ' Ventas ', secret: ' sbc_abc ', baseUrl: 'https://atacante.example' }),
  );
  expect(body).toEqual({
    name: 'Ventas',
    kind: 'bases',
    config: {},
    credentials: { secret: 'sbc_abc' },
  });
});

test('la consulta de Bases está vacía y se reconoce al guardarla en el documento', () => {
  const q: QueryForm = {
    kind: 'bases',
    method: 'GET',
    path: '/otra/ruta',
    params: [],
    rootPath: '',
    sql: 'select 1',
    dataset: '',
    gatewayQuery: '',
  };
  expect(buildQuerySpec(q)).toEqual({ bases: {} });
  expect(sanitizeQuerySpec({ bases: { tabla: 'otra' } })).toEqual({ bases: {} });
  expect(kindOfSpec({ bases: {} })).toBe('bases');
  expect(kindOfSpec({ rest: {} })).toBe('rest');
  expect(
    parseConnections({ connections: [{ id: 'c', name: 'n', kind: 'bases', config: {} }] }),
  ).toHaveLength(1);
});

const PROCEDENCIA = {
  source: 'sytue-bases',
  base_name: 'Tienda',
  table_name: 'Pedidos',
  view_name: 'Grandes',
  granted_by: 'ana',
  granted_at: '2026-10-04T10:00:00Z',
  version: 2,
  token: 'no-debe-pasar',
};

test('la procedencia de Bases se valida: solo lo declarado, sin campos de más', () => {
  const r = parseExecuteResult({
    columns: [{ name: 'a', type: 'text' }],
    rows: [['x']],
    provenance: PROCEDENCIA,
  });
  expect(r?.provenance).toEqual({
    source: 'sytue-bases',
    baseName: 'Tienda',
    tableName: 'Pedidos',
    viewName: 'Grandes',
    grantedBy: 'ana',
    grantedAt: '2026-10-04T10:00:00Z',
    version: 2,
  });
  // De otro origen, sin base o con tipos raros: no hay procedencia (y la tabla sigue).
  expect(sanitizeProvenance({ ...PROCEDENCIA, source: 'otro' })).toBeUndefined();
  expect(sanitizeProvenance({ source: 'sytue-bases', table_name: 'T' })).toBeUndefined();
  expect(sanitizeProvenance('texto')).toBeUndefined();
  expect(parseExecuteResult({ columns: [], rows: [], provenance: 7 })?.provenance).toBeUndefined();
  // Un nombre enorme se recorta: va a parar al documento.
  expect(
    sanitizeProvenance({ ...PROCEDENCIA, base_name: 'x'.repeat(5000) })?.baseName,
  ).toHaveLength(200);
});

test('la tabla externa guarda la procedencia y la conserva al releer el documento', () => {
  const guardada = sanitizeExternalTable({
    table: 'Pedidos',
    connectionId: 'c1',
    anchor: 'A1',
    spec: { bases: {} },
    provenance: sanitizeProvenance(PROCEDENCIA),
  });
  expect(guardada?.provenance?.tableName).toBe('Pedidos');
  expect(sanitizeExternalTable(JSON.parse(JSON.stringify(guardada)))).toEqual(guardada);
});

test('una conexión de Bases revocada pide un token nuevo, no revisar la consulta', () => {
  const a = queryErrorAdvice({ error: { code: 'bad_request' } }, 400, 'bases');
  expect(a.fix).toBe('credential');
  // Lo demás (sesión caducada) sigue el camino común.
  expect(queryErrorAdvice({ error: { code: 'unauthenticated' } }, 401, 'bases').fix).toBe(
    'session',
  );
});

/* Inyección de fórmulas: un registro de Bases puede venir de un formulario PÚBLICO, así
 * que un «=…» en un valor o en el nombre de una columna no puede acabar evaluándose en
 * la hoja de quien refresca la tabla. */
test('un dato externo que empieza por «=» se guarda como texto, no como fórmula', () => {
  expect(literalDeDato('=HYPERLINK("https://malo.example";"ver")')).toBe(
    `'=HYPERLINK("https://malo.example";"ver")`,
  );
  // La celda recorta los espacios al re-renderizarse: con espacios delante también.
  expect(literalDeDato('  =1+1')).toBe(`'  =1+1`);
  expect(literalDeDato('\t=A1')).toBe(`'\t=A1`);
  // Lo que no es una fórmula no se toca.
  for (const normal of ['Ana', '-5', '+34 600', '@ana', 'a=b', '1200', ''])
    expect(literalDeDato(normal)).toBe(normal);
});

test('la hoja materializa los datos externos pasando por literalDeDato', () => {
  const fuente = readFileSync(
    new URL('../src/editor/sheets/SheetCanvas.tsx', import.meta.url),
    'utf8',
  );
  const volcado = fuente.slice(fuente.indexOf('// Vuelca la rejilla (texto escapado'));
  expect(volcado.slice(0, 600)).toContain('cells[ref] = escapeHtml(literalDeDato(txt));');
});
