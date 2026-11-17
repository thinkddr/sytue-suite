/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del portapapeles de cajas de Diapos (slides/portapapeles.ts): qué se recorta
 * (selección + conectores internos), el formato del texto y su lectura defensiva, y la
 * reasignación al pegar (ids de caja, grupo y conector nuevos, anclas, desplazamiento y
 * orden de animación).
 * Ejecutar: cd apps/frontend && bun test packages/office/test/slides-portapapeles.test.ts
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  desplazamientoPegado,
  leerRecorte,
  pegarRecorte,
  PREFIJO_RECORTE,
  recortar,
  serializarRecorte,
} from '../src/editor/slides/portapapeles';
import type { SlideBox, SlideConnector } from '../src/editor/types';
import { calcularVectores } from './vectores/diapos-portapapeles';

const box = (id: string, x = 10, y = 10, extra: Partial<SlideBox> = {}): SlideBox => ({
  id,
  x,
  y,
  w: 20,
  h: 10,
  html: `<p>${id}</p>`,
  ...extra,
});
const conn = (id: string, from?: string, to?: string): SlideConnector => ({
  id,
  kind: 'straight',
  from: from ? { boxId: from } : { x: 5, y: 5 },
  to: to ? { boxId: to } : { x: 50, y: 50 },
});

/** Ids deterministas: n1, n2, … */
const ids = (): (() => string) => {
  let i = 0;
  return () => `n${++i}`;
};

test('recortar: cajas en orden de apilado y solo conectores internos', () => {
  const boxes = [box('a'), box('b'), box('c')];
  const cs = [conn('ab', 'a', 'b'), conn('bc', 'b', 'c'), conn('al', 'a'), conn('libre')];
  const r = recortar(boxes, cs, ['b', 'a']); // el orden de apilado manda, no el de selección
  expect(r.boxes.map((b) => b.id)).toEqual(['a', 'b']);
  // ab (interno) y al (a ↔ punto libre); bc deja fuera a c y «libre» no se ancla a nada.
  expect(r.connectors.map((c) => c.id)).toEqual(['ab', 'al']);
});

test('serializar y leer: ida y vuelta con prefijo de versión', () => {
  const r = recortar([box('a'), box('b')], [conn('ab', 'a', 'b')], ['a', 'b']);
  const txt = serializarRecorte(r);
  expect(txt.startsWith(PREFIJO_RECORTE)).toBe(true);
  expect(leerRecorte(txt)).toEqual(r);
});

test('leerRecorte: rechaza texto ajeno, JSON roto y cajas sin forma de caja', () => {
  expect(leerRecorte('hola')).toBeNull();
  expect(leerRecorte('{"boxes":[]}')).toBeNull(); // sin prefijo
  expect(leerRecorte(`${PREFIJO_RECORTE}{roto`)).toBeNull();
  expect(leerRecorte(`${PREFIJO_RECORTE}{"boxes":[{"id":"x"}]}`)).toBeNull();
  const mixto = leerRecorte(
    `${PREFIJO_RECORTE}${JSON.stringify({
      boxes: [box('a'), { id: 'mal', x: 'a' }],
      connectors: [conn('am', 'a', 'mal')],
    })}`,
  );
  expect(mixto?.boxes.map((b) => b.id)).toEqual(['a']);
  // El extremo hacia una caja que no viajó deja de estar anclado.
  expect(mixto?.connectors[0]?.to.boxId).toBeUndefined();
});

test('pegar en otra diapositiva: ids nuevos, en su sitio y anclas reasignadas', () => {
  const r = recortar([box('a', 30, 40), box('b')], [conn('ab', 'a', 'b')], ['a', 'b']);
  const p = pegarRecorte([box('z')], [], r, { nuevoId: ids(), conConectores: true });
  expect(p.ids).toEqual(['n1', 'n2']);
  expect(p.boxes.map((b) => b.id)).toEqual(['z', 'n1', 'n2']);
  expect(p.boxes[1]).toMatchObject({ x: 30, y: 40, html: '<p>a</p>' });
  expect(p.connectors).toHaveLength(1);
  expect(p.connectors[0]).toMatchObject({ id: 'n3', from: { boxId: 'n1' }, to: { boxId: 'n2' } });
});

