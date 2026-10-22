/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers PUROS de la Tabla de datos (Análisis Y-si): geometría
 * (`planDataTable`, con sus tres orientaciones y aristas) y recorrido con
 * evaluador inyectado (`computeDataTable`), incluido un caso respaldado por el
 * motor de fórmulas REAL, tal como lo usa el lienzo (fija las celdas de entrada
 * en un motor desechable y lee la fórmula).
 * Ejecutar: cd apps/frontend && bun test packages/office/test/sheets-datatable.test.ts
 */
import { expect, test } from 'bun:test';
import { Engine } from '@sytue/formula';
import type { CellValue } from '@sytue/formula';
import {
  DATA_TABLE_MAX_CELLS,
  computeDataTable,
  planDataTable,
} from '../src/editor/sheets/datatable';
import type { DataTableSubs } from '../src/editor/sheets/datatable';

/* ---------- planDataTable: geometría ---------- */

test('planDataTable dos variables: fórmula en la esquina y matriz interior', () => {
  // A1:C3 (0-based c1..c2 = 0..2, r1..r2 = 0..2).
  const plan = planDataTable({ c1: 0, r1: 0, c2: 2, r2: 2 }, true, true);
  if ('error' in plan) throw new Error(plan.error);
  expect(plan.orientation).toBe('both');
  expect(plan.formula).toEqual({ c: 0, r: 0 });
  expect(plan.cells).toHaveLength(4); // 2×2 interior
  // Orden de lectura: primero (c1+1, r1+1).
  expect(plan.cells[0]).toEqual({
    c: 1,
    r: 1,
    rowFrom: { c: 1, r: 0 }, // valor de la fila superior de esa columna
    colFrom: { c: 0, r: 1 }, // valor de la columna izquierda de esa fila
  });
  expect(plan.cells[3]).toEqual({ c: 2, r: 2, rowFrom: { c: 2, r: 0 }, colFrom: { c: 0, r: 2 } });
});

test('planDataTable una variable por COLUMNA: fórmula contigua a la esquina, sin rowFrom', () => {
  // A1:B4: valores en la columna A (filas 2..4), resultados en B.
  const plan = planDataTable({ c1: 0, r1: 0, c2: 1, r2: 3 }, false, true);
  if ('error' in plan) throw new Error(plan.error);
  expect(plan.orientation).toBe('col');
  expect(plan.formula).toEqual({ c: 1, r: 0 });
  expect(plan.cells).toHaveLength(3);
  expect(plan.cells[0]).toEqual({ c: 1, r: 1, colFrom: { c: 0, r: 1 } });
  expect(plan.cells[0]!.rowFrom).toBeUndefined();
});

test('planDataTable una variable por FILA: fórmula bajo la esquina, sin colFrom', () => {
  // A1:D2: valores en la fila 1 (cols B..D), resultados en la fila 2.
  const plan = planDataTable({ c1: 0, r1: 0, c2: 3, r2: 1 }, true, false);
  if ('error' in plan) throw new Error(plan.error);
  expect(plan.orientation).toBe('row');
  expect(plan.formula).toEqual({ c: 0, r: 1 });
  expect(plan.cells).toHaveLength(3);
  expect(plan.cells[0]).toEqual({ c: 1, r: 1, rowFrom: { c: 1, r: 0 } });
  expect(plan.cells[0]!.colFrom).toBeUndefined();
});

test('planDataTable sin entradas → error no-input', () => {
  expect(planDataTable({ c1: 0, r1: 0, c2: 3, r2: 3 }, false, false)).toEqual({
    error: 'no-input',
  });
});

test('planDataTable rango de una sola franja → error too-small', () => {
  // Columna única (c2 = c1): no cabe la fórmula/resultados aunque haya entrada.
  expect(planDataTable({ c1: 0, r1: 0, c2: 0, r2: 5 }, false, true)).toEqual({
    error: 'too-small',
  });
  // Fila única (r2 = r1).
  expect(planDataTable({ c1: 0, r1: 0, c2: 5, r2: 0 }, true, false)).toEqual({
    error: 'too-small',
  });
});

