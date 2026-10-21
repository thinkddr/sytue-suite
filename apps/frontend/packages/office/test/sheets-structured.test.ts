/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las REFERENCIAS ESTRUCTURADAS de tabla `Tabla[Columna]` en el
 * editor Sheets (paridad Excel/Sheets): los helpers PUROS del cableado —
 * collectResolvableTables + resolveStructuredRef (table.ts), tableBracketAt y
 * extractRefs con resolutor (formula-edit.ts), la re-serialización en shiftRefs
 * (format.ts)— y una prueba de integración con el motor real. El motor
 * (@sytue/formula) ya soporta Tabla[Columna]; aquí se comprueba el CABLEADO.
 * Ejecutar: cd apps/frontend/packages/office && bun test test
 */
import { expect, test } from 'bun:test';
import { ERR, Engine, FormulaError } from '@sytue/formula';
import type { RefRange, TableArea } from '@sytue/formula';
import { collectResolvableTables, resolveStructuredRef } from '../src/editor/sheets/table';
import type { ResolvableTable } from '../src/editor/sheets/table';
import { extractRefs, refAtCaret, tableBracketAt } from '../src/editor/sheets/formula-edit';
import { shiftRefs } from '../src/editor/sheets/format';
import type { SheetTable } from '../src/editor/types';

/* ---------- Libro de prueba ---------- */

// Cabeceras (texto crudo): Ventas en Hoja1 (A1:C5, sin totales) y Compras en
// Hoja2 (B2:C4, con fila de totales). Clave «hoja!col,fila» (0-based).
const HEADERS: Record<string, string> = {
  'Hoja1!0,0': 'Region',
  'Hoja1!1,0': 'Ventas',
  'Hoja1!2,0': 'Coste',
  'Hoja2!1,1': 'Fecha',
  'Hoja2!2,1': 'Total',
};
const header = (sheet: string, c: number, r: number): string => HEADERS[`${sheet}!${c},${r}`] ?? '';

const SHEETS = [
  { name: 'Hoja1', tables: [{ name: 'Ventas', range: 'A1:C5' }] as SheetTable[] },
  { name: 'Hoja2', tables: [{ name: 'Compras', range: 'B2:C4', totalsRow: true }] as SheetTable[] },
];

const book = (): ResolvableTable[] => collectResolvableTables(SHEETS, header);

/** Afirma que un resultado es un RefRange (no error ni undefined) y lo devuelve. */
const rr = (v: RefRange | FormulaError | undefined): RefRange => {
  if (!v || v instanceof FormulaError) throw new Error('no es RefRange: ' + String(v));
  return v;
};

const q = (
  table: string,
  column: string | undefined,
  area: TableArea = 'data',
  thisRow = false,
): { table: string; column: string | undefined; area: TableArea; thisRow: boolean } => ({
  table,
  column,
  area,
  thisRow,
});

const HERE = { sheet: 'Hoja1', col: 1, row: 2 }; // dentro de Ventas (fila de datos)

/* ---------- collectResolvableTables ---------- */

test('collectResolvableTables: deriva columnas de la cabecera y el cuerpo', () => {
  const b = book();
  expect(b.length).toBe(2);
  const ventas = b.find((t) => t.table.name === 'Ventas')!;
  expect(ventas.sheet).toBe('Hoja1');
  expect(ventas.columns).toEqual(['Region', 'Ventas', 'Coste']);
  expect([ventas.bodyR1, ventas.bodyR2]).toEqual([1, 4]); // datos filas 1..4 (cabecera fuera)
  const compras = b.find((t) => t.table.name === 'Compras')!;
  expect(compras.columns).toEqual(['Fecha', 'Total']);
  expect([compras.bodyR1, compras.bodyR2]).toEqual([2, 2]); // fila de totales excluida del cuerpo
});

test('collectResolvableTables: cabeceras vacías → «ColumnaN» (criterio Excel)', () => {
  const b = collectResolvableTables(
    [{ name: 'H', tables: [{ name: 'T', range: 'A1:B2' }] }],
    () => '',
  );
  expect(b[0]!.columns).toEqual(['Columna1', 'Columna2']);
});

