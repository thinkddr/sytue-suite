/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la lógica pura de las Tablas con formato (ListObject): estilos,
 * saneado, geometría (rol de celda + banda), auto-expansión, agregación de
 * totales y nombres únicos. Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import { ERR } from '@sytue/formula';
import type { CellValue } from '@sytue/formula';
import {
  TABLE_STYLES,
  autofilterBody,
  compileTables,
  expandRangeForEdit,
  sanitizeTable,
  tableBodyRows,
  tableCellInfo,
  tableStyleOf,
  tableTotalValue,
  uniqueHeaderNames,
  uniqueTableName,
} from '../src/editor/sheets/table';
import type { SheetTable } from '../src/editor/types';

const err = ERR.DIV0;

/* ---------- Estilos ---------- */

test('tableStyleOf: id conocido, y el primero como fallback', () => {
  expect(tableStyleOf('green').id).toBe('green');
  expect(tableStyleOf(undefined).id).toBe(TABLE_STYLES[0]!.id);
  expect(tableStyleOf('inexistente').id).toBe(TABLE_STYLES[0]!.id);
});

/* ---------- Agregación de totales (semántica SUBTOTAL: valores ya filtrados) ---------- */

test('tableTotalValue: suma/promedio/min/max solo sobre números', () => {
  const vals: CellValue[] = [10, 'texto', 20, '', 30];
  expect(tableTotalValue('sum', vals)).toBe(60);
  expect(tableTotalValue('average', vals)).toBe(20);
  expect(tableTotalValue('min', vals)).toBe(10);
  expect(tableTotalValue('max', vals)).toBe(30);
});

test('tableTotalValue: count cuenta números; counta, celdas no vacías (sin errores)', () => {
  const vals: CellValue[] = [1, 'a', '', 2, err];
  expect(tableTotalValue('count', vals)).toBe(2); // 1 y 2
  expect(tableTotalValue('counta', vals)).toBe(3); // 1, 'a', 2 (error y vacío no cuentan)
  expect(tableTotalValue('none', vals)).toBe('');
});

test('tableTotalValue: sin números → sum es 0, el resto celda en blanco', () => {
  const vals: CellValue[] = ['a', '', 'b'];
  expect(tableTotalValue('sum', vals)).toBe(0);
  expect(tableTotalValue('average', vals)).toBe('');
  expect(tableTotalValue('max', vals)).toBe('');
  expect(tableTotalValue('count', vals)).toBe(0);
});

/* ---------- Geometría: cuerpo, rol y banda ---------- */

test('tableBodyRows: excluye cabecera y fila de totales', () => {
  expect(tableBodyRows({ c1: 0, r1: 0, c2: 2, r2: 5 }, true, false)).toEqual({ r1: 1, r2: 5 });
  expect(tableBodyRows({ c1: 0, r1: 0, c2: 2, r2: 5 }, true, true)).toEqual({ r1: 1, r2: 4 });
  expect(tableBodyRows({ c1: 0, r1: 0, c2: 2, r2: 5 }, false, false)).toEqual({ r1: 0, r2: 5 });
});

test('tableCellInfo: cabecera, datos con banda alterna y fila de totales', () => {
  const table: SheetTable = { name: 'T', range: 'A1:B5', totalsRow: true };
  const [ct] = compileTables([table]);
  const roleAt = (c: number, r: number) => tableCellInfo([ct!], c, r);
  expect(roleAt(0, 0)!.role).toBe('header'); // fila 1
  expect(roleAt(0, 1)!.role).toBe('body'); // primera fila de datos
  expect(roleAt(0, 1)!.banded).toBe(false); // sin banda
  expect(roleAt(0, 2)!.banded).toBe(true); // banda
  expect(roleAt(0, 3)!.banded).toBe(false);
  expect(roleAt(1, 4)!.role).toBe('total'); // última fila = totales
  expect(roleAt(1, 4)!.colIndex).toBe(1);
  expect(roleAt(5, 0)).toBeNull(); // fuera del rango
});

test('tableCellInfo: la última tabla que cubre gana (solape)', () => {
  const a: SheetTable = { name: 'A', range: 'A1:C3' };
  const b: SheetTable = { name: 'B', range: 'B2:D4' };
  const compiled = compileTables([a, b]);
  expect(tableCellInfo(compiled, 1, 1)!.ct.table.name).toBe('B');
  expect(tableCellInfo(compiled, 0, 0)!.ct.table.name).toBe('A');
});

/* ---------- Auto-expansión ---------- */

