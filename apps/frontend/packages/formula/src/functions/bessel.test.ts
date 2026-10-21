/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Funciones de Bessel (BESSELJ/Y/I/K).
 *
 * Los valores de referencia salen de tablas publicadas (Abramowitz & Stegun
 * 9.1/9.8) y de los ejemplos documentados de la hoja de cálculo, NO de esta
 * implementación. Además se comprueban los dos WRONSKIANOS, que son identidades
 * exactas: no dependen de ninguna tabla y cazan el error que una tabla de
 * valores sueltos no ve, el de coger la rama equivocada en el cambio de método
 * (serie ↔ cuadratura en J, serie ↔ asintótico en Y).
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

function num(formula: string): number {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  return e.getValue('Hoja1', 'A1') as number;
}
function show(formula: string): string {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  return e.getDisplay('Hoja1', 'A1');
}

test('BESSELJ contra tabla publicada, a un lado y otro del cambio de método', () => {
  // A&S tabla 9.1. El corte serie/cuadratura está en |x| = 15.
  expect(num('=BESSELJ(1,0)')).toBeCloseTo(0.7651976866, 10);
  expect(num('=BESSELJ(1,1)')).toBeCloseTo(0.4400505857, 10);
  expect(num('=BESSELJ(2,0)')).toBeCloseTo(0.2238907791, 10);
  expect(num('=BESSELJ(2,1)')).toBeCloseTo(0.5767248078, 10);
  // Ejemplo documentado de la hoja: BESSELJ(1,9; 2) = 0,329926.
  expect(num('=BESSELJ(1.9,2)')).toBeCloseTo(0.3299257277, 10);
  // Ya en cuadratura: J₀(20) y J₁(20).
  expect(num('=BESSELJ(20,0)')).toBeCloseTo(0.1670246643, 10);
  expect(num('=BESSELJ(20,1)')).toBeCloseTo(0.0668331242, 10);
  // x negativo: Jₙ(−x) = (−1)ⁿJₙ(x), no es un error de dominio.
  expect(num('=BESSELJ(-3,0)')).toBeCloseTo(num('=BESSELJ(3,0)'), 12);
  expect(num('=BESSELJ(-3,1)')).toBeCloseTo(-num('=BESSELJ(3,1)'), 12);
});

test('BESSELY contra tabla publicada, a un lado y otro del cambio de método', () => {
  // A&S tabla 9.1. El corte serie/asintótico está en x = 13.
  expect(num('=BESSELY(1,0)')).toBeCloseTo(0.0882569642, 10);
  expect(num('=BESSELY(1,1)')).toBeCloseTo(-0.7812128213, 10);
  expect(num('=BESSELY(2,0)')).toBeCloseTo(0.5103756726, 10);
  expect(num('=BESSELY(2,1)')).toBeCloseTo(-0.1070324315, 10);
  // Ejemplo documentado de la hoja: BESSELY(2,5; 1) = 0,145918.
  expect(num('=BESSELY(2.5,1)')).toBeCloseTo(0.145918138, 10);
  // Órdenes altos: salen de la recurrencia ascendente, no del método base.
  expect(num('=BESSELY(3,2)')).toBeCloseTo(-0.1604003935, 10);
  expect(num('=BESSELY(5,3)')).toBeCloseTo(0.1462671627, 10);
  // Ya en asintótico.
  expect(num('=BESSELY(20,0)')).toBeCloseTo(0.0626405968, 10);
});

test('BESSELI y BESSELK contra tabla publicada', () => {
  expect(num('=BESSELI(1,0)')).toBeCloseTo(1.2660658778, 10);
  expect(num('=BESSELI(1,1)')).toBeCloseTo(0.565159104, 10);
  // Ejemplo documentado de la hoja: BESSELI(1,5; 1) = 0,981666.
  expect(num('=BESSELI(1.5,1)')).toBeCloseTo(0.9816664286, 10);
  expect(num('=BESSELK(1,0)')).toBeCloseTo(0.4210244382, 10);
  expect(num('=BESSELK(1,1)')).toBeCloseTo(0.6019072302, 10);
  // Ejemplo documentado de la hoja: BESSELK(1,5; 1) = 0,277388.
  expect(num('=BESSELK(1.5,1)')).toBeCloseTo(0.2773878005, 10);
  // I crece como e^x y K decae como e^(−x): ni desbordan ni se van a cero antes
  // de tiempo en el rango normal de una hoja.
  expect(num('=BESSELI(20,0)')).toBeCloseTo(4.35582826e7, -1);
  expect(num('=BESSELK(20,0)')).toBeCloseTo(5.74123782e-10, 18);
  // Cerca del origen K diverge como −ln(x/2)−γ ≈ 18,5366.
  expect(num('=BESSELK(0.00000001,0)')).toBeCloseTo(18.5366122596, 8);
});

