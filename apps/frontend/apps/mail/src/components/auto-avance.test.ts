/**
 * Auto-avance tras archivar (D-MAIL-08): qué conversación se abre cuando la abierta
 * sale de la vista. La lista va de la más reciente a la más antigua.
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { conversacionTrasQuitar } from './autoAvance.js';

const lista = ['a', 'b', 'c', 'd'].map((threadId) => ({ threadId }));

test('por defecto se abre la siguiente, la más antigua', () => {
  expect(conversacionTrasQuitar(lista, 'b', new Set(['b']), 'older')).toBe('c');
});

test('se puede preferir la más reciente o volver a la lista', () => {
  expect(conversacionTrasQuitar(lista, 'b', new Set(['b']), 'newer')).toBe('a');
  expect(conversacionTrasQuitar(lista, 'b', new Set(['b']), 'list')).toBeNull();
});

test('si no queda ninguna en ese sentido, a la lista', () => {
  expect(conversacionTrasQuitar(lista, 'd', new Set(['d']), 'older')).toBeNull();
  expect(conversacionTrasQuitar(lista, 'a', new Set(['a']), 'newer')).toBeNull();
});

test('se salta lo que se quita a la vez', () => {
  expect(conversacionTrasQuitar(lista, 'a', new Set(['a', 'b', 'c']), 'older')).toBe('d');
});

test('si la abierta no se quita, sigue abierta; sin abierta, nada', () => {
  expect(conversacionTrasQuitar(lista, 'a', new Set(['c']), 'older')).toBe('a');
  expect(conversacionTrasQuitar(lista, null, new Set(['c']), 'older')).toBeNull();
});