test('planDataTable respeta el tope de celdas', () => {
  // 2×2 = 4 resultados con tope 3 → too-many.
  expect(planDataTable({ c1: 0, r1: 0, c2: 2, r2: 2 }, true, true, 3)).toEqual({
    error: 'too-many',
  });
  expect(DATA_TABLE_MAX_CELLS).toBeGreaterThan(0);
});

/* ---------- computeDataTable: recorrido con evaluador inyectado ---------- */

test('computeDataTable usa las lecturas de las cabeceras como sustituciones', () => {
  const plan = planDataTable({ c1: 0, r1: 0, c2: 2, r2: 2 }, true, true);
  if ('error' in plan) throw new Error(plan.error);
  // Cabeceras: fila superior B1=2, C1=3; columna izquierda A2=10, A3=20.
  const headers: Record<string, string> = { '1,0': '2', '2,0': '3', '0,1': '10', '0,2': '20' };
  const read = (c: number, r: number): string => headers[`${c},${r}`] ?? '';
  const evaluate = (s: DataTableSubs): CellValue => Number(s.row) * Number(s.col);
  const out = computeDataTable(plan, read, evaluate);
  const at = (c: number, r: number): CellValue => out.find((x) => x.c === c && x.r === r)!.value;
  expect(at(1, 1)).toBe(20); // 2 × 10
  expect(at(2, 1)).toBe(30); // 3 × 10
  expect(at(1, 2)).toBe(40); // 2 × 20
  expect(at(2, 2)).toBe(60); // 3 × 20
});

test('computeDataTable en una variable solo pasa la sustitución de esa entrada', () => {
  const plan = planDataTable({ c1: 0, r1: 0, c2: 1, r2: 3 }, false, true);
  if ('error' in plan) throw new Error(plan.error);
  const headers: Record<string, string> = { '0,1': '5', '0,2': '7', '0,3': '9' };
  const read = (c: number, r: number): string => headers[`${c},${r}`] ?? '';
  const seen: DataTableSubs[] = [];
  const evaluate = (s: DataTableSubs): CellValue => {
    seen.push(s);
    return Number(s.col) * 2;
  };
  const out = computeDataTable(plan, read, evaluate);
  expect(out.map((x) => x.value)).toEqual([10, 14, 18]);
  // La entrada de fila nunca se toca en una tabla de columna.
  expect(seen.every((s) => s.row === undefined)).toBe(true);
});

/* ---------- Respaldo con el motor REAL (como el lienzo) ---------- */

test('computeDataTable respaldado por el Engine reproduce una matriz de sensibilidad', () => {
  const sheet = 'Hoja 1';
  // Modelo: fórmula A1 = F1*F2; F1/F2 son las celdas de entrada (fila/columna).
  const cells: Record<string, string> = {
    A1: '=F1*F2',
    B1: '2',
    C1: '3', // fila superior → entrada de fila F1
    A2: '10',
    A3: '20', // columna izquierda → entrada de columna F2
    F1: '0',
    F2: '0',
  };
  const eng = new Engine({ sheets: [sheet] });
  for (const ref in cells) eng.setCell(sheet, ref, cells[ref]!);

  const plan = planDataTable({ c1: 0, r1: 0, c2: 2, r2: 2 }, true, true);
  if ('error' in plan) throw new Error(plan.error);

  const read = (c: number, r: number): string => {
    const ref = `${String.fromCharCode(65 + c)}${r + 1}`;
    return cells[ref] ?? '';
  };
  // Evaluador idéntico al del comando: fija las entradas y lee la fórmula.
  const evaluate = (s: DataTableSubs): CellValue => {
    if (s.row !== undefined) eng.setCell(sheet, 'F1', s.row);
    if (s.col !== undefined) eng.setCell(sheet, 'F2', s.col);
    return eng.getValue(sheet, 'A1');
  };
  const out = computeDataTable(plan, read, evaluate);
  const at = (c: number, r: number): CellValue => out.find((x) => x.c === c && x.r === r)!.value;
  expect(at(1, 1)).toBe(20); // 2 × 10
  expect(at(2, 1)).toBe(30); // 3 × 10
  expect(at(1, 2)).toBe(40); // 2 × 20
  expect(at(2, 2)).toBe(60); // 3 × 20
  // El motor desechable no debe haber tocado las celdas de datos originales.
  expect(eng.getValue(sheet, 'B1')).toBe(2);
});
