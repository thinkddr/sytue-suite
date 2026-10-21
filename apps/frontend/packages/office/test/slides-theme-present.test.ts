/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de las fases Slides-Temas, Slides-Presentación
 * (vista del moderador) y Slides-Enlaces: resolución de fondo/tinta/fuente por
 * tema y fondo propio (slideVisuals), formato del cronómetro (fmtElapsed) y
 * destino de un enlace de acción (actionTarget).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  actionTarget,
  fmtElapsed,
  SLIDE_THEMES,
  slideVisuals,
} from '../src/editor/slides/SlidesCanvas';
import type { Slide, SlidesModel } from '../src/editor/types';

const slide = (extra: Partial<Slide> = {}): Slide => ({ id: 's', boxes: [], ...extra });
const themeById = (id: string) => SLIDE_THEMES.find((t) => t.id === id)!;

test('slideVisuals: sin tema = Papel (fondo blanco, tinta negra, sin fuente propia)', () => {
  const v = slideVisuals({ slides: [] }, slide());
  expect(v.bg).toContain('white'); // DEFAULT_BG = var(--editor-page-bg, white)
  expect(v.ink).toBe('#0A0A0A');
  expect(v.font).toBeUndefined();
});

test('slideVisuals: el tema fija fondo/tinta/fuente de toda la presentación', () => {
  const arena = themeById('arena');
  const v = slideVisuals({ slides: [], theme: 'arena' }, slide());
  expect(v.bg).toBe(arena.bg);
  expect(v.ink).toBe(arena.ink);
  expect(v.font).toBe(arena.font); // 'Georgia'
});

test('slideVisuals: el fondo PROPIO de la diapositiva manda sobre el del tema', () => {
  const v = slideVisuals({ slides: [], theme: 'carbon' }, slide({ bg: '#BBF7D0' }));
  expect(v.bg).toBe('#BBF7D0');
  // La tinta la sigue poniendo el tema (carbón: blanca), no cambia con el fondo propio.
  expect(v.ink).toBe(themeById('carbon').ink);
});

test('slideVisuals: bg "transparent" usa el del tema; tema desconocido cae a Papel', () => {
  const themed = slideVisuals({ slides: [], theme: 'menta' }, slide({ bg: 'transparent' }));
  expect(themed.bg).toBe(themeById('menta').bg);
  // Doc futuro/corrupto con tema desconocido: Papel (sin reventar).
  // `as unknown` a propósito: el tema no existe, que es justo lo que se está probando.
  const unknown = slideVisuals({ slides: [], theme: 'zzz' } as unknown as SlidesModel, slide());
  expect(unknown.bg).toContain('white');
  expect(unknown.ink).toBe('#0A0A0A');
});

test('fmtElapsed: M:SS, H:MM:SS y nunca negativo', () => {
  expect(fmtElapsed(0)).toBe('0:00');
  expect(fmtElapsed(5_000)).toBe('0:05');
  expect(fmtElapsed(65_000)).toBe('1:05');
  expect(fmtElapsed(600_000)).toBe('10:00');
  expect(fmtElapsed(3_661_000)).toBe('1:01:01');
  expect(fmtElapsed(-9_000)).toBe('0:00');
});

test('actionTarget: relativos con topes y número 1-based dentro de rango', () => {
  // Mazo de 5 (last = 4), diapositiva actual idx 2.
  expect(actionTarget('first', 2, 4)).toBe(0);
  expect(actionTarget('last', 2, 4)).toBe(4);
  expect(actionTarget('next', 2, 4)).toBe(3);
  expect(actionTarget('next', 4, 4)).toBe(4); // tope final
  expect(actionTarget('prev', 2, 4)).toBe(1);
  expect(actionTarget('prev', 0, 4)).toBe(0); // tope inicial
  expect(actionTarget(1, 2, 4)).toBe(0); // diapositiva 1 → índice 0
  expect(actionTarget(5, 2, 4)).toBe(4); // diapositiva 5 → índice 4
  expect(actionTarget(6, 2, 4)).toBeNull(); // fuera de rango
  expect(actionTarget(0, 2, 4)).toBeNull(); // 0 no es un número 1-based válido
});
