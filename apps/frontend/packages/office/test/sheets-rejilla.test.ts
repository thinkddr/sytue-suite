/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la rejilla de Calcus (sheets/rejilla.ts): extensión que se recorre,
 * ventana que se pinta, índice visible y el salto de Ctrl+flecha.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import {
  acumulado,
  indiceEn,
  medidaDe,
  tramoPorPosiciones,
  MAX_COLS,
  MAX_ROWS,
  MIN_COLS,
  MIN_ROWS,
  extension,
  indiceVisible,
  primeroDesde,
  saltoADatos,
  tramo,
} from '../src/editor/sheets/rejilla';

const W = 96;
const H = 24;
const quieta = { top: 0, left: 0, w: 1600, h: 900 };

test('extension: hoja vacía = la mínima; crece con lo usado, la selección y lo que se ve', () => {
  expect(extension(-1, -1, 1, 1, quieta, W, H)).toEqual({ cols: MIN_COLS, rows: MIN_ROWS });
  // Datos importados hasta AD5000: se recorren enteros, con una rejilla mínima de holgura.
  expect(extension(29, 4999, 1, 1, quieta, W, H)).toEqual({
    cols: 30 + MIN_COLS,
    rows: 5000 + MIN_ROWS,
  });
  // La selección manda aunque no haya datos (Ctrl+flecha hasta el borde).
  expect(extension(-1, -1, 1, MAX_ROWS, quieta, W, H).rows).toBe(MAX_ROWS);
  // Al bajar con la rueda, la extensión va por delante de lo que se ve.
  const abajo = extension(-1, -1, 1, 1, { ...quieta, top: 100_000 }, W, H);
  expect(abajo.rows).toBeGreaterThan((100_000 + 900) / H);
  // Y nunca pasa de los límites de Excel.
  expect(extension(MAX_COLS + 5, MAX_ROWS + 5, 1, 1, quieta, W, H)).toEqual({
    cols: MAX_COLS,
    rows: MAX_ROWS,
  });
});

test('tramo: cubre lo que se ve con margen, sin pisar las fijas ni pasar del total', () => {
  // 900 px de alto desde arriba: unas 37 filas más el margen y el bloque de cuantización.
  const t = tramo(0, 900, 48, H, 0, 1_000_000, 'filas');
  expect(t.desde).toBe(0);
  expect(t.hasta).toBeGreaterThanOrEqual(Math.ceil(900 / H));
  expect(t.hasta).toBeLessThan(80); // una ventana, no la hoja
  // Lejos, abajo: empieza cerca de la fila que se ve y no en la 0.
  const lejos = tramo(240_000, 900, 48, H, 0, 1_000_000, 'filas');
  expect(lejos.desde).toBeGreaterThan(9_900);
  expect(lejos.desde).toBeLessThanOrEqual(Math.floor((240_000 - 48) / H));
  expect(lejos.hasta).toBeGreaterThanOrEqual(Math.ceil((240_000 + 900 - 48) / H));
  // Con tres filas inmovilizadas, el tramo nunca las incluye (van aparte).
  expect(tramo(0, 900, 48, H, 3, 1000, 'filas').desde).toBe(3);
  // Y no se sale del total.
  expect(tramo(0, 900, 48, H, 0, 10, 'filas')).toEqual({ desde: 0, hasta: 10 });
});

test('indiceVisible y primeroDesde: búsqueda binaria sobre filas visibles', () => {
  const vis = [0, 1, 2, 5, 6, 9];
  expect(indiceVisible(vis, 5)).toBe(3);
  expect(indiceVisible(vis, 3)).toBe(-1); // oculta
  expect(indiceVisible(vis, 10)).toBe(-1); // fuera
  expect(primeroDesde(vis, 3)).toBe(3);
  expect(primeroDesde(vis, 10)).toBe(6);
  expect(primeroDesde([], 0)).toBe(0);
});

