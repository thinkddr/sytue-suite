/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test EXHAUSTIVO del motor de transformación tabular (querytransform.ts):
 * cada tipo de paso, sus errores por columna/expresión y el orden del pipeline.
 * Además el saneado del pipeline persistido (sanitizeTransform, en dataquery.ts).
 * Puro, sin DOM. Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/querytransform.test.ts
 */
import { expect, test } from 'bun:test';
import {
  applyStep,
  applyTransform,
  describeStep,
  runPipeline,
  type QueryResolver,
  type QueryTable,
} from '../src/editor/sheets/querytransform';
import { gridToQueryTable, sanitizeTransform } from '../src/editor/dataquery';
import type { QueryStep } from '../src/editor/types';

/** Tabla base de pruebas (4 filas, tipos number/text). */
const base = (): QueryTable => ({
  columns: [
    { name: 'id', type: 'number' },
    { name: 'nombre', type: 'text' },
    { name: 'precio', type: 'number' },
    { name: 'ciudad', type: 'text' },
  ],
  rows: [
    [1, 'Ada', 100, 'Madrid'],
    [2, 'Bob', 200, 'Madrid'],
    [3, 'Cid', 150, 'Sevilla'],
    [4, 'Dan', 200, 'Sevilla'],
  ],
});

const names = (t: QueryTable): string[] => t.columns.map((c) => c.name);
const col = (t: QueryTable, name: string): unknown[] => {
  const i = t.columns.findIndex((c) => c.name === name);
  return t.rows.map((r) => r[i]);
};

/* ---------- Filtrar filas ---------- */

test('filter: eq / neq sobre texto', () => {
  expect(
    col(applyStep(base(), { kind: 'filter', column: 'ciudad', op: 'eq', value: 'Madrid' }), 'id'),
  ).toEqual([1, 2]);
  expect(
    col(applyStep(base(), { kind: 'filter', column: 'ciudad', op: 'neq', value: 'Madrid' }), 'id'),
  ).toEqual([3, 4]);
});

test('filter: eq numérico coincide entre número y texto', () => {
  // «200» (texto) contra 200 (número) coincide.
  expect(
    col(applyStep(base(), { kind: 'filter', column: 'precio', op: 'eq', value: '200' }), 'id'),
  ).toEqual([2, 4]);
});

test('filter: contains sin distinguir mayúsculas', () => {
  expect(
    col(
      applyStep(base(), { kind: 'filter', column: 'ciudad', op: 'contains', value: 'MAD' }),
      'id',
    ),
  ).toEqual([1, 2]);
});

test('filter: gt / lt / between numéricos', () => {
  expect(
    col(applyStep(base(), { kind: 'filter', column: 'precio', op: 'gt', value: '150' }), 'id'),
  ).toEqual([2, 4]);
  expect(
    col(applyStep(base(), { kind: 'filter', column: 'precio', op: 'lt', value: '150' }), 'id'),
  ).toEqual([1]);
  expect(
    col(
      applyStep(base(), {
        kind: 'filter',
        column: 'precio',
        op: 'between',
        value: '100',
        value2: '150',
      }),
      'id',
    ),
  ).toEqual([1, 3]);
});

test('filter: empty / nonempty', () => {
  const t: QueryTable = {
    columns: [{ name: 'x', type: 'text' }],
    rows: [['a'], [''], [null], ['b']],
  };
  expect(applyStep(t, { kind: 'filter', column: 'x', op: 'empty' }).rows).toEqual([[''], [null]]);
  expect(applyStep(t, { kind: 'filter', column: 'x', op: 'nonempty' }).rows).toEqual([
    ['a'],
    ['b'],
  ]);
});

/* ---------- Columnas: elegir / quitar / renombrar / reordenar ---------- */

test('select: conserva solo y en el orden dado', () => {
  const t = applyStep(base(), { kind: 'select', columns: ['nombre', 'precio'] });
  expect(names(t)).toEqual(['nombre', 'precio']);
  expect(t.rows[0]).toEqual(['Ada', 100]);
});

test('removeColumns: quita las indicadas', () => {
  expect(names(applyStep(base(), { kind: 'removeColumns', columns: ['id', 'ciudad'] }))).toEqual([
    'nombre',
    'precio',
  ]);
});

test('rename: cambia el nombre y conserva los datos', () => {
  const t = applyStep(base(), { kind: 'rename', column: 'id', to: 'ID' });
  expect(names(t)).toEqual(['ID', 'nombre', 'precio', 'ciudad']);
  expect(col(t, 'ID')).toEqual([1, 2, 3, 4]);
});

test('reorder: las indicadas van al frente; el resto conserva su orden', () => {
  expect(names(applyStep(base(), { kind: 'reorder', columns: ['ciudad'] }))).toEqual([
    'ciudad',
    'id',
    'nombre',
    'precio',
  ]);
});

/* ---------- Cambiar tipo ---------- */