test('expandRangeForEdit: crece por la columna adyacente derecha', () => {
  const [ct] = compileTables([{ name: 'T', range: 'A1:B4' }]);
  const grown = expandRangeForEdit(ct!, 2, 2, 25, 99); // C3, justo a la derecha
  expect(grown).toEqual({ c1: 0, r1: 0, c2: 2, r2: 3 });
});

test('expandRangeForEdit: crece por la fila adyacente inferior (sin totales)', () => {
  const [ct] = compileTables([{ name: 'T', range: 'A1:B4' }]);
  const grown = expandRangeForEdit(ct!, 1, 4, 25, 99); // B5, justo debajo
  expect(grown).toEqual({ c1: 0, r1: 0, c2: 1, r2: 4 });
});

test('expandRangeForEdit: NO crece hacia abajo si hay fila de totales', () => {
  const [ct] = compileTables([{ name: 'T', range: 'A1:B5', totalsRow: true }]);
  expect(expandRangeForEdit(ct!, 1, 5, 25, 99)).toBeNull(); // debajo de los totales
});

test('expandRangeForEdit: celda lejana o interior no amplía; respeta límites', () => {
  const [ct] = compileTables([{ name: 'T', range: 'A1:B4' }]);
  expect(expandRangeForEdit(ct!, 3, 2, 25, 99)).toBeNull(); // dos columnas a la derecha
  expect(expandRangeForEdit(ct!, 0, 0, 25, 99)).toBeNull(); // interior
  expect(expandRangeForEdit(ct!, 2, 2, 1, 99)).toBeNull(); // maxCol=1: no cabe
});

/* ---------- Nombres únicos ---------- */

test('uniqueTableName: primer hueco Tabla1, Tabla2… sin distinguir mayúsculas', () => {
  expect(uniqueTableName([])).toBe('Tabla1');
  expect(uniqueTableName(['Tabla1', 'tabla2'])).toBe('Tabla3');
});

test('uniqueHeaderNames: rellena vacíos con ColumnaN y desambigua duplicados', () => {
  expect(uniqueHeaderNames(['Fecha', '', 'Fecha'])).toEqual(['Fecha', 'Columna2', 'Fecha2']);
  expect(uniqueHeaderNames(['x', 'x', 'x'])).toEqual(['x', 'x2', 'x3']);
});

/* ---------- Saneado ---------- */

test('sanitizeTable: acepta lo válido, normaliza y descarta basura', () => {
  const t = sanitizeTable({
    name: '  Ventas ',
    range: 'A1:C10',
    style: 'blue',
    headers: false,
    totalsRow: true,
    totals: { '0': 'sum', '1': 'none', '2': 'promedio-mal', '3': 'average' },
  });
  expect(t).not.toBeNull();
  expect(t!.name).toBe('Ventas');
  expect(t!.style).toBe('blue');
  expect(t!.headers).toBe(false);
  expect(t!.totalsRow).toBe(true);
  // 'none' y la función inválida se descartan; el JSON queda mínimo.
  expect(t!.totals).toEqual({ '0': 'sum', '3': 'average' });
});

test('sanitizeTable: rechaza sin nombre, sin rango o con rango inválido', () => {
  expect(sanitizeTable({ range: 'A1:B2' })).toBeNull();
  expect(sanitizeTable({ name: 'T' })).toBeNull();
  expect(sanitizeTable({ name: 'T', range: 'no-es-rango' })).toBeNull();
  expect(sanitizeTable(null)).toBeNull();
});

test('sanitizeTable: estilo desconocido se ignora (usa el defecto al pintar)', () => {
  const t = sanitizeTable({ name: 'T', range: 'A1:B2', style: 'neon' });
  expect(t!.style).toBeUndefined();
  expect(tableStyleOf(t!.style).id).toBe(TABLE_STYLES[0]!.id);
});

test('autofilterBody: el cuerpo de la tabla sin totales, o hasta la última fila usada', () => {
  const tablas = compileTables([{ name: 'T', range: 'A1:B5', totalsRow: true }]);
  // El autofiltro de la tabla: filas 2-4 (la 5 es la de totales).
  expect(autofilterBody(tablas, { row: 0, c1: 0, c2: 1 }, [])).toEqual({ r1: 1, r2: 3 });
  // Uno suelto en C:D, que no es el de la tabla: hasta la última fila con algo en C:D,
  // sin contar lo que hay más abajo en otras columnas.
  const refs = ['C3', 'D7', 'E20', 'A30'];
  expect(autofilterBody(tablas, { row: 2, c1: 2, c2: 3 }, refs)).toEqual({ r1: 3, r2: 6 });
  // Sin nada debajo de los botones: r2 ≤ r1, nada que ordenar.
  expect(autofilterBody(tablas, { row: 9, c1: 5, c2: 5 }, refs)).toEqual({ r1: 10, r2: 9 });
});
