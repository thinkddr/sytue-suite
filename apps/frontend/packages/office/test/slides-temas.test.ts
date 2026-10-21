/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los temas propios de una presentación (slides/temas.ts): colores, fuente,
 * saneado en la frontera de confianza, alta/cambio/baja y contraste; y que slideVisuals
 * los usa. Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import {
  MAX_CUSTOM_THEMES,
  addCustomTheme,
  contrastRatio,
  nextCustomThemeId,
  normalizeHex,
  patchCustomTheme,
  removeCustomTheme,
  sanitizeCustomThemes,
  sanitizeThemeFont,
} from '../src/editor/slides/temas';
import { slideVisuals } from '../src/editor/slides/SlidesCanvas';
import type { SlideCustomTheme, SlidesModel } from '../src/editor/types';

const tema = (id: string, extra: Partial<SlideCustomTheme> = {}): SlideCustomTheme => ({
  id,
  name: 'Cooperativa',
  bg: '#F4F1EA',
  ink: '#1B2A41',
  ...extra,
});

test('normalizeHex: #RGB, RGB, RRGGBB y #RRGGBB; lo demás, null', () => {
  expect(normalizeHex('#abc')).toBe('#AABBCC');
  expect(normalizeHex('1b2a41')).toBe('#1B2A41');
  expect(normalizeHex(' #f4f1ea ')).toBe('#F4F1EA');
  for (const malo of ['', 'rojo', '#12', '#12345', '#GGGGGG', 'rgb(1,2,3)', 'url(x)'])
    expect(normalizeHex(malo)).toBeNull();
});

test('sanitizeThemeFont: solo nombres de fuente de verdad', () => {
  expect(sanitizeThemeFont('Georgia')).toBe('Georgia');
  expect(sanitizeThemeFont(' Trebuchet MS ')).toBe('Trebuchet MS');
  expect(sanitizeThemeFont('Source Serif 4')).toBe('Source Serif 4');
  for (const malo of ['', 'Georgia";x:y', 'a'.repeat(41), '<script>', "x'y", 'a;b', 5, null])
    expect(sanitizeThemeFont(malo)).toBe('');
});

test('sanitizeCustomThemes: ids, colores, repetidos, nombre y tope', () => {
  const out = sanitizeCustomThemes([
    tema('custom-1', { bg: '#abc', font: 'Georgia' }),
    tema('custom-1'), // repetido
    tema('carbon'), // no es un id de tema propio
    tema('custom-2', { bg: 'rojo' }), // color roto
    tema('custom-3', { name: 'x'.repeat(100), font: 'mal";' }),
    null,
    'texto',
  ]);
  expect(out.map((t) => t.id)).toEqual(['custom-1', 'custom-3']);
  expect(out[0]).toEqual({
    id: 'custom-1',
    name: 'Cooperativa',
    bg: '#AABBCC',
    ink: '#1B2A41',
    font: 'Georgia',
  });
  expect(out[1]!.name).toHaveLength(60);
  expect(out[1]).not.toHaveProperty('font');
  const muchos = Array.from({ length: 30 }, (_, i) => tema(`custom-${i + 1}`));
  expect(sanitizeCustomThemes(muchos)).toHaveLength(MAX_CUSTOM_THEMES);
  expect(sanitizeCustomThemes('x')).toEqual([]);
});

test('addCustomTheme / patchCustomTheme / removeCustomTheme', () => {
  const a = addCustomTheme([], { bg: '#0A0A0A', ink: '#FFFFFF', font: 'Georgia' }, 'Noche')!;
  expect(a.id).toBe('custom-1');
  const b = addCustomTheme(a.themes, { bg: '#FFFFFF', ink: '#0A0A0A' }, 'Día')!;
  expect(b.themes.map((t) => t.id)).toEqual(['custom-1', 'custom-2']);
  expect(nextCustomThemeId(removeCustomTheme(b.themes, 'custom-1'))).toBe('custom-1');
  const c = patchCustomTheme(b.themes, 'custom-2', { name: 'Claro', ink: 'azul' }); // el color roto no pasa
  expect(c[1]).toEqual({ id: 'custom-2', name: 'Claro', bg: '#FFFFFF', ink: '#0A0A0A' });
  const lleno = Array.from({ length: MAX_CUSTOM_THEMES }, (_, i) => tema(`custom-${i + 1}`));
  expect(addCustomTheme(lleno, { bg: '#FFFFFF', ink: '#000000' }, 'otro')).toBeNull();
});

test('contrastRatio: el del blanco sobre negro y el de dos grises parecidos', () => {
  expect(contrastRatio('#FFFFFF', '#000000')).toBeCloseTo(21, 0);
  expect(contrastRatio('#000000', '#FFFFFF')).toBeCloseTo(21, 0);
  expect(contrastRatio('#777777', '#808080')).toBeLessThan(1.5);
  expect(contrastRatio('rojo', '#000')).toBe(1);
});

test('slideVisuals: un tema propio fija fondo, tinta y fuente; uno roto cae a Papel', () => {
  const m: SlidesModel = {
    slides: [],
    theme: 'custom-1',
    themes: [tema('custom-1', { font: 'Georgia' })],
  };
  const v = slideVisuals(m, { id: 's', boxes: [] });
  expect(v).toEqual({ bg: '#F4F1EA', ink: '#1B2A41', font: 'Georgia' });
  // El fondo propio de la diapositiva manda, como con un preset.
  expect(slideVisuals(m, { id: 's', boxes: [], bg: '#BBF7D0' }).bg).toBe('#BBF7D0');
  const roto: SlidesModel = {
    slides: [],
    theme: 'custom-1',
    themes: [tema('custom-1', { bg: 'rojo' })],
  };
  const p = slideVisuals(roto, { id: 's', boxes: [] });
  expect(p.bg).toContain('white');
  expect(p.ink).toBe('#0A0A0A');
});
