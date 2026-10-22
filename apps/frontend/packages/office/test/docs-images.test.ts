/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del manejo avanzado de imagen (src/editor/images.ts): saneado del
 * recorte, (de)serializado del modelo en `data-*` (round-trip) y el CÁLCULO del
 * `style` en línea — resize, ajuste de texto (float), recorte visible con
 * recuperación de espacio por márgenes negativos y anclaje absoluto.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/docs-images.test.ts
 */
import { expect, test } from 'bun:test';
import {
  clampCrop,
  computeImageStyle,
  imageDataAttrs,
  parseImageModel,
  styleCssText,
  type ImageModel,
} from '../src/editor/images';

const base = (over: Partial<ImageModel> = {}): ImageModel => ({
  wrap: 'inline',
  align: 'left',
  crop: { t: 0, r: 0, b: 0, l: 0 },
  alt: '',
  ...over,
});

/* ---------- saneado del recorte ---------- */

test('clampCrop acota cada lado a [0,0.9] y por eje deja ≥10% visible', () => {
  // Por lado: t=−1→0, r=2→0.9. Por eje l+r = 0.1+0.9 = 1.0 > 0.9 ⇒ ambos se escalan.
  expect(clampCrop({ t: -1, r: 2, b: 0.3, l: 0.1 })).toEqual({ t: 0, r: 0.81, b: 0.3, l: 0.09 });
  // t+b = 1.2 > 0.9 ⇒ se escalan proporcionalmente a sumar 0.9.
  expect(clampCrop({ t: 0.6, r: 0, b: 0.6, l: 0 })).toEqual({ t: 0.45, r: 0, b: 0.45, l: 0 });
});

/* ---------- round-trip modelo ↔ data-* ---------- */

test('imageDataAttrs omite valores por defecto (JSON mínimo)', () => {
  expect(imageDataAttrs(base())).toEqual({});
  expect(imageDataAttrs(base({ wrap: 'square', align: 'right', w: 300, h: 200 }))).toEqual({
    'data-wrap': 'square',
    'data-align': 'right',
    'data-w': '300',
    'data-h': '200',
  });
});

test('serializar → parsear conserva el modelo (round-trip)', () => {
  const model = base({
    wrap: 'behind',
    align: 'left',
    w: 320.5,
    h: 180,
    x: 40,
    y: 96,
    crop: { t: 0.1, r: 0, b: 0.2, l: 0.05 },
    alt: 'Un gráfico «anual»',
    src: 'data:image/png;base64,AAA',
  });
  const attrs = imageDataAttrs(model);
  const map: Record<string, string> = { ...attrs, alt: model.alt, src: model.src! };
  expect(parseImageModel((n) => map[n] ?? null)).toEqual(model);
});

test('parsear es tolerante: wrap/align inválidos → por defecto; x/y solo si es absoluto', () => {
  const map: Record<string, string> = { 'data-wrap': 'raro', 'data-align': 'x', 'data-x': '10' };
  const m = parseImageModel((n) => map[n] ?? null);
  expect(m.wrap).toBe('inline');
  expect(m.align).toBe('left');
  expect(m.x).toBeUndefined(); // en línea no lleva x/y
});

/* ---------- cálculo del style ---------- */

test('en línea con tamaño: inline-block, max-width acotado, sin float', () => {
  const s = computeImageStyle(base({ w: 200, h: 120 }));
  expect(s.display).toBe('inline-block');
  expect(s.width).toBe('200px');
  expect(s.height).toBe('120px');
  expect(s.maxWidth).toBe('100%');
  expect(s.float).toBeUndefined();
});

test('cuadrado a la izquierda: float izquierda + separación a la derecha y abajo', () => {
  const s = computeImageStyle(base({ wrap: 'square', align: 'left', w: 300, h: 200 }));
  expect(s.float).toBe('left');
  expect(s.marginRight).toBe('14px');
  expect(s.marginBottom).toBe('14px');
  expect(s.maxWidth).toBe('none');
});

test('cuadrado a la derecha: float derecha + separación a la izquierda', () => {
  const s = computeImageStyle(base({ wrap: 'square', align: 'right', w: 300, h: 200 }));
  expect(s.float).toBe('right');
  expect(s.marginLeft).toBe('14px');
  expect(s.marginRight).toBeUndefined();
});

test('recorte: clip-path oculta bandas y el margen las recupera (separación − banda)', () => {
  // w=300,h=200; recorte derecha 0.1 → banda 30px; abajo 0.25 → banda 50px.
  const s = computeImageStyle(
    base({ wrap: 'square', align: 'left', w: 300, h: 200, crop: { t: 0, r: 0.1, b: 0.25, l: 0 } }),
  );
  expect(s.clipPath).toBe('inset(0px 30px 50px 0px)');
  // marginRight = separación(14) − banda(30) = −16 ⇒ el texto queda a 14px del borde VISIBLE.
  expect(s.marginRight).toBe('-16px');
  // marginBottom = 14 − 50 = −36.
  expect(s.marginBottom).toBe('-36px');
});

test('ajustado (tight) con src: contorno shape-outside sobre el float', () => {
  const s = computeImageStyle(
    base({ wrap: 'tight', w: 200, h: 200, src: 'data:image/png;base64,AAA' }),
  );
  expect(s.float).toBe('left');
  expect(s.shapeOutside).toBe('inset(0px 0px 0px 0px)');
  expect(s.shapeMargin).toBe('14px');
});

test('detrás del texto: absoluta, z-index bajo el texto, posición x/y', () => {
  const s = computeImageStyle(base({ wrap: 'behind', w: 400, h: 300, x: 50, y: 120 }));
  expect(s.position).toBe('absolute');
  expect(s.left).toBe('50px');
  expect(s.top).toBe('120px');
  expect(s.zIndex).toBe('-1');
});

test('delante del texto: z-index por encima', () => {
  expect(computeImageStyle(base({ wrap: 'front', w: 10, h: 10 })).zIndex).toBe('3');
});

test('arriba y abajo centrado: bloque con márgenes auto', () => {
  const s = computeImageStyle(base({ wrap: 'topBottom', align: 'center', w: 300, h: 100 }));
  expect(s.display).toBe('block');
  expect(s.marginLeft).toBe('auto');
  expect(s.marginRight).toBe('auto');
  expect(s.marginTop).toBe('14px');
});

test('styleCssText serializa camelCase → kebab-case', () => {
  expect(styleCssText({ clipPath: 'inset(0px)', maxWidth: 'none', zIndex: '3' })).toBe(
    'clip-path:inset(0px);max-width:none;z-index:3',
  );
});
