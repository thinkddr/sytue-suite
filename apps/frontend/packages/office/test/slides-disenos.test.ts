/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los diseños de una presentación (slides/disenos.ts): saneado en la frontera
 * de confianza, alta/duplicado/cambio/baja, y el reflujo de un diseño sobre una diapositiva.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import {
  MAX_LAYOUTS,
  MAX_PLACEHOLDERS,
  addLayout,
  boxesFromLayout,
  defaultNewLayoutId,
  duplicateLayout,
  layoutsOf,
  mergeIntoLayout,
  nextLayoutId,
  patchLayout,
  removeLayout,
  sanitizeLayouts,
} from '../src/editor/slides/disenos';
import type { SlideBox, SlideLayout } from '../src/editor/types';

const caja = (id: string, extra: Partial<SlideBox> = {}): SlideBox => ({
  id,
  x: 8,
  y: 8,
  w: 84,
  h: 16,
  html: '<h1>Título</h1>',
  ...extra,
});
const diseno = (id: string, boxes: SlideBox[] = [caja('a')]): SlideLayout => ({
  id,
  name: `Diseño ${id}`,
  boxes,
});
let n = 0;
const nuevoId = (): string => `n${++n}`;

test('sanitizeLayouts: ids, repetidos, solo texto, dentro del lienzo y topes', () => {
  const out = sanitizeLayouts([
    diseno('portada'),
    diseno('portada'), // repetido
    { id: 'x" y="', name: 'id roto', boxes: [] },
    {
      id: 'imgs',
      name: 'x'.repeat(100),
      boxes: [caja('i', { kind: 'image', src: 'x' }), caja('t', { x: -5, y: 120, w: 500, h: NaN })],
    },
    null,
    'texto',
  ]);
  expect(out.map((l) => l.id)).toEqual(['portada', 'imgs']);
  expect(out[1]!.name).toHaveLength(60);
  expect(out[1]!.boxes).toEqual([{ id: 't', x: 0, y: 100, w: 100, h: 0, html: '<h1>Título</h1>' }]);
  const muchos = Array.from({ length: 40 }, (_, i) => diseno(`l${i}`));
  expect(sanitizeLayouts(muchos)).toHaveLength(MAX_LAYOUTS);
  const cajas = Array.from({ length: 30 }, (_, i) => caja(`c${i}`));
  expect(sanitizeLayouts([diseno('a', cajas)])[0]!.boxes).toHaveLength(MAX_PLACEHOLDERS);
  expect(sanitizeLayouts('x')).toEqual([]);
});

test('layoutsOf: sin lista del modelo, los de serie; con ella (aunque vacía), los suyos', () => {
  const serie = (): SlideLayout[] => [diseno('titulo')];
  expect(layoutsOf(undefined, serie).map((l) => l.id)).toEqual(['titulo']);
  expect(layoutsOf([], serie)).toEqual([]);
  expect(layoutsOf([diseno('mio')], serie).map((l) => l.id)).toEqual(['mio']);
});

test('addLayout / duplicateLayout / patchLayout / removeLayout', () => {
  const a = addLayout([], 'Portada', [caja('x')])!;
  expect(a.id).toBe('diseno-1');
  const b = duplicateLayout(a.layouts, 'diseno-1', 'Portada 2', nuevoId)!;
  expect(b.layouts.map((l) => l.id)).toEqual(['diseno-1', 'diseno-2']);
  expect(b.layouts[1]!.boxes[0]!.id).not.toBe(b.layouts[0]!.boxes[0]!.id); // cajas con ids propios
  expect(duplicateLayout(a.layouts, 'no-existe', 'x', nuevoId)).toBeNull();
  const c = patchLayout(b.layouts, 'diseno-2', { name: 'Cierre', boxes: [caja('z', { y: 40 })] });
  expect(c[1]!.name).toBe('Cierre');
  expect(c[1]!.boxes[0]!.y).toBe(40);
  expect(nextLayoutId(removeLayout(c, 'diseno-1'))).toBe('diseno-1');
  const lleno = Array.from({ length: MAX_LAYOUTS }, (_, i) => diseno(`l${i}`));
  expect(addLayout(lleno, 'otro', [])).toBeNull();
});

test('mergeIntoLayout: el texto entra por orden en los marcadores y lo demás se conserva', () => {
  const lay = diseno('dos', [
    caja('a'),
    caja('b', { html: '<p>cuerpo</p>', y: 30 }),
    caja('c', { html: '<p>otro</p>', y: 60 }),
  ]);
  const forma = caja('f', { kind: 'shape', shape: 'rect', html: '' });
  const out = mergeIntoLayout(
    lay,
    [caja('t1', { html: '<h1>Mi título</h1>' }), forma, caja('t2', { html: '<p>Mi texto</p>' })],
    nuevoId,
  );
  expect(out).toHaveLength(4); // 3 marcadores + la forma
  expect(out.map((b) => b.html)).toEqual([
    '<h1>Mi título</h1>',
    '<p>Mi texto</p>',
    '<p>otro</p>',
    '',
  ]);
  expect(out[3]).toBe(forma); // intacta, la misma
  expect(out.slice(0, 3).map((b) => b.y)).toEqual([8, 30, 60]);
  expect(boxesFromLayout(lay, nuevoId).every((b) => !['a', 'b', 'c'].includes(b.id))).toBe(true);
});

test('defaultNewLayoutId: título y cuerpo, si no el de la diapositiva activa, si no el primero con cuerpo', () => {
  const dos = diseno('dos', [caja('a'), caja('b')]);
  const una = diseno('una');
  expect(defaultNewLayoutId([una, diseno('titulo-cuerpo')], 'una')).toBe('titulo-cuerpo');
  expect(defaultNewLayoutId([dos, una], 'una')).toBe('una');
  expect(defaultNewLayoutId([una, dos], 'no-existe')).toBe('dos');
  expect(defaultNewLayoutId([una], undefined)).toBe('una');
  expect(defaultNewLayoutId([], 'x')).toBe('');
});

// Las formas de un diseño importado y si oculta el patrón pasan el saneado (lo mal formado,
// no), y un duplicado se las lleva.
test('sanitizeLayouts: las formas, las líneas y hideMaster del diseño', () => {
  const [l] = sanitizeLayouts([
    {
      id: 'portada',
      name: 'Portada',
      boxes: [],
      decor: [
        caja('logo', { kind: 'image', src: 'data:image/png;base64,AA' }),
        { id: 'x', html: '' },
        'basura',
      ],
      connectors: [{ id: 'c1', kind: 'straight', from: { x: 0, y: 10 }, to: { x: 100, y: 10 } }],
      hideMaster: true,
    },
  ]);
  expect(l!.decor?.map((b) => b.id)).toEqual(['logo']);
  expect(l!.connectors).toHaveLength(1);
  expect(l!.hideMaster).toBe(true);
  const r = duplicateLayout([l!], 'portada', 'Copia', () => 'n1');
  expect(r!.layouts[1]!.decor?.map((b) => b.id)).toEqual(['logo']);
  expect(r!.layouts[1]!.hideMaster).toBe(true);
});
