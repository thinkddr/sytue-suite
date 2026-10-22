// Distribuciones estadísticas (Γ, χ², t, F, Beta, Poisson, Binomial…) y
// agregadas de regresión. Valores contrastados con Excel. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

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
function setCol(e: Engine, letter: string, vals: string[]): void {
  vals.forEach((v, i) => e.setCell('Hoja1', `${letter}${i + 1}`, v));
}

test('GAMMALN / GAMMA', () => {
  expect(num('=GAMMALN(4)')).toBeCloseTo(Math.log(6), 6); // ln(3!)
  expect(num('=GAMMA(5)')).toBeCloseTo(24, 6); // 4!
  expect(num('=GAMMA(0.5)')).toBeCloseTo(Math.sqrt(Math.PI), 6);
  expect(show('=GAMMA(0)')).toBe('#NUM!');
  expect(show('=GAMMALN(-1)')).toBe('#NUM!');
});

test('GAMMA.DIST / GAMMA.INV round-trip', () => {
  // P(9,10) = 1 − PoissonCDF(8;10) = 0.667180.
  expect(num('=GAMMA.DIST(10, 9, 1, TRUE)')).toBeCloseTo(0.66718, 5);
  expect(num('=GAMMA.INV(0.667180, 9, 1)')).toBeCloseTo(10, 3);
});

test('POISSON.DIST', () => {
  expect(num('=POISSON.DIST(2, 5, FALSE)')).toBeCloseTo(0.0842243, 6);
  expect(num('=POISSON.DIST(2, 5, TRUE)')).toBeCloseTo(0.124652, 5);
  expect(num('=POISSON.DIST(0, 0, FALSE)')).toBe(1);
});

test('EXPON.DIST', () => {
  expect(num('=EXPON.DIST(0.2, 10, TRUE)')).toBeCloseTo(0.8646647, 6);
  expect(num('=EXPON.DIST(0.2, 10, FALSE)')).toBeCloseTo(1.3533528, 6);
});

test('BINOM.DIST (pmf y acumulada)', () => {
  expect(num('=BINOM.DIST(3, 10, 0.3, FALSE)')).toBeCloseTo(0.2668279, 6);
  expect(num('=BINOM.DIST(3, 10, 0.3, TRUE)')).toBeCloseTo(0.6496107, 6);
  expect(num('=BINOMDIST(6, 10, 0.5, FALSE)')).toBeCloseTo(0.2050781, 6);
});

test('WEIBULL.DIST', () => {
  expect(num('=WEIBULL.DIST(1, 1, 1, TRUE)')).toBeCloseTo(0.6321206, 6);
  expect(num('=WEIBULL.DIST(1, 1, 1, FALSE)')).toBeCloseTo(0.3678794, 6);
});

test('T.DIST familia', () => {
  expect(num('=T.DIST(2, 10, TRUE)')).toBeCloseTo(0.9633049, 5);
  expect(num('=T.DIST.2T(2, 10)')).toBeCloseTo(0.0733893, 5);
  expect(num('=T.DIST.RT(2, 10)')).toBeCloseTo(0.0366951, 5);
  expect(num('=T.INV(0.05, 10)')).toBeCloseTo(-1.812461, 4);
  expect(num('=T.INV.2T(0.05, 10)')).toBeCloseTo(2.228139, 4);
});

test('CHISQ familia', () => {
  expect(num('=CHISQ.DIST(3, 4, TRUE)')).toBeCloseTo(0.442175, 5);
  expect(num('=CHISQ.DIST.RT(3, 4)')).toBeCloseTo(0.557825, 5);
  expect(num('=CHISQ.INV(0.442175, 4)')).toBeCloseTo(3, 3);
  expect(num('=CHISQ.INV.RT(0.05, 10)')).toBeCloseTo(18.30704, 3);
});

test('F familia', () => {
  expect(num('=F.INV.RT(0.05, 5, 10)')).toBeCloseTo(3.325835, 4);
  expect(num('=F.DIST.RT(3.325835, 5, 10)')).toBeCloseTo(0.05, 5);
  expect(num('=F.DIST(3.325835, 5, 10, TRUE)')).toBeCloseTo(0.95, 5);
});

