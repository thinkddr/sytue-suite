/*
 * «MÍOS / COMPARTIDOS CONMIGO» Y ORDEN POR TÍTULO EN LA PORTADA.
 *
 * Los resuelve el servicio (`?scope=`, `?sort=title`): con paginación keyset, filtrar
 * u ordenar la página traída mentiría en cuanto hubiera dos. La primera página y las
 * siguientes salen de la misma función, o la segunda seguiría otro orden. Sin red, la
 * caché local aplica el mismo alcance y el mismo orden.
 */
import { expect, test } from 'bun:test';
import type { DocumentMeta } from '@sytue/api-clients';
import { parametrosLista, vistaSinRed } from './DocList.js';

const base = { q: '', starred: false, scope: '' as const, sort: 'updated' as const };

test('la consulta lleva el alcance y el orden, también con cursor', () => {
  const p = parametrosLista({ ...base, scope: 'shared', sort: 'title' }, 'document', 't~QQ~x');
  expect(p.get('scope')).toBe('shared');
  expect(p.get('sort')).toBe('title');
  expect(p.get('cursor')).toBe('t~QQ~x');
  expect(p.get('kind')).toBe('document');
});

test('sin filtros, la consulta es la de siempre', () => {
  const p = parametrosLista(base, '');
  expect(p.has('scope')).toBe(false);
  expect(p.has('sort')).toBe(false);
});

const doc = (id: string, title: string, ownerId: string): DocumentMeta => ({
  id,
  title,
  ownerId,
  kind: 'document',
  createdAt: '',
  updatedAt: '',
  version: 0,
  starred: false,
});

test('sin red, «Míos» y «Compartidos conmigo» se separan por dueño y el título ordena sin acentos', () => {
  const docs = [doc('1', 'Diario', 'yo'), doc('2', 'balance', 'otra'), doc('3', 'Álbum', 'yo')];
  expect(vistaSinRed(docs, 'yo', 'mine', 'updated').map((d) => d.id)).toEqual(['1', '3']);
  expect(vistaSinRed(docs, 'yo', 'shared', 'updated').map((d) => d.id)).toEqual(['2']);
  expect(vistaSinRed(docs, 'yo', '', 'title').map((d) => d.title)).toEqual([
    'Álbum',
    'balance',
    'Diario',
  ]);
});
