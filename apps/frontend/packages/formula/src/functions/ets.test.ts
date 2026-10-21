/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Suavizado exponencial (FORECAST.ETS y familia). Las referencias son SOLUCIONES CERRADAS,
 * que no dependen de ninguna implementación: una recta perfecta se extrapola a la recta, una
 * serie estacional exacta se repite, y las reglas de datos son las de la documentación de
 * Microsoft. El contraste con LibreOffice, con sus tolerancias, está en compat/ets.test.ts.
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

/** Evalúa una fórmula con una o varias series en columnas (A, B, …) y devuelve el valor. */
function calc(formula: string, cols: Record<string, (number | string)[]> = {}): number | string {
  const e = new Engine();
  for (const [c, v] of Object.entries(cols))
    v.forEach((x, i) => e.setCell('Hoja1', `${c}${i + 1}`, String(x)));
  e.setCell('Hoja1', 'Z1', formula);
  const v = e.getValue('Hoja1', 'Z1');
  return typeof v === 'object' ? (v as { code: string }).code : (v as number | string);
}
const rango = (n: number, ini = 1): number[] => Array.from({ length: n }, (_, i) => ini + i);
const T = rango(20);
const recta = T.map((t) => 3 + 2 * t);
const EST = [2, -1, -3, 2];
const TS = rango(24);
const estacional = TS.map((t) => 10 + t + EST[t % 4]!);
const multiplicativa = TS.map((t) => (10 + 0.5 * t) * [1.2, 0.8, 1.1, 0.9][t % 4]!);

test('una recta perfecta se extrapola exactamente, con y sin estacionalidad automática', () => {
  for (const sea of ['', ',0', ',1']) {
    expect(calc(`=FORECAST.ETS.ADD(25,B1:B20,A1:A20${sea})`, { A: T, B: recta })).toBeCloseTo(
      53,
      6,
    );
    expect(calc(`=FORECAST.ETS.ADD(22.5,B1:B20,A1:A20${sea})`, { A: T, B: recta })).toBeCloseTo(
      48,
      6,
    );
  }
  // FORECAST.ETS es el modelo aditivo.
  expect(calc('=FORECAST.ETS(25,B1:B20,A1:A20)', { A: T, B: recta })).toBeCloseTo(53, 6);
});

test('una serie estacional aditiva exacta se repite: periodo 4, pronóstico exacto', () => {
  const c = { A: TS, B: estacional };
  expect(calc('=FORECAST.ETS.SEASONALITY(B1:B24,A1:A24)', c)).toBe(4);
  for (const t of [25, 26, 27, 28, 30]) {
    expect(calc(`=FORECAST.ETS.ADD(${t},B1:B24,A1:A24)`, c)).toBeCloseTo(10 + t + EST[t % 4]!, 5);
    expect(calc(`=FORECAST.ETS.ADD(${t},B1:B24,A1:A24,4)`, c)).toBeCloseTo(10 + t + EST[t % 4]!, 5);
  }
  // Sin estacionalidad el mismo dato se pronostica peor: la recta no ve el ciclo.
  const sin = calc('=FORECAST.ETS.ADD(26,B1:B24,A1:A24,0)', c) as number;
  expect(Math.abs(sin - (10 + 26 + EST[26 % 4]!))).toBeGreaterThan(0.5);
});

test('una serie estacional multiplicativa se pronostica con FORECAST.ETS.MULT', () => {
  const c = { A: TS, B: multiplicativa };
  expect(calc('=FORECAST.ETS.SEASONALITY(B1:B24,A1:A24)', c)).toBe(4);
  for (const t of [25, 26, 27, 28]) {
    const real = (10 + 0.5 * t) * [1.2, 0.8, 1.1, 0.9][t % 4]!;
    const m = calc(`=FORECAST.ETS.MULT(${t},B1:B24,A1:A24,4)`, c) as number;
    expect(Math.abs(m / real - 1)).toBeLessThan(0.01);
  }
});

