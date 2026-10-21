/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las ayudas PURAS de bibliografía y citas:
 *  - authorSurname / citationText: apellido y cita en texto «(Autor, año)»,
 *    con caídas s.f./Anónimo.
 *  - apaEntry: entrada APA básica por tipo (cursiva) con campos ESCAPADOS (XSS).
 *  - citationHtml: marcador de cita (id + texto escapados).
 *  - buildBibliographyList: filtra a las CITADAS, dedup, ordena por autor/título,
 *    ignora ids colgantes y escapa.
 * Sin DOM (bun corre sin navegador). El pegamento con el folio vive en
 * ribbon/tabs/Bibliografia.tsx. Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/bibliography.test.ts
 */
import { expect, test } from 'bun:test';
import {
  apaEntry,
  authorSurname,
  buildBibliographyList,
  citationHtml,
  citationText,
} from '../src/editor/bibliography';
import type { Source } from '../src/editor/bibliography';

const libro = (over: Partial<Source> = {}): Source => ({
  id: 'src-1',
  type: 'libro',
  author: 'Delgado, Daniel',
  title: 'Suite soberana',
  year: '2026',
  publisher: 'Editorial Sytue',
  ...over,
});

test('authorSurname: apellido antes de la coma; sin coma, el autor entero', () => {
  expect(authorSurname('Delgado, Daniel')).toBe('Delgado');
  expect(authorSurname('García Márquez, Gabriel')).toBe('García Márquez');
  expect(authorSurname('  Naciones Unidas ')).toBe('Naciones Unidas');
  expect(authorSurname('')).toBe('');
});

test('citationText: «(Apellido, año)» con caídas s.f./título/Anónimo', () => {
  expect(citationText(libro())).toBe('(Delgado, 2026)');
  expect(citationText(libro({ year: '' }))).toBe('(Delgado, s.f.)');
  expect(citationText(libro({ author: '', title: 'Informe anual' }))).toBe('(Informe anual, 2026)');
  expect(citationText(libro({ author: '', title: '', year: '' }))).toBe('(Anónimo, s.f.)');
});

test('apaEntry: libro con título en cursiva y punto final por segmento', () => {
  expect(apaEntry(libro())).toBe('Delgado, Daniel (2026). <i>Suite soberana</i>. Editorial Sytue.');
});

test('apaEntry: artículo pone la REVISTA en cursiva, no el título', () => {
  const art = libro({ type: 'articulo', title: 'Sobre la soberanía', publisher: 'Revista X' });
  expect(apaEntry(art)).toBe('Delgado, Daniel (2026). Sobre la soberanía. <i>Revista X</i>.');
});

test('apaEntry: web añade la URL al final; segmentos vacíos se omiten; año → s.f.', () => {
  const web = libro({
    type: 'web',
    author: '',
    publisher: 'Ministerio',
    year: '',
    url: 'https://ejemplo.es/doc',
  });
  expect(apaEntry(web)).toBe('(s.f.). <i>Suite soberana</i>. Ministerio. https://ejemplo.es/doc');
});

test('apaEntry: escapa los campos del usuario (anti-XSS)', () => {
  const xss = libro({ title: '<img src=x onerror=alert(1)>', author: 'A & B' });
  const out = apaEntry(xss);
  expect(out).toContain('A &amp; B (2026).');
  expect(out).toContain('<i>&lt;img src=x onerror=alert(1)&gt;</i>.');
  expect(out).not.toContain('<img');
});

test('citationHtml: marcador con id y texto escapados', () => {
  expect(citationHtml(libro({ id: 'src-7', author: 'Ada', year: '1852' }))).toBe(
    '<span data-citation="src-7" contenteditable="false">(Ada, 1852)</span>',
  );
});

test('buildBibliographyList: solo citadas, ordenadas por apellido, ids colgantes fuera', () => {
  const sources: Source[] = [
    libro({ id: 'a', author: 'Zúñiga, Z.', title: 'Último' }),
    libro({ id: 'b', author: 'Álvarez, Á.', title: 'Primero' }),
    libro({ id: 'c', author: 'Nadie, N.', title: 'No citado' }),
  ];
  // Se cita b y a (con un id colgante «x» y un duplicado de «a»): sale a+b, ordenadas.
  const html = buildBibliographyList(sources, ['x', 'a', 'b', 'a']);
  const first = html.indexOf('Álvarez');
  const second = html.indexOf('Zúñiga');
  expect(first).toBeGreaterThanOrEqual(0);
  expect(second).toBeGreaterThan(first); // Álvarez antes que Zúñiga
  expect(html).not.toContain('No citado'); // c no citado
  expect(html.match(/data-bib-entry/g)?.length).toBe(2); // dedup: «a» una sola vez
});

test('buildBibliographyList: sin fuentes citadas → cadena vacía', () => {
  expect(buildBibliographyList([libro()], [])).toBe('');
  expect(buildBibliographyList([], ['a'])).toBe('');
});
