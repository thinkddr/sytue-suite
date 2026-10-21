import { expect, test } from 'bun:test';
import { conProporcion } from './proporcion';

const lienzo = { ancho: 1000, alto: 500 };
const casi = (a: number, b: number) => Math.abs(a - b) < 1e-6;

test('un cuadrado es cuadrado EN PÍXELES, no en fracciones', () => {
  const m = conProporcion({ x: 0, y: 0, w: 0.5, h: 0.5 }, 1, lienzo, { x: 0, y: 0 });
  expect(casi(m.w * lienzo.ancho, m.h * lienzo.alto)).toBe(true);
  expect(casi(m.w, 0.25)).toBe(true);
  expect(casi(m.h, 0.5)).toBe(true);
});

test('16:9 sale con esa proporción en píxeles', () => {
  const m = conProporcion({ x: 0, y: 0, w: 0.8, h: 0.8 }, 16 / 9, lienzo, { x: 0, y: 0 });
  expect(casi((m.w * lienzo.ancho) / (m.h * lienzo.alto), 16 / 9)).toBe(true);
});

test('no se sale del lienzo, y encoge manteniendo la proporción', () => {
  const m = conProporcion({ x: 0.9, y: 0, w: 0.3, h: 0.9 }, 1, lienzo, { x: 0.9, y: 0 });
  expect(m.x + m.w).toBeLessThanOrEqual(1 + 1e-9);
  expect(m.y + m.h).toBeLessThanOrEqual(1 + 1e-9);
  expect(casi((m.w * lienzo.ancho) / (m.h * lienzo.alto), 1)).toBe(true);
});

test('arrastrando hacia arriba y a la izquierda el marco queda pegado al ancla', () => {
  const m = conProporcion({ x: 0.2, y: 0.2, w: 0.6, h: 0.6 }, 1, lienzo, { x: 0.8, y: 0.8 });
  expect(casi(m.x + m.w, 0.8)).toBe(true);
  expect(casi(m.y + m.h, 0.8)).toBe(true);
});

test('un lienzo sin medidas o una proporción absurda dejan el marco como estaba', () => {
  const m = { x: 0.1, y: 0.1, w: 0.2, h: 0.2 };
  expect(conProporcion(m, 1, { ancho: 0, alto: 0 }, { x: 0.1, y: 0.1 })).toEqual(m);
  expect(conProporcion(m, 0, lienzo, { x: 0.1, y: 0.1 })).toEqual(m);
});