test('changeType: number parsea texto es-ES', () => {
  const t: QueryTable = {
    columns: [{ name: 'x', type: 'text' }],
    rows: [['1.234,5'], ['abc'], [10]],
  };
  const out = applyStep(t, { kind: 'changeType', column: 'x', type: 'number' });
  expect(out.columns[0]!.type).toBe('number');
  expect(col(out, 'x')).toEqual([1234.5, null, 10]);
});

test('changeType: boolean reconoce sí/no', () => {
  const t: QueryTable = {
    columns: [{ name: 'x', type: 'text' }],
    rows: [['sí'], ['No'], ['1'], ['quizá']],
  };
  expect(col(applyStep(t, { kind: 'changeType', column: 'x', type: 'boolean' }), 'x')).toEqual([
    true,
    false,
    true,
    null,
  ]);
});

test('changeType: date normaliza a cadena canónica es-ES', () => {
  const t: QueryTable = {
    columns: [{ name: 'f', type: 'text' }],
    rows: [['2024-01-15'], ['no-fecha']],
  };
  const out = applyStep(t, { kind: 'changeType', column: 'f', type: 'date' });
  expect(out.columns[0]!.type).toBe('date');
  expect(col(out, 'f')).toEqual(['15/1/2024', null]);
});

test('changeType: date permite filtrar y ordenar cronológicamente', () => {
  const t: QueryTable = {
    columns: [{ name: 'f', type: 'text' }],
    rows: [['2024-01-10'], ['2024-01-09'], ['2024-03-02']],
  };
  const dated = applyStep(t, { kind: 'changeType', column: 'f', type: 'date' });
  // Orden cronológico (no lexicográfico: «10» iría antes que «9» por texto).
  const asc = applyStep(dated, { kind: 'sort', by: [{ column: 'f', dir: 'asc' }] });
  expect(col(asc, 'f')).toEqual(['9/1/2024', '10/1/2024', '2/3/2024']);
  // Entre fechas: del 1 al 20 de enero → excluye el 2 de marzo.
  const between = applyStep(dated, {
    kind: 'filter',
    column: 'f',
    op: 'between',
    value: '1/1/2024',
    value2: '20/1/2024',
  });
  expect(between.rows.length).toBe(2);
});

test('changeType: text', () => {
  const out = applyStep(base(), { kind: 'changeType', column: 'precio', type: 'text' });
  expect(out.columns[2]!.type).toBe('text');
  expect(col(out, 'precio')).toEqual(['100', '200', '150', '200']);
});

/* ---------- Ordenar ---------- */

test('sort: una columna descendente', () => {
  expect(
    col(applyStep(base(), { kind: 'sort', by: [{ column: 'precio', dir: 'desc' }] }), 'id'),
  ).toEqual([2, 4, 3, 1]);
});

test('sort: multi-columna (ciudad asc, precio desc) es estable', () => {
  const t = applyStep(base(), {
    kind: 'sort',
    by: [
      { column: 'ciudad', dir: 'asc' },
      { column: 'precio', dir: 'desc' },
    ],
  });
  expect(col(t, 'id')).toEqual([2, 1, 4, 3]);
});

/* ---------- Agrupar ---------- */

test('groupBy: suma/contar/promedio/mín/máx', () => {
  const t = applyStep(base(), {
    kind: 'groupBy',
    columns: ['ciudad'],
    aggs: [
      { column: 'precio', fn: 'sum', as: 'total' },
      { column: 'precio', fn: 'count', as: 'n' },
      { column: 'precio', fn: 'avg', as: 'media' },
      { column: 'precio', fn: 'min', as: 'mín' },
      { column: 'precio', fn: 'max', as: 'máx' },
    ],
  });
  expect(names(t)).toEqual(['ciudad', 'total', 'n', 'media', 'mín', 'máx']);
  expect(t.rows).toEqual([
    ['Madrid', 300, 2, 150, 100, 200],
    ['Sevilla', 350, 2, 175, 150, 200],
  ]);
});

test('groupBy: count sin columna cuenta filas', () => {
  const t = applyStep(base(), {
    kind: 'groupBy',
    columns: ['ciudad'],
    aggs: [{ column: '', fn: 'count', as: 'n' }],
  });
  expect(t.rows).toEqual([
    ['Madrid', 2],
    ['Sevilla', 2],
  ]);
});

test('groupBy: claves multi-columna con espacios NO colisionan', () => {
  // Con una unión ingenua por espacio, «x»+«y z» y «x y»+«z» darían la misma
  // clave; deben ser dos grupos distintos.
  const t: QueryTable = {
    columns: [
      { name: 'a', type: 'text' },
      { name: 'b', type: 'text' },
      { name: 'v', type: 'number' },
    ],
    rows: [
      ['x', 'y z', 1],
      ['x y', 'z', 10],
      ['x', 'y z', 100],
    ],
  };
  const g = applyStep(t, {
    kind: 'groupBy',
    columns: ['a', 'b'],
    aggs: [{ column: 'v', fn: 'sum', as: 's' }],
  });
  expect(g.rows).toEqual([
    ['x', 'y z', 101],
    ['x y', 'z', 10],
  ]);
});

