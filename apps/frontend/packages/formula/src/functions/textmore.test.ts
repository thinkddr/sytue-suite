// Texto añadido: VALUETOTEXT y los formatos numéricos DOLLAR / FIXED.
// Contrastado con una hoja real. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

function calc(formula: string): CellValue {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}

test('VALUETOTEXT: conciso y estricto', () => {
  expect(calc('=VALUETOTEXT(123)')).toBe('123');
  expect(calc('=VALUETOTEXT(123, 1)')).toBe('123'); // los números no se entrecomillan
  expect(calc('=VALUETOTEXT("abc")')).toBe('abc');
  expect(calc('=VALUETOTEXT("abc", 1)')).toBe('"abc"');
  expect(calc('=VALUETOTEXT(TRUE)')).toBe('TRUE');
  expect(calc('=VALUETOTEXT(NA())')).toBe('#N/A'); // el error se convierte en su código
});

test('DOLLAR: separador de miles, símbolo y paréntesis en negativos', () => {
  expect(calc('=DOLLAR(1234.567)')).toBe('$1,234.57');
  expect(calc('=DOLLAR(1234.567, 0)')).toBe('$1,235');
  expect(calc('=DOLLAR(-1234.567, 2)')).toBe('($1,234.57)');
  expect(calc('=DOLLAR(1234.567, -2)')).toBe('$1,200'); // decimales negativos
  expect(calc('=DOLLAR(0)')).toBe('$0.00');
  expect(calc('=DOLLAR(0.5)')).toBe('$0.50');
});

test('FIXED: decimales fijos, signo menos y opción sin comas', () => {
  expect(calc('=FIXED(1234.567)')).toBe('1,234.57');
  expect(calc('=FIXED(1234.567, 1)')).toBe('1,234.6');
  expect(calc('=FIXED(1234.567, -1)')).toBe('1,230');
  expect(calc('=FIXED(1234.567, 1, TRUE)')).toBe('1234.6'); // sin separador de miles
  expect(calc('=FIXED(-1234.5, 1)')).toBe('-1,234.5');
  expect(calc('=FIXED(0)')).toBe('0.00');
});
