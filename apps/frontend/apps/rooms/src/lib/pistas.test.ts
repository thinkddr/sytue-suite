/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import { pegaEsteVideo } from './pistas.js';

describe('pegaEsteVideo', () => {
  it('la cámara ocupa el hueco cuando nadie comparte pantalla', () => {
    expect(pegaEsteVideo({ esPantalla: false, hayPantalla: false })).toBe(true);
  });

  it('la pantalla compartida gana a la cámara del mismo participante', () => {
    expect(pegaEsteVideo({ esPantalla: false, hayPantalla: true })).toBe(false);
    expect(pegaEsteVideo({ esPantalla: true, hayPantalla: true })).toBe(true);
  });

  // Lo que de verdad protege este test: que la decisión sea ESTABLE. Si las dos
  // pistas se dieran por buenas, cada pasada de reconciliación echaría a la otra
  // y la baldosa parpadearía entre la cara y la presentación para siempre.
  it('con cámara y pantalla, exactamente una gana', () => {
    const candidatas = [
      pegaEsteVideo({ esPantalla: true, hayPantalla: true }),
      pegaEsteVideo({ esPantalla: false, hayPantalla: true }),
    ];
    expect(candidatas.filter(Boolean)).toHaveLength(1);
  });
});
