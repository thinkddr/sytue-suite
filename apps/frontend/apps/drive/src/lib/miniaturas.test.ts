/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { thumbable } from './miniaturas';

const f = (name: string, contentType?: string) => ({
  id: 'x',
  name,
  isDirectory: false,
  ...(contentType ? { contentType } : {}),
});

test('WebP tiene miniatura, por content-type o por extensión', () => {
  expect(thumbable(f('rosa.webp', 'image/webp'))).toBe(true);
  expect(thumbable(f('rosa.WEBP'))).toBe(true);
});

test('los ráster de siempre siguen; AVIF y SVG caen al icono', () => {
  expect(thumbable(f('a.jpg', 'image/jpeg'))).toBe(true);
  expect(thumbable(f('b.png', 'image/png; charset=binary'))).toBe(true);
  expect(thumbable(f('c.avif', 'image/avif'))).toBe(false);
  expect(thumbable(f('d.svg'))).toBe(false);
});

test('sin id o carpeta, no se pide', () => {
  expect(thumbable({ ...f('a.jpg'), id: '' })).toBe(false);
  expect(thumbable({ ...f('a.jpg'), isDirectory: true })).toBe(false);
});
