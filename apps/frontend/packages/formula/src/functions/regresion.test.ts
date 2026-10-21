/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Funciones que devuelven MATRIZ: FREQUENCY, LINEST, LOGEST y GROWTH.
 *
 * Dos cosas se comprueban aquí que no son el cálculo: la FORMA del resultado (un
 * histograma tiene un elemento más que límites; LINEST devuelve la pendiente PRIMERO) y
 * el modo sin constante, que no es «la misma recta con b = 0» sino otra regresión.
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

/** Evalúa en Z1 y devuelve la matriz derramada leyendo el rango resultante. */
function evMatriz(
  formula: string,
  celdas: Record<string, string | number>,
  alto: number,
  ancho = 1,
): unknown[][] {
  const e = new Engine();
  for (const [ref, v] of Object.entries(celdas)) e.setCell('Hoja1', ref, String(v));
  e.setCell('Hoja1', 'P1', formula);
  const out: unknown[][] = [];
  for (let r = 0; r < alto; r += 1) {
    const fila: unknown[] = [];
    for (let c = 0; c < ancho; c += 1) {
      fila.push(e.getValue('Hoja1', `${String.fromCharCode(80 + c)}${r + 1}`));
    }
    out.push(fila);
  }
  return out;
}

/** Primer valor del resultado (para las que devuelven un escalar o el [0][0]). */
function ev1(formula: string, celdas: Record<string, string | number> = {}): unknown {
  const e = new Engine();
  for (const [ref, v] of Object.entries(celdas)) e.setCell('Hoja1', ref, String(v));
  e.setCell('Hoja1', 'P1', formula);
  const v = e.getValue('Hoja1', 'P1');
  return typeof v === 'object' && v !== null && 'code' in v ? (v as { code: string }).code : v;
}

const DATOS = { A1: 1, A2: 3, A3: 5, A4: 7, A5: 9, B1: 2, B2: 4, B3: 6 };

test('FREQUENCY devuelve un elemento MÁS que límites: el desbordamiento', () => {
  // Datos {1,3,5,7,9} con límites {2,4,6}: quedan 4 intervalos, no 3.
  // ≤2 → {1}          = 1
  // ≤4 → {3}          = 1
  // ≤6 → {5}          = 1
  // >6 → {7,9}        = 2   ← este es el que se olvida, y sin él las cuentas no suman 5
  const m = evMatriz('=FREQUENCY(A1:A5,B1:B3)', DATOS, 4);
  expect(m.map((f) => f[0])).toEqual([1, 1, 1, 2]);
  const total = m.reduce((a, f) => a + (f[0] as number), 0);
  expect(total).toBe(5); // el histograma cuadra con el número de datos
});

test('los límites de FREQUENCY son superiores E INCLUSIVOS y se ordenan solos', () => {
  // El 2 y el 4 caen EN su intervalo, no en el siguiente.
  const enElBorde = { A1: 2, A2: 4, B1: 2, B2: 4 };
  expect(evMatriz('=FREQUENCY(A1:A2,B1:B2)', enElBorde, 3).map((f) => f[0])).toEqual([1, 1, 0]);

  // Límites escritos al revés: el usuario espera el mismo histograma.
  const desordenados = { A1: 1, A2: 3, A3: 5, B1: 4, B2: 2 };
  expect(evMatriz('=FREQUENCY(A1:A3,B1:B2)', desordenados, 3).map((f) => f[0])).toEqual([1, 1, 1]);
});

test('LINEST devuelve la PENDIENTE primero y el intercepto después', () => {
  // y = 2x + 1 exacto: {3,5,7} contra {1,2,3}. La primera celda es 2, no 1.
  const recta = { A1: 3, A2: 5, A3: 7, B1: 1, B2: 2, B3: 3 };
  const m = evMatriz('=LINEST(A1:A3,B1:B3)', recta, 1, 2);
  expect(m[0]![0] as number).toBeCloseTo(2, 10); // pendiente
  expect(m[0]![1] as number).toBeCloseTo(1, 10); // intercepto
  // Y coincide con las funciones escalares que ya existían.
  expect(ev1('=SLOPE(A1:A3,B1:B3)', recta) as number).toBeCloseTo(2, 10);
  expect(ev1('=INTERCEPT(A1:A3,B1:B3)', recta) as number).toBeCloseTo(1, 10);
});

