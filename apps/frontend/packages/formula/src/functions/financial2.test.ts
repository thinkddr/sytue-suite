// Financieras añadidas: tasas, conversión dólar y bono simple. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

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

test('PDURATION / RRI', () => {
  expect(num('=PDURATION(0.025, 2000, 2200)')).toBeCloseTo(3.859866, 5);
  expect(num('=RRI(96, 10000, 11000)')).toBeCloseTo(0.0009933, 6);
});

test('EFFECT / NOMINAL round-trip', () => {
  expect(num('=EFFECT(0.0525, 4)')).toBeCloseTo(0.0535427, 6);
  expect(num('=NOMINAL(0.0535427, 4)')).toBeCloseTo(0.0525, 6);
  expect(show('=EFFECT(0.05, 0)')).toBe('#NUM!');
});

test('DOLLARDE / DOLLARFR', () => {
  expect(num('=DOLLARDE(1.02, 16)')).toBeCloseTo(1.125, 9);
  expect(num('=DOLLARDE(1.1, 32)')).toBeCloseTo(1.3125, 9);
  expect(num('=DOLLARFR(1.125, 16)')).toBeCloseTo(1.02, 9);
  expect(show('=DOLLARDE(1.02, 0)')).toBe('#DIV/0!');
});

test('bono a la par: PRICE=100, YIELD=cupón', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=DATE(2020,1,1)');
  e.setCell('Hoja1', 'A2', '=DATE(2025,1,1)');
  e.setCell('Hoja1', 'P1', '=PRICE(A1, A2, 0.05, 0.05, 100, 2)');
  e.setCell('Hoja1', 'Y1', '=YIELD(A1, A2, 0.05, 100, 100, 2)');
  e.setCell('Hoja1', 'D1', '=DURATION(A1, A2, 0.05, 0.05, 2)');
  expect(e.getValue('Hoja1', 'P1') as number).toBeCloseTo(100, 4);
  expect(e.getValue('Hoja1', 'Y1') as number).toBeCloseTo(0.05, 5);
  expect(e.getValue('Hoja1', 'D1') as number).toBeCloseTo(4.48546, 4);
});

test('PRICE cae si el rendimiento supera al cupón', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=DATE(2020,1,1)');
  e.setCell('Hoja1', 'A2', '=DATE(2025,1,1)');
  e.setCell('Hoja1', 'P1', '=PRICE(A1, A2, 0.05, 0.08, 100, 2)');
  expect(e.getValue('Hoja1', 'P1') as number).toBeLessThan(100);
});