test('no hay estacionalidad en una recta ni en una constante', () => {
  expect(calc('=FORECAST.ETS.SEASONALITY(B1:B20,A1:A20)', { A: T, B: recta })).toBe(1);
  expect(calc('=FORECAST.ETS.SEASONALITY(B1:B20,A1:A20)', { A: T, B: T.map(() => 7) })).toBe(1);
});

test('la línea de tiempo desordenada da lo mismo que ordenada', () => {
  const orden = [5, 0, 9, 3, 7, 1, 8, 2, 6, 4, 19, 10, 15, 12, 18, 11, 17, 13, 16, 14];
  const des = {
    A: orden.map((i) => T[i]!),
    B: orden.map((i) => estacional[i]!),
  };
  const ord = { A: T, B: estacional.slice(0, 20) };
  const f = '=FORECAST.ETS.ADD(25,B1:B20,A1:A20)';
  expect(calc(f, des)).toBe(calc(f, ord));
});

test('los instantes repetidos se agregan: media por defecto, suma, mínimo, máximo, mediana, cuenta', () => {
  // La recta con los puntos 5 y 10 duplicados (valores recta±2 que se compensan en la media).
  const x = [...T, 5, 10];
  const y = [...recta, recta[4]! + 2, recta[9]! - 2];
  y[4] = recta[4]! - 2;
  y[9] = recta[9]! + 2;
  const c = { A: x, B: y };
  const f = (agg: string): number =>
    calc(`=FORECAST.ETS.ADD(25,B1:B22,A1:A22,0,1${agg})`, c) as number;
  expect(f('')).toBeCloseTo(53, 6); // media
  expect(f(',1')).toBeCloseTo(53, 6); // media
  // El máximo y el mínimo sesgan en sentidos contrarios.
  expect(f(',4')).toBeGreaterThan(f(',6'));
  // La suma duplica esos dos puntos; la cuenta da un 1 o un 2 por instante.
  expect(f(',7')).not.toBeCloseTo(53, 3);
  expect(Math.abs(f(',2') - 53)).toBeGreaterThan(10);
  expect(f(',2')).toBe(f(',3'));
});

test('los huecos se completan por interpolación (o cuentan como cero)', () => {
  const x = T.filter((t) => t !== 7 && t !== 8 && t !== 15);
  const y = x.map((t) => 3 + 2 * t);
  const c = { A: x, B: y };
  const n = x.length;
  expect(calc(`=FORECAST.ETS.ADD(25,B1:B${n},A1:A${n},0)`, c)).toBeCloseTo(53, 6);
  expect(calc(`=FORECAST.ETS.ADD(25,B1:B${n},A1:A${n},0,1)`, c)).toBeCloseTo(53, 6);
  // Como ceros, los tres huecos arrastran el pronóstico.
  const sinCompletar = calc(`=FORECAST.ETS.ADD(25,B1:B${n},A1:A${n},0,0)`, c) as number;
  expect(Math.abs(sinCompletar - 53)).toBeGreaterThan(1);
});

test('una línea de tiempo en fechas mensuales equivale a la de enteros', () => {
  const fechas = rango(24).map((i) => {
    const d = new Date(Date.UTC(2023 + Math.floor((i - 1) / 12), (i - 1) % 12, 1));
    return Math.round(d.getTime() / 86400000) + 25569;
  });
  const objetivo = Math.round(Date.UTC(2025, 0, 1) / 86400000) + 25569;
  const m = calc(`=FORECAST.ETS.ADD(${objetivo},B1:B24,A1:A24)`, { A: fechas, B: estacional });
  const e = calc('=FORECAST.ETS.ADD(25,B1:B24,A1:A24)', { A: TS, B: estacional });
  expect(m).toBeCloseTo(e as number, 9);
  // El paso es de un mes, no de 28 a 31 días.
  expect(calc('=FORECAST.ETS.STAT(B1:B24,A1:A24,8)', { A: fechas, B: estacional })).toBe(1);
  // Y un fin de mes también.
  const finales = fechas.map((f) => f - 1);
  const m2 = calc(`=FORECAST.ETS.ADD(${objetivo - 1},B1:B24,A1:A24)`, {
    A: finales,
    B: estacional,
  });
  expect(m2).toBeCloseTo(e as number, 9);
});