test('saltoADatos: el Ctrl+flecha de Excel', () => {
  const llenas = [2, 3, 4, 10, 20, 21];
  // Dentro de un bloque: al final del bloque.
  expect(saltoADatos(llenas, 2, 1, 100)).toBe(4);
  expect(saltoADatos(llenas, 4, -1, 100)).toBe(2);
  // Al final de un bloque: a la próxima con datos.
  expect(saltoADatos(llenas, 4, 1, 100)).toBe(10);
  expect(saltoADatos(llenas, 10, 1, 100)).toBe(20);
  expect(saltoADatos(llenas, 20, -1, 100)).toBe(10);
  // Desde una vacía: a la primera con datos en esa dirección.
  expect(saltoADatos(llenas, 0, 1, 100)).toBe(2);
  expect(saltoADatos(llenas, 15, -1, 100)).toBe(10);
  // Sin nada más allá: al borde de la hoja.
  expect(saltoADatos(llenas, 21, 1, MAX_ROWS)).toBe(MAX_ROWS - 1);
  expect(saltoADatos(llenas, 1, -1, 100)).toBe(0);
  expect(saltoADatos([], 7, 1, MAX_COLS)).toBe(MAX_COLS - 1);
  // Ya en el borde: no se mueve.
  expect(saltoADatos(llenas, 0, -1, 100)).toBe(0);
});

test('medidaDe: la propia si es válida; si no, la de serie (entrada no confiable)', () => {
  const m = { '0': 150, '1': 2, '2': Number.NaN, '3': 99999, '4': -5 };
  expect(medidaDe(m, 0, W)).toBe(150);
  expect(medidaDe(m, 1, W)).toBe(W); // por debajo del mínimo
  expect(medidaDe(m, 2, W)).toBe(W);
  expect(medidaDe(m, 3, W)).toBe(1790); // acotada al máximo de Excel
  expect(medidaDe(m, 4, W)).toBe(W);
  expect(medidaDe(m, 9, W)).toBe(W);
  expect(medidaDe(undefined, 0, W)).toBe(W);
});

test('acumulado e indiceEn: posiciones con medidas propias y búsqueda por bisección', () => {
  // Columnas 0..4 con la 1 a 200 px y la 3 a 50 px; la 2 está oculta (no cuenta).
  const medida = (i: number): number => ({ 1: 200, 3: 50 })[i as 1 | 3] ?? W;
  const pos = acumulado(40, [0, 1, 3, 4], medida);
  expect(pos).toEqual([40, 136, 336, 386, 482]);
  expect(indiceEn(pos, 0)).toBe(0); // antes del origen: la primera
  expect(indiceEn(pos, 135)).toBe(0);
  expect(indiceEn(pos, 136)).toBe(1);
  expect(indiceEn(pos, 335)).toBe(1);
  expect(indiceEn(pos, 336)).toBe(2);
  expect(indiceEn(pos, 9999)).toBe(3); // más allá del final: la última
  expect(indiceEn([5], 10)).toBe(0); // sin unidades
});

test('tramoPorPosiciones: cubre la ventana con margen, sin pasar de las fijas ni del total', () => {
  const visibles = Array.from({ length: 1000 }, (_, i) => i);
  const pos = acumulado(24, visibles, (i) => (i % 2 === 0 ? 48 : 24));
  const t = tramoPorPosiciones(pos, 24 * 100, 600, 0, visibles.length, 'filas');
  // Las filas que se ven están dentro del tramo.
  expect(t.desde).toBeLessThanOrEqual(indiceEn(pos, 24 * 100));
  expect(t.hasta).toBeGreaterThan(indiceEn(pos, 24 * 100 + 600));
  // Siempre dentro de [fijas, total].
  const a = tramoPorPosiciones(pos, 0, 600, 3, visibles.length, 'filas');
  expect(a.desde).toBe(3);
  const z = tramoPorPosiciones(pos, 10_000_000, 600, 0, visibles.length, 'filas');
  expect(z.hasta).toBe(visibles.length);
});
