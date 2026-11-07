/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La regla de deslizar (pasar de foto en Photos, de mes en Calendar). Los mismos casos
 * están en Fyne (`TestDeslizarEsLaReglaDeLaWeb`, apps/fyne/ui): si se cambia un umbral
 * aquí, se cambia allí. Vienen de `apps/photos/src/lib/gestos.test.ts`, de donde subió
 * la regla. Vistos en ROJO cambiando `dx < 0 ? 1 : -1` por `dx < 0 ? -1 : 1` y
 * `Math.max(RECORRIDO_MIN, …)` por `FRACCION_ANCHO * ancho`.
 */
import { describe, expect, test } from 'bun:test';
import { decidirDeslizamiento as decidir } from './deslizar.js';

describe('la regla de deslizar', () => {
  test('hacia la izquierda, lo siguiente; hacia la derecha, lo anterior', () => {
    expect(decidir(-120, 4, 390, 300)).toBe(1);
    expect(decidir(120, -4, 390, 300)).toBe(-1);
  });

  test('un roce no pasa, ni despacio ni con un temblor rápido', () => {
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
});