test('BETA.DIST / BETA.INV', () => {
  expect(num('=BETA.DIST(0.5, 2, 3, TRUE)')).toBeCloseTo(0.6875, 6);
  expect(num('=BETA.INV(0.6875, 2, 3)')).toBeCloseTo(0.5, 5);
  // con escala [A,B]
  expect(num('=BETA.DIST(5, 2, 3, TRUE, 0, 10)')).toBeCloseTo(0.6875, 6);
});

test('CONFIDENCE', () => {
  expect(num('=CONFIDENCE.NORM(0.05, 1, 100)')).toBeCloseTo(0.1959964, 6);
  expect(num('=CONFIDENCE(0.05, 1, 100)')).toBeCloseTo(0.1959964, 6);
  expect(num('=CONFIDENCE.T(0.05, 1, 100)')).toBeCloseTo(0.1984217, 5);
});

test('DEVSQ / AVEDEV', () => {
  const e = new Engine();
  setCol(e, 'A', ['2', '4', '4', '4', '5', '5', '7', '9']);
  e.setCell('Hoja1', 'B1', '=DEVSQ(A1:A8)');
  e.setCell('Hoja1', 'B2', '=AVEDEV(A1:A8)');
  expect(e.getValue('Hoja1', 'B1')).toBe(32);
  expect(e.getValue('Hoja1', 'B2')).toBe(1.5);
});

test('SKEW / KURT (ejemplo de Excel)', () => {
  const e = new Engine();
  setCol(e, 'A', ['3', '4', '5', '2', '3', '4', '5', '6', '4', '7']);
  e.setCell('Hoja1', 'B1', '=SKEW(A1:A10)');
  e.setCell('Hoja1', 'B2', '=KURT(A1:A10)');
  expect(e.getValue('Hoja1', 'B1') as number).toBeCloseTo(0.3595431, 5);
  expect(e.getValue('Hoja1', 'B2') as number).toBeCloseTo(-0.151799, 4);
});

test('regresión lineal exacta y = 2x', () => {
  const e = new Engine();
  setCol(e, 'X', ['1', '2', '3']);
  setCol(e, 'Y', ['2', '4', '6']);
  e.setCell('Hoja1', 'A1', '=SLOPE(Y1:Y3, X1:X3)');
  e.setCell('Hoja1', 'A2', '=INTERCEPT(Y1:Y3, X1:X3)');
  e.setCell('Hoja1', 'A3', '=CORREL(Y1:Y3, X1:X3)');
  e.setCell('Hoja1', 'A4', '=RSQ(Y1:Y3, X1:X3)');
  e.setCell('Hoja1', 'A5', '=FORECAST(4, Y1:Y3, X1:X3)');
  e.setCell('Hoja1', 'A6', '=COVARIANCE.P(Y1:Y3, X1:X3)');
  e.setCell('Hoja1', 'A7', '=COVARIANCE.S(Y1:Y3, X1:X3)');
  expect(e.getValue('Hoja1', 'A1')).toBeCloseTo(2, 9);
  expect(e.getValue('Hoja1', 'A2')).toBeCloseTo(0, 9);
  expect(e.getValue('Hoja1', 'A3')).toBeCloseTo(1, 9);
  expect(e.getValue('Hoja1', 'A4')).toBeCloseTo(1, 9);
  expect(e.getValue('Hoja1', 'A5')).toBeCloseTo(8, 9);
  expect(e.getValue('Hoja1', 'A6') as number).toBeCloseTo(4 / 3, 9);
  expect(e.getValue('Hoja1', 'A7')).toBeCloseTo(2, 9);
});

test('TREND derrama predicciones lineales', () => {
  const e = new Engine();
  setCol(e, 'Y', ['2', '4', '6']);
  setCol(e, 'X', ['1', '2', '3']);
  setCol(e, 'N', ['4', '5']);
  e.setCell('Hoja1', 'C1', '=TREND(Y1:Y3, X1:X3, N1:N2)');
  const out: CellValue[] = [e.getValue('Hoja1', 'C1'), e.getValue('Hoja1', 'C2')];
  expect(out).toEqual([8, 10]);
});

test('CORREL con longitudes distintas → #N/A', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3']);
  setCol(e, 'B', ['1', '2']);
  e.setCell('Hoja1', 'Z1', '=CORREL(A1:A3, B1:B2)');
  expect(e.getDisplay('Hoja1', 'Z1')).toBe('#N/A');
});
