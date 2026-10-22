import { expect, test } from 'bun:test';
import type { Note } from '@sytue/api-clients';
import { esOrden, ordenarNotas } from './orden.js';

const n = (id: string, extra: Partial<Note> = {}): Note =>
  ({
    id,
    notebook_id: 'nb',
    kind: 'text',
    title: id,
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...extra,
  }) as Note;

test('las fijadas van delante en todos los órdenes', () => {
  const notas = [n('a'), n('b', { pinned: true }), n('c')];
  for (const orden of ['manual', 'title', 'updated', 'created'] as const) {
    expect(ordenarNotas(notas, orden)[0]?.id).toBe('b');
  }
});

test('por título: alfabético y las sin título al final', () => {
  const notas = [n('zeta'), n('sin', { title: '' }), n('alfa')];
  expect(ordenarNotas(notas, 'title').map((x) => x.id)).toEqual(['alfa', 'zeta', 'sin']);
});

test('por modificación y por creación: lo más reciente primero', () => {
  const notas = [
    n('vieja', { updated_at: '2026-01-01T00:00:00Z', created_at: '2020-01-01T00:00:00Z' }),
    n('nueva', { updated_at: '2026-09-09T00:00:00Z', created_at: '2019-01-01T00:00:00Z' }),
  ];
  expect(ordenarNotas(notas, 'updated').map((x) => x.id)).toEqual(['nueva', 'vieja']);
  expect(ordenarNotas(notas, 'created').map((x) => x.id)).toEqual(['vieja', 'nueva']);
});

test('«manual» no reordena más allá de las fijadas, y no muta', () => {
  const notas = [n('c'), n('a'), n('b')];
  expect(ordenarNotas(notas, 'manual').map((x) => x.id)).toEqual(['c', 'a', 'b']);
  expect(notas.map((x) => x.id)).toEqual(['c', 'a', 'b']);
});

test('esOrden filtra lo que venga de localStorage', () => {
  expect(esOrden('title')).toBe(true);
  expect(esOrden('lo-que-sea')).toBe(false);
  expect(esOrden(null)).toBe(false);
});

test('el movimiento optimista conserva el mismo orden que una recarga', () => {
  const notas = [n('a', { position: 2 }), n('b', { position: 0 }), n('c', { position: 1 })];
  expect(ordenarNotas(notas, 'manual').map((x) => x.id)).toEqual(['b', 'c', 'a']);
  expect(notas.map((x) => x.id)).toEqual(['a', 'b', 'c']);
});
