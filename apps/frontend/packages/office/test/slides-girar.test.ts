/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Giro de las cajas de Diapos (slides/girar.ts): normalización del campo `rot`, el
 * ángulo del asa, girar varias cajas a la vez y el redimensionado de una caja girada
 * (la esquina opuesta se queda quieta). También que la vista previa la pinte girada.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/slides-girar.test.ts
 */
import { expect, test } from 'bun:test';
import {
  anguloDelPuntero,
  conGiro,
  estiloGiro,
  girarCajas,
  normalizarGiro,
  redimensionarGirada,
} from '../src/editor/slides/girar';
import { diapositivasAHtml } from '../src/components/wopiRender';
import type { SlideBox, SlidesModel } from '../src/editor/types';

const caja = (id: string, rot?: number): SlideBox =>
  rot === undefined
    ? { id, x: 10, y: 10, w: 20, h: 20, html: '' }
    : { id, x: 10, y: 10, w: 20, h: 20, html: '', rot };

test('normalizarGiro lleva a [0, 360) y quita el giro nulo o inválido', () => {
  expect(normalizarGiro(30)).toBe(30);
  expect(normalizarGiro(-90)).toBe(270);
  expect(normalizarGiro(720)).toBeUndefined();
  expect(normalizarGiro(0)).toBeUndefined();
  expect(normalizarGiro(359.9999)).toBeUndefined();
  expect(normalizarGiro(Number.NaN)).toBeUndefined();
  expect(normalizarGiro('45')).toBeUndefined();
});

test('estiloGiro usa la propiedad rotate (no transform) y nada sin giro', () => {
  expect(estiloGiro(caja('a', 45))).toEqual({ rotate: '45deg' });
  expect(estiloGiro(caja('a'))).toEqual({});
  expect(estiloGiro({ rot: Number.POSITIVE_INFINITY })).toEqual({});
});

test('conGiro pone el campo y lo quita al enderezar', () => {
  expect(conGiro(caja('a'), 90).rot).toBe(90);
  expect('rot' in conGiro(caja('a', 90), 360)).toBe(false);
});

test('girarCajas suma a las elegidas, cada una sobre su giro, y endereza con null', () => {
  const boxes = [caja('a', 30), caja('b'), caja('c', 300)];
  const g = girarCajas(boxes, ['a', 'c'], 90);
  expect(g.map((b) => b.rot)).toEqual([120, undefined, 30]);
  expect(g[1]).toBe(boxes[1]!); // la que no se toca, intacta
  const rectas = girarCajas(g, ['a', 'c'], null);
  expect(rectas.some((b) => 'rot' in b)).toBe(false);
  // Girar no mueve la caja: el giro es sobre su centro.
  expect(rectas[0]).toMatchObject({ x: 10, y: 10, w: 20, h: 20 });
});

test('anguloDelPuntero mide desde arriba en sentido horario y redondea con paso', () => {
  const c = { x: 100, y: 100 };
  expect(anguloDelPuntero(c, { x: 100, y: 0 })).toBe(0);
  expect(anguloDelPuntero(c, { x: 200, y: 100 })).toBe(90);
  expect(anguloDelPuntero(c, { x: 100, y: 200 })).toBe(180);
  expect(anguloDelPuntero(c, { x: 0, y: 100 })).toBe(270);
  // 40° sin paso; con paso de 15°, 45°.
  const p = { x: 100 + Math.sin((40 * Math.PI) / 180), y: 100 - Math.cos((40 * Math.PI) / 180) };
  expect(anguloDelPuntero(c, p)).toBeCloseTo(40, 2);
  expect(anguloDelPuntero(c, p, 15)).toBe(45);
});

test('redimensionarGirada sin giro es el redimensionado de siempre', () => {
  const o = { x: 10, y: 20, w: 30, h: 40 };
  // 96 px de 960 = 10 %; 54 px de 540 = 10 %.
  expect(redimensionarGirada(o, 96, 54, 960, 540)).toEqual({ x: 10, y: 20, w: 40, h: 50 });
  // Con mínimo.
  expect(redimensionarGirada(o, -2000, -2000, 960, 540)).toMatchObject({ w: 4, h: 4 });
});

test('redimensionarGirada sigue los ejes de la caja y deja quieta la esquina opuesta', () => {
  // Lienzo cuadrado de 100 px para leer los % como px. Caja girada 90°: su «ancho»
  // va hacia abajo en pantalla. Arrastrar 10 px hacia abajo la ensancha 10.
  const o = { x: 40, y: 40, w: 20, h: 10, rot: 90 };
  const r = redimensionarGirada(o, 0, 10, 100, 100);
  expect(r.w).toBeCloseTo(30, 3);
  expect(r.h).toBeCloseTo(10, 3);
  // La esquina superior izquierda (girada) no se mueve.
  const esquina = (b: { x: number; y: number; w: number; h: number }): { x: number; y: number } => {
    const cx = b.x + b.w / 2;
    const cy = b.y + b.h / 2;
    // (−w/2, −h/2) girado 90° en sentido horario = (h/2, −w/2).
    return { x: cx + b.h / 2, y: cy - b.w / 2 };
  };
  const a = esquina(o);
  const z = esquina(r);
  expect(z.x).toBeCloseTo(a.x, 3);
  expect(z.y).toBeCloseTo(a.y, 3);
});

test('la vista previa pinta girada la caja girada', () => {
  const modelo: SlidesModel = {
    slides: [{ id: 's', boxes: [{ ...caja('a', 30), html: '<p>Hola</p>' }, caja('b')] }],
  } as SlidesModel;
  const html = diapositivasAHtml(modelo);
  expect(html).toContain('rotate:30deg');
  expect(html.match(/rotate:/g)?.length).toBe(1);
});
