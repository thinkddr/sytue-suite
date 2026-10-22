/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * UNA NOTA LARGA SE VE ENTERA. Este test existe porque se intentó lo contrario:
 * recortar el extracto con un `max-h` para que la pared de notas no midiera
 * varias pantallas. Es justo lo que no se quiere — una nota ocupa lo que ocupa—
 * y el fallo real estaba en la rejilla del panel, cuya fila no crecía con el
 * texto y dejaba el fondo de la tarjeta a media nota.
 *
 * Se comprueban las clases y no el alto porque el DOM de los tests no hace
 * layout: `getBoundingClientRect` devuelve ceros y un test de píxeles pasaría
 * siempre. El alto se midió con un navegador de verdad.
 */
import { expect, test } from 'bun:test';
import { claseCuerpo } from './NoteCard.js';

test('el cuerpo no recorta la nota, por larga que sea', () => {
  for (const llenar of [true, false]) {
    const c = claseCuerpo(llenar);
    expect(c).not.toContain('max-h-');
    expect(c).not.toContain('overflow-hidden');
    expect(c).not.toContain('line-clamp');
  }
});

test('con la tarjeta estirada el cuerpo crece hasta llenarla', () => {
  expect(claseCuerpo(true)).toContain('flex-1');
  expect(claseCuerpo(false)).not.toContain('flex-1');
});
