// Remodelado de arrays dinámicos: TAKE/DROP/EXPAND, CHOOSEROWS/CHOOSECOLS,
// TOROW/TOCOL, WRAPROWS/WRAPCOLS y HSTACK/VSTACK. Se ejercita el derrame real
// del motor. Valores contrastados con Excel 365. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

/** Coloca una matriz numérica por filas desde `topLeft` (columnas A-Z contiguas). */
function put(e: Engine, topLeft: string, rows: number[][]): void {
  const col0 = topLeft.charCodeAt(0) - 65;
  const row0 = Number(topLeft.slice(1));
  rows.forEach((r, i) =>
    r.forEach((v, j) =>
      e.setCell('Hoja1', `${String.fromCharCode(65 + col0 + j)}${row0 + i}`, String(v)),
    ),
  );
}
/** Lee un bloque R×C desde el ancla como valores crudos. */
function block(e: Engine, topLeft: string, R: number, C: number): CellValue[][] {
  const col0 = topLeft.charCodeAt(0) - 65;
  const row0 = Number(topLeft.slice(1));
  const out: CellValue[][] = [];
  for (let i = 0; i < R; i++) {
    const row: CellValue[] = [];
    for (let j = 0; j < C; j++)
      row.push(e.getValue('Hoja1', `${String.fromCharCode(65 + col0 + j)}${row0 + i}`));
    out.push(row);
  }
  return out;
}
function disp(e: Engine, cell: string): string {
  return e.getDisplay('Hoja1', cell);
}

const M = [
  [1, 2, 3],
  [4, 5, 6],
  [7, 8, 9],
];

test('TAKE: primeras/últimas filas y columnas', () => {
  const e = new Engine();
  put(e, 'A1', M);
  e.setCell('Hoja1', 'G1', '=TAKE(A1:C3, 2)');
  expect(block(e, 'G1', 2, 3)).toEqual([
    [1, 2, 3],
    [4, 5, 6],
  ]);
  e.setCell('Hoja1', 'K1', '=TAKE(A1:C3, 2, 2)');
  expect(block(e, 'K1', 2, 2)).toEqual([
    [1, 2],
    [4, 5],
  ]);
  e.setCell('Hoja1', 'K5', '=TAKE(A1:C3, -1)'); // última fila
  expect(block(e, 'K5', 1, 3)).toEqual([[7, 8, 9]]);
  e.setCell('Hoja1', 'K8', '=TAKE(A1:C3, 2, -1)'); // 2 filas, última col
  expect(block(e, 'K8', 2, 1)).toEqual([[3], [6]]);
});

test('DROP: descarta filas y columnas del principio o del final', () => {
  const e = new Engine();
  put(e, 'A1', M);
  e.setCell('Hoja1', 'G1', '=DROP(A1:C3, 1)');
  expect(block(e, 'G1', 2, 3)).toEqual([
    [4, 5, 6],
    [7, 8, 9],
  ]);
  e.setCell('Hoja1', 'K1', '=DROP(A1:C3, 1, 1)');
  expect(block(e, 'K1', 2, 2)).toEqual([
    [5, 6],
    [8, 9],
  ]);
  e.setCell('Hoja1', 'K5', '=DROP(A1:C3, -1)'); // sin la última fila
  expect(block(e, 'K5', 2, 3)).toEqual([
    [1, 2, 3],
    [4, 5, 6],
  ]);
  e.setCell('Hoja1', 'K9', '=DROP(A1:C3, 0, -2)'); // sin las 2 últimas columnas
  expect(block(e, 'K9', 3, 1)).toEqual([[1], [4], [7]]);
});

test('CHOOSEROWS / CHOOSECOLS: selección y reordenación por índice', () => {
  const e = new Engine();
  put(e, 'A1', M);
  e.setCell('Hoja1', 'G1', '=CHOOSEROWS(A1:C3, 3, 1)');
  expect(block(e, 'G1', 2, 3)).toEqual([
    [7, 8, 9],
    [1, 2, 3],
  ]);
  e.setCell('Hoja1', 'K1', '=CHOOSECOLS(A1:C3, 3, 1)');
  expect(block(e, 'K1', 3, 2)).toEqual([
    [3, 1],
    [6, 4],
    [9, 7],
  ]);
  e.setCell('Hoja1', 'P1', '=CHOOSEROWS(A1:C3, -1)'); // última fila
  expect(block(e, 'P1', 1, 3)).toEqual([[7, 8, 9]]);
  e.setCell('Hoja1', 'P3', '=CHOOSECOLS(A1:C3, 4)'); // fuera de rango
  expect(disp(e, 'P3')).toBe('#VALUE!');
});

