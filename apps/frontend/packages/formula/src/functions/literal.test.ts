/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El texto ESCRITO como argumento de un agregado (SUM, AVERAGE, COUNT…) se convierte a
 * número, como en Excel; el de una celda referenciada o el que devuelve una función se ignora.
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

function calc(formula: string): number | string {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '3');
  e.setCell('Hoja1', 'A2', '7');
  e.setCell('Hoja1', 'A3', '="2"'); // un texto «2» en una celda
  e.setCell('Hoja1', 'Z1', formula);
  const v = e.getValue('Hoja1', 'Z1');
  return typeof v === 'object' ? (v as { code: string }).code : (v as number | string);
}

test('el texto numérico escrito como argumento se convierte (Excel: SUM(1;"2") = 3)', () => {
  expect(calc('=SUM(1,"2")')).toBe(3);
  expect(calc('=SUM(A1:A2,"2")')).toBe(12);
  expect(calc('=SUM("1"&"0",1)')).toBe(11);
  expect(calc('=SUM("50%",1)')).toBe(1.5);
  expect(calc('=AVERAGE("5",3)')).toBe(4);
  expect(calc('=MAX("7",3)')).toBe(7);
  expect(calc('=MIN("-7",3)')).toBe(-7);
  expect(calc('=PRODUCT("2",3)')).toBe(6);
  expect(calc('=COUNT("1",1,TRUE)')).toBe(3);
});

test('el texto no numérico escrito como argumento es #VALUE!, y COUNT no lo cuenta', () => {
  expect(calc('=SUM("a",1)')).toBe('#VALUE!');
  expect(calc('=SUM("",1)')).toBe('#VALUE!');
  expect(calc('=AVERAGE("a",1)')).toBe('#VALUE!');
  expect(calc('=COUNT("a",1)')).toBe(1);
  expect(calc('=COUNT("",1)')).toBe(1);
});

test('el texto de una celda, de un rango o de una función se sigue ignorando', () => {
  expect(calc('=SUM(A3,1)')).toBe(1);
  expect(calc('=SUM(A1:A3)')).toBe(10);
  expect(calc('=COUNT(A1:A3)')).toBe(2);
  expect(calc('=COUNT(A3)')).toBe(0);
  expect(calc('=AVERAGE(A1:A3)')).toBe(5);
  // INDEX y OFFSET devuelven referencias en Excel, cuyo texto se ignora.
  expect(calc('=SUM(INDEX(A1:A3,3),1)')).toBe(1);
  expect(calc('=SUM(OFFSET(A1,2,0),1)')).toBe(1);
  // ponytail: Excel sí convierte el texto de una función que NO devuelve una referencia
  // (SUM(LEFT("12";1)) = 1); aquí se ignora, porque el motor no distingue unas de otras.
  expect(calc('=SUM(LEFT("12",1),1)')).toBe(1);
});