/* ---------- Añadir columna ---------- */

test('addColumn: constante numérica y de texto', () => {
  expect(col(applyStep(base(), { kind: 'addColumn', name: 'k', expr: '42' }), 'k')).toEqual([
    42, 42, 42, 42,
  ]);
  const txt = applyStep(base(), { kind: 'addColumn', name: 'k', expr: 'activo' });
  expect(txt.columns.at(-1)!.type).toBe('text');
  expect(col(txt, 'k')).toEqual(['activo', 'activo', 'activo', 'activo']);
});

test('addColumn: interpolación de texto {col}', () => {
  const t = applyStep(base(), { kind: 'addColumn', name: 'etq', expr: '{nombre} de {ciudad}' });
  expect(col(t, 'etq')).toEqual([
    'Ada de Madrid',
    'Bob de Madrid',
    'Cid de Sevilla',
    'Dan de Sevilla',
  ]);
});

test('addColumn: aritmética con precedencia y paréntesis', () => {
  expect(
    col(applyStep(base(), { kind: 'addColumn', name: 'iva', expr: '{precio} * 1,21' }), 'iva'),
  ).toEqual([121, 242, 181.5, 242]);
  expect(
    col(applyStep(base(), { kind: 'addColumn', name: 'y', expr: '({precio} + 100) / 2' }), 'y'),
  ).toEqual([100, 150, 125, 150]);
  expect(
    col(applyStep(base(), { kind: 'addColumn', name: 'y', expr: '-{precio} + {id}' }), 'y'),
  ).toEqual([-99, -198, -147, -196]);
});

test('addColumn: ref numérica inválida → celda vacía (null), no rompe la fila', () => {
  const t: QueryTable = { columns: [{ name: 'x', type: 'text' }], rows: [['5'], ['ab']] };
  expect(col(applyStep(t, { kind: 'addColumn', name: 'y', expr: '{x} * 2' }), 'y')).toEqual([
    10,
    null,
  ]);
});

/* ---------- Reemplazar ---------- */

test('replace: sustituye todas las apariciones y pasa la columna a texto', () => {
  const out = applyStep(base(), {
    kind: 'replace',
    column: 'ciudad',
    find: 'Madrid',
    replace: 'MAD',
  });
  expect(col(out, 'ciudad')).toEqual(['MAD', 'MAD', 'Sevilla', 'Sevilla']);
  expect(out.columns[3]!.type).toBe('text');
});

/* ---------- Quitar duplicados ---------- */

test('dedupe: por todas las columnas y por subconjunto', () => {
  const t: QueryTable = {
    columns: [
      { name: 'a', type: 'text' },
      { name: 'b', type: 'number' },
    ],
    rows: [
      ['x', 1],
      ['x', 1],
      ['x', 2],
      ['y', 2],
    ],
  };
  expect(applyStep(t, { kind: 'dedupe' }).rows).toEqual([
    ['x', 1],
    ['x', 2],
    ['y', 2],
  ]);
  // Solo por «a»: conserva la primera de cada valor.
  expect(applyStep(t, { kind: 'dedupe', columns: ['a'] }).rows).toEqual([
    ['x', 1],
    ['y', 2],
  ]);
});

test('dedupe: claves multi-columna con espacios NO colisionan', () => {
  const t: QueryTable = {
    columns: [
      { name: 'a', type: 'text' },
      { name: 'b', type: 'text' },
    ],
    rows: [
      ['x', 'y z'],
      ['x y', 'z'],
    ],
  };
  expect(applyStep(t, { kind: 'dedupe' }).rows).toEqual([
    ['x', 'y z'],
    ['x y', 'z'],
  ]);
});

/* ---------- Mantener / quitar N filas ---------- */

test('keepRows / removeRows: primeras y últimas', () => {
  expect(col(applyStep(base(), { kind: 'keepRows', end: 'first', count: 2 }), 'id')).toEqual([
    1, 2,
  ]);
  expect(col(applyStep(base(), { kind: 'keepRows', end: 'last', count: 2 }), 'id')).toEqual([3, 4]);
  expect(col(applyStep(base(), { kind: 'removeRows', end: 'first', count: 1 }), 'id')).toEqual([
    2, 3, 4,
  ]);
  expect(col(applyStep(base(), { kind: 'removeRows', end: 'last', count: 1 }), 'id')).toEqual([
    1, 2, 3,
  ]);
  // count fuera de rango se acota.
  expect(applyStep(base(), { kind: 'keepRows', end: 'first', count: 99 }).rows.length).toBe(4);
  expect(applyStep(base(), { kind: 'removeRows', end: 'first', count: 99 }).rows.length).toBe(0);
});

