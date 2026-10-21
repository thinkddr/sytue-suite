/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El catálogo de FIRMAS del editor (sheets/formula-catalog.ts) frente al catálogo
 * versionado del motor (@sytue/formula): ninguna firma de una función que no existe, y
 * las funciones nuevas con su firma.
 * Ejecutar: cd apps/frontend/packages/office && bun test test/sheets-catalogo.test.ts
 */
import { expect, test } from 'bun:test';
import { catalogo } from '@sytue/formula';
import {
  FORMULA_CATALOG,
  lookupSignature,
  splitSignature,
} from '../src/editor/sheets/formula-catalog';

test('toda firma del editor es de una función que existe y lleva su propio nombre', () => {
  const existentes = new Set(catalogo().map((f) => f.nombre));
  for (const [nombre, firma] of Object.entries(FORMULA_CATALOG)) {
    expect(existentes.has(nombre)).toBe(true);
    expect(splitSignature(firma.args).name).toBe(nombre);
  }
});

test('las funciones de la versión 2 del catálogo y las de LAMBDA tienen firma', () => {
  const nuevas = catalogo().filter((f) => f.desde >= 2);
  expect(nuevas.length).toBeGreaterThan(0);
  for (const f of nuevas) expect(lookupSignature(f.nombre)).toBeDefined();
  for (const n of ['LET', 'LAMBDA', 'MAP', 'REDUCE', 'SCAN', 'MAKEARRAY', 'BYROW', 'BYCOL']) {
    expect(lookupSignature(n)).toBeDefined();
  }
});

test('las firmas de GROUPBY y PIVOTBY parten bien sus muchos parámetros', () => {
  expect(splitSignature(lookupSignature('GROUPBY')!.args).params).toHaveLength(7);
  expect(splitSignature(lookupSignature('PIVOTBY')!.args).params).toHaveLength(10);
});
