/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del Solver (Análisis Y-si, optimización multivariable con
 * restricciones): lineal con restricción activa (LP), cuadrático simple,
 * cuadrático con restricción activa e igualdad, sin solución (infactible), llevar
 * a un valor, y las salvaguardas (máximo de evaluaciones). Incluye un caso
 * respaldado por el motor de fórmulas real (@sytue/formula), que es como lo usa
 * el lienzo: fijar las celdas variable y leer objetivo/restricciones tras cada tanteo.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/sheets-solver.test.ts
 */
import { expect, test } from 'bun:test';
import { Engine } from '@sytue/formula';
import { solve } from '../src/editor/sheets/solver';
import type { SolverConstraint, SolverEvalResult } from '../src/editor/sheets/solver';
import { classifyConstraintRhs, parseEsNumber, singleCellRef } from '../src/editor/sheets/format';

/** Envuelve una función objetivo pura + restricciones (lhs, rhs) en un evaluador. */
const evaluator =
  (
    objective: (x: readonly number[]) => number,
    cons: readonly ((x: readonly number[]) => { lhs: number; rhs: number })[],
  ) =>
  (x: readonly number[]): SolverEvalResult => ({
    objective: objective(x),
    constraints: cons.map((c) => c(x)),
  });

test('lineal con restricción activa (LP): max 3x+2y s.t. x+y≤4, x≤3, x,y≥0 → (3,1)', () => {
  const cons: SolverConstraint[] = [
    { relation: 'lte' }, // x+y ≤ 4
    { relation: 'lte' }, // x ≤ 3
    { relation: 'gte' }, // x ≥ 0
    { relation: 'gte' }, // y ≥ 0
  ];
  const ev = evaluator(
    ([x, y]) => 3 * x! + 2 * y!,
    [
      ([x, y]) => ({ lhs: x! + y!, rhs: 4 }),
      ([x]) => ({ lhs: x!, rhs: 3 }),
      ([x]) => ({ lhs: x!, rhs: 0 }),
      ([, y]) => ({ lhs: y!, rhs: 0 }),
    ],
  );
  const r = solve(ev, [0, 0], { kind: 'max' }, cons);
  expect(r.feasible).toBe(true);
  expect(r.values[0]).toBeCloseTo(3, 2);
  expect(r.values[1]).toBeCloseTo(1, 2);
  expect(r.objective).toBeCloseTo(11, 2);
});

test('cuadrático simple: min (x−1)²+(y−2)² → (1,2), objetivo 0', () => {
  const ev = evaluator(([x, y]) => (x! - 1) ** 2 + (y! - 2) ** 2, []);
  const r = solve(ev, [0, 0], { kind: 'min' }, []);
  expect(r.feasible).toBe(true);
  expect(r.values[0]).toBeCloseTo(1, 3);
  expect(r.values[1]).toBeCloseTo(2, 3);
  expect(r.objective).toBeCloseTo(0, 4);
});

test('cuadrático con restricción activa: min (x−3)²+(y−3)² s.t. x+y≤4 → (2,2)', () => {
  const ev = evaluator(
    ([x, y]) => (x! - 3) ** 2 + (y! - 3) ** 2,
    [([x, y]) => ({ lhs: x! + y!, rhs: 4 })],
  );
  const r = solve(ev, [0, 0], { kind: 'min' }, [{ relation: 'lte' }]);
  expect(r.feasible).toBe(true);
  expect(r.values[0]).toBeCloseTo(2, 2);
  expect(r.values[1]).toBeCloseTo(2, 2);
  expect(r.objective).toBeCloseTo(2, 2);
  expect(r.maxViolation).toBeLessThanOrEqual(1e-6);
});

test('igualdad activa: min x²+y² s.t. x+y=4 → (2,2), objetivo 8', () => {
  const ev = evaluator(([x, y]) => x! ** 2 + y! ** 2, [([x, y]) => ({ lhs: x! + y!, rhs: 4 })]);
  const r = solve(ev, [0, 0], { kind: 'min' }, [{ relation: 'eq' }]);
  expect(r.feasible).toBe(true);
  expect(r.values[0]).toBeCloseTo(2, 2);
  expect(r.values[1]).toBeCloseTo(2, 2);
  expect(r.objective).toBeCloseTo(8, 2);
});

test('sin solución (infactible): x≥5 y x≤1 → no factible, best-effort acotado', () => {
  const ev = evaluator(
    ([x]) => x!,
    [([x]) => ({ lhs: x!, rhs: 5 }), ([x]) => ({ lhs: x!, rhs: 1 })],
  );
  const r = solve(ev, [0], { kind: 'min' }, [{ relation: 'gte' }, { relation: 'lte' }]);
  expect(r.feasible).toBe(false);
  expect(r.maxViolation).toBeGreaterThan(1e-6);
  expect(Number.isFinite(r.values[0]!)).toBe(true);
});

test('llevar a un valor: objetivo x+y = 10 → objetivo ≈ 10, factible', () => {
  const ev = evaluator(([x, y]) => x! + y!, []);
  const r = solve(ev, [0, 0], { kind: 'value', target: 10 }, []);
  expect(r.feasible).toBe(true);
  expect(r.objective).toBeCloseTo(10, 3);
});

