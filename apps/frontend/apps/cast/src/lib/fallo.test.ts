import { expect, test } from 'bun:test';
import { FalloDelServicio, tipoDeFallo } from './fallo.js';

test('sin conexión manda sobre cualquier otra cosa', () => {
  expect(tipoDeFallo(new FalloDelServicio(500), false)).toBe('offline');
  expect(tipoDeFallo(new TypeError('Failed to fetch'))).toBe('offline');
});

test('no existe y no puedes verlo se pintan igual', () => {
  expect(tipoDeFallo(new FalloDelServicio(404))).toBe('sinPermiso');
  expect(tipoDeFallo(new FalloDelServicio(403))).toBe('sinPermiso');
});

test('lo demás es un error del servicio', () => {
  expect(tipoDeFallo(new FalloDelServicio(503))).toBe('error');
  expect(tipoDeFallo(new Error('x'))).toBe('error');
});
