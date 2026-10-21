/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Anclas de fila y columna (sheets/anclas.ts): el cursor de hoja de otra persona sigue a
 * su fila aunque se inserte o se borre por encima, también con inserciones concurrentes.
 * Ejecutar: cd apps/frontend/packages/office && bun test test/sheets-anclas.test.ts
 */
import { expect, test } from 'bun:test';
import { fusionar3 } from '../src/editor/state/fusion';
import { conEstructura, idDe, indiceDe, insertar, quitar } from '../src/editor/sheets/anclas';
import { anclarPosHoja, parseCursorPos, resolverPosHoja } from '../src/editor/state/cursors';
import type { SheetData } from '../src/editor/types';

const hoja = (extra: Partial<SheetData> = {}): SheetData => ({
  id: 'h1',
  name: 'Hoja 1',
  cells: {},
  ...extra,
});
const filas = (s: SheetData, desde: number, n: number): SheetData => ({
  ...s,
  ...conEstructura(s, 'fila', desde, n),
});

const vacio = {};
const eje = (r: { ids: { id: string }[]; base: number }) => ({ ids: r.ids, base: r.base });

test('sin listas todo es implícito y estable en el índice', () => {
  expect(idDe(vacio, 7)).toBe('t7');
  expect(indiceDe(vacio, 't7')).toBe(7);
  expect(indiceDe(vacio, 'basura')).toBeNull();
});

test('insertar y borrar por encima mueve el índice pero no la identidad, ni al materializar', () => {
  const id = idDe(vacio, 10);
  const a = eje(insertar(vacio, 3, 2));
  expect(indiceDe(a, id)).toBe(12); // las implícitas conservan su nombre
  const b = eje(insertar(a, 0, 1));
  expect(indiceDe(b, id)).toBe(13);
  const c = eje(quitar(b, 0, 12)); // borra también filas que eran implícitas
  expect(indiceDe(c, id)).toBe(1);
  const cola = idDe(c, 500);
  expect(indiceDe(eje(insertar(c, 2, 3)), cola)).toBe(503);
  expect(indiceDe(eje(quitar(c, 400, 200)), cola)).toBeNull(); // borrada: caen los índices
});

test('el cursor de un peer sigue a su fila tras una inserción ajena', () => {
  let s = hoja();
  s = filas(s, 0, 1); // materializa
  const pos = anclarPosHoja([s], 0, { c1: 1, r1: 5, c2: 1, r2: 5 });
  const tras = filas(s, 2, 3); // otra persona inserta tres filas por encima
  const r = resolverPosHoja([tras], pos);
  expect(r?.pos).toMatchObject({ r1: 8, r2: 8, c1: 1 });
});

test('inserciones concurrentes: las dos se conservan y todos resuelven la misma fila', () => {
  const base = filas(hoja(), 0, 1);
  const pos = anclarPosHoja([base], 0, { c1: 0, r1: 1, c2: 0, r2: 1 });
  const a = filas(base, 1, 1); // A inserta arriba del cursor
  const b = filas(base, 1, 2); // B inserta también arriba, a la vez
  const ab = fusionar3(base, a, b) as SheetData;
  const ba = fusionar3(base, b, a) as SheetData;
  for (const m of [ab, ba]) {
    expect(m.rowIds?.length).toBe(4);
    expect(new Set(m.rowIds?.map((x) => x.id)).size).toBe(4);
    expect(resolverPosHoja([m], pos)?.pos.r1).toBe(4);
  }
});

test('columnas: ancla y foco siguen a su columna', () => {
  const s = hoja();
  const pos = anclarPosHoja([s], 0, { c1: 2, r1: 0, c2: 4, r2: 0 });
  const tras = { ...s, ...conEstructura(s, 'columna', 0, 2) };
  expect(resolverPosHoja([tras], pos)?.pos).toMatchObject({ c1: 4, c2: 6 });
});

test('la hoja se localiza por id aunque cambie de sitio; si no existe, no hay cursor', () => {
  const pos = anclarPosHoja([hoja({ id: 'x' }), hoja({ id: 'h1' })], 1, {
    c1: 0,
    r1: 0,
    c2: 0,
    r2: 0,
  });
  expect(resolverPosHoja([hoja({ id: 'h1' }), hoja({ id: 'x' })], pos)?.sheet).toBe(0);
  expect(resolverPosHoja([hoja({ id: 'x' })], pos)).toBeNull();
});

test('compatibilidad v1: sin anclas mandan los índices; con anclas se parsean y validan', () => {
  const viejo = parseCursorPos({ kind: 'sheet', sheet: 0, c1: 1, r1: 2, c2: 3, r2: 4 });
  expect(viejo).toEqual({ kind: 'sheet', sheet: 0, c1: 1, r1: 2, c2: 3, r2: 4 });
  const r = resolverPosHoja([hoja({ rowIds: [{ id: 'a' }] })], viejo as never);
  expect(r?.pos).toMatchObject({ r1: 2, r2: 4, c1: 1, c2: 3 });
  const nuevo = parseCursorPos({
    kind: 'sheet',
    sheet: 0,
    c1: 0,
    r1: 0,
    c2: 0,
    r2: 0,
    sheetId: 'h1',
    rows: ['a', 'b'],
    cols: ['t0', 't0'],
  });
  expect(nuevo).toMatchObject({ sheetId: 'h1', rows: ['a', 'b'], cols: ['t0', 't0'] });
  const hostil = parseCursorPos({
    kind: 'sheet',
    sheet: 0,
    c1: 0,
    r1: 0,
    c2: 0,
    r2: 0,
    rows: ['a', 3],
    cols: 'x',
    sheetId: 7,
  });
  expect(hostil).toEqual({ kind: 'sheet', sheet: 0, c1: 0, r1: 0, c2: 0, r2: 0 });
});
