/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros del patrón de diapositiva (slide master):
 * sustitución de campos {n}/{total} (substituteMasterFields), cajas del patrón
 * resueltas por diapositiva (masterBoxesFor, con hideMaster) y cajas efectivas =
 * patrón detrás + propias delante (resolveSlideBoxes).
 * Ejecutar: cd apps/frontend && bun test packages/office/test/slides-master.test.ts
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  layoutConnectorsFor,
  masterBoxesFor,
  resolveSlideBoxes,
  substituteMasterFields,
} from '../src/editor/slides/SlidesCanvas';
import type { SlideBox, SlidesModel } from '../src/editor/types';

const tbox = (id: string, html: string): SlideBox => ({ id, x: 0, y: 0, w: 10, h: 10, html });
const imgBox = (id: string): SlideBox => ({
  id,
  kind: 'image',
  x: 0,
  y: 0,
  w: 10,
  h: 10,
  html: '',
  src: 'x',
});

const model = (over: Partial<SlidesModel> = {}): SlidesModel => ({
  slides: [
    { id: 's1', boxes: [tbox('b1', '<p>uno</p>')] },
    { id: 's2', boxes: [] },
  ],
  ...over,
});

test('substituteMasterFields: {n} y {total}, repetidos y ausentes', () => {
  expect(substituteMasterFields('<p>{n} de {total}</p>', 2, 5)).toBe('<p>2 de 5</p>');
  expect(substituteMasterFields('{n}{n}', 3, 9)).toBe('33');
  expect(substituteMasterFields('sin campos', 1, 1)).toBe('sin campos');
});

test('masterBoxesFor: sin patrón = vacío', () => {
  expect(masterBoxesFor(model(), 0)).toEqual([]);
});

test('masterBoxesFor: sustituye campos por diapositiva (n = índice+1, total)', () => {
  const m = model({ master: { boxes: [tbox('m1', '<p>{n}/{total}</p>')] } });
  expect(masterBoxesFor(m, 0)[0]!.html).toBe('<p>1/2</p>');
  expect(masterBoxesFor(m, 1)[0]!.html).toBe('<p>2/2</p>');
});

test('masterBoxesFor: hideMaster oculta el patrón solo en esa diapositiva', () => {
  const m = model({ master: { boxes: [tbox('m1', 'x')] } });
  m.slides[1]!.hideMaster = true;
  expect(masterBoxesFor(m, 0)).toHaveLength(1);
  expect(masterBoxesFor(m, 1)).toEqual([]);
});

test('masterBoxesFor: las cajas no-texto pasan intactas (sin sustitución)', () => {
  const m = model({ master: { boxes: [imgBox('m-img')] } });
  expect(masterBoxesFor(m, 0)[0]).toEqual(imgBox('m-img'));
});

test('resolveSlideBoxes: patrón detrás + propias delante; fuera de rango = vacío', () => {
  const m = model({ master: { boxes: [tbox('m1', '<p>{n}</p>')] } });
  const out = resolveSlideBoxes(m, 0);
  expect(out.map((b) => b.id)).toEqual(['m1', 'b1']); // master primero (detrás)
  expect(out[0]!.html).toBe('<p>1</p>');
  expect(resolveSlideBoxes(m, 9)).toEqual([]);
});

test('resolveSlideBoxes: sin patrón = solo las propias', () => {
  expect(resolveSlideBoxes(model(), 0).map((b) => b.id)).toEqual(['b1']);
});

// Las formas de un diseño (un logo, una franja, una línea de un .pptx importado) se guardan una
// vez en el diseño y se pintan, encima del patrón, en cada diapositiva que lo lleva; un diseño
// que oculta el patrón deja solo las suyas, y la diapositiva que lo oculta, ninguna.
test('masterBoxesFor: las formas del diseño de la diapositiva, encima del patrón', () => {
  const linea = {
    id: 'l1',
    kind: 'straight' as const,
    from: { x: 5, y: 20 },
    to: { x: 95, y: 20 },
  };
  const m = model({
    master: {
      boxes: [tbox('m1', '<p>{n}</p>')],
      layouts: [
        {
          id: 'portada',
          name: 'Portada',
          boxes: [],
          decor: [imgBox('logo'), tbox('pie', '<p>{total}</p>')],
          connectors: [linea],
        },
        { id: 'limpio', name: 'Limpio', boxes: [], decor: [imgBox('marca')], hideMaster: true },
      ],
    },
  });
  m.slides[0]!.layout = 'portada';
  m.slides[1]!.layout = 'limpio';
  expect(masterBoxesFor(m, 0).map((b) => b.id)).toEqual(['m1', 'logo', 'pie']);
  expect(masterBoxesFor(m, 0)[2]!.html).toBe('<p>2</p>'); // sus campos, como los del patrón
  expect(layoutConnectorsFor(m, 0)).toEqual([linea]);
  expect(masterBoxesFor(m, 1).map((b) => b.id)).toEqual(['marca']); // el diseño oculta el patrón
  m.slides[0]!.hideMaster = true;
  expect(masterBoxesFor(m, 0)).toEqual([]);
  expect(layoutConnectorsFor(m, 0)).toEqual([]);
});