test('TOROW / TOCOL: aplanado por filas o por columnas', () => {
  const e = new Engine();
  put(e, 'A1', M);
  e.setCell('Hoja1', 'G1', '=TOROW(A1:C3)');
  expect(block(e, 'G1', 1, 9)).toEqual([[1, 2, 3, 4, 5, 6, 7, 8, 9]]);
  e.setCell('Hoja1', 'A6', '=TOCOL(A1:C3)');
  expect(block(e, 'A6', 9, 1).flat()).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9]);
  e.setCell('Hoja1', 'C6', '=TOCOL(A1:C3, 0, TRUE)'); // por columnas
  expect(block(e, 'C6', 9, 1).flat()).toEqual([1, 4, 7, 2, 5, 8, 3, 6, 9]);
});

test('TOCOL ignora blancos y errores', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '1');
  e.setCell('Hoja1', 'A2', ''); // blanco
  e.setCell('Hoja1', 'A3', '=1/0'); // error
  e.setCell('Hoja1', 'A4', '4');
  e.setCell('Hoja1', 'C1', '=TOCOL(A1:A4, 3)'); // 3 = ignora blancos y errores
  expect(block(e, 'C1', 2, 1).flat()).toEqual([1, 4]);
});

test('WRAPROWS / WRAPCOLS: envuelve un vector con relleno', () => {
  const e = new Engine();
  put(e, 'A1', [[1, 2, 3, 4, 5]]); // vector fila E... en A1:E1
  e.setCell('Hoja1', 'A3', '=WRAPROWS(A1:E1, 2)');
  expect(block(e, 'A3', 2, 2)).toEqual([
    [1, 2],
    [3, 4],
  ]);
  expect(e.getValue('Hoja1', 'A5')).toBe(5);
  expect(disp(e, 'B5')).toBe('#N/A'); // relleno por defecto
  e.setCell('Hoja1', 'H3', '=WRAPROWS(A1:E1, 2, 0)'); // relleno 0
  expect(block(e, 'H3', 3, 2)).toEqual([
    [1, 2],
    [3, 4],
    [5, 0],
  ]);
  e.setCell('Hoja1', 'A8', '=WRAPCOLS(A1:E1, 2)');
  expect(block(e, 'A8', 2, 3).slice(0, 1)).toEqual([[1, 3, 5]]);
  expect(e.getValue('Hoja1', 'A9')).toBe(2);
  expect(disp(e, 'C9')).toBe('#N/A');
});

test('HSTACK: apila en horizontal y rellena huecos con #N/A', () => {
  const e = new Engine();
  put(e, 'A1', M);
  e.setCell('Hoja1', 'G1', '=HSTACK(A1:A3, B1:B3)');
  expect(block(e, 'G1', 3, 2)).toEqual([
    [1, 2],
    [4, 5],
    [7, 8],
  ]);
  e.setCell('Hoja1', 'K1', '=HSTACK(A1:A2, B1:B3)'); // alturas distintas
  expect(e.getValue('Hoja1', 'K1')).toBe(1);
  expect(e.getValue('Hoja1', 'L3')).toBe(8);
  expect(disp(e, 'K3')).toBe('#N/A'); // hueco de la 1ª matriz
});

test('VSTACK: apila en vertical y rellena huecos con #N/A', () => {
  const e = new Engine();
  put(e, 'A1', M);
  e.setCell('Hoja1', 'G1', '=VSTACK(A1:C1, A2:C2)');
  expect(block(e, 'G1', 2, 3)).toEqual([
    [1, 2, 3],
    [4, 5, 6],
  ]);
  e.setCell('Hoja1', 'K1', '=VSTACK(A1:B1, A2:C2)'); // anchuras distintas
  expect(block(e, 'K1', 1, 3).flat().slice(0, 2)).toEqual([1, 2]);
  expect(disp(e, 'M1')).toBe('#N/A'); // hueco de la 1ª matriz
  expect(block(e, 'K2', 1, 3)).toEqual([[4, 5, 6]]);
});

test('EXPAND: amplía con relleno; encoger es #VALUE!', () => {
  const e = new Engine();
  put(e, 'A1', [
    [1, 2],
    [4, 5],
  ]);
  e.setCell('Hoja1', 'G1', '=EXPAND(A1:B2, 3, 3, 0)');
  expect(block(e, 'G1', 3, 3)).toEqual([
    [1, 2, 0],
    [4, 5, 0],
    [0, 0, 0],
  ]);
  e.setCell('Hoja1', 'K1', '=EXPAND(A1:B2, 3)'); // solo filas, relleno #N/A
  expect(block(e, 'K1', 2, 2)).toEqual([
    [1, 2],
    [4, 5],
  ]);
  expect(disp(e, 'K3')).toBe('#N/A');
  e.setCell('Hoja1', 'P1', '=EXPAND(A1:B2, 1)'); // encoger
  expect(disp(e, 'P1')).toBe('#VALUE!');
});
