// Estadística moderna: distribución normal (NORM.*) y variantes .S/.P.
// Valores contrastados con una hoja real (Excel). `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

function setCol(e: Engine, letter: string, vals: string[]): void {
  vals.forEach((v, i) => e.setCell('Hoja1', `${letter}${i + 1}`, v));
}
function num(formula: string): number {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1') as number;
}
function show(formula: string): string {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

test('NORM.S.DIST: CDF y densidad estándar', () => {
  expect(num('=NORM.S.DIST(0, TRUE)')).toBeCloseTo(0.5, 6);
  expect(num('=NORM.S.DIST(1.96, TRUE)')).toBeCloseTo(0.975, 4);
  expect(num('=NORM.S.DIST(0, FALSE)')).toBeCloseTo(0.3989422804, 5);
});

test('NORM.DIST: normal general, acumulada y densidad', () => {
  expect(num('=NORM.DIST(50, 50, 10, TRUE)')).toBeCloseTo(0.5, 6);
  expect(num('=NORM.DIST(60, 50, 10, TRUE)')).toBeCloseTo(0.8413447, 4);
  expect(num('=NORM.DIST(50, 50, 10, FALSE)')).toBeCloseTo(0.0398942, 5);
  expect(show('=NORM.DIST(1, 0, 0, TRUE)')).toBe('#NUM!'); // desviación 0
});

test('NORM.INV / NORM.S.INV: inversas (probit)', () => {
  expect(num('=NORM.S.INV(0.975)')).toBeCloseTo(1.959964, 4);
  expect(num('=NORM.S.INV(0.5)')).toBeCloseTo(0, 6);
  expect(num('=NORM.INV(0.5, 50, 10)')).toBeCloseTo(50, 6);
  expect(num('=NORM.INV(0.975, 0, 1)')).toBeCloseTo(1.959964, 4);
  expect(show('=NORM.S.INV(0)')).toBe('#NUM!'); // fuera de (0,1)
  expect(show('=NORM.INV(1, 0, 1)')).toBe('#NUM!');
});

test('inversa y CDF son consistentes (round-trip)', () => {
  const p = num('=NORM.S.DIST(NORM.S.INV(0.3), TRUE)');
  expect(p).toBeCloseTo(0.3, 4);
});

test('STDEV.S/.P y VAR.S/.P con {2,4,4,4,5,5,7,9} (media 5, ΣΣ=32)', () => {
  const e = new Engine();
  setCol(e, 'A', ['2', '4', '4', '4', '5', '5', '7', '9']);
  e.setCell('Hoja1', 'B1', '=VAR.P(A1:A8)');
  e.setCell('Hoja1', 'B2', '=STDEV.P(A1:A8)');
  e.setCell('Hoja1', 'B3', '=VAR.S(A1:A8)');
  e.setCell('Hoja1', 'B4', '=STDEV.S(A1:A8)');
  expect(e.getValue('Hoja1', 'B1')).toBe(4);
  expect(e.getValue('Hoja1', 'B2')).toBe(2);
  expect(e.getValue('Hoja1', 'B3') as number).toBeCloseTo(32 / 7, 10);
  expect(e.getValue('Hoja1', 'B4') as number).toBeCloseTo(Math.sqrt(32 / 7), 10);
  e.setCell('Hoja1', 'B5', '=STDEV.S(5)'); // <2 valores → #DIV/0!
  expect(e.getDisplay('Hoja1', 'B5')).toBe('#DIV/0!');
});
