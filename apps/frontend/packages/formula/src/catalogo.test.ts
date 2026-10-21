/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El catálogo versionado de funciones (catalogo.ts): recorre TODAS las funciones y vigila
 * que no se añadan ni se quiten sin declararlo.
 */
import { expect, test } from 'bun:test';
import { CATALOGO_VERSION, catalogo, FUNCIONES_DEL_MOTOR } from './catalogo.js';
import { Engine } from './engine.js';
import { buildRegistry } from './functions/index.js';
import { FormulaError } from './values.js';

/**
 * Cuántas funciones hay por versión del catálogo. NO se retoca para que el test pase: la
 * versión 1 es la de antes de GROUPBY y compañía y no cambia nunca; una función nueva va
 * en una versión nueva, declarada en `DESDE` de catalogo.ts, y aquí se anota su cuenta.
 */
const FUNCIONES_EN_VERSION: Record<number, number> = { 1: 461, 2: 5, 3: 29, 4: 14 };

test('cada versión del catálogo tiene las funciones que se anotaron', () => {
  const c = catalogo();
  for (let v = 1; v <= CATALOGO_VERSION; v++) {
    expect(c.filter((f) => f.desde === v).length).toBe(FUNCIONES_EN_VERSION[v]!);
  }
  expect(c.every((f) => f.desde >= 1 && f.desde <= CATALOGO_VERSION)).toBe(true);
  expect(Object.keys(FUNCIONES_EN_VERSION).length).toBe(CATALOGO_VERSION);
});

test('el catálogo trae todo el registro y todo lo del motor, sin repetir', () => {
  const c = catalogo();
  const nombres = c.map((f) => f.nombre);
  expect(new Set(nombres).size).toBe(nombres.length);
  for (const n of buildRegistry().names()) expect(nombres).toContain(n);
  for (const n of FUNCIONES_DEL_MOTOR) expect(nombres).toContain(n);
  // Las del motor no están en el registro: si lo estuvieran, el motor las despacharía antes.
  const registro = new Set(buildRegistry().names());
  for (const f of c) expect(f.motor).toBe(!registro.has(f.nombre));
});

test('las funciones de las matrices dinámicas se declaran solo de Excel 365', () => {
  const por = Object.fromEntries(catalogo().map((f) => [f.nombre, f]));
  for (const n of ['LAMBDA', 'GROUPBY', 'PIVOTBY', 'XLOOKUP', 'TRIMRANGE']) {
    expect(por[n]!.compat).toBe('excel365');
  }
  for (const n of ['SUM', 'VLOOKUP', 'IFS', 'TEXTJOIN']) expect(por[n]!.compat).toBe('clasica');
});

test('cada ejemplo del catálogo da lo que dice', () => {
  const con = catalogo().filter((f) => f.ejemplo);
  expect(con.length).toBeGreaterThanOrEqual(FUNCIONES_EN_VERSION[2]! + FUNCIONES_EN_VERSION[3]!);
  for (const f of con) {
    const ej = f.ejemplo!;
    const e = new Engine();
    for (const [n, d] of Object.entries(ej.nombres ?? {})) expect(e.defineName(n, d)).toBe(true);
    for (const [ref, v] of Object.entries(ej.datos)) e.setCell('Hoja1', ref, String(v));
    e.setCell('Hoja1', 'P1', ej.formula);
    const v = e.getValue('Hoja1', 'P1');
    expect(v instanceof FormulaError ? v.code : v).toEqual(ej.resultado);
  }
});