test('el paso de la línea de tiempo se publica en STAT 8, también si es fraccionario', () => {
  expect(calc('=FORECAST.ETS.STAT(B1:B20,A1:A20,8)', { A: T, B: recta })).toBe(1);
  expect(calc('=FORECAST.ETS.STAT(B1:B20,A1:A20,8)', { A: T.map((t) => t * 7), B: recta })).toBe(7);
  expect(calc('=FORECAST.ETS.STAT(B1:B20,A1:A20,8)', { A: T.map((t) => t * 0.5), B: recta })).toBe(
    0.5,
  );
});

test('el intervalo de confianza crece con el horizonte y con la confianza, y es 0 sin error', () => {
  const ruido = T.map((t) => 3 + 2 * t + [0.4, -0.3, 0.2, -0.5, 0.1][t % 5]!);
  const c = { A: T, B: ruido };
  const ci = (t: number, nc: string): number =>
    calc(`=FORECAST.ETS.CONFINT(${t},B1:B20,A1:A20,${nc},0)`, c) as number;
  expect(ci(21, '0.95')).toBeGreaterThan(0);
  expect(ci(30, '0.95')).toBeGreaterThan(ci(21, '0.95'));
  expect(ci(21, '0.99')).toBeGreaterThan(ci(21, '0.95'));
  expect(ci(21, '0.5')).toBeLessThan(ci(21, '0.95'));
  // Aditiva y multiplicativa tienen su nombre propio, y CONFINT es PI.ADD.
  expect(calc('=FORECAST.ETS.PI.ADD(21,B1:B20,A1:A20,0.95,0)', c)).toBe(ci(21, '0.95'));
  // Recta perfecta: error cero, intervalo cero.
  expect(calc('=FORECAST.ETS.CONFINT(25,B1:B20,A1:A20,0.95,0)', { A: T, B: recta })).toBeCloseTo(
    0,
    6,
  );
});

test('FORECAST.ETS.STAT: parámetros en [0,1], MAE ≤ RMSE y MASE de una recta perfecta', () => {
  const ruido = TS.map((t) => 10 + t + EST[t % 4]! + [0.4, -0.3, 0.2, -0.5, 0.1][t % 5]!);
  const c = { A: TS, B: ruido };
  const st = (k: number): number => calc(`=FORECAST.ETS.STAT(B1:B24,A1:A24,${k})`, c) as number;
  for (const k of [1, 2, 3]) {
    expect(st(k)).toBeGreaterThanOrEqual(0);
    expect(st(k)).toBeLessThanOrEqual(1);
  }
  expect(st(6)).toBeLessThanOrEqual(st(7) + 1e-12);
  expect(st(4)).toBeGreaterThan(0);
  expect(st(5)).toBeGreaterThan(0);
  expect(calc('=FORECAST.ETS.STAT(B1:B20,A1:A20,6)', { A: T, B: recta })).toBeCloseTo(0, 6);
  // La gamma de un modelo sin estacionalidad es 0.
  expect(calc('=FORECAST.ETS.STAT(B1:B20,A1:A20,3,0)', { A: T, B: recta })).toBe(0);
});

test('los alias de Excel y de LibreOffice son la misma función', () => {
  const c = { A: TS, B: estacional };
  expect(calc('=FORECAST.ETS(27,B1:B24,A1:A24)', c)).toBe(
    calc('=FORECAST.ETS.ADD(27,B1:B24,A1:A24)', c),
  );
  expect(calc('=FORECAST.ETS.STAT(B1:B24,A1:A24,7)', c)).toBe(
    calc('=FORECAST.ETS.STAT.ADD(B1:B24,A1:A24,7)', c),
  );
  expect(calc('=FORECAST.ETS.MULT(27,B1:B24,A1:A24,4)', c)).toBe(
    calc('=FORECAST.ETS.MULT(27,B1:B24,A1:A24,4)', c),
  );
});

