/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de la fase Slides-Tamaño: saneado del tamaño
 * del lienzo (sanitizeSlideSize), proporción efectiva (slideAspect), alto del
 * escenario (stageHeight) y su efecto sobre el encuadre de imagen (imageBoxRect).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  imageBoxRect,
  sanitizeSlideSize,
  slideAspect,
  stageHeight,
} from '../src/editor/slides/SlidesCanvas';

/* ---------- sanitizeSlideSize ---------- */

test('sanitizeSlideSize: ausente/no-objeto/16:9/desconocido = por defecto (undefined)', () => {
  expect(sanitizeSlideSize(undefined)).toBeUndefined();
  expect(sanitizeSlideSize(null)).toBeUndefined();
  expect(sanitizeSlideSize('16:9')).toBeUndefined();
  expect(sanitizeSlideSize({ preset: '16:9' })).toBeUndefined();
  expect(sanitizeSlideSize({ preset: 'raro' })).toBeUndefined();
});

test('sanitizeSlideSize: 4:3 se conserva como preset', () => {
  expect(sanitizeSlideSize({ preset: '4:3' })).toEqual({ preset: '4:3' });
});

test('sanitizeSlideSize: custom válido conserva dimensiones y unidad (px por defecto)', () => {
  expect(sanitizeSlideSize({ preset: 'custom', w: 1024, h: 768 })).toEqual({
    preset: 'custom',
    w: 1024,
    h: 768,
    unit: 'px',
  });
  expect(sanitizeSlideSize({ preset: 'custom', w: 25, h: 19, unit: 'cm' })).toEqual({
    preset: 'custom',
    w: 25,
    h: 19,
    unit: 'cm',
  });
});

test('sanitizeSlideSize: custom sin dimensiones válidas (≤0/NaN/faltan) = por defecto', () => {
  expect(sanitizeSlideSize({ preset: 'custom' })).toBeUndefined();
  expect(sanitizeSlideSize({ preset: 'custom', w: 0, h: 100 })).toBeUndefined();
  expect(sanitizeSlideSize({ preset: 'custom', w: -5, h: 100 })).toBeUndefined();
  expect(sanitizeSlideSize({ preset: 'custom', w: 100, h: Infinity })).toBeUndefined();
  expect(sanitizeSlideSize({ preset: 'custom', w: '100', h: 50 })).toBeUndefined();
});

/* ---------- slideAspect ---------- */

test('slideAspect: por defecto 16:9, preset 4:3 y custom por cociente', () => {
  expect(slideAspect(undefined)).toBeCloseTo(16 / 9, 6);
  expect(slideAspect({ preset: '4:3' })).toBeCloseTo(4 / 3, 6);
  expect(slideAspect({ preset: 'custom', w: 1000, h: 500, unit: 'px' })).toBeCloseTo(2, 6);
  // La unidad es irrelevante: solo cuenta el cociente ancho/alto.
  expect(slideAspect({ preset: 'custom', w: 20, h: 10, unit: 'cm' })).toBeCloseTo(2, 6);
});

test('slideAspect: topa proporciones degeneradas al rango [0.2, 5]', () => {
  expect(slideAspect({ preset: 'custom', w: 10000, h: 1, unit: 'px' })).toBe(5); // panorámico extremo
  expect(slideAspect({ preset: 'custom', w: 1, h: 10000, unit: 'px' })).toBe(0.2); // tira vertical
});

/* ---------- stageHeight ---------- */

test('stageHeight: 16:9 → 540, 4:3 → 720 (ancho interno fijo 960)', () => {
  expect(stageHeight(undefined)).toBe(540);
  expect(stageHeight({ preset: '4:3' })).toBe(720);
  // 1:1 → cuadrado (alto = ancho interno).
  expect(stageHeight({ preset: 'custom', w: 500, h: 500, unit: 'px' })).toBe(960);
});

/* ---------- imageBoxRect con alto efectivo ---------- */

test('imageBoxRect: la misma imagen ocupa MENOS % de alto en 4:3 que en 16:9', () => {
  // Imagen apaisada 2:1 (no llega al tope del 70 % de alto en ningún caso): el
  // ancho en % queda fijo (40) y solo cambia el alto, que baja al crecer el alto
  // efectivo del lienzo (4:3 es más alto) — prueba de que sh entra en el cálculo.
  const wide = imageBoxRect(200, 100, stageHeight(undefined)); // 16:9 → sh 540
  const tall = imageBoxRect(200, 100, stageHeight({ preset: '4:3' })); // 4:3 → sh 720
  expect(wide.w).toBeCloseTo(40, 6);
  expect(tall.w).toBeCloseTo(40, 6);
  expect(tall.h).toBeLessThan(wide.h);
  expect(tall.h).toBeGreaterThan(0);
});

test('imageBoxRect: sin alto efectivo mantiene el comportamiento clásico 16:9', () => {
  expect(imageBoxRect(100, 100)).toEqual(imageBoxRect(100, 100, 540));
});
