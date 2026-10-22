// Álgebra matricial: MMULT/MINVERSE/MDETERM/MUNIT y SUMX2MY2/…. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

/** Coloca una matriz 2D a partir de la celda A1 dada, por filas. */
function put(e: Engine, topLeft: string, rows: number[][]): void {
  const col0 = topLeft.charCodeAt(0) - 65;
  const row0 = Number(topLeft.slice(1));
  rows.forEach((r, i) =>
    r.forEach((v, j) =>
      e.setCell('Hoja1', `${String.fromCharCode(65 + col0 + j)}${row0 + i}`, String(v)),
    ),
  );
}
/** Lee un bloque R×C a partir del ancla. */
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

test('MMULT producto matricial', () => {
  const e = new Engine();
  put(e, 'A1', [
    [1, 2],
    [3, 4],
  ]);
  put(e, 'D1', [
    [5, 6],
    [7, 8],
  ]);
  e.setCell('Hoja1', 'G1', '=MMULT(A1:B2, D1:E2)');
  expect(block(e, 'G1', 2, 2)).toEqual([
    [19, 22],
    [43, 50],
  ]);
});

test('MMULT dimensiones incompatibles → #VALUE!', () => {
  const e = new Engine();
  put(e, 'A1', [[1, 2, 3]]); // 1×3
  put(e, 'A3', [[1, 2]]); // 1×2
  e.setCell('Hoja1', 'G1', '=MMULT(A1:C1, A3:B3)');
  expect(e.getDisplay('Hoja1', 'G1')).toBe('#VALUE!');
});

test('MDETERM', () => {
  const e = new Engine();
  put(e, 'A1', [
    [1, 2],
    [3, 4],
  ]);
  e.setCell('Hoja1', 'G1', '=MDETERM(A1:B2)');
  expect(e.getValue('Hoja1', 'G1')).toBeCloseTo(-2, 9);
  put(e, 'A4', [
    [6, 1, 1],
    [4, -2, 5],
    [2, 8, 7],
  ]);
  e.setCell('Hoja1', 'G2', '=MDETERM(A4:C6)');
  expect(e.getValue('Hoja1', 'G2')).toBeCloseTo(-306, 9);
});

test('MINVERSE y su producto por A = identidad', () => {
  const e = new Engine();
  put(e, 'A1', [
    [4, 7],
    [2, 6],
  ]);
  e.setCell('Hoja1', 'G1', '=MINVERSE(A1:B2)');
  const inv = block(e, 'G1', 2, 2) as number[][];
  expect(inv[0]![0]).toBeCloseTo(0.6, 9);
  expect(inv[0]![1]).toBeCloseTo(-0.7, 9);
  expect(inv[1]![0]).toBeCloseTo(-0.2, 9);
  expect(inv[1]![1]).toBeCloseTo(0.4, 9);
  // A · A⁻¹ = I
  e.setCell('Hoja1', 'J1', '=MMULT(A1:B2, G1:H2)');
  const id = block(e, 'J1', 2, 2) as number[][];
  expect(id[0]![0]).toBeCloseTo(1, 9);
  expect(id[0]![1]).toBeCloseTo(0, 9);
  expect(id[1]![0]).toBeCloseTo(0, 9);
  expect(id[1]![1]).toBeCloseTo(1, 9);
});

test('MINVERSE de matriz singular → #NUM!', () => {
  const e = new Engine();
  put(e, 'A1', [
    [1, 2],
    [2, 4],
  ]); // filas proporcionales
  e.setCell('Hoja1', 'G1', '=MINVERSE(A1:B2)');
  expect(e.getDisplay('Hoja1', 'G1')).toBe('#NUM!');
});

test('MUNIT identidad', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=MUNIT(3)');
  expect(block(e, 'A1', 3, 3)).toEqual([
    [1, 0, 0],
    [0, 1, 0],
    [0, 0, 1],
  ]);
});

test('SUMX2MY2 / SUMX2PY2 / SUMXMY2', () => {
  const e = new Engine();
  put(e, 'A1', [[1], [2], [3]]);
  put(e, 'B1', [[4], [5], [6]]);
  e.setCell('Hoja1', 'D1', '=SUMX2MY2(A1:A3, B1:B3)');
  e.setCell('Hoja1', 'D2', '=SUMX2PY2(A1:A3, B1:B3)');
  e.setCell('Hoja1', 'D3', '=SUMXMY2(A1:A3, B1:B3)');
  expect(e.getValue('Hoja1', 'D1')).toBe(-63);
  expect(e.getValue('Hoja1', 'D2')).toBe(91);
  expect(e.getValue('Hoja1', 'D3')).toBe(27);
});