/* ---------- Pureza (no muta la entrada) ---------- */

test('applyStep no muta la tabla de entrada', () => {
  const t = base();
  applyStep(t, { kind: 'filter', column: 'id', op: 'gt', value: '2' });
  applyStep(t, { kind: 'rename', column: 'id', to: 'X' });
  expect(t.rows.length).toBe(4);
  expect(names(t)).toEqual(['id', 'nombre', 'precio', 'ciudad']);
});

/* ---------- Errores por paso (columna inexistente, etc.) ---------- */

test('errores: columna inexistente en cada paso relevante', () => {
  const t = base();
  const bad: QueryStep[] = [
    { kind: 'filter', column: 'zzz', op: 'eq', value: 'x' },
    { kind: 'select', columns: ['zzz'] },
    { kind: 'removeColumns', columns: ['zzz'] },
    { kind: 'rename', column: 'zzz', to: 'q' },
    { kind: 'reorder', columns: ['zzz'] },
    { kind: 'changeType', column: 'zzz', type: 'number' },
    { kind: 'sort', by: [{ column: 'zzz', dir: 'asc' }] },
    { kind: 'groupBy', columns: ['zzz'], aggs: [{ column: 'precio', fn: 'sum', as: 't' }] },
    { kind: 'replace', column: 'zzz', find: 'a', replace: 'b' },
    { kind: 'dedupe', columns: ['zzz'] },
  ];
  for (const step of bad) {
    expect(() => applyStep(t, step)).toThrow('La columna «zzz» no existe.');
  }
});

test('errores: rename vacío, addColumn sin nombre y ref inexistente', () => {
  expect(() => applyStep(base(), { kind: 'rename', column: 'id', to: '  ' })).toThrow('vacío');
  expect(() => applyStep(base(), { kind: 'addColumn', name: '', expr: '1' })).toThrow('vacío');
  expect(() => applyStep(base(), { kind: 'addColumn', name: 'k', expr: '{zzz} + 1' })).toThrow(
    'La columna «zzz» no existe.',
  );
  expect(() => applyStep(base(), { kind: 'addColumn', name: 'k', expr: '{precio} +' })).toThrow();
});

/* ---------- Pipeline: orden, parada en error, retrocompat ---------- */

test('runPipeline: aplica en orden (filtra y luego agrupa)', () => {
  const { table, error } = runPipeline(base(), [
    { kind: 'filter', column: 'ciudad', op: 'eq', value: 'Madrid' },
    { kind: 'groupBy', columns: ['ciudad'], aggs: [{ column: 'precio', fn: 'sum', as: 'total' }] },
  ]);
  expect(error).toBeNull();
  expect(table.rows).toEqual([['Madrid', 300]]);
});

test('runPipeline: se detiene en el paso que falla y señala su índice', () => {
  const { table, error } = runPipeline(base(), [
    { kind: 'keepRows', end: 'first', count: 2 },
    { kind: 'filter', column: 'zzz', op: 'eq', value: 'x' },
    { kind: 'removeColumns', columns: ['id'] },
  ]);
  expect(error?.index).toBe(1);
  expect(error?.message).toContain('zzz');
  expect(table.rows.length).toBe(2); // resultado tras el paso 0 (previo al fallo)
  expect(names(table)).toEqual(['id', 'nombre', 'precio', 'ciudad']); // el paso 2 no llegó a correr
});

test('applyTransform: sin pasos = datos crudos; con error = entrada intacta', () => {
  expect(applyTransform(base(), [])).toEqual(base());
  const out = applyTransform(base(), [{ kind: 'filter', column: 'zzz', op: 'eq', value: 'x' }]);
  expect(out).toEqual(base()); // fallback al crudo, la hoja nunca queda a medias
});

/* ---------- describeStep ---------- */

test('describeStep: etiquetas legibles', () => {
  expect(describeStep({ kind: 'filter', column: 'precio', op: 'gt', value: '10' })).toBe(
    'Filtrar «precio» > 10',
  );
  expect(describeStep({ kind: 'changeType', column: 'f', type: 'date' })).toBe(
    'Tipo de «f» → fecha',
  );
  expect(describeStep({ kind: 'sort', by: [{ column: 'a', dir: 'asc' }] })).toBe('Ordenar por a ↑');
  expect(
    describeStep({
      kind: 'groupBy',
      columns: ['ciudad'],
      aggs: [{ column: 'precio', fn: 'sum', as: 'total' }],
    }),
  ).toContain('Agrupar por ciudad');
});

/* ---------- sanitizeTransform (frontera de confianza) ---------- */