/* ---------- resolveStructuredRef ---------- */

test('resolve: columna de datos → rango del cuerpo (sin cabecera)', () => {
  expect(rr(resolveStructuredRef(book(), q('Ventas', 'Ventas'), HERE))).toEqual({
    sheet: 'Hoja1',
    c1: 1,
    r1: 1,
    c2: 1,
    r2: 4,
  });
});

test('resolve: tabla y columna insensibles a mayúsculas', () => {
  expect(rr(resolveStructuredRef(book(), q('ventas', 'coste'), HERE))).toEqual({
    sheet: 'Hoja1',
    c1: 2,
    r1: 1,
    c2: 2,
    r2: 4,
  });
});

test('resolve: @ fila actual dentro del cuerpo → celda única de esa fila', () => {
  expect(
    rr(
      resolveStructuredRef(book(), q('Ventas', 'Ventas', 'data', true), {
        sheet: 'Hoja1',
        col: 5,
        row: 3,
      }),
    ),
  ).toEqual({
    sheet: 'Hoja1',
    c1: 1,
    r1: 3,
    c2: 1,
    r2: 3,
  });
});

test('resolve: @ fuera del cuerpo de datos → #VALUE!', () => {
  expect(
    resolveStructuredRef(book(), q('Ventas', 'Ventas', 'data', true), {
      sheet: 'Hoja1',
      col: 5,
      row: 0,
    }),
  ).toBe(ERR.VALUE);
});

test('resolve: áreas #Cabeceras/#Todo/#Totales', () => {
  const b = book();
  expect(rr(resolveStructuredRef(b, q('Ventas', 'Ventas', 'headers'), HERE))).toEqual({
    sheet: 'Hoja1',
    c1: 1,
    r1: 0,
    c2: 1,
    r2: 0,
  });
  expect(rr(resolveStructuredRef(b, q('Ventas', undefined, 'all'), HERE))).toEqual({
    sheet: 'Hoja1',
    c1: 0,
    r1: 0,
    c2: 2,
    r2: 4,
  });
  // Ventas no tiene fila de totales → #Totales es #REF!.
  expect(resolveStructuredRef(b, q('Ventas', undefined, 'totals'), HERE)).toBe(ERR.REF);
  // Compras SÍ la tiene.
  expect(rr(resolveStructuredRef(b, q('Compras', 'Total', 'totals'), HERE))).toEqual({
    sheet: 'Hoja2',
    c1: 2,
    r1: 3,
    c2: 2,
    r2: 3,
  });
});

test('resolve: tabla desconocida → #NAME?; columna desconocida → #REF!', () => {
  expect(resolveStructuredRef(book(), q('Foo', 'Bar'), HERE)).toBe(ERR.NAME);
  expect(resolveStructuredRef(book(), q('Ventas', 'NoExiste'), HERE)).toBe(ERR.REF);
});

test('resolve: tabla implícita (table = "") = la que contiene `here`', () => {
  expect(
    rr(
      resolveStructuredRef(book(), q('', 'Coste', 'data', true), {
        sheet: 'Hoja1',
        col: 2,
        row: 2,
      }),
    ),
  ).toEqual({
    sheet: 'Hoja1',
    c1: 2,
    r1: 2,
    c2: 2,
    r2: 2,
  });
  // fuera de toda tabla → undefined (el motor lo trata como #REF!).
  expect(
    resolveStructuredRef(book(), q('', 'Coste', 'data', true), { sheet: 'Hoja1', col: 9, row: 9 }),
  ).toBeUndefined();
});

test('resolve: tabla nombrada de OTRA hoja (referencia de libro)', () => {
  expect(rr(resolveStructuredRef(book(), q('Compras', 'Fecha'), HERE))).toEqual({
    sheet: 'Hoja2',
    c1: 1,
    r1: 2,
    c2: 1,
    r2: 2,
  });
});