test('errores: tamaños, tipos, línea de tiempo, objetivo y argumentos fuera de rango', () => {
  const c = { A: T, B: recta };
  const e = (f: string, cols = c): number | string => calc(f, cols);
  expect(e('=FORECAST.ETS.ADD(25,B1:B20,A1:A19)')).toBe('#N/A'); // tamaños distintos
  expect(
    e('=FORECAST.ETS.ADD(25,B1:B20,A1:A20)', { A: T, B: recta.map((v, i) => (i === 3 ? 'x' : v)) }),
  ).toBe('#VALUE!');
  expect(
    e('=FORECAST.ETS.ADD(25,B1:B20,A1:A20)', { A: T.map((v, i) => (i === 3 ? 'x' : v)), B: recta }),
  ).toBe('#VALUE!');
  expect(e('=FORECAST.ETS.ADD(20,B1:B20,A1:A20)')).toBe('#NUM!'); // el objetivo no es posterior
  expect(e('=FORECAST.ETS.ADD(5,B1:B20,A1:A20)')).toBe('#NUM!');
  expect(e('=FORECAST.ETS.ADD(25,B1:B2,A1:A2)')).toBe('#NUM!'); // menos de tres puntos
  // Paso no constante: 1, 2, 3,5, 5… (la diferencia de 1,5 no es múltiplo de la de 1).
  expect(e('=FORECAST.ETS.ADD(25,B1:B5,A1:A5)', { A: [1, 2, 3.5, 5, 6], B: [1, 2, 3, 4, 5] })).toBe(
    '#NUM!',
  );
  // Pero 1, 2, 3, 5, 8, 13 son una línea regular con huecos, y se completa.
  expect(
    typeof e('=FORECAST.ETS.ADD(25,B1:B6,A1:A6)', {
      A: [1, 2, 3, 5, 8, 13],
      B: [1, 2, 3, 4, 5, 6],
    }),
  ).toBe('number');
  expect(e('=FORECAST.ETS.ADD(25,B1:B20,A1:A20,-1)')).toBe('#NUM!');
  expect(e('=FORECAST.ETS.ADD(25,B1:B20,A1:A20,8761)')).toBe('#NUM!');
  expect(e('=FORECAST.ETS.ADD(25,B1:B20,A1:A20,11)')).toBe('#NUM!'); // 2·11 > 20 puntos
  expect(e('=FORECAST.ETS.ADD(25,B1:B20,A1:A20,1,2)')).toBe('#NUM!'); // data_completion 0/1
  expect(e('=FORECAST.ETS.ADD(25,B1:B20,A1:A20,1,1,8)')).toBe('#NUM!'); // agregación 0..7
  expect(e('=FORECAST.ETS.ADD(25,B1:B20,A1:A20,"a")')).toBe('#VALUE!');
  expect(e('=FORECAST.ETS.ADD(25,B1:B20)')).toBe('#VALUE!');
  expect(e('=FORECAST.ETS.CONFINT(25,B1:B20,A1:A20,0)')).toBe('#NUM!');
  expect(e('=FORECAST.ETS.CONFINT(25,B1:B20,A1:A20,1)')).toBe('#NUM!');
  expect(e('=FORECAST.ETS.STAT(B1:B20,A1:A20,9)')).toBe('#NUM!');
  expect(e('=FORECAST.ETS.STAT(B1:B20,A1:A20,0)')).toBe('#NUM!');
  // El modelo multiplicativo exige estrictamente positivos.
  expect(
    e('=FORECAST.ETS.MULT(25,B1:B20,A1:A20,4)', {
      A: T,
      B: recta.map((v, i) => (i === 3 ? 0 : v)),
    }),
  ).toBe('#NUM!');
  // Un error en los datos se propaga.
  expect(
    e('=FORECAST.ETS.ADD(25,B1:B20,A1:A20)', {
      A: T,
      B: recta.map((v, i) => (i === 3 ? '=1/0' : v)),
    }),
  ).toBe('#DIV/0!');
});