test('sanitizeTransform: descarta pasos malformados y conserva los válidos', () => {
  const raw = [
    { kind: 'filter', column: 'a', op: 'eq', value: 'x' }, // ok
    { kind: 'filter', column: 'a', op: 'noexiste' }, // op inválido → fuera
    { kind: 'select', columns: [] }, // sin columnas → fuera
    { kind: 'select', columns: ['a', 'b', 3] }, // 3 no es string → se filtra, quedan a,b
    { kind: 'desconocido' }, // kind inválido → fuera
    { kind: 'keepRows', end: 'last', count: 5 }, // ok
    { kind: 'keepRows', end: 'first', count: 'x' }, // count no numérico → fuera
    'no-objeto', // fuera
  ];
  const out = sanitizeTransform(raw);
  expect(out.map((s) => s.kind)).toEqual(['filter', 'select', 'keepRows']);
  expect(out[1]).toEqual({ kind: 'select', columns: ['a', 'b'] });
});

test('sanitizeTransform: no-array → []', () => {
  expect(sanitizeTransform(null)).toEqual([]);
  expect(sanitizeTransform({ kind: 'filter' })).toEqual([]);
});

/* ==================================================================
 * Combinar / reestructurar: unpivot · pivot · append · merge
 * ================================================================== */

/** Tabla ancha para despivotar (region + tres meses numéricos). */
const wide = (): QueryTable => ({
  columns: [
    { name: 'region', type: 'text' },
    { name: 'ene', type: 'number' },
    { name: 'feb', type: 'number' },
    { name: 'mar', type: 'number' },
  ],
  rows: [
    ['Norte', 1, 2, 3],
    ['Sur', 4, 5, 6],
  ],
});

/** Tabla larga para pivotar (producto/mes/ventas, con una clave repetida y un hueco). */
const long = (): QueryTable => ({
  columns: [
    { name: 'producto', type: 'text' },
    { name: 'mes', type: 'text' },
    { name: 'ventas', type: 'number' },
  ],
  rows: [
    ['A', 'ene', 10],
    ['A', 'feb', 20],
    ['B', 'ene', 5],
    ['A', 'ene', 3], // segunda A/ene → agrega con la primera
  ],
});

/** Izquierda de merge/append (id + nombre). */
const leftT = (): QueryTable => ({
  columns: [
    { name: 'id', type: 'number' },
    { name: 'nombre', type: 'text' },
  ],
  rows: [
    [1, 'Ada'],
    [2, 'Bob'],
    [3, 'Cid'],
  ],
});

/** Derecha de merge (uid + ciudad; uid=2 aparece dos veces → multi-match, uid=4 sin par). */
const rightT = (): QueryTable => ({
  columns: [
    { name: 'uid', type: 'number' },
    { name: 'ciudad', type: 'text' },
  ],
  rows: [
    [1, 'Madrid'],
    [2, 'Sevilla'],
    [2, 'Bilbao'],
    [4, 'Vigo'],
  ],
});

/** Resolutor fijo: mapea la referencia «otra» a la tabla dada. */
const fixed =
  (t: QueryTable): QueryResolver =>
  (ref) =>
    ref === 'otra' ? t : undefined;

/* ---------- Despivotar (columnas → filas) ---------- */

test('unpivot: colapsa columnas atributo repitiendo las ancla', () => {
  const u = applyStep(wide(), {
    kind: 'unpivot',
    columns: ['ene', 'feb', 'mar'],
    attribute: 'mes',
    value: 'ventas',
  });
  expect(names(u)).toEqual(['region', 'mes', 'ventas']);
  expect(u.columns[2]!.type).toBe('number'); // tipos uniformes → hereda 'number'
  expect(u.rows).toEqual([
    ['Norte', 'ene', 1],
    ['Norte', 'feb', 2],
    ['Norte', 'mar', 3],
    ['Sur', 'ene', 4],
    ['Sur', 'feb', 5],
    ['Sur', 'mar', 6],
  ]);
});

test('unpivot: tipos dispares → columna valor de tipo texto; conserva vacíos', () => {
  const t: QueryTable = {
    columns: [
      { name: 'k', type: 'text' },
      { name: 'n', type: 'number' },
      { name: 't', type: 'text' },
    ],
    rows: [['r', 5, 'hola']],
  };
  const u = applyStep(t, { kind: 'unpivot', columns: ['n', 't'], attribute: 'attr', value: 'val' });
  expect(u.columns[2]).toEqual({ name: 'val', type: 'text' });
  expect(u.rows).toEqual([
    ['r', 'n', 5],
    ['r', 't', 'hola'],
  ]);
});

test('unpivot: sin columnas o con nombres vacíos → error', () => {
  expect(() =>
    applyStep(wide(), { kind: 'unpivot', columns: [], attribute: 'a', value: 'b' }),
  ).toThrow('al menos una columna');
  expect(() =>
    applyStep(wide(), { kind: 'unpivot', columns: ['ene'], attribute: '  ', value: 'b' }),
  ).toThrow('vacíos');
});

/* ---------- Pivotar (filas → columnas) ---------- */