test('pegar sobre las originales: +2 % y escalonado en pegados sucesivos', () => {
  const orig = [box('a', 10, 10)];
  const r = recortar(orig, [], ['a']);
  expect(desplazamientoPegado([box('z', 50, 50)], r)).toBe(0);
  const p1 = pegarRecorte(orig, [], r, { nuevoId: ids(), conConectores: true });
  expect(p1.boxes[1]).toMatchObject({ x: 12, y: 12 });
  const p2 = pegarRecorte(p1.boxes, [], r, { nuevoId: () => 'otra', conConectores: true });
  expect(p2.boxes[2]).toMatchObject({ x: 14, y: 14 });
});

test('pegar varias veces en otra diapositiva también escalona', () => {
  const r = recortar([box('a', 10, 10)], [], ['a']);
  const p1 = pegarRecorte([box('z', 50, 50)], [], r, { nuevoId: ids(), conConectores: true });
  expect(p1.boxes[1]).toMatchObject({ x: 10, y: 10 }); // primer pegado: en su sitio
  const p2 = pegarRecorte(p1.boxes, [], r, { nuevoId: () => 'otra', conConectores: true });
  expect(p2.boxes[2]).toMatchObject({ x: 12, y: 12 });
});

test('pegar desplazado no saca la caja del lienzo y mueve los puntos libres', () => {
  const orig = [box('a', 80, 90)];
  const r = recortar(orig, [conn('al', 'a')], ['a']);
  const p = pegarRecorte(orig, [], r, { nuevoId: ids(), conConectores: true });
  expect(p.boxes[1]).toMatchObject({ x: 80, y: 90 }); // 100 - w, 100 - h
  expect(p.connectors[0]?.to).toEqual({ x: 52, y: 52 });
});

test('pegar un grupo: groupId nuevo y común; una caja suelta de un grupo pierde el grupo', () => {
  const g = [box('a', 10, 10, { groupId: 'g' }), box('b', 40, 10, { groupId: 'g' })];
  const p = pegarRecorte([], [], recortar(g, [], ['a', 'b']), {
    nuevoId: ids(),
    conConectores: true,
  });
  expect(p.boxes[0]?.groupId).toBe('n2');
  expect(p.boxes[1]?.groupId).toBe('n2');
  const solo = pegarRecorte([], [], recortar(g, [], ['a']), {
    nuevoId: ids(),
    conConectores: true,
  });
  expect(solo.boxes[0]?.groupId).toBeUndefined();
});

test('pegar: las animaciones van detrás de las del destino, renumeradas 1..N', () => {
  const dest = [box('z', 0, 0, { anims: [{ type: 'fade', order: 1 }] })];
  const src = [
    box('a', 0, 0, { anims: [{ type: 'appear', order: 2 }] }),
    box('b', 0, 0, { anims: [{ type: 'zoom', order: 1 }] }),
  ];
  const p = pegarRecorte(dest, [], recortar(src, [], ['a', 'b']), {
    nuevoId: ids(),
    conConectores: true,
  });
  const orden = (id: string): number[] =>
    (p.boxes.find((b) => b.id === id)?.anims ?? []).map((a) => a.order);
  expect(orden('z')).toEqual([1]);
  expect(orden('n2')).toEqual([2]); // b iba primero en el origen
  expect(orden('n1')).toEqual([3]);
});

test('pegar en el patrón: sin conectores', () => {
  const r = recortar([box('a'), box('b')], [conn('ab', 'a', 'b')], ['a', 'b']);
  const p = pegarRecorte([], [], r, { nuevoId: ids(), conConectores: false });
  expect(p.connectors).toEqual([]);
  expect(p.boxes).toHaveLength(2);
});

/*
 * Los VECTORES COMPARTIDOS con la app nativa (`vectores/diapos-portapapeles.ts`): lo que
 * la web calcula hoy tiene que ser lo guardado en `vectores/diapos-portapapeles.json`, que
 * es lo mismo que exige `apps/fyne/diapos/portapapeles_test.go` al pegado de Go. Si esto
 * se pone rojo tras un cambio DELIBERADO, se regenera el JSON y el lado de Go se ajusta.
 */
test('los vectores compartidos con la app nativa siguen dando lo mismo en la web', async () => {
  const guardados: unknown = await Bun.file(
    new URL('./vectores/diapos-portapapeles.json', import.meta.url),
  ).json();
  expect(JSON.parse(JSON.stringify(calcularVectores()))).toEqual(guardados);
});
