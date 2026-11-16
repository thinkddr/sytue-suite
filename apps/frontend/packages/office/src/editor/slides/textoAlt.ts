/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lógica PURA del texto alternativo de una imagen de diapositiva (`SlideBox.alt`): sin
 * él, toda imagen salía con `alt=""` —decorativa para un lector de pantalla— al editar,
 * al presentar y en lo que se comparte, y eso incumple WCAG 1.1.1. Sin Preact ni DOM: se
 * prueba con bun (test/slides-alt.test.ts).
 *
 * Frontera de confianza: el modelo lo escriben otras personas y otras versiones, así que
 * `altDe` rehace el texto cada vez que se va a pintar en vez de fiarse del campo.
 */
import type { SlideBox } from '../types';

/** Techo del texto alternativo, en caracteres. Un alt útil cabe en una o dos frases; el
 *  techo solo impide que un modelo manipulado infle cada autosave con un párrafo enorme. */
export const MAX_ALT_CHARS = 1000;

/** Texto alternativo de la caja listo para pintar: una sola línea (los saltos y tabuladores
 *  pasan a espacio, como los leería un lector de pantalla), recortado y con techo. `''` si
 *  la caja no tiene o no es texto. */
export function altDe(box: Pick<SlideBox, 'alt'>): string {
  const raw: unknown = box.alt;
  if (typeof raw !== 'string') return '';
  // Techo por caracteres (como las runas de `AltDe` en Fyne): cortar por unidades UTF-16
  // podría dejar medio emoji.
  return Array.from(raw.replace(/\s+/g, ' ').trim()).slice(0, MAX_ALT_CHARS).join('').trim();
}

/** La caja con su texto alternativo. Vacío = SIN campo (JSON limpio: «sin alt = como
 *  antes»), para que una imagen sin describir no ensucie el modelo ni lo que lee Fyne. */
export function conAlt(box: SlideBox, alt: string): SlideBox {
  const limpio = altDe({ alt });
  const { alt: _antes, ...resto } = box;
  return limpio === '' ? resto : { ...resto, alt: limpio };
}
