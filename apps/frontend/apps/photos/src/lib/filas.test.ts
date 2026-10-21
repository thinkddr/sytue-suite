/**
 * Tests de la proporción de una tesela en las filas justificadas. Ejecutar: `bun test`.
 *
 * VISTOS EN ROJO intercambiando los dos topes en `proporcion()`: caen cuatro de
 * los cinco (todo queda acotado a 9:20); solo se salva el de «sin dimensiones».
 */
import { expect, test } from 'bun:test';
import { PROPORCION_MAX, PROPORCION_MIN, estiloDeTesela, proporcion } from './filas.js';

test('una foto corriente conserva su proporción, apaisada o vertical', () => {
  expect(proporcion({ width: 4000, height: 3000 })).toBeCloseTo(4 / 3, 6);
  expect(proporcion({ width: 3000, height: 4000 })).toBeCloseTo(3 / 4, 6);
  expect(proporcion({ width: 2000, height: 2000 })).toBe(1);
});

// Lo que no puede recortarse: la captura de un móvil y el 21:9.
test('la captura de un móvil y el 21:9 caben enteros', () => {
  expect(proporcion({ width: 1170, height: 2532 })).toBeCloseTo(1170 / 2532, 6);
  expect(proporcion({ width: 2560, height: 1080 })).toBeCloseTo(2560 / 1080, 6);
});

test('una panorámica se acota por arriba y una tira vertical por abajo', () => {
  expect(proporcion({ width: 9000, height: 2250 })).toBe(PROPORCION_MAX);
  expect(proporcion({ width: 900, height: 3600 })).toBe(PROPORCION_MIN);
});

// El fallo silencioso: 0/0 es NaN, y `aspect-ratio: NaN` deja la tesela sin alto.
test('sin dimensiones usables es un cuadrado', () => {
  for (const [width, height] of [
    [0, 0],
    [4000, 0],
    [0, 3000],
    [-4, 3],
    [Number.NaN, 3],
  ] as const) {
    expect(proporcion({ width, height })).toBe(1);
  }
  expect(proporcion({} as { width: number; height: number })).toBe(1);
});

test('la tesela lleva su proporción en --r, que es lo que lee .filas', () => {
  expect(estiloDeTesela({ width: 6000, height: 4000 })).toEqual({ '--r': 1.5 });
  expect(estiloDeTesela({ width: 0, height: 0 })).toEqual({ '--r': 1 });
});
