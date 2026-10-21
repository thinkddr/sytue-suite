/**
 * Un atajo que dispara donde no debe se come caracteres que alguien escribió;
 * uno que no dispara es solo un atajo que no existe. Por eso los negativos
 * importan más que los positivos.
 */
import { expect, test } from 'bun:test';
import { atajoMarkdown } from '../client/markdown.js';

test('guion, asterisco o más y espacio abren una lista', () => {
  for (const m of ['-', '*', '+'])
    expect(atajoMarkdown(m, ' ')).toEqual({ cmd: 'insertUnorderedList', borrar: 1 });
});

test('«1.» o «1)» y espacio abren una lista numerada', () => {
  expect(atajoMarkdown('1.', ' ')).toEqual({ cmd: 'insertOrderedList', borrar: 2 });
  expect(atajoMarkdown('1)', ' ')).toEqual({ cmd: 'insertOrderedList', borrar: 2 });
  expect(atajoMarkdown('2.', ' ')).toBeNull();
});

test('de una a cinco almohadillas son títulos; seis, no (el editor llega a h5)', () => {
  expect(atajoMarkdown('#', ' ')).toEqual({ cmd: 'formatBlock', valor: '<h1>', borrar: 1 });
  expect(atajoMarkdown('#####', ' ')).toEqual({ cmd: 'formatBlock', valor: '<h5>', borrar: 5 });
  expect(atajoMarkdown('######', ' ')).toBeNull();
});

test('cita y bloque de código', () => {
  expect(atajoMarkdown('>', ' ')).toEqual({ cmd: 'formatBlock', valor: '<blockquote>', borrar: 1 });
  expect(atajoMarkdown('```', ' ')).toEqual({ cmd: 'formatBlock', valor: '<pre>', borrar: 3 });
});

test('cuadros de tareas y listas de completado con espacio', () => {
  expect(atajoMarkdown('[]', ' ')?.cmd).toBe('insertHTML');
  expect(atajoMarkdown('[ ]', ' ')?.cmd).toBe('insertHTML');
  expect(atajoMarkdown('- [ ]', ' ')?.cmd).toBe('insertHTML');
});

test('tres guiones o asteriscos con Intro son una regla; con espacio, nada', () => {
  expect(atajoMarkdown('---', 'Enter')).toEqual({ cmd: 'insertHorizontalRule', borrar: 3 });
  expect(atajoMarkdown('*****', 'Enter')).toEqual({ cmd: 'insertHorizontalRule', borrar: 5 });
  expect(atajoMarkdown('--', 'Enter')).toBeNull();
  expect(atajoMarkdown('---', ' ')).toBeNull();
});

test('un marcador que no abre la línea no dispara nada', () => {
  expect(atajoMarkdown('hola -', ' ')).toBeNull();
  expect(atajoMarkdown('a#', ' ')).toBeNull();
  expect(atajoMarkdown('', ' ')).toBeNull();
  expect(atajoMarkdown('- ', ' ')).toBeNull();
});
