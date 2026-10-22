import { expect, test } from 'bun:test';
import { coerceNumber, ERR } from './values.js';

test('coerceNumber acepta decimales y exponentes sin ambigüedad', () => {
  expect(coerceNumber('12')).toBe(12);
  expect(coerceNumber('12.')).toBe(12);
  expect(coerceNumber('.5')).toBe(0.5);
  expect(coerceNumber('1.25e-2')).toBe(0.0125);
  expect(coerceNumber('1.2.3')).toBe(ERR.VALUE);
});

test('coerceNumber maneja una entrada numérica larga', () => {
  expect(coerceNumber(`${'1'.repeat(100_000)}x`)).toBe(ERR.VALUE);
});
