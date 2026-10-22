/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La regla de deslizar entre fotos. Los mismos casos están en Fyne
 * (`TestDeslizarEsLaReglaDeLaWeb`, apps/fyne/apps/photos): si se cambia un umbral aquí,
 * se cambia allí.
 */
import { describe, expect, test } from 'bun:test';
import { anclar, decidir, limitar } from './gestos.js';

describe('los gestos del visor', () => {
  test('hacia la izquierda, la siguiente; hacia la derecha, la anterior', () => {
    expect(decidir(-120, 4, 390, 300)).toBe(1);
    expect(decidir(120, -4, 390, 300)).toBe(-1);
  });

  test('un roce no pasa de foto, ni despacio ni con un temblor rápido', () => {
    expect(decidir(-40, 0, 390, 400)).toBe(0); // 40 < 56 y lento
    expect(decidir(-20, 0, 390, 10)).toBe(0); // rápido pero menos de 24 px
  });

  test('en una pantalla estrecha manda el mínimo de 56 px', () => {
    // El 18 % de 300 son 54: ahí el suelo es RECORRIDO_MIN.
    expect(decidir(-55, 0, 300, 500)).toBe(0);
    expect(decidir(-60, 0, 300, 500)).toBe(1);
  });

  test('en una tableta el recorrido mínimo es una fracción del ancho', () => {
    // 18 % de 820 son 147,6: 120 px despacio no bastan, en un teléfono sí.
    expect(decidir(-120, 0, 820, 500)).toBe(0);
    expect(decidir(-120, 0, 390, 500)).toBe(1);
  });

  test('un golpe rápido pasa aunque recorra poco', () => {
    expect(decidir(-40, 0, 820, 60)).toBe(1); // 0,67 px/ms
  });

  test('lo que es más vertical que horizontal no es un deslizamiento', () => {
    expect(decidir(-100, 80, 390, 200)).toBe(0);
    expect(decidir(-150, 100, 390, 200)).toBe(1); // 150 ≥ 1,4 × 100
  });

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
