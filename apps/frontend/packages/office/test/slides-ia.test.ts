/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { bloquesDe } from '@sytue/ai';
import {
  diapositivasDe,
  htmlDeCajas,
  htmlDeCuerpo,
  htmlDeTrozos,
  PUNTOS_POR_DIAPO,
} from '../src/editor/slides/iaslides.js';

const escapar = (s: string): string =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

test('cada título abre una diapositiva y lo de debajo es su cuerpo', () => {
  const d = diapositivasDe(bloquesDe('## Uno\n- a\n- b\n## Dos\ntexto'));
  expect(d).toHaveLength(2);
  expect(d[0]?.titulo[0]?.texto).toBe('Uno');
  expect(d[0]?.cuerpo).toHaveLength(2);
  expect(d[1]?.titulo[0]?.texto).toBe('Dos');
});

// Preferible una lámina densa a inventarse cortes donde el modelo no los puso.
test('sin títulos sale una sola diapositiva', () => {
  expect(diapositivasDe(bloquesDe('uno\ndos\ntres'))).toHaveLength(1);
});

// Un apartado con veinte viñetas es una lámina que nadie lee desde la tercera
// fila: se parte, y la continuación conserva el título.
test('un apartado largo se parte conservando el título', () => {
  const puntos = Array.from({ length: PUNTOS_POR_DIAPO + 2 }, (_, i) => `- p${i}`).join('\n');
  const d = diapositivasDe(bloquesDe(`## Largo\n${puntos}`));
  expect(d.length).toBeGreaterThan(1);
  expect(d[1]?.titulo[0]?.texto).toBe('Largo');
});

test('un texto vacío no crea diapositivas', () => {
  expect(diapositivasDe(bloquesDe('   \n\n'))).toEqual([]);
});

// Lo que sostiene que esto se pueda meter en el `html` de una caja: el modelo
// aporta texto, y las únicas etiquetas son las que escribimos aquí.
test('el texto del modelo se escapa y solo salen nuestras etiquetas', () => {
  const html = htmlDeTrozos(bloquesDe('**<script>alert(1)</script>**')[0]?.trozos ?? [], escapar);
  expect(html).not.toContain('<script>');
  expect(html).toContain('&lt;script&gt;');
});

test('el cuerpo sale como viñetas si el modelo dio lista', () => {
  expect(htmlDeCuerpo(bloquesDe('- a\n- b'), escapar)).toBe('<ul><li>a</li><li>b</li></ul>');
  expect(htmlDeCuerpo(bloquesDe('hola'), escapar)).toBe('<p>hola</p>');
});

// Sin título, la disposición tiene una sola caja: esa caja lleva el CUERPO. Antes
// recibía el título vacío y el cuerpo se perdía (la nativa ya lo hacía bien).
test('una lámina sin título pone su cuerpo en la caja única', () => {
  const [d] = diapositivasDe(bloquesDe('- a\n- b'));
  expect(htmlDeCajas(d!, 1, escapar)).toEqual(['<ul><li>a</li><li>b</li></ul>']);
  const [c] = diapositivasDe(bloquesDe('## T\n- a'));
  expect(htmlDeCajas(c!, 2, escapar)).toEqual(['T', '<ul><li>a</li></ul>']);
});
