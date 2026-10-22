/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Aritmética y comparación como Excel: igualdad a 15 cifras y desbordamiento = #NUM!.
 */
import { expect, test } from 'bun:test';
import { Engine } from './engine.js';
import { FormulaError } from './values.js';

const val = (f: string): unknown => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', f);
  const v = e.getValue('Hoja1', 'A1');
  return v instanceof FormulaError ? v.code : v;
};

test('dos números iguales a 15 cifras son iguales', () => {
  expect(val('=0.1+0.2=0.3')).toBe(true);
  expect(val('=0.1+0.2<>0.3')).toBe(false);
  expect(val('=0.1+0.2<0.3')).toBe(false);
  expect(val('=0.1+0.2>=0.3')).toBe(true);
  expect(val('=(1/3)*3=1')).toBe(true);
});

test('lo que difiere de verdad sigue difiriendo', () => {
  expect(val('=1=1.000000000001')).toBe(false);
  expect(val('=0.3<0.30000001')).toBe(true);
  expect(val('=1E-20=2E-20')).toBe(false);
  expect(val('=2>1')).toBe(true);
});

test('un desbordamiento es #NUM!, no infinito', () => {
  expect(val('=1E+308*10')).toBe('#NUM!');
  expect(val('=1E+308+1E+308')).toBe('#NUM!');
  expect(val('=-1E+308-1E+308')).toBe('#NUM!');
  expect(val('=1E+308*10-1')).toBe('#NUM!');
  expect(val('=1/0')).toBe('#DIV/0!');
  expect(val('=2^1024')).toBe('#NUM!');
});
