import { expect, test } from 'bun:test';
import {
  autorDeActividad,
  filtrarActividad,
  lineaDeActividad,
  personasDeActividad,
  queToco,
} from './actividad.js';

test('queToco: rutas de parche sin los tramos estructurales; la clave si no hay rutas', () => {
  expect(
    queToco({ kind: 'set', key: 'doc:sheet', paths: ['sheets/h1/cells/B2', 'sheets/h1/cells/B3'] }),
  ).toBe('B2, B3');
  expect(queToco({ kind: 'set', key: 'doc:slides', paths: ['slides/s1/boxes/b1/html'] })).toBe(
    'boxes·html',
  );
  expect(queToco({ kind: 'set', key: 'doc:sheet' })).toBe('doc:sheet');
  expect(queToco({ kind: 'set', paths: ['sheets/h1/freeze'] })).toBe('freeze');
  expect(queToco({ kind: 'insert' })).toBe('insert');
  expect(queToco({ kind: 'set', paths: ['a', 'b', 'c', 'd'] })).toBe('a, b, c, …');
});

test('lineaDeActividad', () => {
  const e = {
    seq_desde: 1,
    seq_hasta: 3,
    author_sub: 'user-ada-123456',
    kind: 'set',
    count: 3,
    at: '',
    paths: ['sheets/h1/cells/A1'],
  };
  expect(lineaDeActividad(e, '10:30')).toBe('user-ada — A1 ×3 · 10:30');
});

const entrada = (author_sub: string, kind: string, seq: number) => ({
  seq_desde: seq,
  seq_hasta: seq,
  author_sub,
  kind,
  count: 1,
  at: '',
});

test('lineaDeActividad y autorDeActividad usan el nombre si lo hay', () => {
  const e = entrada('user-ada-123456', 'set', 1);
  expect(lineaDeActividad(e, '10:30', { 'user-ada-123456': 'Ada Lovelace' })).toBe(
    'Ada Lovelace — set ×1 · 10:30',
  );
  expect(autorDeActividad('user-ada-123456', {})).toBe('user-ada');
});

test('filtrarActividad: por persona, por tipo y por las dos; vacío = todo', () => {
  const todas = [entrada('a', 'set', 1), entrada('b', 'set', 2), entrada('a', 'insert', 3)];
  expect(filtrarActividad(todas, '', '')).toHaveLength(3);
  expect(filtrarActividad(todas, 'a', '').map((e) => e.seq_desde)).toEqual([1, 3]);
  expect(filtrarActividad(todas, '', 'set').map((e) => e.seq_desde)).toEqual([1, 2]);
  expect(filtrarActividad(todas, 'a', 'set').map((e) => e.seq_desde)).toEqual([1]);
  expect(filtrarActividad(todas, 'zz', '')).toEqual([]);
  expect(personasDeActividad(todas)).toEqual(['a', 'b']);
});
