/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Ir a» de Gmail: «g» y la letra de la carpeta, por ROL. La segunda tecla solo cuenta
 * dentro de la ventana, y la hoja de «?» los enseña (un atajo que no está en la hoja no
 * lo descubre nadie). Fyne: `TestIrACarpetaConG`.
 */
import { expect, test } from 'bun:test';
import { useT as traductor } from '../i18n/index.js';
import { VENTANA_IR_MS, destinoDeIr, gruposDeAtajos } from './hojaDeAtajos.js';

test('la letra tras «g» elige la carpeta por su rol, y solo a tiempo', () => {
  const g = 1_000;
  expect(destinoDeIr('i', g, g + 200)).toBe('inbox');
  expect(destinoDeIr('t', g, g + 200)).toBe('sent');
  expect(destinoDeIr('d', g, g + 200)).toBe('drafts');
  expect(destinoDeIr('a', g, g + VENTANA_IR_MS)).toBe('archive');
  expect(destinoDeIr('a', g, g + VENTANA_IR_MS + 1)).toBeNull();
  expect(destinoDeIr('a', 0, g)).toBeNull();
  expect(destinoDeIr('x', g, g + 1)).toBeNull();
});

test('la hoja de atajos enseña los de «ir a»', () => {
  const t = traductor('es');
  const filas = gruposDeAtajos(t).flatMap((g) => g.atajos);
  for (const [letra, clave] of [
    ['i', 'shortcuts.goInbox'],
    ['t', 'shortcuts.goSent'],
    ['d', 'shortcuts.goDrafts'],
    ['a', 'shortcuts.goArchive'],
  ] as const)
    expect(
      filas.some((f) => f.teclas.join(' ') === `g ${letra}` && f.descripcion === t(clave)),
    ).toBe(true);
});