test('sin constante NO es la misma recta con b = 0: es otra regresión', () => {
  // {3,5,7} contra {1,2,3}. Con constante: m = 2. Forzando el paso por el origen,
  // m = Σxy/Σx² = (3+10+21)/(1+4+9) = 34/14 ≈ 2.4286 — distinto, y el correcto para
  // ese modelo. Usar las sumas centradas y luego poner b a cero daría 2, que está mal.
  const recta = { A1: 3, A2: 5, A3: 7, B1: 1, B2: 2, B3: 3 };
  const m = evMatriz('=LINEST(A1:A3,B1:B3,FALSE)', recta, 1, 2);
  expect(m[0]![0] as number).toBeCloseTo(34 / 14, 10);
  expect(m[0]![1]).toBe(0);
});

test('LINEST sin xs usa 1,2,3… como abscisas', () => {
  // {2,4,6} sin xs equivale a xs = {1,2,3}: pendiente 2, intercepto 0.
  const y = { A1: 2, A2: 4, A3: 6 };
  const m = evMatriz('=LINEST(A1:A3)', y, 1, 2);
  expect(m[0]![0] as number).toBeCloseTo(2, 10);
  expect(m[0]![1] as number).toBeCloseTo(0, 10);
});

test('LOGEST ajusta y = b·mˣ y rechaza los y no positivos', () => {
  // y = 2ˣ exacto: {2,4,8} contra {1,2,3} → m = 2, b = 1.
  const exp = { A1: 2, A2: 4, A3: 8, B1: 1, B2: 2, B3: 3 };
  const m = evMatriz('=LOGEST(A1:A3,B1:B3)', exp, 1, 2);
  expect(m[0]![0] as number).toBeCloseTo(2, 8); // m
  expect(m[0]![1] as number).toBeCloseTo(1, 8); // b

  // Un y ≤ 0 no tiene logaritmo y ninguna exponencial pasa por él: es #NUM!, no un
  // punto que se descarte en silencio dejando un ajuste sobre los demás.
  const conCero = { A1: 2, A2: 0, A3: 8, B1: 1, B2: 2, B3: 3 };
  expect(ev1('=LOGEST(A1:A3,B1:B3)', conCero)).toBe('#NUM!');
});

test('GROWTH predice con el modelo exponencial ajustado', () => {
  // Ajusta y = 2ˣ sobre {2,4,8} y predice en x = 4 → 16.
  const exp = { A1: 2, A2: 4, A3: 8, B1: 1, B2: 2, B3: 3, C1: 4 };
  const m = evMatriz('=GROWTH(A1:A3,B1:B3,C1)', exp, 1);
  expect(m[0]![0] as number).toBeCloseTo(16, 6);

  // Sin «nuevas x» se predice sobre las mismas x, así que reproduce los datos.
  const m2 = evMatriz('=GROWTH(A1:A3,B1:B3)', exp, 3);
  expect(m2[0]![0] as number).toBeCloseTo(2, 6);
  expect(m2[2]![0] as number).toBeCloseTo(8, 6);
});

test('las regresiones exigen dos puntos y abscisas de la misma longitud', () => {
  const uno = { A1: 5, B1: 1 };
  expect(ev1('=LINEST(A1:A1,B1:B1)', uno)).toBe('#DIV/0!');
  const desigual = { A1: 1, A2: 2, A3: 3, B1: 1, B2: 2 };
  expect(ev1('=LINEST(A1:A3,B1:B2)', desigual)).toBe('#REF!');
});
