/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Distribuciones discretas (hipergeométrica, binomial negativa, BINOM.INV), lognormal,
 * variantes «A» y PROB.
 *
 * Los valores de referencia salen de la definición cerrada de cada distribución
 * —calculada a mano en el comentario de cada caso—, no de copiar la salida del propio
 * código: comprobar que un cálculo devuelve lo que devuelve no prueba nada.
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

function ev(formula: string, celdas: Record<string, string | number> = {}): CellValue | string {
  const e = new Engine();
  for (const [ref, v] of Object.entries(celdas)) e.setCell('Hoja1', ref, String(v));
  e.setCell('Hoja1', 'Z1', formula);
  const v = e.getValue('Hoja1', 'Z1');
  return typeof v === 'object' && v !== null && 'code' in v ? (v as { code: string }).code : v;
}

test('las variantes «A» cuentan el texto como cero, y por eso dan otro resultado', () => {
  // Con solo números, STDEV y STDEVA coinciden. La diferencia aparece al meter texto:
  // una lo ignora y la otra lo cuenta como 0, así que la muestra es otra. Es el único
  // motivo de que existan las dos, y un test con solo números no las distingue.
  const soloNums = { A1: 1, A2: 2, A3: 3 };
  expect(ev('=STDEV(A1:A3)', soloNums)).toBe(ev('=STDEVA(A1:A3)', soloNums));

  const conTexto = { A1: 1, A2: 2, A3: 'sin dato' };
  // STDEV ve {1,2}: varianza muestral 0.5 → desviación = √0.5
  expect(ev('=STDEV(A1:A3)', conTexto) as number).toBeCloseTo(Math.SQRT1_2, 10);
  // STDEVA ve {1,2,0}: media 1, Σ(x−x̄)² = 2, /2 = 1 → desviación 1
  expect(ev('=STDEVA(A1:A3)', conTexto) as number).toBeCloseTo(1, 10);
  expect(ev('=VARA(A1:A3)', conTexto) as number).toBeCloseTo(1, 10);
});

test('PERMUTATIONA es CON repetición: nᵏ, no n!/(n−k)!', () => {
  expect(ev('=PERMUTATIONA(3,2)')).toBe(9); // 3² = 9
  expect(ev('=PERMUT(3,2)')).toBe(6); // 3!/1! = 6
  expect(ev('=PERMUTATIONA(-1,2)')).toBe('#NUM!');
});

test('la lognormal se apoya en la normal del logaritmo, y x ≤ 0 está fuera del soporte', () => {
  // LOGNORM.DIST(1, 0, 1, TRUE) = Φ(ln 1) = Φ(0) = 0.5
  expect(ev('=LOGNORM.DIST(1,0,1,TRUE)') as number).toBeCloseTo(0.5, 6);
  // ln(0) y ln(negativo) no existen: es #NUM!, no probabilidad 0.
  expect(ev('=LOGNORM.DIST(0,0,1,TRUE)')).toBe('#NUM!');
  expect(ev('=LOGNORM.DIST(-1,0,1,TRUE)')).toBe('#NUM!');
  // La inversa deshace la directa.
  expect(ev('=LOGNORM.INV(0.5,0,1)') as number).toBeCloseTo(1, 6);
  expect(ev('=LOGNORM.INV(0,0,1)')).toBe('#NUM!');
});

test('la binomial negativa cuenta FRACASOS antes del s-ésimo éxito', () => {
  // P(0 fracasos antes del 1.er éxito) con p=0.5 → 0.5
  expect(ev('=NEGBINOM.DIST(0,1,0.5)') as number).toBeCloseTo(0.5, 10);
  // P(2 fracasos antes del 1.er éxito) = 0.5·0.5² = 0.125
  expect(ev('=NEGBINOM.DIST(2,1,0.5)') as number).toBeCloseTo(0.125, 10);
  // Acumulada hasta 2: 0.5 + 0.25 + 0.125 = 0.875
  expect(ev('=NEGBINOM.DIST(2,1,0.5,TRUE)') as number).toBeCloseTo(0.875, 10);
  expect(ev('=NEGBINOM.DIST(-1,1,0.5)')).toBe('#NUM!');
});

