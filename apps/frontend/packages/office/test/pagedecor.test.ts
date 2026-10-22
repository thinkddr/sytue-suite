/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de bordes de página + marca de agua (src/editor/pagedecor.ts):
 * saneado de ajustes, horneado de la estructura + CSS (escape anti-XSS del texto
 * y de la URL de imagen, data-URI de fondo seguro para el atributo `style="…"`) y
 * round-trip de los `data-*`. Ejecutar: cd apps/frontend && bun test packages/office/test/pagedecor.test.ts
 */
import { expect, test } from 'bun:test';
import {
  DEFAULT_WATERMARK_COLOR,
  isSafeWatermarkImage,
  pageBorderHtml,
  pageBorderStyle,
  sanitizePageBorderSpec,
  sanitizeWatermarkSpec,
  watermarkHtml,
  watermarkStyle,
  watermarkSvg,
  WATERMARK_TILE_H,
} from '../src/editor/pagedecor';

const PNG = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAAA';

/* ---------- marca de agua: saneado ---------- */

test('sanitizeWatermarkSpec: texto válido, recorta y aplica reservas', () => {
  const spec = sanitizeWatermarkSpec({ text: '  BORRADOR  ' });
  expect(spec).toEqual({
    kind: 'text',
    text: 'BORRADOR',
    color: DEFAULT_WATERMARK_COLOR,
    opacity: 0.15,
    size: 120,
    diagonal: false,
  });
});

test('sanitizeWatermarkSpec: sin texto ni imagen → null', () => {
  expect(sanitizeWatermarkSpec({ text: '   ' })).toBeNull();
  expect(sanitizeWatermarkSpec({})).toBeNull();
  // kind image pero imagen no segura → cae a texto, y sin texto → null.
  expect(sanitizeWatermarkSpec({ kind: 'image', image: 'javascript:alert(1)' })).toBeNull();
});

test('sanitizeWatermarkSpec: coacciona strings (atributos data-*) y acota opacidad/tamaño', () => {
  const spec = sanitizeWatermarkSpec({
    text: 'X',
    color: '#ff0000',
    opacity: '5', // > 1 → 1
    size: '9', // < 24 → 24
    diagonal: '1',
  });
  expect(spec).toEqual({
    kind: 'text',
    text: 'X',
    color: '#FF0000',
    opacity: 1,
    size: 24,
    diagonal: true,
  });
});

test('sanitizeWatermarkSpec: color inválido → reserva', () => {
  expect(sanitizeWatermarkSpec({ text: 'X', color: 'rojo' })?.color).toBe(DEFAULT_WATERMARK_COLOR);
  expect(sanitizeWatermarkSpec({ text: 'X', color: '#abc' })?.color).toBe(DEFAULT_WATERMARK_COLOR);
});

test('sanitizeWatermarkSpec: imagen ráster segura → kind image (tamaño = %)', () => {
  const spec = sanitizeWatermarkSpec({ kind: 'image', image: PNG, size: '200' });
  expect(spec?.kind).toBe('image');
  expect(spec?.image).toBe(PNG);
  expect(spec?.size).toBe(100); // % acotado a 100
  expect(spec?.text).toBe('');
});

test('isSafeWatermarkImage: solo data-URI ráster; rechaza svg y esquemas', () => {
  expect(isSafeWatermarkImage(PNG)).toBe(true);
  expect(isSafeWatermarkImage('data:image/jpeg;base64,/9j/AAAA')).toBe(true);
  expect(isSafeWatermarkImage('data:image/svg+xml;base64,PHN2Zz4=')).toBe(false);
  expect(isSafeWatermarkImage('https://x/y.png')).toBe(false);
  expect(isSafeWatermarkImage('javascript:alert(1)')).toBe(false);
});

/* ---------- marca de agua: horneado SVG / CSS ---------- */

test('watermarkSvg: texto centrado, con viewBox A4 y rotación si diagonal', () => {
  const svg = watermarkSvg({
    kind: 'text',
    text: 'CONFIDENCIAL',
    color: '#94A3B8',
    opacity: 0.2,
    size: 100,
    diagonal: true,
  });
  expect(svg.startsWith('<svg')).toBe(true);
  expect(svg).toContain(`viewBox="0 0 794 ${WATERMARK_TILE_H}"`);
  expect(svg).toContain('CONFIDENCIAL');
  expect(svg).toContain('transform="rotate(-45 397 561.5)"');
  // Horizontal: sin transform.
  expect(
    watermarkSvg({
      kind: 'text',
      text: 'A',
      color: '#000000',
      opacity: 0.2,
      size: 80,
      diagonal: false,
    }),
  ).not.toContain('rotate');
});

test('watermarkSvg: kind image emite <image href=…> con el data-URI', () => {
  const svg = watermarkSvg({
    kind: 'image',
    text: '',
    image: PNG,
    color: '#000000',
    opacity: 0.2,
    size: 60,
    diagonal: false,
  });
  expect(svg).toContain('<image href="data:image/png;base64,');
  expect(svg).toContain('preserveAspectRatio="xMidYMid meet"');
});

