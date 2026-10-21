/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tabla dinámica contra los vectores COMPARTIDOS con la app nativa
 * (apps/nucleo/formula/analisis/testdata/dinamica.json, que Go recorre en
 * dinamica_test.go). Con GRABAR=1 escribe lo que da esta implementación como
 * `esperado`; lo grabado se revisa a mano antes de confirmarlo, y los casos de abajo
 * comprueban a mano las cifras que importan.
 */
import { expect, test } from 'bun:test';
import { readFileSync, writeFileSync } from 'node:fs';
import { FormulaError, formatNumber, type CellValue } from '@sytue/formula';
import { computePivotTable, sanitizePivot } from '../src/editor/sheets/pivot';
import type { PivotCell, PivotDef, PivotLabels } from '../src/editor/sheets/pivot';

const ruta = new URL('../../../../nucleo/formula/analisis/testdata/dinamica.json', import.meta.url);
type Crudo = string | number | boolean | { e: string };
interface Caso {
  nombre: string;
  origen: Crudo[][];
  def: PivotDef;
  esperado?: (Record<string, unknown> | null)[][];
}
const vectores = JSON.parse(readFileSync(ruta, 'utf8')) as {
  _: string;
  casos: Caso[];
  saneado?: unknown;
};

const valor = (x: Crudo): CellValue => (typeof x === 'object' ? new FormulaError(x.e as never) : x);

const texto = (v: CellValue): string =>
  v instanceof FormulaError
    ? v.code
    : typeof v === 'boolean'
      ? v
        ? 'TRUE'
        : 'FALSE'
      : typeof v === 'number'
        ? formatNumber(v)
        : v;

const ROTULOS: PivotLabels = {
  blank: '(en blanco)',
  grand: 'Total general',
  totalOf: (k) => `Total ${k}`,
  valueName: (fn, f) => `${fn} de ${f}`,
  all: '(Todos)',
  multiple: '(Varios)',
  quarter: (q) => `T${q}`,
  month: (m) => `M${m}`,
  day: (d) => `D${formatNumber(d)}`,
  show: (v, f) => (f >= 0 ? `${texto(v)}@${f}` : texto(v)),
};

/** La casilla como la escribe el vector. */
function aVector(c: PivotCell | null): Record<string, unknown> | null {
  if (!c) return null;
  const o: Record<string, unknown> = { v: c.v };
  if (c.bold) o['b'] = true;
  if (c.fmt)
    o['f'] = 'field' in c.fmt ? `c${c.fmt.field}` : 'value' in c.fmt ? `v${c.fmt.value}` : 'p';
  return o;
}

const calcular = (c: Caso): (Record<string, unknown> | null)[][] =>
  computePivotTable(
    c.origen.map((f) => f.map(valor)),
    c.def,
    ROTULOS,
  ).map((f) => f.map(aVector));

if (process.env['GRABAR'] === '1') {
  for (const c of vectores.casos) c.esperado = calcular(c);
  writeFileSync(ruta, `${JSON.stringify(vectores).replace(/\{"nombre"/g, '\n{"nombre"')}\n`);
}

for (const c of vectores.casos) {
  test(`dinámica: ${c.nombre}`, () => {
    const got = calcular(c);
    expect(got.length).toBe(c.esperado!.length);
    got.forEach((fila, i) =>
      fila.forEach((celda, j) => {
        const quiero = c.esperado![i]![j]!;
        if (celda && quiero && typeof celda['v'] === 'number' && typeof quiero['v'] === 'number') {
          expect(Math.abs((celda['v'] as number) - (quiero['v'] as number))).toBeLessThanOrEqual(
            1e-12 * Math.max(1, Math.abs(quiero['v'] as number)),
          );
          expect({ ...celda, v: 0 }).toEqual({ ...quiero, v: 0 });
        } else expect(celda).toEqual(quiero);
      }),
    );
  });
}

/* Las cifras que importan, contadas a mano sobre el origen de los vectores. */
const ventas = vectores.casos[0]!.origen.map((f) => f.map(valor));
const tabla = (def: PivotDef): (PivotCell | null)[][] => computePivotTable(ventas, def, ROTULOS);

test('suma por zona: Norte 700, Sur 980, Este 420, en blanco 50, y el texto no suma', () => {
  const t = tabla({ rows: [{ field: 0 }], cols: [], values: [{ field: 5, fn: 'sum' }] });
  const fila = (k: string): (PivotCell | null)[] | undefined => t.find((f) => f[0]?.v === k);
  expect(fila('Norte')?.[1]?.v).toBe(700);
  expect(fila('Sur')?.[1]?.v).toBe(980);
  expect(fila('Este')?.[1]?.v).toBe(420);
  expect(fila('(en blanco)')?.[1]?.v).toBe(50);
  expect(fila('Oeste')?.[1]?.v).toBe(0); // «n/d» es texto: el registro está, la suma es 0
  expect(fila('Total general')?.[1]?.v).toBe(2150);
  // Los números van antes que el texto y los vacíos al final.
  expect(t.map((f) => f[0]?.v)).toEqual([
    'Zona',
    'Este',
    'Norte',
    'Oeste',
    'Sur',
    '(en blanco)',
    'Total general',
  ]);
});

test('el promedio de un subtotal es el de sus registros, no el de los promedios', () => {
  const t = tabla({
    rows: [{ field: 0 }, { field: 1 }],
    cols: [],
    values: [{ field: 4, fn: 'avg' }],
  });
  // Norte: Bilbao (3, 1) y Oviedo (5): promedio de registros 3, no (2 + 5) / 2.
  expect(t.find((f) => f[0]?.v === 'Total Norte')?.[2]?.v).toBe(3);
});

test('los porcentajes del total general suman uno', () => {
  const t = tabla({
    rows: [{ field: 0 }],
    cols: [],
    values: [{ field: 4, fn: 'sum', show: 'pctGrand' }],
  });
  const cuerpo = t.slice(1, -1).map((f) => f[1]?.v as number);
  expect(cuerpo.reduce((a, b) => a + b, 0)).toBeCloseTo(1, 12);
  expect(t.at(-1)?.[1]).toEqual({ v: 1, bold: true, fmt: { pct: true } });
});

/* El saneado de lo que se guarda: el mismo documento se lee igual en la app nativa. */
const saneado = (
  JSON.parse(readFileSync(ruta, 'utf8')) as {
    saneado: { nombre: string; crudo: unknown; esperado: unknown }[];
  }
).saneado;
for (const c of saneado) {
  test(`saneado: ${c.nombre}`, () => {
    expect(JSON.parse(JSON.stringify(sanitizePivot(c.crudo)))).toEqual(c.esperado as never);
  });
}