test('la hipergeométrica es SIN reposición y valida que la muestra quepa', () => {
  // Urna de 4 con 2 buenas, saco 2. P(1 buena) = C(2,1)·C(2,1)/C(4,2) = 4/6
  expect(ev('=HYPGEOM.DIST(1,2,2,4)') as number).toBeCloseTo(2 / 3, 10);
  // P(0 buenas) = C(2,0)·C(2,2)/C(4,2) = 1/6
  expect(ev('=HYPGEOM.DIST(0,2,2,4)') as number).toBeCloseTo(1 / 6, 10);
  // Acumulada hasta 1: 1/6 + 4/6 = 5/6
  expect(ev('=HYPGEOM.DIST(1,2,2,4,TRUE)') as number).toBeCloseTo(5 / 6, 10);
  // Incoherencias de datos: más éxitos en la muestra que en la población, o muestra
  // mayor que la población. Devolver 0 las escondería.
  expect(ev('=HYPGEOM.DIST(3,2,2,4)')).toBe('#NUM!');
  expect(ev('=HYPGEOM.DIST(1,5,2,4)')).toBe('#NUM!');
});

test('la hipergeométrica no desborda con poblaciones grandes', () => {
  // C(2000,1000) no cabe en un double; por logaritmos, la probabilidad sí.
  const r = ev('=HYPGEOM.DIST(500,1000,1000,2000)') as number;
  expect(Number.isFinite(r)).toBe(true);
  expect(r).toBeGreaterThan(0);
});

test('BINOM.INV devuelve el escalón exacto, no el punto medio de un salto', () => {
  // n=10, p=0.5: la acumulada alcanza 0.5 por primera vez en k=5. Una bisección sobre
  // esta escalera puede devolver el escalón de al lado.
  expect(ev('=BINOM.INV(10,0.5,0.5)')).toBe(5);
  // Caso fino: la masa en k=0 es 0.5¹⁰ = 0.0009766, que NO llega a α = 0.001, así que
  // el resultado es 1 y no 0. Es justo el borde donde un «>» en vez de un «>=», o
  // acumular después de comparar, devuelve el escalón anterior.
  expect(ev('=BINOM.INV(10,0.5,0.001)')).toBe(1);
  // Y con α por debajo de esa masa, el primer escalón sí basta.
  expect(ev('=BINOM.INV(10,0.5,0.0009)')).toBe(0);
  expect(ev('=BINOM.INV(10,0.5,0)')).toBe('#NUM!');
  expect(ev('=BINOM.INV(10,0.5,1)')).toBe('#NUM!');
});

test('Z.TEST usa la desviación MUESTRAL cuando se omite sigma', () => {
  // {1,2,3,4,5}: media 3. Con μ₀ = 3 el estadístico es 0 y la cola derecha vale 0.5.
  const datos = { A1: 1, A2: 2, A3: 3, A4: 4, A5: 5 };
  expect(ev('=Z.TEST(A1:A5,3)', datos) as number).toBeCloseTo(0.5, 6);
  // Con μ₀ por debajo de la media, la cola derecha se estrecha.
  expect(ev('=Z.TEST(A1:A5,2)', datos) as number).toBeLessThan(0.5);
  // Dar sigma explícito cambia el resultado: si no, el argumento no se estaría usando.
  expect(ev('=Z.TEST(A1:A5,2,1)', datos)).not.toBe(ev('=Z.TEST(A1:A5,2)', datos));
});

test('PROB exige que las probabilidades sumen 1', () => {
  const d = { A1: 1, A2: 2, A3: 3, B1: 0.2, B2: 0.3, B3: 0.5 };
  // P(2 ≤ x ≤ 3) = 0.3 + 0.5 = 0.8
  expect(ev('=PROB(A1:A3,B1:B3,2,3)', d) as number).toBeCloseTo(0.8, 10);
  // Sin límite superior, solo el valor exacto.
  expect(ev('=PROB(A1:A3,B1:B3,2)', d) as number).toBeCloseTo(0.3, 10);
  // Una lista que no suma 1 no es una distribución: devolver un número igualmente
  // escondería un error de datos en la hoja del usuario.
  const mala = { A1: 1, A2: 2, B1: 0.2, B2: 0.3 };
  expect(ev('=PROB(A1:A2,B1:B2,1,2)', mala)).toBe('#NUM!');
});