test('watermarkSvg: escapa el texto del usuario (anti-XSS)', () => {
  const svg = watermarkSvg({
    kind: 'text',
    text: '</text><script>alert(1)</script>',
    color: '#000000',
    opacity: 0.2,
    size: 80,
    diagonal: false,
  });
  expect(svg).not.toContain('<script>');
  expect(svg).not.toContain('</text><script>');
  expect(svg).toContain('&lt;script&gt;');
});

test('watermarkStyle: fondo SVG detrás del contenido, repetido por página, sin comillas dobles crudas', () => {
  const style = watermarkStyle({
    kind: 'text',
    text: 'BORRADOR',
    color: '#94A3B8',
    opacity: 0.15,
    size: 120,
    diagonal: true,
  });
  expect(style).toContain('z-index:-1');
  expect(style).toContain('pointer-events:none');
  expect(style).toContain('opacity:0.15');
  expect(style).toContain("background-image:url('data:image/svg+xml,");
  expect(style).toContain(`background-size:100% ${WATERMARK_TILE_H}px`);
  // El SVG va URL-codificado: sin '<', '>' ni comillas dobles crudas que rompan style="…".
  expect(style).not.toContain('<');
  expect(style).not.toContain('>');
  expect(style).not.toContain('"');
  expect(style).toContain('%3C'); // '<' codificado
});

test('watermarkHtml: data-* (fuente de verdad) + style + inerte, con texto escapado', () => {
  const html = watermarkHtml({
    kind: 'text',
    text: 'a"b&c<d',
    color: '#94A3B8',
    opacity: 0.15,
    size: 120,
    diagonal: false,
  });
  expect(html).toContain('data-sytue-watermark');
  expect(html).toContain('data-kind="text"');
  expect(html).toContain('contenteditable="false"');
  expect(html).toContain('aria-hidden="true"');
  // El texto del usuario, escapado en el atributo data-text.
  expect(html).toContain('data-text="a&quot;b&amp;c&lt;d"');
  expect(html).not.toContain('data-text="a"b');
});

test('marca de agua: round-trip data-* → sanitize reconstruye el spec', () => {
  const spec = sanitizeWatermarkSpec({
    text: 'BORRADOR',
    color: '#DC2626',
    opacity: 0.25,
    size: 160,
    diagonal: true,
  });
  expect(spec).not.toBeNull();
  // Simula releer los data-* (como strings) del elemento insertado.
  const reread = sanitizeWatermarkSpec({
    kind: 'text',
    text: spec!.text,
    color: spec!.color,
    opacity: String(spec!.opacity),
    size: String(spec!.size),
    diagonal: spec!.diagonal ? '1' : '0',
  });
  expect(reread).toEqual(spec);
});

/* ---------- borde de página ---------- */

test('sanitizePageBorderSpec: grosor ≤ 0 → null (sin borde)', () => {
  expect(sanitizePageBorderSpec({ width: 0 })).toBeNull();
  expect(sanitizePageBorderSpec({})).toBeNull();
  expect(sanitizePageBorderSpec({ width: -3 })).toBeNull();
});

test('sanitizePageBorderSpec: estilo desconocido → solid, acota grosor, color de reserva', () => {
  const spec = sanitizePageBorderSpec({ style: 'zigzag', width: '99', color: 'nope' });
  expect(spec).toEqual({ style: 'solid', width: 12, color: '#475569' });
});

test('sanitizePageBorderSpec: valores válidos pasan', () => {
  expect(sanitizePageBorderSpec({ style: 'double', width: 4, color: '#0a0a0a' })).toEqual({
    style: 'double',
    width: 4,
    color: '#0A0A0A',
  });
});

test('pageBorderStyle: recuadro absoluto embutido en el margen vivo, sin capturar ratón', () => {
  const style = pageBorderStyle({ style: 'double', width: 4, color: '#475569' });
  expect(style).toContain('position:absolute');
  expect(style).toContain('pointer-events:none');
  expect(style).toContain('border:4px double #475569');
  expect(style).toContain('var(--doc-page-margin, 96px)');
});

test('pageBorderHtml: data-* + style + inerte', () => {
  const html = pageBorderHtml({ style: 'dashed', width: 2, color: '#94A3B8' });
  expect(html).toContain('data-sytue-page-border');
  expect(html).toContain('data-style="dashed"');
  expect(html).toContain('data-width="2"');
  expect(html).toContain('contenteditable="false"');
  expect(html).toContain('border:2px dashed #94A3B8');
  expect(html.endsWith('></div>')).toBe(true);
});

test('borde: round-trip data-* → sanitize reconstruye el spec', () => {
  const spec = sanitizePageBorderSpec({ style: 'dotted', width: 3, color: '#2563EB' });
  const reread = sanitizePageBorderSpec({
    style: spec!.style,
    width: String(spec!.width),
    color: spec!.color,
  });
  expect(reread).toEqual(spec);
});
