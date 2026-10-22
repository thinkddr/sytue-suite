/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { esLegible, juntar, MAX_PAGINAS, paginasALeer } from './ocr.js';

// Se empieza por donde está mirando quien pregunta, no por la portada.
test('lee desde la página actual hacia delante', () => {
  expect(paginasALeer(100, 40, 5)).toEqual([40, 41, 42, 43, 44]);
});

test('cerca del final completa hacia atrás sin pasarse del tope', () => {
  expect(paginasALeer(10, 9, 5)).toEqual([5, 6, 7, 8, 9, 10].slice(-5));
  expect(paginasALeer(10, 9, 5)).toHaveLength(5);
});

test('un documento corto se lee entero y una sola vez', () => {
  expect(paginasALeer(3, 2)).toEqual([1, 2, 3]);
});

test('los bordes no se salen del documento', () => {
  expect(paginasALeer(0, 1)).toEqual([]);
  expect(paginasALeer(5, 99, 3)).toEqual([3, 4, 5]);
  expect(paginasALeer(5, -4, 2)).toEqual([1, 2]);
});

test('el tope por defecto acota lo que se paga', () => {
  expect(paginasALeer(500, 1)).toHaveLength(MAX_PAGINAS);
});

// La marca de página es lo que deja al asistente citar dónde está lo que dice.
test('lo leído se junta en orden y con su página', () => {
  expect(
    juntar([
      { pagina: 3, texto: ' tres ' },
      { pagina: 1, texto: 'uno' },
    ]),
  ).toBe('[página 1]\nuno\n\n[página 3]\ntres');
});

// Una página vacía le dice al modelo que ahí no hay nada, y es falso.
test('las páginas sin texto no salen', () => {
  expect(juntar([{ pagina: 1, texto: '   ' }])).toBe('');
});

test('cuatro caracteres sueltos no son un documento legible', () => {
  expect(esLegible('a b c')).toBe(false);
  expect(esLegible('x'.repeat(40))).toBe(true);
});