/* ---------- tableBracketAt (corchete bajo el cursor) ---------- */

test('tableBracketAt: Tabla[ con prefijo de columna', () => {
  expect(tableBracketAt('=Ventas[Reg', 11)).toEqual({
    table: 'Ventas',
    prefix: 'Reg',
    start: 8,
    end: 11,
    thisRow: false,
  });
});

test('tableBracketAt: @ (fila actual) y corchete recién abierto (prefijo vacío)', () => {
  expect(tableBracketAt('=Ventas[@V', 10)).toEqual({
    table: 'Ventas',
    prefix: 'V',
    start: 9,
    end: 10,
    thisRow: true,
  });
  expect(tableBracketAt('=Ventas[', 8)).toEqual({
    table: 'Ventas',
    prefix: '',
    start: 8,
    end: 8,
    thisRow: false,
  });
});

test('tableBracketAt: implícita [@Col] (tabla vacía)', () => {
  expect(tableBracketAt('=[@Co', 5)).toEqual({
    table: '',
    prefix: 'Co',
    start: 3,
    end: 5,
    thisRow: true,
  });
});

test('tableBracketAt: null fuera de contexto (cerrado / área / anidado / sin corchete / sin =)', () => {
  expect(tableBracketAt('=Ventas[Region]', 15)).toBeNull(); // cursor tras «]» (ya cerrado)
  expect(tableBracketAt('=Ventas[#Da', 11)).toBeNull(); // especificador de área
  expect(tableBracketAt('=Ventas[[#Datos],[Re', 20)).toBeNull(); // forma anidada
  expect(tableBracketAt('=SUM(A1)', 6)).toBeNull(); // no hay corchete
  expect(tableBracketAt('Ventas[Reg', 10)).toBeNull(); // no es una fórmula
});

/* ---------- extractRefs con resolutor (resaltado) ---------- */

const resolve = (spec: {
  table: string;
  column: string | undefined;
  area: TableArea;
  thisRow: boolean;
}): { sheet: string; rect: { c1: number; r1: number; c2: number; r2: number } } | null => {
  const r = resolveStructuredRef(book(), spec, { sheet: 'Hoja1', col: 1, row: 2 });
  if (!r || r instanceof FormulaError) return null;
  return { sheet: r.sheet, rect: { c1: r.c1, r1: r.r1, c2: r.c2, r2: r.r2 } };
};

test('extractRefs: reconoce Tabla[Columna] con resolutor (rect de la columna)', () => {
  const refs = extractRefs('=SUM(Ventas[Ventas])', resolve);
  expect(refs.length).toBe(1);
  expect(refs[0]!.text).toBe('Ventas[Ventas]');
  expect(refs[0]!.sheet).toBe('Hoja1');
  expect(refs[0]!.rect).toEqual({ c1: 1, r1: 1, c2: 1, r2: 4 });
});

test('extractRefs: sin resolutor ignora las estructuradas (RETROCOMPAT)', () => {
  expect(extractRefs('=SUM(Ventas[Ventas])')).toEqual([]);
});

test('extractRefs: A1 + estructurada, color por posición de aparición', () => {
  const refs = extractRefs('=A1+Ventas[Coste]', resolve);
  expect(refs.map((r) => r.text)).toEqual(['A1', 'Ventas[Coste]']);
  expect(refs.map((r) => r.colorIndex)).toEqual([0, 1]);
});

test('extractRefs: ignora un corchete dentro de una cadena', () => {
  expect(extractRefs('="Ventas[Ventas]"', resolve)).toEqual([]);
});

test('extractRefs: forma anidada [[#Cabeceras],[Coste]] → cabecera de esa columna', () => {
  const refs = extractRefs('=Ventas[[#Cabeceras],[Coste]]', resolve);
  expect(refs.length).toBe(1);
  expect(refs[0]!.rect).toEqual({ c1: 2, r1: 0, c2: 2, r2: 0 });
});

