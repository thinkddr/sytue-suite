/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { cadena, fechaHora, lista, objetoDe } from './json.js';

test('un JSON pelado se lee', () => {
  expect(objetoDe('{"a":1}')).toEqual({ a: 1 });
});

// Los modelos envuelven en ```json aunque se les pida que no.
test('se le quita la valla de código', () => {
  expect(objetoDe('```json\n{"a":1}\n```')).toEqual({ a: 1 });
});

test('sobrevive a la prosa de alrededor', () => {
  expect(objetoDe('Aquí tienes el evento:\n{"a":1}\nEspero que sirva.')).toEqual({ a: 1 });
});

// Nunca lanza: quien llama ya tiene un camino para «el asistente no supo».
test('lo que no es un objeto devuelve null', () => {
  expect(objetoDe('no soy json')).toBeNull();
  expect(objetoDe('[1,2,3]')).toBeNull();
  expect(objetoDe('')).toBeNull();
  expect(objetoDe('null')).toBeNull();
});

test('cadena exige texto no vacío y recorta', () => {
  expect(cadena({ t: '  hola  ' }, 't')).toBe('hola');
  expect(cadena({ t: '   ' }, 't')).toBeUndefined();
  expect(cadena({ t: 42 }, 't')).toBeUndefined();
  expect(cadena({}, 't')).toBeUndefined();
  expect(cadena({ t: 'x'.repeat(600) }, 't', 10)).toHaveLength(10);
});

// Se exige ISO: adivinar si «12/03» es marzo o diciembre por el usuario es
// equivocarse en una reunión, que no se perdona igual que en un párrafo.
test('fechaHora solo acepta lo que el navegador sabe leer', () => {
  expect(fechaHora({ d: '2026-03-12T14:00:00Z' }, 'd')).toBe('2026-03-12T14:00:00.000Z');
  expect(fechaHora({ d: '2026-03-12T14:00' }, 'd')).toBe('2026-03-12T14:00:00.000Z');
  // `new Date` acepta esto y devuelve el 1 de febrero de 2001: si nos fiáramos
  // de él, una frase sin resolver acabaría siendo una cita en la agenda.
  expect(fechaHora({ d: 'el jueves a las 2' }, 'd')).toBeUndefined();
  expect(fechaHora({ d: '12/03/2026' }, 'd')).toBeUndefined();
  expect(fechaHora({ d: '2026-13-45T99:99' }, 'd')).toBeUndefined();
  expect(fechaHora({ d: '' }, 'd')).toBeUndefined();
});

test('la lista tolera una cadena suelta y quita duplicados', () => {
  expect(lista({ e: ['a@b.es', ' a@b.es ', '', 'c@d.es'] }, 'e')).toEqual(['a@b.es', 'c@d.es']);
  expect(lista({ e: 'solo@uno.es' }, 'e')).toEqual(['solo@uno.es']);
  expect(lista({ e: 42 }, 'e')).toEqual([]);
});
