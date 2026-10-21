/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de slides (fase Slides-Elementos):
 * encuadre inicial de imágenes (imageBoxRect) y reflujo de plantilla que
 * conserva imágenes/formas (mergeLayoutBoxes).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import { imageBoxRect, mergeLayoutBoxes } from '../src/editor/slides/SlidesCanvas';
import type { SlideBox } from '../src/editor/types';
import { docsT } from '../src/i18n';

// Los marcadores dependen del idioma: se fija uno para que la aserción sea estable.
const t = docsT('es');

test('imageBoxRect: conserva el aspecto (corrigiendo el 16:9) y centra', () => {
  // Imagen 16:9 → mismo aspecto que el escenario: 40 % × 40 % centrado.
  const r = imageBoxRect(1600, 900);
  expect(r.w).toBe(40);
  expect(r.h).toBeCloseTo(40, 6);
  expect(r.x).toBeCloseTo(30, 6);
  expect(r.y).toBeCloseTo(30, 6);
  // Cuadrada: el % vertical crece ×16/9 para que EN PÍXELES sea cuadrada.
  const q = imageBoxRect(500, 500);
  expect((q.h * 540) / (q.w * 960)).toBeCloseTo(1, 6);
});

test('imageBoxRect: topes 70 % de alto y 4 % mínimo', () => {
  const alta = imageBoxRect(900, 3000); // vertical extrema → capa el alto
  expect(alta.h).toBe(70);
  expect(alta.w).toBeCloseTo((40 * 70) / ((40 * (3000 / 900) * 960) / 540), 6);
  expect(imageBoxRect(4000, 40).h).toBe(4); // panorámica extrema → alto mínimo
  expect(imageBoxRect(0, 0).h).toBe(4); // degenerada: no divide por cero
});

test('mergeLayoutBoxes: el texto refluye por orden; imagen y forma sobreviven', () => {
  const text = (id: string, html: string): SlideBox => ({ id, x: 0, y: 0, w: 10, h: 10, html });
  const img: SlideBox = {
    id: 'i',
    kind: 'image',
    x: 5,
    y: 5,
    w: 20,
    h: 20,
    html: '',
    src: 'data:x',
  };
  const shape: SlideBox = {
    id: 's',
    kind: 'shape',
    shape: 'rect',
    x: 1,
    y: 1,
    w: 9,
    h: 9,
    html: '',
    fill: '#475569',
    stroke: '#0A0A0A',
  };

  const out = mergeLayoutBoxes(
    'titulo-cuerpo',
    [text('a', '<h1>A</h1>'), img, text('b', '<p>B</p>'), shape],
    t,
  );
  // Plantilla «titulo-cuerpo»: 2 cajas de texto + imagen y forma conservadas al final.
  expect(out).toHaveLength(4);
  expect(out[0]!.html).toBe('<h1>A</h1>');
  expect(out[1]!.html).toBe('<p>B</p>');
  expect(out[2]).toEqual(img); // intactas: misma posición, mismo contenido
  expect(out[3]).toEqual(shape);
});

test('mergeLayoutBoxes: sin texto previo, la plantilla sale con sus rótulos', () => {
  const img: SlideBox = {
    id: 'i',
    kind: 'image',
    x: 5,
    y: 5,
    w: 20,
    h: 20,
    html: '',
    src: 'data:x',
  };
  const out = mergeLayoutBoxes('titulo', [img], t);
  expect(out).toHaveLength(2);
  expect(out[0]!.html).toContain('título');
  expect(out[1]).toEqual(img);
});
