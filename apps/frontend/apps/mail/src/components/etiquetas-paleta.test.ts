/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los colores de una etiqueta son la paleta de colores de usuario de la suite. Las ocho
 * etiquetas de antes guardan su token de siempre y se pintan con el tono nuevo; los
 * cuatro tonos que se suman ya no caen en el gris neutro.
 *
 * Vistos en ROJO con la paleta sobria de antes: ocho tokens y no doce, `brass` pintaba
 * #9a7b3f (rótulo blanco a 3,98) y `red` salía neutro.
 */
import { expect, test } from 'bun:test';
import { PALETA_DE_USUARIO } from '@sytue/config';
import { LABEL_COLOR_TOKENS, labelDot } from './utils.js';

test('el selector ofrece los doce tonos de la paleta de usuario, en su orden', () => {
  expect(LABEL_COLOR_TOKENS).toEqual(PALETA_DE_USUARIO.map((c) => c.id));
});

test('cada token se pinta con su tono, y los de antes también', () => {
  for (const c of PALETA_DE_USUARIO) expect(labelDot(c.id)).toBe(c.valor);
  expect(labelDot('brass')).toBe('#946a10');
  expect(labelDot('red')).toBe('#c44744');
});

test('sin color, o con un token que no se conoce, el punto es neutro', () => {
  expect(labelDot('')).toStartWith('var(--sytue-border-strong');
  expect(labelDot('neon')).toStartWith('var(--sytue-border-strong');
});
