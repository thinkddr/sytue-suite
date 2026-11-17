/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Diapositivas ocultas (se saltan al presentar): la tabla de navegación compartida con la
 * app nativa (`vectores/diapos-ocultas.json`, que lee también
 * `apps/fyne/diapos/ocultas_test.go`) y lo propio de la web: el carril y el atajo.
 * Ejecutar: cd apps/frontend/packages/office && bun test test/slides-ocultas.test.ts
 */
import { describe, expect, test } from 'bun:test';
import {
  actionTarget,
  isHideSlideKey,
  presentCommand,
  presentKeyTarget,
  toggleSlideHidden,
} from '../src/editor/slides/SlidesCanvas';
import { hiddenMask, nextShown, presentAdvance } from '../src/editor/slides/anim';
import { posicionDe } from '../src/editor/slides/cast';
import type { Slide, SlideBox } from '../src/editor/types';
import vectores from './vectores/diapos-ocultas.json';

/** Una caja con `n` animaciones de entrada, cada una en su clic: `n` pasos. */
const conPasos = (n: number): SlideBox[] =>
  n === 0
    ? []
    : [
        {
          id: 'b',
          x: 0,
          y: 0,
          w: 10,
          h: 10,
          html: '',
          anims: Array.from({ length: n }, (_, k) => ({
            type: 'fade' as const,
            order: k + 1,
            trigger: 'click' as const,
          })),
        },
      ];

const baraja = (ocultas: boolean[], pasos: number[]): Slide[] =>
  ocultas.map((o, i) => ({
    id: `s${i}`,
    boxes: conPasos(pasos[i] ?? 0),
    ...(o ? { hidden: true } : {}),
  }));

describe('vectores compartidos con Fyne', () => {
  for (const c of vectores.casos) {
    test(c.nombre, () => {
      const slides = baraja(c.ocultas, c.pasos);
      const last = slides.length - 1;
      const mask = hiddenMask(slides);
      const av = presentAdvance(slides, c.diapo, c.paso, 1);
      expect([av.slide, av.step]).toEqual(c.avanzar);
      const re = presentAdvance(slides, c.diapo, c.paso, -1);
      expect([re.slide, re.step]).toEqual(c.retroceder);
      expect(presentKeyTarget('ArrowRight', c.diapo, last, mask)).toBe(c.teclas.siguiente);
      expect(presentKeyTarget('PageDown', c.diapo, last, mask)).toBe(c.teclas.siguiente);
      expect(presentKeyTarget('ArrowLeft', c.diapo, last, mask)).toBe(c.teclas.anterior);
      expect(presentKeyTarget('PageUp', c.diapo, last, mask)).toBe(c.teclas.anterior);
      expect(presentKeyTarget('Home', c.diapo, last, mask)).toBe(c.teclas.inicio);
      expect(presentKeyTarget('End', c.diapo, last, mask)).toBe(c.teclas.fin);
      expect(nextShown(mask, c.diapo, 1) ?? -1).toBe(c.moderador);
      const p = posicionDe(slides, c.diapo, 'd1');
      expect(p ? [p.index, p.total] : null).toEqual(c.cast);
    });
  }
});

describe('presentar con ocultas', () => {
  const slides = baraja([false, true, false, true], [0, 0, 0, 0]);
  const mask = hiddenMask(slides);

  test('número + Intro sí lleva a una oculta', () => {
    const r = presentCommand('Enter', {
      idx: 0,
      last: 3,
      digits: '2',
      blanked: false,
      hidden: mask,
    });
    expect(r.cmd).toEqual({ kind: 'go', to: 1 });
  });

  test('Fin por teclado va a la última que se enseña', () => {
    const r = presentCommand('End', { idx: 0, last: 3, digits: '', blanked: false, hidden: mask });
    expect(r.cmd).toEqual({ kind: 'go', to: 2 });
  });

  test('los enlaces relativos saltan las ocultas; el que nombra un número, no', () => {
    expect(actionTarget('next', 0, 3, mask)).toBe(2);
    expect(actionTarget('prev', 2, 3, mask)).toBe(0);
    expect(actionTarget('last', 0, 3, mask)).toBe(2);
    expect(actionTarget('first', 2, 3, mask)).toBe(0);
    expect(actionTarget(2, 0, 3, mask)).toBe(1);
  });

  test('sin máscara, presentKeyTarget y actionTarget son los de siempre', () => {
    expect(presentKeyTarget('ArrowRight', 0, 3)).toBe(1);
    expect(presentKeyTarget('End', 0, 3)).toBe(3);
    expect(actionTarget('next', 0, 3)).toBe(1);
  });
});

describe('ocultar desde el carril', () => {
  test('toggleSlideHidden marca y desmarca sin mutar y sin dejar `hidden: false`', () => {
    const slides = baraja([false, false], [0, 0]);
    const una = toggleSlideHidden(slides, 1);
    expect(una[1]!.hidden).toBe(true);
    expect(slides[1]!.hidden).toBeUndefined();
    expect(una[0]).toBe(slides[0]!); // las demás conservan su identidad
    const otra = toggleSlideHidden(una, 1);
    expect('hidden' in otra[1]!).toBe(false);
    expect(toggleSlideHidden(slides, 5)).toBe(slides);
  });

  test('Ctrl+Mayús+H (o Cmd) es el atajo; con Alt o sin Mayúsculas, no', () => {
    const k = { key: 'H', ctrlKey: true, metaKey: false, shiftKey: true, altKey: false };
    expect(isHideSlideKey(k)).toBe(true);
    expect(isHideSlideKey({ ...k, ctrlKey: false, metaKey: true })).toBe(true);
    expect(isHideSlideKey({ ...k, altKey: true })).toBe(false);
    expect(isHideSlideKey({ ...k, shiftKey: false, key: 'h' })).toBe(false);
  });
});