test('extractRefs: columna con forma de celda A1 (T1/Q1) NO genera ref A1 fantasma', () => {
  // Trim con columnas T1..T4 (forma de celda A1): editar =SUM(Trim[T1]) debe dar
  // UNA sola ref (la estructurada), no una fantasma sobre la celda real T1.
  const trim = collectResolvableTables(
    [{ name: 'Hoja1', tables: [{ name: 'Trim', range: 'A1:D5' }] as SheetTable[] }],
    (_s, c, r) => (r === 0 ? (['T1', 'T2', 'T3', 'T4'][c] ?? '') : ''),
  );
  const resolveTrim = (spec: {
    table: string;
    column: string | undefined;
    area: TableArea;
    thisRow: boolean;
  }) => {
    const r = resolveStructuredRef(trim, spec, { sheet: 'Hoja1', col: 0, row: 1 });
    if (!r || r instanceof FormulaError) return null;
    return { sheet: r.sheet, rect: { c1: r.c1, r1: r.r1, c2: r.c2, r2: r.r2 } };
  };
  const refs = extractRefs('=SUM(Trim[T1])', resolveTrim);
  expect(refs.length).toBe(1);
  expect(refs[0]!.text).toBe('Trim[T1]');
  expect(refs[0]!.sheet).toBe('Hoja1');
  expect(refs[0]!.rect).toEqual({ c1: 0, r1: 1, c2: 0, r2: 4 }); // columna T1 = cuerpo A2:A5
  // SIEMPRE (aun sin resolutor): protege a refAtCaret/cycleRefAt del point-and-click.
  expect(extractRefs('=SUM(Trim[T1])')).toEqual([]);
  expect(refAtCaret('=SUM(Trim[T1])', 11)).toBeNull(); // cursor dentro del nombre de columna «T1»
});

/* ---------- shiftRefs (re-serialización de format.ts) ---------- */

test('shiftRefs: una referencia estructurada no se desplaza (es por-nombre)', () => {
  expect(shiftRefs('=SUM(Ventas[Ventas])', 3, 5)).toBe('=SUM(Ventas[Ventas])');
  expect(shiftRefs('=[@Coste]', 1, 1)).toBe('=[@Coste]');
  expect(shiftRefs('=Ventas[#Totales]', 4, 4)).toBe('=Ventas[#Totales]');
  // La A1 SÍ se desplaza; la estructurada de al lado no.
  expect(shiftRefs('=A1+Ventas[[#Cabeceras],[Coste]]', 0, 1)).toBe(
    '=A2+Ventas[[#Cabeceras],[Coste]]',
  );
});

/* ---------- Integración con el motor real ---------- */

test('integración: el motor evalúa Tabla[Columna] con el resolutor del editor', () => {
  const b = book();
  const e = new Engine({
    sheets: ['Hoja1', 'Hoja2'],
    resolveTable: (query, here) => resolveStructuredRef(b, query, here),
  });
  ['Region', 'Ventas', 'Coste'].forEach((h, c) =>
    e.setCell('Hoja1', `${String.fromCharCode(65 + c)}1`, h),
  );
  [10, 20, 30, 40].forEach((v, i) => e.setCell('Hoja1', `B${i + 2}`, String(v)));
  e.setCell('Hoja1', 'E1', '=SUM(Ventas[Ventas])');
  expect(e.getValue('Hoja1', 'E1')).toBe(100);

  // @ fila actual desde una celda de la misma fila (E3 → fila de datos 2 → B3).
  e.setCell('Hoja1', 'E3', '=Ventas[@Ventas]*2');
  expect(e.getValue('Hoja1', 'E3')).toBe(40);

  // Tabla implícita [@Col] desde una celda DENTRO de la tabla (C3, col Coste).
  e.setCell('Hoja1', 'C3', '=[@Ventas]');
  expect(e.getValue('Hoja1', 'C3')).toBe(20);

  // Recálculo incremental: editar el cuerpo propaga a la suma.
  e.setCell('Hoja1', 'B2', '100');
  expect(e.getValue('Hoja1', 'E1')).toBe(190);
});