test('el intervalo es simétrico: no cambia al invertir ni al desplazar la serie, y el pronóstico sí', () => {
  const y = TS.map((t) => 10 + t + EST[t % 4]! + [0.4, -0.3, 0.2, -0.5, 0.1][t % 5]!);
  const ci = (v: number[]): number =>
    calc('=FORECAST.ETS.CONFINT(27,B1:B24,A1:A24,0.95,4)', { A: TS, B: v }) as number;
  const f = (v: number[]): number =>
    calc('=FORECAST.ETS.ADD(27,B1:B24,A1:A24,4)', { A: TS, B: v }) as number;
  expect(ci(y.map((v) => -v))).toBeCloseTo(ci(y), 9);
  expect(f(y.map((v) => -v))).toBeCloseTo(-f(y), 9);
  expect(ci(y.map((v) => v + 1000))).toBeCloseTo(ci(y), 6);
  expect(f(y.map((v) => v + 1000))).toBeCloseTo(f(y) + 1000, 6);
});

test('cobertura: con ruido gaussiano conocido, el intervalo del 95 % contiene el valor real ~95 % de las veces', () => {
  // Generador congruencial fijo y suma de doce uniformes (σ = 1): sin Math.random.
  let s = 20261004;
  const u = (): number => {
    s = (Math.imul(s, 1664525) + 1013904223) >>> 0;
    return s / 4294967296;
  };
  const gauss = (): number => {
    let g = -6;
    for (let i = 0; i < 12; i++) g += u();
    return g;
  };
  const SIGMA = 2;
  const N = 40;
  const SERIES = 150;
  const t = rango(N);
  let dentro95 = 0;
  let dentro80 = 0;
  let anchura = 0;
  for (let k = 0; k < SERIES; k++) {
    const y = t.map((x) => 5 + 0.7 * x + SIGMA * gauss());
    const real = 5 + 0.7 * (N + 1) + SIGMA * gauss();
    const c = { A: t, B: y };
    const p = calc(`=FORECAST.ETS.ADD(${N + 1},B1:B${N},A1:A${N},0)`, c) as number;
    const i95 = calc(`=FORECAST.ETS.CONFINT(${N + 1},B1:B${N},A1:A${N},0.95,0)`, c) as number;
    const i80 = calc(`=FORECAST.ETS.CONFINT(${N + 1},B1:B${N},A1:A${N},0.8,0)`, c) as number;
    if (Math.abs(real - p) <= i95) dentro95++;
    if (Math.abs(real - p) <= i80) dentro80++;
    anchura += i95;
  }
  // El intervalo nominal a un paso es 1,96·σ = 3,92; el modelo añade su propia incertidumbre.
  expect(anchura / SERIES).toBeGreaterThan(1.96 * SIGMA * 0.9);
  expect(anchura / SERIES).toBeLessThan(1.96 * SIGMA * 1.5);
  expect(dentro95 / SERIES).toBeGreaterThan(0.9);
  expect(dentro95 / SERIES).toBeLessThan(0.995);
  expect(dentro80 / SERIES).toBeGreaterThan(0.72);
  expect(dentro80 / SERIES).toBeLessThan(0.9);
}, 120_000);

test('determinista: la misma entrada da exactamente el mismo resultado', () => {
  const y = TS.map((t) => 10 + t + EST[t % 4]! + [0.4, -0.3, 0.2, -0.5, 0.1][t % 5]!);
  const c = { A: TS, B: y };
  for (const f of [
    '=FORECAST.ETS.ADD(27,B1:B24,A1:A24)',
    '=FORECAST.ETS.MULT(27,B1:B24,A1:A24)',
    '=FORECAST.ETS.CONFINT(27,B1:B24,A1:A24)',
    '=FORECAST.ETS.STAT(B1:B24,A1:A24,1)',
  ]) {
    expect(calc(f, c)).toBe(calc(f, c));
  }
});
