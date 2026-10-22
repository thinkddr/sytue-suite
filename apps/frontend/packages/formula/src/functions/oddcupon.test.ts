/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cupones irregulares (ODDFPRICE, ODDFYIELD, ODDLPRICE, ODDLYIELD).
 *
 * Referencias: los EJEMPLOS DOCUMENTADOS de Microsoft (113,597717; 7,72 %; 99,87829; 4,52 %)
 * y las IDENTIDADES, que no dependen de ninguna tabla: con la emisión (o el último interés)
 * un periodo regular antes del cupón, el bono no tiene nada de irregular y el precio ha de
 * ser exactamente el de PRICE; y el rendimiento invierte al precio. LibreOffice contesta en
 * ODDLPRICE/ODDLYIELD (compat/) pero devuelve #VALUE! en ODDFPRICE y ODDFYIELD.
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

function num(formula: string): number {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  return e.getValue('Hoja1', 'A1') as number;
}
function err(formula: string): string {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  return e.getDisplay('Hoja1', 'A1');
}

const D = (y: number, m: number, d: number): string => `DATE(${y},${m},${d})`;
const F = `${D(2008, 11, 11)},${D(2021, 3, 1)},${D(2008, 10, 15)},${D(2009, 3, 1)}`;
const L = `${D(2008, 2, 7)},${D(2008, 6, 15)},${D(2007, 10, 15)}`;

test('ejemplos documentados de Microsoft', () => {
  expect(num(`=ODDFPRICE(${F},0.0785,0.0625,100,2,1)`)).toBeCloseTo(113.597717, 6);
  expect(num(`=ODDFYIELD(${F},0.0575,84.5,100,2,0)`)).toBeCloseTo(0.0772, 4);
  expect(num(`=ODDLPRICE(${L},0.0375,0.0405,100,2,0)`)).toBeCloseTo(99.87829, 5);
  expect(
    num(`=ODDLYIELD(${D(2008, 4, 20)},${D(2008, 6, 15)},${D(2007, 12, 24)},0.0375,99.875,100,2,0)`),
  ).toBeCloseTo(0.045192, 6);
});

test('sin nada irregular dan exactamente PRICE, con las bases 0, 1 y 4', () => {
  // Con las bases 2 y 3 los días son reales y la longitud del periodo es nominal (360 o 365 entre
  // la frecuencia), así que un periodo regular no mide exactamente 1: la misma rareza que ya
  // tienen COUPDAYBS y PRICE, y la que da LibreOffice (compat/).
  for (const base of [0, 1, 4]) {
    // Emisión = el cupón regular anterior al primero (1-sep-2008).
    const f = `${D(2008, 11, 11)},${D(2021, 3, 1)}`;
    expect(
      num(`=ODDFPRICE(${f},${D(2008, 9, 1)},${D(2009, 3, 1)},0.0785,0.0625,100,2,${base})`),
    ).toBeCloseTo(num(`=PRICE(${f},0.0785,0.0625,100,2,${base})`), 9);
    // Último interés = el cupón regular anterior al vencimiento (15-dic-2007).
    const l = `${D(2008, 2, 7)},${D(2008, 6, 15)}`;
    expect(num(`=ODDLPRICE(${l},${D(2007, 12, 15)},0.0375,0.0405,100,2,${base})`)).toBeCloseTo(
      num(`=PRICE(${l},0.0375,0.0405,100,2,${base})`),
      9,
    );
  }
});

test('el rendimiento invierte al precio, en periodos cortos y largos', () => {
  for (const emision of [D(2008, 10, 15), D(2008, 1, 15), D(2008, 9, 20)]) {
    const f = `${D(2008, 11, 11)},${D(2021, 3, 1)},${emision},${D(2009, 3, 1)}`;
    const precio = num(`=ODDFPRICE(${f},0.0785,0.0625,100,2,1)`);
    expect(num(`=ODDFYIELD(${f},0.0785,${precio},100,2,1)`)).toBeCloseTo(0.0625, 9);
  }
  for (const ultimo of [D(2007, 10, 15), D(2007, 12, 24), D(2007, 7, 1)]) {
    const l = `${D(2008, 2, 7)},${D(2008, 9, 15)},${ultimo}`;
    const precio = num(`=ODDLPRICE(${l},0.0375,0.0405,100,2,0)`);
    expect(num(`=ODDLYIELD(${l},0.0375,${precio},100,2,0)`)).toBeCloseTo(0.0405, 12);
  }
});

test('un primer periodo largo cuesta lo que suman sus cuasi-cupones', () => {
  // Emisión 14 meses antes del primer cupón: dos cuasi-cupones. Con el rendimiento igual al
  // cupón y sin devengo (liquidación = emisión + 1 día) el precio ronda la par.
  const f = `${D(2008, 1, 16)},${D(2021, 3, 1)},${D(2008, 1, 15)},${D(2009, 3, 1)}`;
  expect(num(`=ODDFPRICE(${f},0.06,0.06,100,2,0)`)).toBeGreaterThan(99);
  expect(num(`=ODDFPRICE(${f},0.06,0.06,100,2,0)`)).toBeLessThan(105);
});

test('errores: fechas fuera de orden, primer cupón fuera del calendario, argumentos inválidos', () => {
  // Orden: vencimiento > primer cupón > liquidación > emisión.
  expect(
    err(
      `=ODDFPRICE(${D(2009, 4, 1)},${D(2021, 3, 1)},${D(2008, 10, 15)},${D(2009, 3, 1)},0.05,0.05,100,2,1)`,
    ),
  ).toBe('#NUM!');
  expect(
    err(
      `=ODDFPRICE(${D(2008, 10, 1)},${D(2021, 3, 1)},${D(2008, 10, 15)},${D(2009, 3, 1)},0.05,0.05,100,2,1)`,
    ),
  ).toBe('#NUM!');
  // El primer cupón (15-feb-2009) no es una fecha del calendario del vencimiento (1-mar).
  expect(
    err(
      `=ODDFPRICE(${D(2008, 11, 11)},${D(2021, 3, 1)},${D(2008, 10, 15)},${D(2009, 2, 15)},0.05,0.05,100,2,1)`,
    ),
  ).toBe('#NUM!');
  expect(err(`=ODDFPRICE(${F},-0.01,0.05,100,2,1)`)).toBe('#NUM!');
  expect(err(`=ODDFPRICE(${F},0.05,-0.05,100,2,1)`)).toBe('#NUM!');
  expect(err(`=ODDFPRICE(${F},0.05,0.05,0,2,1)`)).toBe('#NUM!');
  expect(err(`=ODDFPRICE(${F},0.05,0.05,100,3,1)`)).toBe('#NUM!');
  expect(err(`=ODDFPRICE(${F},0.05,0.05,100,2,5)`)).toBe('#NUM!');
  expect(err(`=ODDFYIELD(${F},0.05,0,100,2,1)`)).toBe('#NUM!');
  expect(
    err(`=ODDLPRICE(${D(2007, 9, 1)},${D(2008, 6, 15)},${D(2007, 10, 15)},0.05,0.05,100,2,0)`),
  ).toBe('#NUM!');
  expect(err(`=ODDLYIELD(${L},0.0375,0,100,2,0)`)).toBe('#NUM!');
  expect(err(`=ODDLPRICE(${L},0.0375,0.04,100,2,9)`)).toBe('#NUM!');
  expect(err('=ODDFPRICE(1,2,3)')).toBe('#VALUE!');
  expect(err('=ODDLPRICE("x",2,1,0.05,0.05,100,2)')).toBe('#VALUE!');
});
