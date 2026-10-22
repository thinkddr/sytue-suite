/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El esquema y el asistente de Subtotales contra los vectores COMPARTIDOS con la app
 * nativa (apps/nucleo/formula/analisis/testdata/esquema.json, que Go recorre en
 * esquema_test.go): el mismo caso, el mismo resultado en las dos.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  addGroup,
  applySubtotals,
  groupsAtLevel,
  hiddenUnits,
  maxOutlineLevel,
  removeGroup,
  sanitizeGroups,
  toggleGroup,
} from '../src/editor/sheets/outline';
import type { SubtotalRequest } from '../src/editor/sheets/outline';
import type { SheetGroup } from '../src/editor/types';

interface CasoGrupo {
  nombre: string;
  op: 'agrupar' | 'desagrupar' | 'nivel' | 'conmutar' | 'ocultas' | 'niveles' | 'sanear';
  grupos?: SheetGroup[];
  crudo?: unknown;
  a?: number;
  b?: number;
  max?: number;
  nivel?: number;
  esperado: unknown;
}
interface CasoSubtotal {
  nombre: string;
  celdas: Record<string, string>;
  peticion: Omit<SubtotalRequest, 'maxUsedRow'>;
  esperado: Record<string, unknown>;
}

const v = JSON.parse(
  readFileSync(
    new URL('../../../../nucleo/formula/analisis/testdata/esquema.json', import.meta.url),
    'utf8',
  ),
) as { grupos: CasoGrupo[]; subtotales: CasoSubtotal[] };

const ref = (c: number, r: number): string => `${String.fromCharCode(65 + c)}${r + 1}`;

for (const c of v.grupos) {
  test(`esquema: ${c.nombre}`, () => {
    const g = c.grupos ?? [];
    const got = {
      agrupar: () => addGroup(g, c.a!, c.b!, c.max!),
      desagrupar: () => removeGroup(g, c.a!, c.b!),
      nivel: () => groupsAtLevel(g, c.nivel!),
      conmutar: () => toggleGroup(g, c.a!, c.b!, c.nivel!),
      ocultas: () => [...hiddenUnits(g)].sort((x, y) => x - y),
      niveles: () => maxOutlineLevel(g),
      sanear: () => sanitizeGroups(c.crudo, c.max!),
    }[c.op]();
    expect(got).toEqual(c.esperado as never);
  });
}

for (const c of v.subtotales) {
  test(`subtotales: ${c.nombre}`, () => {
    const raw = (col: number, r: number): string => c.celdas[ref(col, r)] ?? '';
    let maxUsedRow = -1;
    for (const k of Object.keys(c.celdas))
      maxUsedRow = Math.max(maxUsedRow, Number(k.slice(1)) - 1);
    const res = applySubtotals(
      { ...c.peticion, maxUsedRow },
      {
        raw,
        key: raw,
        isNumber: (col, r) => raw(col, r).trim() !== '' && Number.isFinite(Number(raw(col, r))),
        hasValue: (col, r) => raw(col, r) !== '',
      },
      { grand: 'Total general', total: 'Total', totalOf: (k) => `Total ${k}` },
    );
    if ('error' in res) {
      expect({ error: res.error }).toEqual(c.esperado as never);
      return;
    }
    expect({
      desde: res.from.map((x) => x ?? -1),
      escrituras: res.writes,
      grupos: res.groups,
      ultimaFila: res.lastRow,
      bloques: res.blocks,
    }).toEqual(c.esperado as never);
  });
}
