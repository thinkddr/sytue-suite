// Checks de la búsqueda de personas de otra organización (ADR 0040). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { ChatHttpError } from '../client/chatApi.js';
import { claveDeFalloDeBusqueda, pareceIdentificador } from './personas.js';

test('un correo o un @usuario se buscan en Sytue One; un nombre, en el equipo', () => {
  expect(pareceIdentificador('ana@acme.com')).toBe(true);
  expect(pareceIdentificador('@ana')).toBe(true);
  expect(pareceIdentificador('  @ana  ')).toBe(true);
  expect(pareceIdentificador('Ana Pérez')).toBe(false);
  expect(pareceIdentificador('@')).toBe(false);
  // Con espacios es una frase a medio escribir, no una dirección.
  expect(pareceIdentificador('ana @acme')).toBe(false);
});

test('el fallo distingue «no existe» de «espera un momento»', () => {
  expect(claveDeFalloDeBusqueda(new ChatHttpError(404, 'x'))).toBe('people.notFound');
  expect(claveDeFalloDeBusqueda(new ChatHttpError(429, 'x'))).toBe('people.tooMany');
  expect(claveDeFalloDeBusqueda(new ChatHttpError(400, 'x'))).toBe('people.invalid');
  expect(claveDeFalloDeBusqueda(new ChatHttpError(502, 'x'))).toBe('people.failed');
  expect(claveDeFalloDeBusqueda(new Error('red'))).toBe('people.failed');
});
