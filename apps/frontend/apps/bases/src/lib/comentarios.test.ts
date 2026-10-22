// Los hilos de un registro se agrupan sin perder comentarios y el enlace de un aviso
// solo abre ids con forma de UUID. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesComment } from '@sytue/api-clients/bases';
import { hilosDe, registroDelEnlace } from './comentarios.js';
import { puedeComentar } from './valores.js';

const c = (id: string, parent_id?: string): BasesComment => ({
  id,
  record_id: 'r',
  ...(parent_id ? { parent_id } : {}),
  author_sub: 'ana',
  body: id,
  created_at: '',
  can_delete: false,
});

test('agrupa respuestas bajo su raíz y conserva el orden', () => {
  const hilos = hilosDe([c('a'), c('b'), c('a1', 'a'), c('b1', 'b'), c('a2', 'a')]);
  expect(hilos.map((h) => h.raiz.id)).toEqual(['a', 'b']);
  expect(hilos[0]!.respuestas.map((x) => x.id)).toEqual(['a1', 'a2']);
  expect(hilos[1]!.respuestas.map((x) => x.id)).toEqual(['b1']);
});

test('una respuesta sin su raíz no se pierde: sale como hilo propio', () => {
  const hilos = hilosDe([c('x1', 'no-esta'), c('a')]);
  expect(hilos.map((h) => h.raiz.id)).toEqual(['x1', 'a']);
});

test('el enlace de un aviso solo abre una tabla y un registro con forma de UUID', () => {
  const u = '0b6a4c8e-1f2d-4e3a-9b5c-7d8e9f0a1b2c';
  expect(registroDelEnlace(`?record=${u}&table=${u}`)).toEqual({ tabla: u, registro: u });
  expect(registroDelEnlace(`?record=${u}`)).toBeNull();
  expect(registroDelEnlace(`?record=${u}&table=../x`)).toBeNull();
  expect(registroDelEnlace('')).toBeNull();
});

test('comenta el comentarista y quien escribe; el lector no', () => {
  expect(puedeComentar('commenter')).toBe(true);
  expect(puedeComentar('editor')).toBe(true);
  expect(puedeComentar('owner')).toBe(true);
  expect(puedeComentar('viewer')).toBe(false);
  expect(puedeComentar('')).toBe(false);
  expect(puedeComentar(undefined)).toBe(false);
});
