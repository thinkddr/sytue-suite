/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la parte PURA de la protección de Diapos (slides/proteger.ts): la misma regla
 * que cumple el servidor (`domain/proteccion_diapos.go`), escrita aparte para avisar antes de
 * enviar. Los casos de abajo son los mismos de `TestDiapasBloqueadasCajasYPatron`.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/slides-protect.test.ts
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  alternarId,
  cambioProtegido,
  canonico,
  conBloqueadoOriginal,
  sanitizeSlidesProtection,
} from '../src/editor/slides/proteger';
import type { SlideBox, SlidesModel } from '../src/editor/types';

const caja = (id: string, html: string): SlideBox => ({ id, x: 10, y: 10, w: 50, h: 20, html });

function modelo(protect: SlidesModel['protect'], extra: Partial<SlidesModel> = {}): SlidesModel {
  return {
    slides: [
      { id: 's1', boxes: [caja('b1', 'a')] },
      { id: 's2', boxes: [caja('b2', 'b')] },
      { id: 's3', boxes: [caja('b3', 'c'), caja('b4', 'd')] },
    ],
    theme: 'carbon',
    master: { boxes: [caja('m1', 'logo')] },
    ...(protect ? { protect } : {}),
    ...extra,
  };
}

const PROTEGE = { slides: ['s1'], boxes: ['b3'], master: true };
const base = modelo(PROTEGE);

function con(f: (m: SlidesModel) => void): SlidesModel {
  const m = structuredClone(base);
  f(m);
  return m;
}

test('lo que se puede cambiar con la presentación protegida', () => {
  expect(
    cambioProtegido(
      base,
      con((m) => (m.slides[1]!.boxes[0]!.html = 'b nueva')),
    ),
  ).toBe(false);
  expect(
    cambioProtegido(
      base,
      con((m) => m.slides.reverse()),
    ),
  ).toBe(false); // reordenar
  expect(
    cambioProtegido(
      base,
      con((m) => (m.slides[2]!.boxes[1]!.html = 'D nueva')),
    ),
  ).toBe(false);
  // Las mismas claves en otro orden no son un cambio.
  const reordenada = con((m) => {
    m.slides[0]!.boxes[0] = Object.fromEntries(
      Object.entries(m.slides[0]!.boxes[0]!).reverse(),
    ) as unknown as SlideBox;
  });
  expect(cambioProtegido(base, reordenada)).toBe(false);
});

test('lo que la protección fija', () => {
  const cambios: Record<string, SlidesModel> = {
    'editar la diapositiva bloqueada': con((m) => (m.slides[0]!.boxes[0]!.html = 'X')),
    'borrar la diapositiva bloqueada': con((m) => m.slides.shift()),
    'editar la caja bloqueada': con((m) => (m.slides[2]!.boxes[0]!.html = 'X')),
    'borrar la caja bloqueada': con((m) => m.slides[2]!.boxes.shift()),
    'mover la caja bloqueada a otra diapositiva': con((m) => {
      m.slides[1]!.boxes.push(m.slides[2]!.boxes.shift()!);
    }),
    'duplicar la caja bloqueada': con((m) => m.slides[2]!.boxes.push(caja('b3', 'copia'))),
    'duplicar la diapositiva bloqueada': con((m) => m.slides.unshift({ id: 's1', boxes: [] })),
    'añadir una caja a la diapositiva bloqueada': con((m) =>
      m.slides[0]!.boxes.push(caja('n', '')),
    ),
    'cambiar el tema': con((m) => (m.theme = 'arena')),
    'cambiar el patrón': con((m) => (m.master!.boxes[0]!.html = 'otro')),
    'quitar el patrón': con((m) => delete m.master),
    'cambiar el tamaño': con((m) => (m.size = { preset: '4:3' })),
    'quitar la protección': con((m) => delete m.protect),
    'desbloquear una diapositiva': con((m) => (m.protect!.slides = [])),
    'bloquear una más': con((m) => m.protect!.slides!.push('s2')),
  };
  for (const [nombre, m] of Object.entries(cambios))
    expect([nombre, cambioProtegido(base, m)]).toEqual([nombre, true]);
});

test('proteger es de cualquiera; sin protección nada se veta', () => {
  const libre = modelo(undefined);
  expect(cambioProtegido(libre, base)).toBe(false);
  expect(
    cambioProtegido(
      libre,
      con((m) => (m.slides[0]!.boxes[0]!.html = 'X')),
    ),
  ).toBe(false);
});

test('hasPassword lo fija el servidor y no cuenta como cambio de la protección', () => {
  const conClave = modelo({ ...PROTEGE, hasPassword: true });
  expect(cambioProtegido(conClave, modelo(PROTEGE))).toBe(false);
});

test('lo bloqueado se guarda tal cual lo trajo el servidor', () => {
  // El editor sanea al cargar: la diapositiva bloqueada queda con otra forma que la original.
  const original = modelo(PROTEGE);
  original.slides[0] = {
    id: 's1',
    boxes: [{ ...caja('b1', 'a'), extra: 'dato del servidor' } as SlideBox],
  };
  const saneado = structuredClone(base);
  saneado.slides[1]!.boxes[0]!.html = 'editada por quien no puede tocar s1';
  const salida = conBloqueadoOriginal(saneado, original);
  expect(canonico(salida.slides[0])).toBe(canonico(original.slides[0]));
  expect(salida.slides[1]!.boxes[0]!.html).toBe('editada por quien no puede tocar s1');
  expect(cambioProtegido(original, salida)).toBe(false);
  // La protección sale como la trajo el servidor, aunque el editor la haya saneado.
  const sucia = { ...original, protect: { ...PROTEGE, slides: ['s1', 's1'] } } as SlidesModel;
  expect(conBloqueadoOriginal(saneado, sucia).protect).toEqual(sucia.protect);
  // Sin protección, o sin original, no toca nada.
  expect(conBloqueadoOriginal(saneado, null)).toBe(saneado);
  expect(conBloqueadoOriginal(saneado, modelo(undefined))).toBe(saneado);
});

test('sanitizeSlidesProtection solo deja lo bien formado', () => {
  expect(sanitizeSlidesProtection(undefined)).toBeUndefined();
  expect(sanitizeSlidesProtection([1])).toBeUndefined();
  expect(
    sanitizeSlidesProtection({
      slides: ['a', 'a', 3, ''],
      boxes: 'x',
      master: 'sí',
      hasPassword: true,
    }),
  ).toEqual({ hasPassword: true, slides: ['a'] });
  expect(sanitizeSlidesProtection({})).toEqual({});
});

test('alternarId', () => {
  expect(alternarId(undefined, 'a')).toEqual(['a']);
  expect(alternarId(['a', 'b'], 'a')).toEqual(['b']);
  expect(alternarId(['a'], 'a')).toBeUndefined();
});
