/**
 * Markdown en la línea al teclear la marca que cierra. Un atajo que dispara donde no
 * debe se come lo que alguien escribió: los negativos importan más que los positivos.
 */
import { expect, test } from 'bun:test';
import { atajoEnLinea } from './autoformato.js';

test('las cuatro marcas convierten al cerrarse', () => {
  expect(atajoEnLinea('dijo **sí**')).toEqual({ desde: 5, etiqueta: 'b', texto: 'sí' });
  expect(atajoEnLinea('*vale*')).toEqual({ desde: 0, etiqueta: 'i', texto: 'vale' });
  expect(atajoEnLinea('es _así_')).toEqual({ desde: 3, etiqueta: 'i', texto: 'así' });
  expect(atajoEnLinea('usa `npm i`')).toEqual({ desde: 4, etiqueta: 'code', texto: 'npm i' });
  expect(atajoEnLinea('~~no~~')).toEqual({ desde: 0, etiqueta: 's', texto: 'no' });
  // El espacio de final de línea que pone el navegador es un no separable.
  expect(atajoEnLinea('a *b*')).toEqual({ desde: 2, etiqueta: 'i', texto: 'b' });
});

test('la primera marca de cierre de una doble no convierte en cursiva', () => {
  expect(atajoEnLinea('**hola*')).toBeNull();
  expect(atajoEnLinea('**')).toBeNull();
  expect(atajoEnLinea('*')).toBeNull();
});

test('dentro de una palabra, de un número o de un nombre, nada', () => {
  expect(atajoEnLinea('2*3*')).toBeNull();
  expect(atajoEnLinea('snake_case_')).toBeNull();
  expect(atajoEnLinea('a__init__')).toBeNull();
  expect(atajoEnLinea('x`y`')).toBeNull();
});

test('lo de dentro no empieza ni acaba en blanco, ni está vacío', () => {
  expect(atajoEnLinea('* no*')).toBeNull();
  expect(atajoEnLinea('*no *')).toBeNull();
  expect(atajoEnLinea('``')).toBeNull();
  expect(atajoEnLinea('~~ ~~')).toBeNull();
});
