/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo propio del visor: lo ampliado no se sale y el pellizco ancla bajo los dedos. La
 * regla de deslizar subió a @sytue/ui con sus casos (`packages/ui/src/lib/deslizar.test.ts`).
 */
import { describe, expect, test } from 'bun:test';
import { anclar, limitar } from './gestos.js';

describe('los gestos del visor', () => {
  test('lo ampliado no se sale: el borde de la foto no entra en el hueco', () => {
    // Una foto de 390 a ×2 mide 780: sobresale 195 por cada lado de un hueco de 390.
    expect(limitar(300, 2, 390, 390)).toBe(195);
    expect(limitar(-300, 2, 390, 390)).toBe(-195);
    // Encajada no se mueve, aunque se tire de ella.
    expect(limitar(50, 1, 390, 390)).toBe(0);
  });

  test('ampliar alrededor de los dedos deja quieto el punto que está debajo', () => {
    // El punto a 100 px del centro, de ×1 a ×2: la foto se va 100 a la izquierda para
    // que lo que estaba bajo los dedos siga ahí (2·100 − 100 = 100).
    expect(anclar(100, 1, 0, 2)).toBe(-100);
    // Sin cambiar de escala no se mueve nada.
    expect(anclar(100, 2, 30, 2)).toBe(30);
  });
});
