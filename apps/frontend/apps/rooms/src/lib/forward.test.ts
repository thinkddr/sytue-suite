// Checks del cuerpo del mensaje reenviado. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { forwardedBody } from './forward.js';

const LINK = 'https://rooms.sytue.eu/channels/c1?message=m9';

test('cita el original, atribuye al autor y añade el enlace canónico', () => {
  const out = forwardedBody('hola', 'Ana', LINK);
  expect(out).toBe(`Reenviado de **Ana**:\n> hola\n\n${LINK}`);
});

test('cada línea del original va citada con «> »', () => {
  const out = forwardedBody('uno\ndos', 'Ana', LINK);
  expect(out).toContain('> uno\n> dos');
});

test('un mensaje sin texto (solo adjuntos) usa un marcador', () => {
  const out = forwardedBody('   ', 'Ana', LINK);
  expect(out).toContain('> _(sin texto)_');
  expect(out).toContain(LINK);
});
