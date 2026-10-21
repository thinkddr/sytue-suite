/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LAMBDA y las funciones de orden superior (MAP, REDUCE, SCAN, MAKEARRAY, BYROW, BYCOL).
 *
 * Lo que se comprueba aquí, además del cálculo, es la FORMA del resultado: BYROW devuelve
 * una columna y BYCOL una fila; REDUCE devuelve un valor y SCAN el rastro entero. Son las
 * confusiones que dan un resultado plausible con la orientación equivocada, y solo se ven
 * al derramar el array junto a los datos.
 */
import { expect, test } from 'bun:test';
import { Engine } from './engine.js';
import type { CellValue } from './values.js';

/** Evalúa en P1 y devuelve el rectángulo derramado (alto × ancho). */
function matriz(formula: string, datos: Record<string, string | number>, alto: number, ancho = 1) {
  const e = new Engine();
  for (const [ref, v] of Object.entries(datos)) e.setCell('Hoja1', ref, String(v));
  e.setCell('Hoja1', 'P1', formula);
  const out: CellValue[][] = [];
  for (let r = 0; r < alto; r += 1) {
    const fila: CellValue[] = [];
    for (let c = 0; c < ancho; c += 1) {
      fila.push(e.getValue('Hoja1', `${String.fromCharCode(80 + c)}${r + 1}`));
    }
    out.push(fila);
  }
  return out;
}

function uno(formula: string, datos: Record<string, string | number> = {}): CellValue | string {
  const e = new Engine();
  for (const [ref, v] of Object.entries(datos)) e.setCell('Hoja1', ref, String(v));
  e.setCell('Hoja1', 'P1', formula);
  const v = e.getValue('Hoja1', 'P1');
  return typeof v === 'object' && v !== null && 'code' in v ? (v as { code: string }).code : v;
}

/** Columna A con 1..4 y una fila B1:D1 con 10,20,30. */
const DATOS = { A1: 1, A2: 2, A3: 3, A4: 4, B1: 10, C1: 20, D1: 30 };

test('MAP aplica la lambda a cada elemento y conserva la forma', () => {
  const m = matriz('=MAP(A1:A4,LAMBDA(x,x*2))', DATOS, 4);
  expect(m.map((f) => f[0])).toEqual([2, 4, 6, 8]);
});

test('REDUCE devuelve UN valor y SCAN el rastro: esa es toda la diferencia', () => {
  // Sumar 1..4 desde 0. REDUCE da 10; SCAN da los parciales 1,3,6,10.
  expect(uno('=REDUCE(0,A1:A4,LAMBDA(acc,x,acc+x))', DATOS)).toBe(10);
  const s = matriz('=SCAN(0,A1:A4,LAMBDA(acc,x,acc+x))', DATOS, 4);
  expect(s.map((f) => f[0])).toEqual([1, 3, 6, 10]);
});

test('BYROW devuelve una COLUMNA y BYCOL una FILA', () => {
  // La orientación es lo que se equivoca: cada fila produce UN valor, así que el
  // resultado se derrama hacia abajo. Devolverlo tumbado da números correctos en el
  // sitio equivocado, y solo se nota al ponerlo al lado de los datos.
  const porFila = matriz('=BYROW(A1:A4,LAMBDA(f,SUM(f)))', DATOS, 4);
  expect(porFila.map((f) => f[0])).toEqual([1, 2, 3, 4]);

  const porCol = matriz('=BYCOL(B1:D1,LAMBDA(c,SUM(c)))', DATOS, 1, 3);
  expect(porCol[0]).toEqual([10, 20, 30]);
});

test('MAKEARRAY recibe índices 1-based, como los de una hoja', () => {
  // Si fueran 0-based, la primera celda saldría 0 y el usuario que escribe r*10+c
  // obtendría toda la tabla desplazada sin ningún error.
  const m = matriz('=MAKEARRAY(2,3,LAMBDA(r,c,r*10+c))', {}, 2, 3);
  expect(m).toEqual([
    [11, 12, 13],
    [21, 22, 23],
  ]);
});

test('MAKEARRAY rechaza tamaños absurdos en vez de colgar la hoja', () => {
  expect(uno('=MAKEARRAY(0,3,LAMBDA(r,c,1))')).toBe('#VALUE!');
  expect(uno('=MAKEARRAY(-1,3,LAMBDA(r,c,1))')).toBe('#VALUE!');
  // Un millón de celdas es el mismo tope que usa SEQUENCE: mejor #NUM! que un cuelgue.
  expect(uno('=MAKEARRAY(2000,2000,LAMBDA(r,c,1))')).toBe('#NUM!');
});

test('la aridad de la lambda tiene que cuadrar con lo que le pasa la función', () => {
  // MAP entrega UN elemento; una lambda de dos parámetros no encaja y decirlo es mejor
  // que ligar el segundo a nada y devolver un número.
  expect(uno('=MAP(A1:A4,LAMBDA(x,y,x+y))', DATOS)).toBe('#VALUE!');
  // Y REDUCE entrega dos (acumulado y elemento).
  expect(uno('=REDUCE(0,A1:A4,LAMBDA(x,x))', DATOS)).toBe('#VALUE!');
});

test('el segundo argumento tiene que ser un LAMBDA de verdad', () => {
  expect(uno('=MAP(A1:A4,5)', DATOS)).toBe('#VALUE!');
  expect(uno('=BYROW(A1:A4,SUM(A1:A4))', DATOS)).toBe('#VALUE!');
});

test('un LAMBDA suelto no es un valor que se pueda poner en una celda', () => {
  // No es un error de sintaxis: es que no hay nada que calcular sin quien lo aplique.
  expect(uno('=LAMBDA(x,x*2)')).toBe('#CALC!');
});

test('la lambda ve las variables de un LET que la envuelve', () => {
  // El entorno se hereda, así que componer las dos cosas funciona — que es justo para
  // lo que sirven juntas.
  expect(uno('=LET(factor,3,REDUCE(0,A1:A4,LAMBDA(acc,x,acc+x*factor)))', DATOS)).toBe(30);
});

test('un error dentro de la lambda se propaga en vez de convertirse en un número', () => {
  expect(uno('=REDUCE(0,A1:A4,LAMBDA(acc,x,acc/0))', DATOS)).toBe('#DIV/0!');
});
