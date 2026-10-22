// Ingeniería añadida: DELTA/GESTEP, función error (ERF/ERFC) y conversiones de
// base cruzadas (BIN/OCT/HEX). Valores contrastados con una hoja real. `bun test`.
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

test('DELTA: delta de Kronecker', () => {
  expect(calc('=DELTA(5, 5)')).toBe(1);
  expect(calc('=DELTA(5, 4)')).toBe(0);
  expect(calc('=DELTA(0)')).toBe(1); // n2 = 0 por defecto
  expect(calc('=DELTA(2)')).toBe(0);
  expect(show('=DELTA("x")')).toBe('#VALUE!');
});

test('GESTEP: función escalón', () => {
  expect(calc('=GESTEP(5, 4)')).toBe(1);
  expect(calc('=GESTEP(3, 4)')).toBe(0);
  expect(calc('=GESTEP(4, 4)')).toBe(1); // ≥ es inclusivo
  expect(calc('=GESTEP(5)')).toBe(1); // step = 0
  expect(calc('=GESTEP(-1)')).toBe(0);
});

test('ERF / ERFC / .PRECISE', () => {
  expect(calc('=ERF(1)') as number).toBeCloseTo(0.8427008, 6);
  expect(calc('=ERF(0)') as number).toBeCloseTo(0, 6);
  expect(calc('=ERF(0.5)') as number).toBeCloseTo(0.5204999, 6);
  expect(calc('=ERF(1, 2)') as number).toBeCloseTo(0.1526213, 6); // erf(2)-erf(1)
  expect(calc('=ERFC(1)') as number).toBeCloseTo(0.1572992, 6);
  expect(calc('=ERFC(0)') as number).toBeCloseTo(1, 6);
  expect(calc('=ERF.PRECISE(1)') as number).toBeCloseTo(0.8427008, 6);
  expect(calc('=ERFC.PRECISE(1)') as number).toBeCloseTo(0.1572992, 6);
  expect(calc('=ERF(-1)') as number).toBeCloseTo(-0.8427008, 6); // erf es impar
});

test('conversiones de base cruzadas', () => {
  expect(calc('=BIN2OCT("1001")')).toBe('11');
  expect(calc('=BIN2OCT("1001", 3)')).toBe('011'); // relleno a 3
  expect(calc('=BIN2HEX("1111011")')).toBe('7B');
  expect(calc('=BIN2HEX("1111111111")')).toBe('FFFFFFFFFF'); // negativo → 40 bits
  expect(calc('=OCT2BIN("11")')).toBe('1001');
  expect(calc('=OCT2HEX("144")')).toBe('64');
  expect(calc('=HEX2BIN("64")')).toBe('1100100');
  expect(calc('=HEX2BIN("F", 8)')).toBe('00001111');
  expect(calc('=HEX2OCT("64")')).toBe('144');
  expect(calc('=OCT2HEX("7777777777")')).toBe('FFFFFFFFFF'); // -1
});

test('conversiones cruzadas: errores de dominio', () => {
  expect(show('=HEX2BIN("200")')).toBe('#NUM!'); // 512 no cabe en 10 bits con signo
  expect(show('=BIN2OCT("2")')).toBe('#NUM!'); // dígito no binario
  expect(show('=HEX2OCT("ZZ")')).toBe('#NUM!'); // no hexadecimal
});

test('regresión: base conversion básica intacta tras el refactor', () => {
  expect(calc('=DEC2BIN(9)')).toBe('1001');
  expect(calc('=DEC2BIN(5, 4)')).toBe('0101');
  expect(calc('=DEC2HEX(-1)')).toBe('FFFFFFFFFF');
  expect(calc('=BIN2DEC("1111111111")')).toBe(-1);
  expect(calc('=HEX2DEC("FF")')).toBe(255);
});
