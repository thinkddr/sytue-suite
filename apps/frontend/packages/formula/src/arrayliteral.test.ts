/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Constantes matriciales: {1,2;3,4}. Columnas con «,», filas con «;», solo literales.
 */
import { expect, test } from 'bun:test';
import { Engine } from './engine.js';
import { ParseError, parseArrayLiteral, parseFormula } from './parser.js';
import { FormulaError } from './values.js';

const val = (f: string): unknown => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', f);
  const v = e.getValue('Hoja1', 'A1');
  return v instanceof FormulaError ? v.code : v;
};

test('el interior se lee a filas y columnas', () => {
  expect(parseArrayLiteral('1,2;3,4')).toEqual([
    [1, 2],
    [3, 4],
  ]);
  expect(parseArrayLiteral('"a;b","c,d";TRUE,-1.5e2')).toEqual([
    ['a;b', 'c,d'],
    [true, -150],
  ]);
  expect(parseArrayLiteral('1,,3')).toEqual([[1, '', 3]]);
  expect(parseArrayLiteral('"di ""hola"""')).toEqual([['di "hola"']]);
  expect(parseArrayLiteral('#N/A,1')[0]![0]).toBeInstanceOf(FormulaError);
});

test('lo que no es un literal, o no es rectangular, no se parsea', () => {
  for (const malo of ['1,2;3', 'A1', '1+1', '"abierto', '1;;2,3'])
    expect(() => parseArrayLiteral(malo)).toThrow(ParseError);
  expect(() => parseFormula('{1,2')).toThrow(ParseError);
});

test('se opera con ellas como con un rango', () => {
  expect(val('=SUM({1,2;3,4})')).toBe(10);
  expect(val('=SUM({1,2,3}*2)')).toBe(12);
  expect(val('=INDEX({10,20,30},2)')).toBe(20);
  expect(val('=ROWS({1;2;3})')).toBe(3);
  expect(val('=COLUMNS({1,2,3})')).toBe(3);
  expect(val('=SUMPRODUCT({1,2,3},{4,5,6})')).toBe(32);
  expect(val('=MATCH(2,{1,2,3},0)')).toBe(2);
  expect(val('=PROB({1;2;3;4},{0.1;0.2;0.3;0.4},2,3)')).toBeCloseTo(0.5, 12);
  expect(val('=FVSCHEDULE(1000,{0.09,0.11,0.1})')).toBeCloseTo(1330.89, 8);
  expect(val('=INDEX(FREQUENCY({1,2,3,4,5},{2,4}),2)')).toBe(2);
});

test('suelta, derrama como cualquier matriz', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '={1,2;3,4}');
  expect(e.getValue('Hoja1', 'B2')).toBe(4);
  e.setCell('Hoja1', 'D1', '={"x","y"}');
  expect(e.getValue('Hoja1', 'E1')).toBe('y');
  e.setCell('Hoja1', 'G1', '={1,#N/A}');
  expect((e.getValue('Hoja1', 'H1') as FormulaError).code).toBe('#N/A');
});

test('un texto con llaves dentro no se confunde con una constante', () => {
  expect(val('=LEN("{1,2}")')).toBe(5);
  expect(val('=SUM({1,2},{3})')).toBe(6);
});
