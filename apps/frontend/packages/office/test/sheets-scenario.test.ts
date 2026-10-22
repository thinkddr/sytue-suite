/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers PUROS del Administrador de escenarios: aplicar un
 * escenario a un mapa de celdas (conservando formato), construir el informe
 * resumen (con un caso respaldado por el motor de fórmulas real, como lo usa el
 * lienzo) y sanear un escenario persistido.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/sheets-scenario.test.ts
 */
import { expect, test } from 'bun:test';
import { Engine } from '@sytue/formula';
import { formatCellValue } from '../src/editor/sheets/format';
import {
  SCENARIO_CHANGING_LABEL,
  SCENARIO_REPORT_TITLE,
  SCENARIO_RESULT_LABEL,
  applyScenarioValues,
  buildScenarioReport,
  sanitizeScenario,
} from '../src/editor/sheets/scenario';
import type { Scenario } from '../src/editor/types';

/* ---------- applyScenarioValues ---------- */

test('applyScenarioValues escribe los valores y no muta la entrada', () => {
  const cells = { B3: '10', B4: '5', C1: 'fijo' };
  const out = applyScenarioValues(cells, { B3: '20', B4: '8' });
  expect(out).toEqual({ B3: '20', B4: '8', C1: 'fijo' });
  expect(cells).toEqual({ B3: '10', B4: '5', C1: 'fijo' }); // no muta
});

test('applyScenarioValues conserva negrita/cursiva de la celda', () => {
  const out = applyScenarioValues({ B3: '<b>10</b>', B4: '<i>5</i>' }, { B3: '20', B4: '8' });
  expect(out['B3']).toBe('<b>20</b>');
  expect(out['B4']).toBe('<i>8</i>');
});

test('applyScenarioValues con valor vacío borra la celda', () => {
  const out = applyScenarioValues({ B3: '10', B4: '5' }, { B3: '' });
  expect('B3' in out).toBe(false);
  expect(out['B4']).toBe('5');
});

test('applyScenarioValues escapa el HTML del valor', () => {
  const out = applyScenarioValues({}, { A1: '<b>x' });
  expect(out['A1']).toBe('&lt;b&gt;x');
});

/* ---------- sanitizeScenario ---------- */

test('sanitizeScenario acepta uno válido y pasa las refs a mayúsculas', () => {
  const sc = sanitizeScenario({ name: '  Base  ', values: { b3: '10', B4: '5' } });
  expect(sc).toEqual({ name: 'Base', values: { B3: '10', B4: '5' } });
});

test('sanitizeScenario rechaza nombre vacío, values no-objeto y sin celdas válidas', () => {
  expect(sanitizeScenario({ name: '', values: { B3: '10' } })).toBeNull();
  expect(sanitizeScenario({ name: 'X', values: 3 })).toBeNull();
  expect(sanitizeScenario({ name: 'X', values: { 'B3:B5': '10', foo: 'x' } })).toBeNull();
  expect(sanitizeScenario('nope')).toBeNull();
});

test('sanitizeScenario descarta rangos y valores no-texto pero conserva las celdas buenas', () => {
  const sc = sanitizeScenario({ name: 'X', values: { B3: '10', 'A1:B2': '9', C4: 42 } });
  expect(sc).toEqual({ name: 'X', values: { B3: '10' } });
});

/* ---------- buildScenarioReport ---------- */

const SCENARIOS: Scenario[] = [
  { name: 'Base', values: { B1: '10', B2: '5' } },
  { name: 'Alza', values: { B1: '12', B2: '8' } },
];

/** evaluate respaldado por el motor real: fija los overrides y lee las celdas
 *  pedidas, tal y como hace el comando en el lienzo. */
const makeEvaluate =
  () =>
  (overrides: Record<string, string>, refs: readonly string[]): Record<string, string> => {
    const e = new Engine({ sheets: ['Hoja1'] });
    e.setCell('Hoja1', 'B1', '10');
    e.setCell('Hoja1', 'B2', '5');
    e.setCell('Hoja1', 'B3', '=B1*B2');
    for (const ref in overrides) e.setCell('Hoja1', ref, overrides[ref] ?? '');
    const out: Record<string, string> = {};
    for (const ref of refs) out[ref] = formatCellValue(e.getValue('Hoja1', ref));
    return out;
  };

test('buildScenarioReport arma título, secciones y una columna por escenario', () => {
  const grid = buildScenarioReport(SCENARIOS, ['B3'], makeEvaluate(), {
    currentLabel: 'Valores actuales',
    current: { B1: '10', B2: '5' },
  });
  expect(grid[0]).toEqual([SCENARIO_REPORT_TITLE, 'Valores actuales', 'Base', 'Alza']);
  expect(grid[1]).toEqual([SCENARIO_CHANGING_LABEL, '', '', '']);
  expect(grid[2]).toEqual(['B1', '10', '10', '12']);
  expect(grid[3]).toEqual(['B2', '5', '5', '8']);
  expect(grid[4]).toEqual([SCENARIO_RESULT_LABEL, '', '', '']);
  // Resultado B3 = B1*B2: actual 50, Base 50, Alza 96 (lo calcula el motor real).
  expect(grid[5]).toEqual(['B3', '50', '50', '96']);
  // Todas las filas comparten ancho.
  for (const row of grid) expect(row.length).toBe(4);
});

test('buildScenarioReport sin currentLabel omite la columna de valores actuales', () => {
  const grid = buildScenarioReport(SCENARIOS, ['B3'], makeEvaluate());
  expect(grid[0]).toEqual([SCENARIO_REPORT_TITLE, 'Base', 'Alza']);
  expect(grid[2]).toEqual(['B1', '10', '12']); // sin respaldo current → valor del escenario
});

test('buildScenarioReport sin celdas de resultado omite esa sección', () => {
  const grid = buildScenarioReport(SCENARIOS, [], makeEvaluate(), { currentLabel: 'Actual' });
  expect(grid.some((r) => r[0] === SCENARIO_RESULT_LABEL)).toBe(false);
  expect(grid.some((r) => r[0] === SCENARIO_CHANGING_LABEL)).toBe(true);
});

test('buildScenarioReport une y ordena las celdas de entrada de todos los escenarios', () => {
  const scs: Scenario[] = [
    { name: 'A', values: { B4: '1', B2: '2' } },
    { name: 'B', values: { B3: '3' } },
  ];
  const grid = buildScenarioReport(scs, [], (_o, _r) => ({}));
  const inputRows = grid.slice(2).map((r) => r[0]);
  expect(inputRows).toEqual(['B2', 'B3', 'B4']); // orden lectura (fila)
  // La entrada que un escenario no fija se muestra vacía (sin current).
  expect(grid[2]).toEqual(['B2', '2', '']); // B2 solo en A
});

test('buildScenarioReport respalda una entrada no fijada con el valor actual', () => {
  const scs: Scenario[] = [
    { name: 'A', values: { B2: '2' } },
    { name: 'B', values: { B3: '3' } },
  ];
  const grid = buildScenarioReport(scs, [], (_o, _r) => ({}), { current: { B2: '9', B3: '7' } });
  // B2 en escenario B (no lo fija) cae al actual 9; B3 en A cae al actual 7.
  expect(grid[2]).toEqual(['B2', '2', '9']);
  expect(grid[3]).toEqual(['B3', '7', '3']);
});