test('pivot: suma con clave repetida y celda ausente (null)', () => {
  const p = applyStep(long(), { kind: 'pivot', key: 'mes', value: 'ventas', fn: 'sum' });
  expect(names(p)).toEqual(['producto', 'ene', 'feb']); // orden de aparición: ene, feb
  expect(p.rows).toEqual([
    ['A', 13, 20], // A/ene = 10+3, A/feb = 20
    ['B', 5, null], // B/ene = 5, B/feb ausente → vacío
  ]);
});

test('pivot: agregación primero / contar', () => {
  expect(
    applyStep(long(), { kind: 'pivot', key: 'mes', value: 'ventas', fn: 'first' }).rows,
  ).toEqual([
    ['A', 10, 20],
    ['B', 5, null],
  ]);
  expect(
    applyStep(long(), { kind: 'pivot', key: 'mes', value: 'ventas', fn: 'count' }).rows,
  ).toEqual([
    ['A', 2, 1],
    ['B', 1, null],
  ]);
});

test('pivot: clave = valor → error', () => {
  expect(() => applyStep(long(), { kind: 'pivot', key: 'mes', value: 'mes', fn: 'sum' })).toThrow(
    'distintas',
  );
});

test('unpivot → pivot (ida y vuelta) reconstruye la tabla ancha', () => {
  const u = applyStep(wide(), {
    kind: 'unpivot',
    columns: ['ene', 'feb', 'mar'],
    attribute: 'mes',
    value: 'ventas',
  });
  const back = applyStep(u, { kind: 'pivot', key: 'mes', value: 'ventas', fn: 'first' });
  expect(back).toEqual(wide());
});

/* ---------- Anexar (unión de filas por nombre) ---------- */

test('append: alinea por nombre; columnas dispares → null', () => {
  const A: QueryTable = {
    columns: [
      { name: 'a', type: 'number' },
      { name: 'b', type: 'text' },
    ],
    rows: [[1, 'x']],
  };
  const B: QueryTable = {
    columns: [
      { name: 'b', type: 'text' },
      { name: 'c', type: 'number' },
    ],
    rows: [['y', 9]],
  };
  const out = applyStep(A, { kind: 'append', from: 'otra' }, fixed(B));
  expect(names(out)).toEqual(['a', 'b', 'c']); // unión: a, b, (c nueva de B)
  expect(out.rows).toEqual([
    [1, 'x', null], // fila de A: c ausente
    [null, 'y', 9], // fila de B: a ausente, b alineada por nombre
  ]);
});

/* ---------- Combinar (merge / join) ---------- */

test('merge inner: solo coincidencias, multi-match = producto', () => {
  const out = applyStep(
    leftT(),
    {
      kind: 'merge',
      from: 'otra',
      join: 'inner',
      on: [{ left: 'id', right: 'uid' }],
      bring: ['ciudad'],
    },
    fixed(rightT()),
  );
  expect(names(out)).toEqual(['id', 'nombre', 'ciudad']);
  expect(out.rows).toEqual([
    [1, 'Ada', 'Madrid'],
    [2, 'Bob', 'Sevilla'],
    [2, 'Bob', 'Bilbao'], // id 2 casa dos filas → producto
  ]);
});

test('merge left: añade las filas izquierdas sin par con columnas traídas a null', () => {
  const out = applyStep(
    leftT(),
    {
      kind: 'merge',
      from: 'otra',
      join: 'left',
      on: [{ left: 'id', right: 'uid' }],
      bring: ['ciudad'],
    },
    fixed(rightT()),
  );
  expect(out.rows).toEqual([
    [1, 'Ada', 'Madrid'],
    [2, 'Bob', 'Sevilla'],
    [2, 'Bob', 'Bilbao'],
    [3, 'Cid', null], // sin par
  ]);
});

test('merge right: todas las derechas; izquierda a null en las sobrantes', () => {
  const out = applyStep(
    leftT(),
    {
      kind: 'merge',
      from: 'otra',
      join: 'right',
      on: [{ left: 'id', right: 'uid' }],
      bring: ['ciudad'],
    },
    fixed(rightT()),
  );
  expect(out.rows).toEqual([
    [1, 'Ada', 'Madrid'],
    [2, 'Bob', 'Sevilla'],
    [2, 'Bob', 'Bilbao'],
    [null, null, 'Vigo'], // uid 4 sin par a la izquierda
  ]);
});

test('merge full: unión de left y right', () => {
  const out = applyStep(
    leftT(),
    {
      kind: 'merge',
      from: 'otra',
      join: 'full',
      on: [{ left: 'id', right: 'uid' }],
      bring: ['ciudad'],
    },
    fixed(rightT()),
  );
  expect(out.rows).toEqual([
    [1, 'Ada', 'Madrid'],
    [2, 'Bob', 'Sevilla'],
    [2, 'Bob', 'Bilbao'],
    [3, 'Cid', null], // izquierda sin par
    [null, null, 'Vigo'], // derecha sin par
  ]);
});

