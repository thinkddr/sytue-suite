/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Buscar objetivo, el Solver y el plan de la Tabla de datos contra los vectores
 * COMPARTIDOS con la app nativa (apps/nucleo/formula/analisis/testdata/hipotesis.json,
 * que Go recorre en hipotesis_test.go): mismo libro, mismo resultado y mismo número
 * de evaluaciones. Con GRABAR=1 escribe lo que da la web como `esperado`.
 */
import { expect, test } from 'bun:test';
import { readFileSync, writeFileSync } from 'node:fs';
import { Engine } from '@sytue/formula';
import { goalSeek } from '../src/editor/sheets/goalseek';
import { solve } from '../src/editor/sheets/solver';
import type { SolverConstraint, SolverGoal } from '../src/editor/sheets/solver';
import { planDataTable } from '../src/editor/sheets/datatable';

const ruta = new URL(
  '../../../../nucleo/formula/analisis/testdata/hipotesis.json',
  import.meta.url,
);
interface Objetivo {
  nombre: string;
  celdas: Record<string, string>;
  objetivo: string;
  variable: string;
  valor: number;
  esperado?: Record<string, unknown>;
}
interface Solver {
  nombre: string;
  celdas: Record<string, string>;
  objetivo: string;
  variables: string[];
  meta: { tipo: 'max' | 'min' | 'value'; valor?: number };
  restricciones: { lhs: string; rel: 'lte' | 'gte' | 'eq'; rhs: string }[];
  esperado?: Record<string, unknown>;
}
interface Tabla {
  nombre: string;
  rect: [number, number, number, number];
  fila: boolean;
  col: boolean;
  esperado?: unknown;
}
const v = JSON.parse(readFileSync(ruta, 'utf8')) as {
  _: string;
  objetivo: Objetivo[];
  solver: Solver[];
  tabla: Tabla[];
};

const motor = (celdas: Record<string, string>): Engine => {
  const e = new Engine();
  for (const [ref, raw] of Object.entries(celdas)) e.setCell('Hoja1', ref, raw);
  return e;
};
const num = (x: unknown): number | null => (typeof x === 'number' && Number.isFinite(x) ? x : null);

function objetivo(c: Objetivo): Record<string, unknown> {
  const e = motor(c.celdas);
  const r = goalSeek(
    (x) => {
      e.setCell('Hoja1', c.variable, String(x));
      return num(e.getValue('Hoja1', c.objetivo));
    },
    c.valor,
    { x0: num(e.getValue('Hoja1', c.variable)) ?? 0 },
  );
  return { resuelto: r.solved, valor: r.value, iteraciones: r.iterations };
}

function solver(c: Solver): Record<string, unknown> {
  const e = motor(c.celdas);
  const lado = (s: string): number | null =>
    /^[A-Z]+[0-9]+$/.test(s) ? num(e.getValue('Hoja1', s)) : Number(s);
  const goal: SolverGoal =
    c.meta.tipo === 'value' ? { kind: 'value', target: c.meta.valor ?? 0 } : { kind: c.meta.tipo };
  const cons: SolverConstraint[] = c.restricciones.map((r) => ({ relation: r.rel }));
  const r = solve(
    (x) => {
      c.variables.forEach((ref, i) => e.setCell('Hoja1', ref, String(x[i])));
      return {
        objective: num(e.getValue('Hoja1', c.objetivo)),
        constraints: c.restricciones.map((r) => ({ lhs: lado(r.lhs), rhs: lado(r.rhs) })),
      };
    },
    c.variables.map((ref) => num(e.getValue('Hoja1', ref)) ?? 0),
    goal,
    cons,
  );
  return {
    factible: r.feasible,
    valores: r.values,
    objetivo: r.objective,
    iteraciones: r.iterations,
  };
}

function tabla(c: Tabla): unknown {
  const [c1, r1, c2, r2] = c.rect;
  return planDataTable({ c1, r1, c2, r2 }, c.fila, c.col);
}

if (process.env['GRABAR'] === '1') {
  for (const c of v.objetivo) c.esperado = objetivo(c);
  for (const c of v.solver) c.esperado = solver(c);
  for (const c of v.tabla) c.esperado = tabla(c);
  writeFileSync(ruta, `${JSON.stringify(v).replace(/\{"nombre"/g, '\n{"nombre"')}\n`);
}

const cerca = (a: unknown, b: unknown): void => {
  if (typeof a === 'number' && typeof b === 'number')
    expect(Math.abs(a - b)).toBeLessThanOrEqual(1e-9 * Math.max(1, Math.abs(b)));
  else if (Array.isArray(a) && Array.isArray(b)) a.forEach((x, i) => cerca(x, b[i]));
  else expect(a).toEqual(b as never);
};

for (const c of v.objetivo) {
  test(`objetivo: ${c.nombre}`, () => {
    const got = objetivo(c);
    for (const k of Object.keys(c.esperado!)) cerca(got[k], c.esperado![k]);
  });
}
for (const c of v.solver) {
  test(`solver: ${c.nombre}`, () => {
    const got = solver(c);
    for (const k of Object.keys(c.esperado!)) cerca(got[k], c.esperado![k]);
  });
}
for (const c of v.tabla) {
  test(`tabla: ${c.nombre}`, () => {
    expect(tabla(c)).toEqual(c.esperado as never);
  });
}

test('las cifras que importan, a mano', () => {
  const cuadrado = objetivo(v.objetivo[0]!);
  expect(cuadrado['resuelto']).toBe(true);
  expect(Math.abs((cuadrado['valor'] as number) - 3)).toBeLessThan(1e-6);
  const lineal = solver(v.solver[0]!);
  expect(lineal['factible']).toBe(true);
  expect(Math.abs((lineal['objetivo'] as number) - 18)).toBeLessThan(1e-3); // 3·4 + 2·3
});
