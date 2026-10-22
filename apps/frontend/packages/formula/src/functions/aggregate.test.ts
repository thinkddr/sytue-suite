/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SUBTOTAL y AGGREGATE contra los vectores COMPARTIDOS con el motor de Go
 * (apps/nucleo/formula/testdata/subtotal.json): el mismo caso tiene que dar lo mismo
 * en la web y en la app nativa. Aquí solo se recorren; los casos y su porqué viven en
 * el JSON, que es el contrato de los dos.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { Engine } from '../engine.js';
import { FormulaError } from '../values.js';

interface Caso {
  nombre: string;
  celdas: Record<string, string>;
  filtradas?: number[];
  manuales?: number[];
  formula: string;
  esperado: { n?: number; e?: string };
}

const { casos } = JSON.parse(
  readFileSync(
    new URL('../../../../../nucleo/formula/testdata/subtotal.json', import.meta.url),
    'utf8',
  ),
) as { casos: Caso[] };

test('hay vectores que recorrer', () => {
  expect(casos.length).toBeGreaterThan(40);
});

for (const c of casos) {
  test(c.nombre, () => {
    const e = new Engine();
    for (const [ref, raw] of Object.entries(c.celdas)) e.setCell('Hoja1', ref, raw);
    e.setHiddenRows('Hoja1', c.filtradas ?? [], c.manuales ?? []);
    e.setCell('Hoja1', 'Z1', c.formula);
    const v = e.getValue('Hoja1', 'Z1');
    if (c.esperado.e !== undefined) {
      expect(v instanceof FormulaError ? v.code : v).toBe(c.esperado.e);
      return;
    }
    expect(typeof v).toBe('number');
    const n = c.esperado.n!;
    expect(Math.abs((v as number) - n)).toBeLessThanOrEqual(1e-12 * Math.max(1, Math.abs(n)));
  });
}

test('contraer un grupo recalcula lo que ya estaba calculado', () => {
  // El motor memoriza: sin vaciar la caché al cambiar las filas ocultas, el total se
  // quedaría con el valor de antes de contraer el grupo.
  const e = new Engine();
  for (const [ref, raw] of Object.entries({
    A1: '1',
    A2: '2',
    A3: '3',
    B1: '=SUBTOTAL(109,A1:A3)',
  }))
    e.setCell('Hoja1', ref, raw);
  expect(e.getValue('Hoja1', 'B1')).toBe(6);
  e.setHiddenRows('Hoja1', [], [1]);
  expect(e.getValue('Hoja1', 'B1')).toBe(4);
  e.setHiddenRows('Hoja1', [], []);
  expect(e.getValue('Hoja1', 'B1')).toBe(6);
});