test('wronskiano Jₙ·Yₙ₊₁ − Jₙ₊₁·Yₙ = −2/(πx): identidad exacta en todo el dominio', () => {
  // Cruza los dos cambios de método (x = 13 en Y, x = 15 en J). Si una rama se
  // eligiera mal, o el corte estuviera donde ninguno de los dos ha convergido,
  // aquí saldría enseguida; una tabla de valores sueltos podría no pisarlo.
  for (const x of [0.5, 1, 3, 7, 12.9, 13.1, 14.9, 15.1, 20, 50]) {
    for (const n of [0, 1, 2, 3, 5]) {
      const w =
        num(`=BESSELJ(${x},${n})*BESSELY(${x},${n + 1})`) -
        num(`=BESSELJ(${x},${n + 1})*BESSELY(${x},${n})`);
      expect(w / (-2 / (Math.PI * x))).toBeCloseTo(1, 9);
    }
  }
});

test('wronskiano Iₙ·Kₙ₊₁ + Iₙ₊₁·Kₙ = 1/x: identidad exacta en todo el dominio', () => {
  for (const x of [0.01, 0.5, 1, 3, 7, 15, 30, 60]) {
    for (const n of [0, 1, 2, 3, 5]) {
      const w =
        num(`=BESSELI(${x},${n})*BESSELK(${x},${n + 1})`) +
        num(`=BESSELI(${x},${n + 1})*BESSELK(${x},${n})`);
      expect(w * x).toBeCloseTo(1, 10);
    }
  }
});

test('dominios: orden negativo y el origen de las que divergen', () => {
  expect(show('=BESSELJ(1,-1)')).toBe('#NUM!'); // orden < 0
  expect(show('=BESSELI(1,-1)')).toBe('#NUM!');
  // Y y K divergen en el origen: x ≤ 0 no es un cero grande, es #NUM!.
  expect(show('=BESSELY(0,0)')).toBe('#NUM!');
  expect(show('=BESSELY(-1,0)')).toBe('#NUM!');
  expect(show('=BESSELK(0,0)')).toBe('#NUM!');
  expect(show('=BESSELK(-1,0)')).toBe('#NUM!');
  // J e I sí valen en el origen: J₀(0) = I₀(0) = 1 y Jₙ(0) = Iₙ(0) = 0 con n ≥ 1.
  expect(num('=BESSELJ(0,0)')).toBeCloseTo(1, 12);
  expect(num('=BESSELI(0,0)')).toBeCloseTo(1, 12);
  expect(num('=BESSELJ(0,3)')).toBeCloseTo(0, 12);
  // El orden se trunca, no se redondea: 2,9 es orden 2.
  expect(num('=BESSELJ(1.9,2.9)')).toBeCloseTo(num('=BESSELJ(1.9,2)'), 12);
  expect(show('=BESSELJ(1)')).toBe('#VALUE!'); // faltan argumentos
  expect(show('=BESSELJ(1,"x")')).toBe('#VALUE!');
});

test('BESSELK a ~1e-15: contra la serie A&S 9.6.11 en precisión arbitraria (50 cifras)', () => {
  // Valores calculados con la serie en `decimal` de 50 cifras (scripts del PR): la
  // aproximación polinómica de LibreOffice se queda a ~3e-9 en el primero.
  const casos: [number, number, number][] = [
    [1.5, 1, 0.2773878004568438],
    [1, 0, 0.42102443824070834],
    [1, 1, 0.60190723019723458],
    [2, 2, 0.25375975456605587],
    [0.1, 0, 2.4270690247020168],
    [0.5, 3, 62.057909529930257],
    [5, 1, 0.0040446134454521646],
    [10, 2, 2.150981700693277e-5],
    [2.5, 0, 0.062347553200366189],
    [4, 5, 0.15434254872599718],
  ];
  for (const [x, n, k] of casos) {
    expect(Math.abs(num(`=BESSELK(${x},${n})`) / k - 1)).toBeLessThan(1e-13);
  }
});
