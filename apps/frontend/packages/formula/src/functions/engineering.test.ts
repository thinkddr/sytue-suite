// Ingeniería: conversión de base, operaciones de bits y CONVERT. Valores
// contrastados con una hoja real (Excel). `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

function calc(formula: string): CellValue {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}
function show(formula: string): string {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

test('DEC2BIN / BIN2DEC (complemento a dos 10 bits)', () => {
  expect(calc('=DEC2BIN(9)')).toBe('1001');
  expect(calc('=DEC2BIN(5, 4)')).toBe('0101'); // relleno a 4
  expect(calc('=DEC2BIN(-1)')).toBe('1111111111'); // negativo → 10 bits
  expect(show('=DEC2BIN(512)')).toBe('#NUM!'); // fuera de [-512,511]
  expect(calc('=BIN2DEC("1001")')).toBe(9);
  expect(calc('=BIN2DEC("11")')).toBe(3);
  expect(calc('=BIN2DEC("1111111111")')).toBe(-1);
  expect(show('=BIN2DEC("2")')).toBe('#NUM!'); // dígito no binario
});

test('DEC2HEX / HEX2DEC (complemento a dos 40 bits)', () => {
  expect(calc('=DEC2HEX(255)')).toBe('FF');
  expect(calc('=DEC2HEX(28)')).toBe('1C');
  expect(calc('=DEC2HEX(-1)')).toBe('FFFFFFFFFF');
  expect(calc('=HEX2DEC("FF")')).toBe(255);
  expect(calc('=HEX2DEC("1C")')).toBe(28);
  expect(calc('=HEX2DEC("FFFFFFFFFF")')).toBe(-1);
});

test('DEC2OCT / OCT2DEC', () => {
  expect(calc('=DEC2OCT(8)')).toBe('10');
  expect(calc('=DEC2OCT(-1)')).toBe('7777777777');
  expect(calc('=OCT2DEC("10")')).toBe(8);
  expect(calc('=OCT2DEC("7777777777")')).toBe(-1);
});

test('BITAND / BITOR / BITXOR / BITLSHIFT / BITRSHIFT', () => {
  expect(calc('=BITAND(5, 3)')).toBe(1);
  expect(calc('=BITOR(5, 3)')).toBe(7);
  expect(calc('=BITXOR(5, 3)')).toBe(6);
  expect(calc('=BITLSHIFT(4, 2)')).toBe(16);
  expect(calc('=BITRSHIFT(16, 2)')).toBe(4);
  expect(calc('=BITRSHIFT(4, -1)')).toBe(8); // desplazamiento negativo = a la izquierda
  expect(show('=BITAND(-1, 3)')).toBe('#NUM!'); // operando negativo
});

test('CONVERT: masa, longitud, tiempo con prefijos SI', () => {
  expect(calc('=CONVERT(1, "km", "m")')).toBe(1000);
  expect(calc('=CONVERT(1, "hr", "sec")')).toBe(3600);
  expect(calc('=CONVERT(1, "mi", "yd")') as number).toBeCloseTo(1760, 6);
  expect(calc('=CONVERT(1, "lbm", "g")') as number).toBeCloseTo(453.59237, 5);
  expect(calc('=CONVERT(1000, "g", "kg")')).toBe(1);
});

test('CONVERT: temperatura (conversión afín) y unidades incompatibles', () => {
  expect(calc('=CONVERT(100, "C", "F")') as number).toBeCloseTo(212, 9);
  expect(calc('=CONVERT(32, "F", "C")') as number).toBeCloseTo(0, 9);
  expect(calc('=CONVERT(0, "C", "K")') as number).toBeCloseTo(273.15, 9);
  expect(show('=CONVERT(1, "m", "g")')).toBe('#N/A'); // categorías distintas
  expect(show('=CONVERT(1, "m", "zzz")')).toBe('#N/A'); // unidad desconocida
});
