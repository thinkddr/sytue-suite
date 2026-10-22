// Estadística añadida: percentiles/cuartiles .INC/.EXC, RANK.EQ/.AVG, PERCENTRANK,
// TRIMMEAN, STEYX, MODE.SNGL/.MULT y utilidades normales (STANDARDIZE/FISHER/
// GAUSS/PHI). Valores contrastados con Excel. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

function setCol(e: Engine, letter: string, vals: (string | number)[]): void {
  vals.forEach((v, i) => e.setCell('Hoja1', `${letter}${i + 1}`, String(v)));
}
function num(formula: string, cells: Record<string, string> = {}): number {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1') as number;
}
function show(formula: string): string {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

test('PERCENTILE.INC / .EXC y QUARTILE.INC / .EXC con {1,2,3,4,5}', () => {
  const e = new Engine();
  setCol(e, 'A', [1, 2, 3, 4, 5]);
  e.setCell('Hoja1', 'B1', '=PERCENTILE.INC(A1:A5, 0.25)');
  e.setCell('Hoja1', 'B2', '=PERCENTILE.EXC(A1:A5, 0.25)');
  e.setCell('Hoja1', 'B3', '=PERCENTILE.EXC(A1:A5, 0.5)');
  e.setCell('Hoja1', 'B4', '=QUARTILE.INC(A1:A5, 1)');
  e.setCell('Hoja1', 'B5', '=QUARTILE.EXC(A1:A5, 1)');
  expect(e.getValue('Hoja1', 'B1')).toBe(2); // inclusivo = clásico
  expect(e.getValue('Hoja1', 'B2') as number).toBeCloseTo(1.5, 10);
  expect(e.getValue('Hoja1', 'B3') as number).toBeCloseTo(3, 10);
  expect(e.getValue('Hoja1', 'B4')).toBe(2);
  expect(e.getValue('Hoja1', 'B5') as number).toBeCloseTo(1.5, 10);
  e.setCell('Hoja1', 'B6', '=QUARTILE.EXC(A1:A5, 0)'); // exclusivo no admite 0
  expect(e.getDisplay('Hoja1', 'B6')).toBe('#NUM!');
  e.setCell('Hoja1', 'B7', '=PERCENTILE.EXC(A1:A5, 0.1)'); // < 1/(n+1)
  expect(e.getDisplay('Hoja1', 'B7')).toBe('#NUM!');
});

test('RANK.EQ y RANK.AVG con empates', () => {
  const e = new Engine();
  setCol(e, 'B', [10, 20, 20, 30, 40]);
  e.setCell('Hoja1', 'C1', '=RANK.EQ(20, B1:B5)'); // descendente
  e.setCell('Hoja1', 'C2', '=RANK.AVG(20, B1:B5)');
  e.setCell('Hoja1', 'C3', '=RANK.EQ(40, B1:B5)');
  expect(e.getValue('Hoja1', 'C1')).toBe(3);
  expect(e.getValue('Hoja1', 'C2') as number).toBeCloseTo(3.5, 10);
  expect(e.getValue('Hoja1', 'C3')).toBe(1);
  e.setCell('Hoja1', 'C4', '=RANK.AVG(99, B1:B5)'); // ausente
  expect(e.getDisplay('Hoja1', 'C4')).toBe('#N/A');
});

test('PERCENTRANK.INC / .EXC con {1,2,3,4}', () => {
  const e = new Engine();
  setCol(e, 'A', [1, 2, 3, 4]);
  expect(num('=PERCENTRANK.INC(A1:A4, 3)', colCells())).toBeCloseTo(0.666, 10);
  expect(num('=PERCENTRANK.INC(A1:A4, 1)', colCells())).toBeCloseTo(0, 10);
  expect(num('=PERCENTRANK.INC(A1:A4, 4)', colCells())).toBeCloseTo(1, 10);
  expect(num('=PERCENTRANK.INC(A1:A4, 2.5)', colCells())).toBeCloseTo(0.5, 10);
  expect(num('=PERCENTRANK.EXC(A1:A4, 3)', colCells())).toBeCloseTo(0.6, 10);
  expect(num('=PERCENTRANK.EXC(A1:A4, 1)', colCells())).toBeCloseTo(0.2, 10);
  expect(num('=PERCENTRANK.INC(A1:A4, 3, 2)', colCells())).toBeCloseTo(0.66, 10); // significancia 2
  setCol(e, 'A', [1, 2, 3, 4]);
  e.setCell('Hoja1', 'Z1', '=PERCENTRANK.INC(A1:A4, 5)'); // fuera de rango
  expect(e.getDisplay('Hoja1', 'Z1')).toBe('#N/A');
});

function colCells(): Record<string, string> {
  return { A1: '1', A2: '2', A3: '3', A4: '4' };
}

test('TRIMMEAN recorta los extremos', () => {
  const e = new Engine();
  setCol(e, 'A', [1, 2, 3, 4, 100]);
  e.setCell('Hoja1', 'B1', '=TRIMMEAN(A1:A5, 0.4)'); // recorta 1 de cada extremo
  expect(e.getValue('Hoja1', 'B1') as number).toBeCloseTo(3, 10);
  e.setCell('Hoja1', 'B2', '=TRIMMEAN(A1:A5, 0)'); // sin recorte = media
  expect(e.getValue('Hoja1', 'B2') as number).toBeCloseTo(22, 10);
  e.setCell('Hoja1', 'B3', '=TRIMMEAN(A1:A5, 1)');
  expect(e.getDisplay('Hoja1', 'B3')).toBe('#NUM!');
});

test('STEYX: error típico de la regresión', () => {
  const e = new Engine();
  setCol(e, 'A', [1, 2, 3, 5]); // y
  setCol(e, 'B', [1, 2, 3, 4]); // x
  e.setCell('Hoja1', 'C1', '=STEYX(A1:A4, B1:B4)');
  expect(e.getValue('Hoja1', 'C1') as number).toBeCloseTo(0.3872983, 6);
  // ajuste perfecto → 0
  setCol(e, 'D', [2, 4, 6, 8]);
  e.setCell('Hoja1', 'C2', '=STEYX(D1:D4, B1:B4)');
  expect(e.getValue('Hoja1', 'C2') as number).toBeCloseTo(0, 9);
});

test('MODE.SNGL y MODE.MULT', () => {
  const e = new Engine();
  setCol(e, 'A', [1, 2, 2, 3, 3, 4]);
  e.setCell('Hoja1', 'C1', '=MODE.SNGL(A1:A6)');
  expect(e.getValue('Hoja1', 'C1')).toBe(2); // el primero en empate
  e.setCell('Hoja1', 'E1', '=MODE.MULT(A1:A6)'); // derrama 2 y 3
  expect(e.getValue('Hoja1', 'E1')).toBe(2);
  expect(e.getValue('Hoja1', 'E2')).toBe(3);
  setCol(e, 'G', [1, 2, 3]);
  e.setCell('Hoja1', 'H1', '=MODE.MULT(G1:G3)'); // ninguno se repite
  expect(e.getDisplay('Hoja1', 'H1')).toBe('#N/A');
});

test('STANDARDIZE / FISHER / FISHERINV / GAUSS / PHI', () => {
  expect(num('=STANDARDIZE(42, 40, 1.5)')).toBeCloseTo(1.3333333, 6);
  expect(show('=STANDARDIZE(1, 0, 0)')).toBe('#NUM!');
  expect(num('=FISHER(0.75)')).toBeCloseTo(0.9729551, 6);
  expect(num('=FISHERINV(FISHER(0.75))')).toBeCloseTo(0.75, 9); // ida y vuelta
  expect(show('=FISHER(1)')).toBe('#NUM!');
  expect(num('=GAUSS(2)')).toBeCloseTo(0.4772499, 5);
  expect(num('=PHI(0)')).toBeCloseTo(0.3989423, 6);
  expect(num('=PHI(1)')).toBeCloseTo(0.2419707, 6);
});
