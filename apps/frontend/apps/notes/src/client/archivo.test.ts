import { expect, test } from 'bun:test';
import type { Notebook } from '@sytue/api-clients';
import { enArbol, raicesArchivadas } from './archivo.js';

const nb = (id: string, extra: Partial<Notebook> = {}): Notebook =>
  ({ id, title: id, layout: 'list', ...extra }) as Notebook;

const AYER = '2026-09-30T10:00:00Z';
const cuadernos = [
  nb('activo'),
  nb('viejo', { archived_at: AYER }),
  nb('hijo', { parent_id: 'viejo', archived_at: AYER }),
  nb('suelto', { parent_id: 'activo', archived_at: AYER }),
];

test('el árbol no enseña lo archivado', () => {
  expect(enArbol(cuadernos).map((c) => c.id)).toEqual(['activo']);
});

test('el Archivo enseña la cabeza de cada rama, no sus hijos', () => {
  expect(raicesArchivadas(cuadernos).map((c) => c.id)).toEqual(['viejo', 'suelto']);
});
