/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El hilo del lector va sobre la hoja y no sobre `bg-bg`: sobre papel (ADR 0080) la
 * hoja es blanca y `bg` es el gris del lienzo, que salía como una mancha dentro de ella.
 * En el cromo de color la hoja es `bg`, así que allí no cambia nada. Se mira el código
 * porque montar el lector exige el servicio detrás. Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-06): el contenedor del hilo llevaba `bg-bg`.
 */
import { expect, test } from 'bun:test';

const lector = await Bun.file(new URL('./MessageReader.tsx', import.meta.url)).text();

test('el contenedor del hilo pinta el fondo de la hoja', () => {
  const hilo = lector.match(/<div class="([^"]*@container[^"]*)"/);
  expect(hilo).not.toBeNull();
  const clases = hilo![1]!.split(/\s+/);
  expect(clases).toContain('bg-[var(--sytue-sheet-bg)]');
  expect(clases).not.toContain('bg-bg');
});