test('respeta maxEvaluations', () => {
  let calls = 0;
  const ev = (x: readonly number[]): SolverEvalResult => {
    calls++;
    return { objective: (x[0]! - 1) ** 2 + (x[1]! - 1) ** 2, constraints: [] };
  };
  const r = solve(ev, [5, 5], { kind: 'min' }, [], { maxEvaluations: 10 });
  expect(calls).toBeLessThanOrEqual(10);
  expect(r.iterations).toBeLessThanOrEqual(10);
});

test('clasificación del RHS: la CELDA gana al número (parseEsNumber("D3")===3 es la trampa)', () => {
  // La trampa que hacía inalcanzable la rama RHS-celda: parseEsNumber elimina las
  // letras, así que una ref se malinterpretaba como su nº de fila.
  expect(parseEsNumber('D3')).toBe(3);
  expect(parseEsNumber('B5')).toBe(5);
  expect(parseEsNumber('AA10')).toBe(10);
  // Con la celda probada primero, una ref real se clasifica como CELDA (no número).
  expect(classifyConstraintRhs('D3')).toEqual({ cell: 'D3', num: null });
  expect(classifyConstraintRhs('$d$3')).toEqual({ cell: 'D3', num: null }); // normaliza
  expect(classifyConstraintRhs('AA10')).toEqual({ cell: 'AA10', num: null });
  // Un literal numérico (es-ES) sigue siendo número: no casa parseRangeRef.
  expect(classifyConstraintRhs('10')).toEqual({ cell: null, num: 10 });
  expect(classifyConstraintRhs('3,5')).toEqual({ cell: null, num: 3.5 });
  expect(classifyConstraintRhs('-2')).toEqual({ cell: null, num: -2 });
  // Ni celda ni número → inválida (y un rango no se coacciona a número).
  expect(classifyConstraintRhs('foo')).toBeNull();
  expect(classifyConstraintRhs('A1:B2')).toBeNull();
});

test('cableado RHS-celda: «B1≤D3, B2≤D3» con D3=40 usa el valor vivo (no la fila 3)', () => {
  // Reproduce el pegado del lienzo (sheetSolverReader): clasifica las refs y arma el
  // mismo `evaluate` (motor desechable, fija variables, lee objetivo + LHS/RHS). Si
  // el RHS-celda se malinterpretara como el nº de fila (3), el máximo sería 6, no 80.
  const engine = new Engine({ sheets: ['Hoja1'] });
  engine.setCell('Hoja1', 'B1', '0');
  engine.setCell('Hoja1', 'B2', '0');
  engine.setCell('Hoja1', 'B3', '=B1+B2');
  engine.setCell('Hoja1', 'D3', '40'); // celda de valor: tope real de las variables
  const varCells = ['B1', 'B2'];
  const resolved = [
    { lhs: 'B1', rhs: 'D3' },
    { lhs: 'B2', rhs: 'D3' },
  ].map((c) => {
    const rhs = classifyConstraintRhs(c.rhs);
    if (!rhs) throw new Error('RHS inválido');
    return { lhsCell: singleCellRef(c.lhs)!, rhsNum: rhs.num, rhsCell: rhs.cell };
  });
  expect(resolved.every((r) => r.rhsCell === 'D3' && r.rhsNum === null)).toBe(true);
  const numAt = (ref: string): number | null => {
    const v = engine.getValue('Hoja1', ref);
    return typeof v === 'number' && Number.isFinite(v) ? v : null;
  };
  const evaluate = (x: readonly number[]): SolverEvalResult => {
    for (let i = 0; i < varCells.length; i++)
      engine.setCell('Hoja1', varCells[i]!, String(x[i] ?? 0));
    return {
      objective: numAt('B3'),
      constraints: resolved.map((r) => ({
        lhs: numAt(r.lhsCell),
        rhs: r.rhsNum !== null ? r.rhsNum : numAt(r.rhsCell!),
      })),
    };
  };
  const r = solve(evaluate, [0, 0], { kind: 'max' }, [{ relation: 'lte' }, { relation: 'lte' }]);
  expect(r.feasible).toBe(true);
  expect(r.values[0]).toBeCloseTo(40, 1);
  expect(r.values[1]).toBeCloseTo(40, 1);
  expect(r.objective).toBeCloseTo(80, 1);
});

test('respaldado por el motor: vars B1,B2, objetivo B3=«(B1-2)²+(B2-5)²» → (2,5)', () => {
  // Reproduce el uso real: motor desechable, fijar las variables y leer objetivo.
  const engine = new Engine({ sheets: ['Hoja1'] });
  engine.setCell('Hoja1', 'B1', '0');
  engine.setCell('Hoja1', 'B2', '0');
  engine.setCell('Hoja1', 'B3', '=(B1-2)*(B1-2)+(B2-5)*(B2-5)');
  const ev = (x: readonly number[]): SolverEvalResult => {
    engine.setCell('Hoja1', 'B1', String(x[0]));
    engine.setCell('Hoja1', 'B2', String(x[1]));
    const v = engine.getValue('Hoja1', 'B3');
    return { objective: typeof v === 'number' && Number.isFinite(v) ? v : null, constraints: [] };
  };
  const r = solve(ev, [0, 0], { kind: 'min' }, []);
  expect(r.feasible).toBe(true);
  expect(r.values[0]).toBeCloseTo(2, 2);
  expect(r.values[1]).toBeCloseTo(5, 2);
  expect(r.objective).toBeCloseTo(0, 3);
});
