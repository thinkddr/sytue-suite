/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Organizar cajas en unidades arbitrarias (la escena trabaja en px, Diapos en %).
 * El grueso de los casos vive en packages/office/test/slides-organize.test.ts.
 */
import { expect, test } from 'bun:test';
import { alignBoxes, boundingBox, distributeBoxes, reorderZ } from './organizar.js';

const caja = (id: string, x: number, y: number, w: number, h: number) => ({ id, x, y, w, h });

test('boundingBox y z-order sobre elementos con más campos que la caja', () => {
  const l = [
    { ...caja('a', 0, 0, 10, 10), extra: 1 },
    caja('b', 20, 5, 10, 10),
    caja('c', 5, 5, 1, 1),
  ];
  expect(boundingBox(l)).toEqual({ x: 0, y: 0, w: 30, h: 15 });
  expect(reorderZ(l, ['a'], 'front').map((x) => x.id)).toEqual(['b', 'c', 'a']);
});

test('alinear al lienzo respeta SU tamaño, no 100', () => {
  const lienzo = { x: 0, y: 0, w: 800, h: 600 };
  const r = alignBoxes([caja('a', 10, 10, 100, 50)], ['a'], 'hcenter', lienzo, lienzo);
  expect(r[0]!.x).toBe(350);
  const fuera = alignBoxes(
    [caja('a', 10, 10, 100, 50)],
    ['a'],
    'left',
    { x: -50, y: 0, w: 10, h: 10 },
    lienzo,
  );
  expect(fuera[0]!.x).toBe(0); // el tope al lienzo manda
});

test('distribuir con 2 cajas no hace nada', () => {
  const l = [caja('a', 0, 0, 10, 10), caja('b', 50, 0, 10, 10)];
  expect(distributeBoxes(l, ['a', 'b'], 'h')).toBe(l);
});

/* Guías inteligentes: los vectores son compartidos con el espejo Go de Fyne. */
import { guiasDeAjuste } from './organizar.js';
import type { AnclaGuia, AjusteGuias, Rect } from './organizar.js';
import vectores from './guias.vectores.json';

interface CasoGuias {
  nombre: string;
  mov: Rect;
  otros: Rect[];
  lienzo: Rect;
  tolerancia: number | { x: number; y: number };
  anclas?: { x?: AnclaGuia[]; y?: AnclaGuia[] };
  esperado: AjusteGuias;
}

for (const c of vectores.casos as CasoGuias[]) {
  test(`guiasDeAjuste: ${c.nombre}`, () => {
    expect(guiasDeAjuste(c.mov, c.otros, c.lienzo, c.tolerancia, c.anclas)).toEqual(c.esperado);
  });
}

test('los vectores de guías son los mismos que prueba el espejo Go', async () => {
  const go = await Bun.file(
    new URL('../../../../fyne/diapos/testdata/guias.vectores.json', import.meta.url),
  ).json();
  expect(go).toEqual(vectores);
});
