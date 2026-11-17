/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las filas y columnas ocultas a mano contra los vectores COMPARTIDOS con la app nativa
 * (apps/nucleo/formula/analisis/testdata/ocultas.json, que Go recorre en ocultas_test.go):
 * el mismo caso, el mismo resultado en las dos.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  hideSpan,
  isSpanHidden,
  moveSpans,
  sanitizeSpans,
  showSpan,
  spanUnits,
} from '../src/editor/sheets/ocultas';
import type { SheetSpan } from '../src/editor/types';

interface Caso {
  nombre: string;
  op: 'sanear' | 'ocultar' | 'mostrar' | 'mover';
  max?: number;
  crudo?: unknown;
  tramos?: SheetSpan[];
  a?: number;
  b?: number;
  desde?: number;
  cantidad?: number;
  esperado: SheetSpan[] | null;
}

const v = JSON.parse(
  readFileSync(
    new URL('../../../../nucleo/formula/analisis/testdata/ocultas.json', import.meta.url),
    'utf8',
  ),
) as { casos: Caso[] };

test('hay vectores', () => {
  expect(v.casos.length).toBeGreaterThan(0);
});

for (const c of v.casos) {
  test(`ocultas: ${c.nombre}`, () => {
    const t = c.tramos ?? [];
    const got = {
      sanear: () => sanitizeSpans(c.crudo, c.max!),
      ocultar: () => hideSpan(t, c.a!, c.b!, c.max!),
      mostrar: () => showSpan(t, c.a!, c.b!),
      mover: () => moveSpans(t, c.desde!, c.cantidad!, c.max!),
    }[c.op]();
    expect(got).toEqual(c.esperado as never);
  });
}

test('las unidades de un tramo hasta el final se cortan en el límite', () => {
  const u = spanUnits(
    [
      { start: 2, end: 3 },
      { start: 8, end: 1 << 20 },
    ],
    10,
  );
  expect([...u].sort((a, b) => a - b)).toEqual([2, 3, 8, 9]);
  expect(isSpanHidden([{ start: 2, end: 3 }], 3)).toBe(true);
  expect(isSpanHidden([{ start: 2, end: 3 }], 4)).toBe(false);
  expect(isSpanHidden(undefined, 0)).toBe(false);
});

test('ocultar y mostrar no mutan la lista de entrada', () => {
  const t: SheetSpan[] = [{ start: 2, end: 4 }];
  hideSpan(t, 5, 6, 100);
  showSpan(t, 3, 3);
  moveSpans(t, 0, 1, 100);
  expect(t).toEqual([{ start: 2, end: 4 }]);
});
