import { expect, test } from 'bun:test';
import { duracion } from './formato.js';

test('duración de reproductor, con horas solo cuando las hay', () => {
  expect(duracion(7_000)).toBe('0:07');
  expect(duracion(245_900)).toBe('4:05');
  expect(duracion(3_723_000)).toBe('1:02:03');
});

test('lo que no es una duración se pinta como cero, no como NaN', () => {
  expect(duracion(null)).toBe('0:00');
  expect(duracion(-5)).toBe('0:00');
  expect(duracion(Number.NaN)).toBe('0:00');
});