test('merge antiLeft: solo izquierdas sin par; no trae columnas', () => {
  const out = applyStep(
    leftT(),
    {
      kind: 'merge',
      from: 'otra',
      join: 'antiLeft',
      on: [{ left: 'id', right: 'uid' }],
      bring: ['ciudad'],
    },
    fixed(rightT()),
  );
  expect(names(out)).toEqual(['id', 'nombre']); // antiLeft = filtro, sin columnas traídas
  expect(out.rows).toEqual([[3, 'Cid']]);
});

test('merge: bring vacío = todas las columnas de la derecha menos las clave', () => {
  const out = applyStep(
    leftT(),
    { kind: 'merge', from: 'otra', join: 'inner', on: [{ left: 'id', right: 'uid' }], bring: [] },
    fixed(rightT()),
  );
  expect(names(out)).toEqual(['id', 'nombre', 'ciudad']); // uid es clave → excluida
});

test('merge: colisión de nombres → prefijo, o sufijo « (2)» sin prefijo', () => {
  const rightName = (): QueryTable => ({
    columns: [
      { name: 'uid', type: 'number' },
      { name: 'nombre', type: 'text' },
    ],
    rows: [
      [1, 'Madrid'],
      [2, 'Sevilla'],
    ],
  });
  const withPrefix = applyStep(
    leftT(),
    {
      kind: 'merge',
      from: 'otra',
      join: 'inner',
      on: [{ left: 'id', right: 'uid' }],
      bring: ['nombre'],
      prefix: 'der.',
    },
    fixed(rightName()),
  );
  expect(names(withPrefix)).toEqual(['id', 'nombre', 'der.nombre']);
  expect(withPrefix.rows).toEqual([
    [1, 'Ada', 'Madrid'],
    [2, 'Bob', 'Sevilla'],
  ]);
  const noPrefix = applyStep(
    leftT(),
    {
      kind: 'merge',
      from: 'otra',
      join: 'inner',
      on: [{ left: 'id', right: 'uid' }],
      bring: ['nombre'],
    },
    fixed(rightName()),
  );
  expect(names(noPrefix)).toEqual(['id', 'nombre', 'nombre (2)']);
});

test('merge: la clave casa número con su texto (comparación por texto de celda)', () => {
  const right: QueryTable = {
    columns: [
      { name: 'uid', type: 'text' },
      { name: 'ciudad', type: 'text' },
    ],
    rows: [['2', 'Sevilla']], // uid como TEXTO «2»
  };
  const out = applyStep(
    leftT(),
    {
      kind: 'merge',
      from: 'otra',
      join: 'inner',
      on: [{ left: 'id', right: 'uid' }],
      bring: ['ciudad'],
    },
    fixed(right),
  );
  expect(out.rows).toEqual([[2, 'Bob', 'Sevilla']]); // 2 (número) ↔ «2» (texto)
});

test('merge: columna clave inexistente en la otra tabla → error claro', () => {
  expect(() =>
    applyStep(
      leftT(),
      { kind: 'merge', from: 'otra', join: 'inner', on: [{ left: 'id', right: 'zzz' }], bring: [] },
      fixed(rightT()),
    ),
  ).toThrow('«zzz» no existe en la otra tabla');
});

/* ---------- Resolutor: ausente, no encontrada, ciclo ---------- */

test('append/merge sin resolutor → error de paso claro (falta el resolutor)', () => {
  const r1 = runPipeline(leftT(), [{ kind: 'append', from: 'otra' }]);
  expect(r1.error?.index).toBe(0);
  expect(r1.error?.message).toContain('resolutor');
  const r2 = runPipeline(leftT(), [
    { kind: 'merge', from: 'otra', join: 'inner', on: [{ left: 'id', right: 'uid' }], bring: [] },
  ]);
  expect(r2.error?.message).toContain('resolutor');
});

test('append: tabla no encontrada por el resolutor → error claro', () => {
  const r = runPipeline(leftT(), [{ kind: 'append', from: 'zzz' }], () => undefined);
  expect(r.error?.message).toContain('no existe');
});

test('resolutor transitivo: ciclo A→B→A → error (termina, no cuelga)', () => {
  const defs: Record<string, { input: QueryTable; steps: QueryStep[] }> = {
    A: {
      input: { columns: [{ name: 'v', type: 'number' }], rows: [[1]] },
      steps: [{ kind: 'append', from: 'B' }],
    },
    B: {
      input: { columns: [{ name: 'v', type: 'number' }], rows: [[2]] },
      steps: [{ kind: 'append', from: 'A' }],
    },
  };
  // Resolutor que RE-EJECUTA el pipeline de la otra tabla (transitivo) con guardia
  // de ciclos: volver a pedir una tabla en curso de resolución → lanza.
  const visiting = new Set<string>();
  const resolve: QueryResolver = (ref) => {
    const def = defs[ref];
    if (!def) return undefined;
    if (visiting.has(ref)) throw new Error(`Ciclo de tablas detectado en «${ref}».`);
    visiting.add(ref);
    try {
      const { table, error } = runPipeline(def.input, def.steps, resolve);
      if (error) throw new Error(error.message);
      return table;
    } finally {
      visiting.delete(ref);
    }
  };
  const res = runPipeline(defs.A!.input, defs.A!.steps, resolve);
  expect(res.error).not.toBeNull();
  expect(res.error?.message).toContain('Ciclo');
});

