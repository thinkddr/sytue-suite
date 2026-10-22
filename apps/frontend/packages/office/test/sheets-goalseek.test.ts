/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del solver de Buscar objetivo (Goal Seek): lineal exacto, no lineal,
 * sin solución, cerca de división por cero, y las salvaguardas (tolerancia,
 * máximo de iteraciones, punto de partida en una pared). Incluye un caso
 * respaldado por el motor de fórmulas real (@sytue/formula), que es como lo usa
 * el lienzo: fijar la celda variable y leer la celda objetivo tras cada tanteo.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/sheets-goalseek.test.ts
 */
import { expect, test } from 'bun:test';
import { Engine } from '@sytue/formula';
import { goalSeek } from '../src/editor/sheets/goalseek';

test('lineal exacto: 3x + 6 = 0 → x = −2', () => {
  const r = goalSeek((x) => 3 * x + 6, 0);
  expect(r.solved).toBe(true);
  expect(r.value).toBeCloseTo(-2, 9);
  expect(r.finalError).toBeLessThanOrEqual(1e-7);
  expect(r.iterations).toBeLessThanOrEqual(100);
});

test('lineal exacto con objetivo y arranque no triviales', () => {
  // 2.5·x − 7 = 18 → x = 10
  const r = goalSeek((x) => 2.5 * x - 7, 18, { x0: 1000 });
  expect(r.solved).toBe(true);
  expect(r.value).toBeCloseTo(10, 6);
});

test('no lineal: x² = 9 desde 0 (función par, arranque simétrico)', () => {
  const r = goalSeek((x) => x * x, 9, { x0: 0 });
  expect(r.solved).toBe(true);
  expect(Math.abs(r.value)).toBeCloseTo(3, 4); // ±3 valen
});

test('no lineal: x³ = 27 → x = 3', () => {
  const r = goalSeek((x) => x * x * x, 27, { x0: 1 });
  expect(r.solved).toBe(true);
  expect(r.value).toBeCloseTo(3, 5);
});

test('no lineal trascendente: eˣ = 10 → x = ln 10', () => {
  const r = goalSeek((x) => Math.exp(x), 10, { x0: 0 });
  expect(r.solved).toBe(true);
  expect(r.value).toBeCloseTo(Math.log(10), 5);
});

test('sin solución: x² = −1 → no resuelve, best-effort acotado', () => {
  const r = goalSeek((x) => x * x, -1, { x0: 0 });
  expect(r.solved).toBe(false);
  expect(r.finalError).toBeGreaterThan(0);
  expect(r.iterations).toBeLessThanOrEqual(100);
  expect(Number.isFinite(r.value)).toBe(true);
});

test('cerca de división por cero: 100/x = 5 → x = 20, y f(0) es una pared', () => {
  let wallHits = 0;
  const r = goalSeek(
    (x) => {
      if (x === 0) {
        wallHits++;
        return null; // #DIV/0!: pared
      }
      return 100 / x;
    },
    5,
    { x0: 0 },
  );
  expect(r.solved).toBe(true);
  expect(r.value).toBeCloseTo(20, 4);
  expect(wallHits).toBeGreaterThan(0); // arrancó en la pared y la sorteó
});

test('función racional con polo fuera del camino: 10/(x+1) = 2 → x = 4', () => {
  // Polo en x = −1; arrancando en 0 la búsqueda se aleja del polo y resuelve.
  const r = goalSeek((x) => (x === -1 ? null : 10 / (x + 1)), 2, { x0: 0 });
  expect(r.solved).toBe(true);
  expect(r.value).toBeCloseTo(4, 4);
});

test('respeta maxIterations', () => {
  let calls = 0;
  const r = goalSeek(
    (x) => {
      calls++;
      return x * x;
    },
    9,
    { x0: 0, maxIterations: 3 },
  );
  expect(calls).toBeLessThanOrEqual(3);
  expect(r.iterations).toBeLessThanOrEqual(3);
});

test('respeta la tolerancia: finalError ≤ tolerance cuando resuelve', () => {
  const tol = 1e-4;
  const r = goalSeek((x) => x * x * x, 50, { x0: 3, tolerance: tol });
  expect(r.solved).toBe(true);
  expect(r.finalError).toBeLessThanOrEqual(tol);
});

test('la variable ya está en el objetivo: 0 iteraciones de refinamiento', () => {
  const r = goalSeek((x) => x + 5, 5, { x0: 0 }); // f(0)=5=objetivo
  expect(r.solved).toBe(true);
  expect(r.value).toBe(0);
  expect(r.iterations).toBe(1); // basta una evaluación
});

test('respaldado por el motor: variable B1, objetivo B2 = «=B1*B1+B1»', () => {
  // Reproduce el uso real: motor desechable, fijar la variable y leer el objetivo.
  const build = (): Engine => {
    const e = new Engine({ sheets: ['Hoja1'] });
    e.setCell('Hoja1', 'B1', '1');
    e.setCell('Hoja1', 'B2', '=B1*B1+B1'); // x²+x
    return e;
  };
  const engine = build();
  const f = (x: number): number | null => {
    engine.setCell('Hoja1', 'B1', String(x));
    const v = engine.getValue('Hoja1', 'B2');
    return typeof v === 'number' && Number.isFinite(v) ? v : null;
  };
  // x² + x = 6 → x = 2 (raíz positiva)
  const r = goalSeek(f, 6, { x0: 1 });
  expect(r.solved).toBe(true);
  expect(r.value).toBeCloseTo(2, 5);
});
