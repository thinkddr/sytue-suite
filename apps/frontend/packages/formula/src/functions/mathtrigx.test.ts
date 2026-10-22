/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Trigonometría recíproca, inversas hiperbólicas, paridad y series.
 *
 * Lo que se prueba aquí NO es el caso feliz —`1/cos(x)` no se equivoca— sino los tres
 * sitios donde estas funciones se implementan mal: el POLO (dividir por cero no es
 * «un número muy grande»), el DOMINIO (salirse no es lo mismo que dividir por cero) y
 * el SIGNO (redondear «hacia arriba» rompe los negativos).
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

/** Evalúa «=…» en A1. Los errores se comparan por su texto («#DIV/0!»). */
function ev(formula: string): CellValue | string {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  const v = e.getValue('Hoja1', 'A1');
  return typeof v === 'object' && v !== null && 'code' in v ? (v as { code: string }).code : v;
}

const CERCA = 1e-10;

test('las recíprocas dan #DIV/0! en el polo, no un número enorme', () => {
  // 1/tan(0) es infinito. Una hoja dice #DIV/0! ahí, y esa es la diferencia entre que el
  // usuario vea el problema o que se le cuele un número absurdo en una suma.
  expect(ev('=COT(0)')).toBe('#DIV/0!');
  expect(ev('=CSC(0)')).toBe('#DIV/0!');
  expect(ev('=CSCH(0)')).toBe('#DIV/0!');
  expect(ev('=COTH(0)')).toBe('#DIV/0!');
});

test('SEC y SECH no tienen polo en 0: valen 1', () => {
  // cos(0)=1 y cosh(0)=1, así que aquí NO hay división por cero. Devolver #DIV/0! por
  // simetría con las otras cuatro sería el error opuesto.
  expect(ev('=SEC(0)')).toBe(1);
  expect(ev('=SECH(0)')).toBe(1);
});

test('salirse del dominio es #NUM!, que no es lo mismo que dividir por cero', () => {
  expect(ev('=ACOSH(0.5)')).toBe('#NUM!'); // ACOSH exige x ≥ 1
  expect(ev('=ATANH(1)')).toBe('#NUM!'); // el límite en ±1 es infinito
  expect(ev('=ATANH(2)')).toBe('#NUM!');
  expect(ev('=ACOTH(1)')).toBe('#NUM!'); // ACOTH exige |x| > 1
  expect(ev('=SQRTPI(-1)')).toBe('#NUM!');
});

test('ACOT devuelve el ángulo del cuadrante correcto para negativos', () => {
  // Implementarlo como `atan(1/x)` da -π/4 aquí, que es el error clásico: el rango de
  // ACOT es (0, π) entero, no (-π/2, π/2).
  expect(ev('=ACOT(-1)') as number).toBeCloseTo((3 * Math.PI) / 4, 10);
  expect(ev('=ACOT(1)') as number).toBeCloseTo(Math.PI / 4, 10);
  // Y en 0 es continua (π/2), no una división por cero.
  expect(ev('=ACOT(0)') as number).toBeCloseTo(Math.PI / 2, 10);
});

test('las inversas hiperbólicas dentro de dominio', () => {
  expect(ev('=ASINH(0)')).toBe(0);
  expect(ev('=ACOSH(1)')).toBe(0);
  expect(ev('=ATANH(0)')).toBe(0);
  expect(Math.abs((ev('=ACOTH(2)') as number) - 0.5493061443340549)).toBeLessThan(CERCA);
});

test('EVEN y ODD se alejan de cero, también en negativo', () => {
  // El error clásico es redondear «hacia arriba»: con eso -1.5 daría -2 por casualidad
  // pero -2.5 daría -2, que está MÁS CERCA de cero y contradice la función.
  expect(ev('=EVEN(1.5)')).toBe(2);
  expect(ev('=EVEN(-1.5)')).toBe(-2);
  expect(ev('=EVEN(-2.5)')).toBe(-4);
  expect(ev('=ODD(1.5)')).toBe(3);
  expect(ev('=ODD(-1.5)')).toBe(-3);
  expect(ev('=ODD(-2.5)')).toBe(-3);
});

test('un valor que ya tiene la paridad pedida no se mueve; ODD(3) es 3 y EVEN(3) es 4', () => {
  expect(ev('=EVEN(2)')).toBe(2);
  expect(ev('=EVEN(3)')).toBe(4);
  expect(ev('=ODD(3)')).toBe(3);
  expect(ev('=ODD(2)')).toBe(3);
});

test('el cero: EVEN(0) es 0 pero ODD(0) es 1', () => {
  // Cero es par, así que EVEN se queda. No es impar, así que ODD tiene que alejarse.
  expect(ev('=EVEN(0)')).toBe(0);
  expect(ev('=ODD(0)')).toBe(1);
});

test('COMBINA cuenta CON repetición, que es otra cosa que COMBIN', () => {
  // Tres flores de cinco especies pudiendo repetir: 35. Sin repetir serían 10.
  expect(ev('=COMBINA(5,3)')).toBe(35);
  expect(ev('=COMBIN(5,3)')).toBe(10);
  expect(ev('=COMBINA(0,0)')).toBe(1);
  expect(ev('=COMBINA(-1,2)')).toBe('#NUM!');
});

test('MULTINOMIAL no desborda donde el factorial sí lo haría', () => {
  expect(ev('=MULTINOMIAL(2,3,4)')).toBe(1260);
  // 60! ≈ 8e81, muy por encima del máximo de un double; el resultado, en cambio, cabe.
  // Calcularlo dividiendo factoriales daría Infinity o NaN.
  const r = ev('=MULTINOMIAL(20,20,20)') as number;
  expect(Number.isFinite(r)).toBe(true);
  expect(r).toBeGreaterThan(0);
});

test('SQRTPI y las recíprocas con valores conocidos', () => {
  expect(ev('=SQRTPI(4)') as number).toBeCloseTo(Math.sqrt(4 * Math.PI), 10);
  expect(ev('=SEC(0)')).toBe(1);
  expect(ev('=CSC(1)') as number).toBeCloseTo(1 / Math.sin(1), 10);
  expect(ev('=COT(1)') as number).toBeCloseTo(1 / Math.tan(1), 10);
});