/* ---------- Agregación «primero» reutilizada por group-by ---------- */

test('groupBy: agregación primero (first)', () => {
  const g = applyStep(base(), {
    kind: 'groupBy',
    columns: ['ciudad'],
    aggs: [{ column: 'nombre', fn: 'first', as: 'primero' }],
  });
  expect(g.rows).toEqual([
    ['Madrid', 'Ada'],
    ['Sevilla', 'Cid'],
  ]);
});

/* ---------- gridToQueryTable (instantánea de la hoja → tabla del motor) ---------- */

test('gridToQueryTable: cabecera + filas; rellena filas cortas; vacío → null', () => {
  expect(
    gridToQueryTable([
      ['a', 'b'],
      ['1', 'x'],
      ['2', 'y'],
    ]),
  ).toEqual({
    columns: [
      { name: 'a', type: 'text' },
      { name: 'b', type: 'text' },
    ],
    rows: [
      ['1', 'x'],
      ['2', 'y'],
    ],
  });
  expect(gridToQueryTable([['a', 'b'], ['1']])!.rows).toEqual([['1', '']]); // fila corta rellenada
  expect(gridToQueryTable([])).toBeNull();
  expect(gridToQueryTable(null)).toBeNull();
});

/* ---------- describeStep + sanitizeTransform de los nuevos pasos ---------- */

test('describeStep: pasos de combinar/reestructurar', () => {
  expect(
    describeStep({ kind: 'unpivot', columns: ['ene', 'feb'], attribute: 'mes', value: 'v' }),
  ).toBe('Despivotar ene, feb → «mes»/«v»');
  expect(describeStep({ kind: 'pivot', key: 'mes', value: 'ventas', fn: 'sum' })).toBe(
    'Pivotar «mes» (suma de «ventas»)',
  );
  expect(describeStep({ kind: 'append', from: 'Ventas2' })).toBe('Anexar «Ventas2»');
  expect(
    describeStep({
      kind: 'merge',
      from: 'Precios',
      join: 'left',
      on: [{ left: 'id', right: 'pid' }],
      bring: [],
    }),
  ).toBe('Combinar «Precios» (izquierda) por id=pid');
});

test('sanitizeTransform: nuevos pasos válidos y malformados', () => {
  const raw = [
    { kind: 'unpivot', columns: ['a', 'b'], attribute: 'k', value: 'v' }, // ok
    { kind: 'unpivot', columns: [], attribute: 'k', value: 'v' }, // sin columnas → fuera
    { kind: 'pivot', key: 'm', value: 'x', fn: 'sum' }, // ok
    { kind: 'pivot', key: 'm', value: 'x', fn: 'zzz' }, // fn inválido → fuera
    { kind: 'append', from: 'Otra' }, // ok
    { kind: 'append', from: '' }, // vacío → fuera
    { kind: 'merge', from: 'Otra', join: 'left', on: [{ left: 'a', right: 'b' }], bring: ['c'] }, // ok
    { kind: 'merge', from: 'Otra', join: 'zzz', on: [{ left: 'a', right: 'b' }], bring: [] }, // join inválido → fuera
    { kind: 'merge', from: 'Otra', join: 'left', on: [], bring: [] }, // sin pares → fuera
  ];
  expect(sanitizeTransform(raw).map((s) => s.kind)).toEqual([
    'unpivot',
    'pivot',
    'append',
    'merge',
  ]);
});

test('sanitizeTransform: merge conserva prefijo y descarta el vacío', () => {
  expect(
    sanitizeTransform([
      {
        kind: 'merge',
        from: 'O',
        join: 'inner',
        on: [{ left: 'a', right: 'b' }],
        bring: [],
        prefix: 'p.',
      },
    ])[0],
  ).toEqual({
    kind: 'merge',
    from: 'O',
    join: 'inner',
    on: [{ left: 'a', right: 'b' }],
    bring: [],
    prefix: 'p.',
  });
  expect(
    sanitizeTransform([
      {
        kind: 'merge',
        from: 'O',
        join: 'inner',
        on: [{ left: 'a', right: 'b' }],
        bring: [],
        prefix: '',
      },
    ])[0],
  ).toEqual({
    kind: 'merge',
    from: 'O',
    join: 'inner',
    on: [{ left: 'a', right: 'b' }],
    bring: [],
  });
});
