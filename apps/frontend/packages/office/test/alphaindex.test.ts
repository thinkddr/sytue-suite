/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las ayudas PURAS del índice alfabético:
 *  - parseIndexTerm: «término» / «término:subtérmino», recortes y caídas a null.
 *  - buildIndex: agrupa por término y subtérmino, ordena alfabéticamente (acentos
 *    y mayúsculas plegadas), conserva el orden de aparición de las referencias,
 *    dedup de refs repetidas y descarte de términos vacíos.
 *  - buildIndexHtml: estructura (términos + subentradas + anclas #idx-N) y escapado
 *    de los términos del usuario (anti-XSS).
 * Sin DOM (bun corre sin navegador). El pegamento con el folio vive en
 * ribbon/tabs/Referencias.tsx. Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/alphaindex.test.ts
 */
import { expect, test } from 'bun:test';
import { buildIndex, buildIndexHtml, parseIndexTerm } from '../src/editor/alphaindex';
import type { IndexEntry } from '../src/editor/alphaindex';

test('parseIndexTerm: término solo, con subtérmino y recortes', () => {
  expect(parseIndexTerm('Perros')).toEqual({ term: 'Perros', sub: '' });
  expect(parseIndexTerm('  Animales : Perros ')).toEqual({ term: 'Animales', sub: 'Perros' });
  // El PRIMER dos puntos separa; los siguientes quedan en el subtérmino.
  expect(parseIndexTerm('A:B:C')).toEqual({ term: 'A', sub: 'B:C' });
  // Subtérmino vacío permitido (equivale a entrada de primer nivel).
  expect(parseIndexTerm('Solo:')).toEqual({ term: 'Solo', sub: '' });
});

test('parseIndexTerm: sin término principal → null', () => {
  expect(parseIndexTerm('')).toBeNull();
  expect(parseIndexTerm('   ')).toBeNull();
  expect(parseIndexTerm(':huérfano')).toBeNull();
});

test('buildIndex: ordena términos alfabéticamente (acentos/mayúsculas plegados)', () => {
  const entries: IndexEntry[] = [
    { term: 'zeta', sub: '', ref: '1' },
    { term: 'Álvarez', sub: '', ref: '2' },
    { term: 'banana', sub: '', ref: '3' },
    { term: 'Ábaco', sub: '', ref: '4' },
  ];
  expect(buildIndex(entries).map((g) => g.term)).toEqual(['Ábaco', 'Álvarez', 'banana', 'zeta']);
});

test('buildIndex: agrupa refs por término en orden de aparición y las dedup', () => {
  const entries: IndexEntry[] = [
    { term: 'Sytue', sub: '', ref: '1' },
    { term: 'otro', sub: '', ref: '2' },
    { term: 'Sytue', sub: '', ref: '3' },
    { term: 'Sytue', sub: '', ref: '3' }, // duplicado exacto: no se repite
  ];
  const groups = buildIndex(entries);
  expect(groups.map((g) => g.term)).toEqual(['otro', 'Sytue']); // «otro» (o) antes que «Sytue» (s)
  expect(groups.find((g) => g.term === 'Sytue')?.refs).toEqual(['1', '3']); // dedup del '3'
});

test('buildIndex: separa refs directas de subentradas y ordena las subentradas', () => {
  const entries: IndexEntry[] = [
    { term: 'Animales', sub: '', ref: '1' },
    { term: 'Animales', sub: 'Perros', ref: '2' },
    { term: 'Animales', sub: 'Gatos', ref: '3' },
    { term: 'Animales', sub: 'Perros', ref: '5' },
  ];
  const [animales] = buildIndex(entries);
  expect(animales?.refs).toEqual(['1']); // solo la aparición sin subentrada
  expect(animales?.subs.map((s) => s.sub)).toEqual(['Gatos', 'Perros']); // ordenadas
  expect(animales?.subs.find((s) => s.sub === 'Perros')?.refs).toEqual(['2', '5']);
});

test('buildIndex: descarta entradas con término vacío', () => {
  const entries: IndexEntry[] = [
    { term: '  ', sub: '', ref: '1' },
    { term: 'Real', sub: '', ref: '2' },
  ];
  expect(buildIndex(entries).map((g) => g.term)).toEqual(['Real']);
});

test('buildIndexHtml: estructura con anclas #idx-N y sublista', () => {
  const html = buildIndexHtml([
    { term: 'Animales', sub: '', ref: '1' },
    { term: 'Animales', sub: 'Perros', ref: '2' },
  ]);
  expect(html).toContain('<li data-index-term>Animales <a href="#idx-1">1</a>');
  expect(html).toContain('<li data-index-sub>Perros <a href="#idx-2">2</a></li>');
});

test('buildIndexHtml: sin entradas → cadena vacía', () => {
  expect(buildIndexHtml([])).toBe('');
  expect(buildIndexHtml([{ term: '   ', sub: '', ref: '1' }])).toBe('');
});

test('buildIndexHtml: escapa términos y subtérminos del usuario (anti-XSS)', () => {
  const html = buildIndexHtml([{ term: '<img src=x onerror=alert(1)>', sub: 'A & B', ref: '1' }]);
  expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;');
  expect(html).toContain('A &amp; B');
  expect(html).not.toContain('<img');
});
