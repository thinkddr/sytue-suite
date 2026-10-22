/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import {
  alternar,
  bytesDe,
  esRecomendada,
  quitadas,
  recomendada,
  sumar,
  totalAlSubir,
} from './calidades.js';
import type { Estimacion } from './tipos.js';

// Lo que contesta el servicio para un minuto a 1080p y otro a 720p.
const e1080: Estimacion = {
  options: [
    { quality: 1080, allowed: true, recommended: true, bytes: 61_680_000 },
    { quality: 720, allowed: true, recommended: true, bytes: 33_180_000 },
    { quality: 360, allowed: true, recommended: true, bytes: 11_430_000 },
  ],
  recommended: [1080, 720, 360],
  original_bytes: 500_000_000,
  duration_ms: 60_000,
  approximate: false,
};
const e720: Estimacion = {
  options: [
    { quality: 1080, allowed: false, recommended: false, bytes: 0 },
    { quality: 720, allowed: true, recommended: true, bytes: 33_180_000 },
    { quality: 360, allowed: true, recommended: true, bytes: 11_430_000 },
  ],
  recommended: [720, 360],
  original_bytes: 100_000_000,
  duration_ms: 60_000,
  approximate: true,
};

describe('calidades', () => {
  test('lo que ocupa una elección y el total con el original', () => {
    expect(bytesDe(e1080, [720, 360])).toBe(44_610_000);
    expect(totalAlSubir(e1080, [360])).toBe(511_430_000);
    // Una calidad no permitida no suma aunque llegue elegida.
    const conBytes = {
      ...e720,
      options: [
        { quality: 1080, allowed: false, recommended: false, bytes: 999 },
        ...e720.options.slice(1),
      ],
    };
    expect(bytesDe(conBytes, [1080, 720])).toBe(33_180_000);
  });

  test('varios ficheros: se suma lo de los que la permiten', () => {
    const s = sumar([e1080, e720])!;
    expect(s.options.map((o) => [o.quality, o.allowed, o.bytes])).toEqual([
      [1080, true, 61_680_000],
      [720, true, 66_360_000],
      [360, true, 22_860_000],
    ]);
    expect(s.original_bytes).toBe(600_000_000);
    expect(s.approximate).toBe(true);
    expect(s.recommended).toEqual([1080, 720, 360]);
    expect(sumar([])).toBeNull();
  });

  test('al menos una: desmarcar la última no la quita', () => {
    expect(alternar([720, 360], 360)).toEqual([720]);
    expect(alternar([720], 720)).toEqual([720]);
    expect(alternar([360], 1080)).toEqual([1080, 360]);
  });

  test('la recomendada es todas las que permite el original', () => {
    expect(recomendada(e720)).toEqual([720, 360]);
    expect(esRecomendada(e720, [360, 720])).toBe(true);
    expect(esRecomendada(e1080, [720, 360])).toBe(false);
  });

  test('qué se quita al cambiar', () => {
    expect(quitadas([1080, 720, 360], [720])).toEqual([1080, 360]);
    expect(quitadas([720], [1080, 720])).toEqual([]);
  });
});
