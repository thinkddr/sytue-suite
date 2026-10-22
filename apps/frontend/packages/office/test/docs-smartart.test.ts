/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del SmartArt del EDITOR DE DOCUMENTOS (src/editor/smartart.ts):
 * (de)serializado de los elementos (data-items), saneado de disposición y
 * horneado del bloque <figure> / SVG (escape anti-XSS, atributos data-*, reuso
 * de la geometría). No cubre la geometría pura en sí (ya la valida
 * slides-smartart.test.ts): aquí solo el serializado/parseo y el volcado.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import type { SmartArtItem } from '../src/editor/types';
import {
  DEFAULT_SMARTART_ITEMS,
  parseSmartArtItems,
  sanitizeSmartArtLayout,
  serializeSmartArtItems,
  smartArtFigureHtml,
  smartArtSvg,
} from '../src/editor/smartart';

/* ---------- serializado / parseo de elementos ---------- */

test('serializar → parsear conserva texto y sangría (round-trip)', () => {
  const items: SmartArtItem[] = [
    { text: 'Raíz' },
    { text: 'Hijo', level: 1 },
    { text: 'Nieto', level: 2 },
  ];
  expect(parseSmartArtItems(serializeSmartArtItems(items))).toEqual(items);
});

test('serializar omite level cuando es 0/ausente (JSON mínimo)', () => {
  const json = serializeSmartArtItems([{ text: 'A' }, { text: 'B', level: 0 }]);
  expect(json).toBe('[{"text":"A"},{"text":"B"}]');
  expect(JSON.parse(json)[0].level).toBeUndefined();
});

test('serializar normaliza level a entero ≥1 y castea texto', () => {
  const json = serializeSmartArtItems([
    { text: 'x', level: 2.9 },
    { text: 'y', level: -3 },
  ]);
  expect(parseSmartArtItems(json)).toEqual([{ text: 'x', level: 2 }, { text: 'y' }]);
});

test('parsear es tolerante: JSON inválido o no-array → []', () => {
  expect(parseSmartArtItems('no es json')).toEqual([]);
  expect(parseSmartArtItems('{"a":1}')).toEqual([]);
  expect(parseSmartArtItems('42')).toEqual([]);
  expect(parseSmartArtItems('')).toEqual([]);
  expect(parseSmartArtItems(null)).toEqual([]);
});

test('parsear tolera entradas incompletas: texto ausente → "", level saneado, extras fuera', () => {
  const raw = JSON.stringify([{ level: 1 }, { text: 'ok', level: '3', foo: 9 }, null, 'x']);
  expect(parseSmartArtItems(raw)).toEqual([
    { text: '', level: 1 },
    { text: 'ok', level: 3 },
    { text: '' },
    { text: '' },
  ]);
});

test('sanitizeSmartArtLayout: válida pasa, desconocida/nula → list', () => {
  expect(sanitizeSmartArtLayout('cycle')).toBe('cycle');
  expect(sanitizeSmartArtLayout('hierarchy')).toBe('hierarchy');
  expect(sanitizeSmartArtLayout('inventada')).toBe('list');
  expect(sanitizeSmartArtLayout(null)).toBe('list');
});

test('los elementos por defecto de cada disposición round-tripean', () => {
  for (const layout of ['list', 'process', 'cycle', 'hierarchy'] as const) {
    const items = DEFAULT_SMARTART_ITEMS[layout];
    expect(items.length).toBeGreaterThan(0);
    expect(parseSmartArtItems(serializeSmartArtItems(items))).toEqual(items);
  }
});

/* ---------- horneado del bloque figure / SVG ---------- */

test('la figura lleva data-* (fuente de verdad) + SVG + contenteditable=false', () => {
  const items: SmartArtItem[] = [{ text: 'Uno' }, { text: 'Dos' }];
  const html = smartArtFigureHtml('process', items);
  expect(html).toContain('data-smartart');
  expect(html).toContain('data-layout="process"');
  expect(html).toContain('contenteditable="false"');
  expect(html).toContain('<svg');
  // El data-items horneado (escapado) decodifica a los mismos elementos.
  const decoded = html
    .match(/data-items="([^"]*)"/)![1]!
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');
  expect(parseSmartArtItems(decoded)).toEqual(items);
});

test('escapa el texto del usuario al hornear (anti-XSS)', () => {
  const html = smartArtFigureHtml('list', [
    { text: '<script>alert(1)</script>' },
    { text: 'a"b&c' },
  ]);
  expect(html).not.toContain('<script>');
  expect(html).toContain('&lt;script&gt;');
  // El atributo tampoco rompe con comillas/ampersand del texto.
  expect(html).toContain('&amp;');
});

test('SVG vacío muestra aviso; con elementos pinta todos los textos (reuso de geometría)', () => {
  expect(smartArtSvg('list', [])).toContain('Empty diagram');
  const items: SmartArtItem[] = [{ text: 'Alfa' }, { text: 'Beta' }, { text: 'Gamma' }];
  // Cambiar de disposición conserva los mismos elementos (mismos textos pintados).
  for (const layout of ['list', 'process', 'cycle', 'hierarchy'] as const) {
    const svg = smartArtSvg(layout, items);
    expect(svg.startsWith('<svg')).toBe(true);
    for (const it of items) expect(svg).toContain(it.text);
  }
});

test('SVG de proceso lleva flechas; de jerarquía, codos', () => {
  const items: SmartArtItem[] = [{ text: 'A' }, { text: 'B', level: 1 }];
  // Jerarquía: conector en codo (path con V…H…V), sin punta de flecha en <line>.
  expect(smartArtSvg('hierarchy', items)).toMatch(/<path d="M[\d.]+ [\d.]+V[\d.]+H/);
  // Proceso: flecha recta (line + path de punta dentro de un grupo de trazo).
  expect(smartArtSvg('process', [{ text: 'A' }, { text: 'B' }])).toContain('<line');
});
