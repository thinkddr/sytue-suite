/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers PUROS de «Organizar objetos» (slides/organize.ts):
 * z-order (reorderZ), alineación (alignBoxes), distribución (distributeBoxes) y
 * grupos (expandGroups/groupBoxes/ungroupBoxes) + boundingBox.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/slides-organize.test.ts
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  alignBoxes,
  boundingBox,
  distributeBoxes,
  expandGroups,
  groupBoxes,
  reorderZ,
  ungroupBoxes,
} from '../src/editor/slides/organize';
import type { SlideBox } from '../src/editor/types';

const box = (id: string, x: number, y: number, w: number, h: number, groupId?: string): SlideBox =>
  groupId !== undefined ? { id, x, y, w, h, html: '', groupId } : { id, x, y, w, h, html: '' };

const ids = (bs: SlideBox[]): string[] => bs.map((b) => b.id);
const byId = (bs: SlideBox[], id: string): SlideBox => bs.find((b) => b.id === id)!;

/* ---------- boundingBox ---------- */

test('boundingBox envuelve el conjunto; vacío = origen', () => {
  expect(boundingBox([box('a', 10, 20, 20, 10), box('b', 50, 40, 10, 30)])).toEqual({
    x: 10,
    y: 20,
    w: 50,
    h: 50,
  });
  expect(boundingBox([])).toEqual({ x: 0, y: 0, w: 0, h: 0 });
});

/* ---------- Z-order ---------- */

const abcd = (): SlideBox[] => [
  box('a', 0, 0, 10, 10),
  box('b', 0, 0, 10, 10),
  box('c', 0, 0, 10, 10),
  box('d', 0, 0, 10, 10),
];

test('reorderZ front/back llevan al extremo (una caja)', () => {
  expect(ids(reorderZ(abcd(), ['b'], 'front'))).toEqual(['a', 'c', 'd', 'b']);
  expect(ids(reorderZ(abcd(), ['c'], 'back'))).toEqual(['c', 'a', 'b', 'd']);
});

test('reorderZ forward/backward saltan un vecino (una caja)', () => {
  expect(ids(reorderZ(abcd(), ['b'], 'forward'))).toEqual(['a', 'c', 'b', 'd']);
  expect(ids(reorderZ(abcd(), ['c'], 'backward'))).toEqual(['a', 'c', 'b', 'd']);
});

test('reorderZ no-op en los extremos', () => {
  expect(ids(reorderZ(abcd(), ['d'], 'forward'))).toEqual(['a', 'b', 'c', 'd']);
  expect(ids(reorderZ(abcd(), ['a'], 'backward'))).toEqual(['a', 'b', 'c', 'd']);
});

test('reorderZ mueve un bloque conservando su orden interno; una selección dispersa se compacta', () => {
  expect(ids(reorderZ(abcd(), ['b', 'c'], 'forward'))).toEqual(['a', 'd', 'b', 'c']);
  expect(ids(reorderZ(abcd(), ['a', 'c'], 'front'))).toEqual(['b', 'd', 'a', 'c']);
});

/* ---------- Alineación ---------- */

test('alignBoxes al lienzo (izquierda/derecha/centro horizontal)', () => {
  const bs = [box('a', 30, 5, 20, 10), box('b', 60, 40, 10, 10)];
  expect(byId(alignBoxes(bs, ['a', 'b'], 'left', 'canvas'), 'a').x).toBe(0);
  expect(byId(alignBoxes(bs, ['a', 'b'], 'left', 'canvas'), 'b').x).toBe(0);
  expect(byId(alignBoxes(bs, ['a', 'b'], 'right', 'canvas'), 'a').x).toBe(80); // 100-20
  expect(byId(alignBoxes(bs, ['a', 'b'], 'right', 'canvas'), 'b').x).toBe(90); // 100-10
  expect(byId(alignBoxes(bs, ['a', 'b'], 'hcenter', 'canvas'), 'a').x).toBe(40); // (100-20)/2
  expect(byId(alignBoxes(bs, ['a', 'b'], 'hcenter', 'canvas'), 'b').x).toBe(45); // (100-10)/2
});

test('alignBoxes a la selección (vertical: arriba/abajo/centro)', () => {
  const bs = [box('a', 10, 10, 20, 20), box('b', 50, 40, 10, 10)]; // bbox y=10..50 (h=40)
  expect(byId(alignBoxes(bs, ['a', 'b'], 'top', 'selection'), 'a').y).toBe(10);
  expect(byId(alignBoxes(bs, ['a', 'b'], 'top', 'selection'), 'b').y).toBe(10);
  expect(byId(alignBoxes(bs, ['a', 'b'], 'bottom', 'selection'), 'a').y).toBe(30); // 50-20
  expect(byId(alignBoxes(bs, ['a', 'b'], 'bottom', 'selection'), 'b').y).toBe(40); // 50-10
  expect(byId(alignBoxes(bs, ['a', 'b'], 'vmiddle', 'selection'), 'a').y).toBe(20); // 10+(40-20)/2
  expect(byId(alignBoxes(bs, ['a', 'b'], 'vmiddle', 'selection'), 'b').y).toBe(25); // 10+(40-10)/2
});

test('alignBoxes solo toca las seleccionadas y no muta el original', () => {
  const bs = [box('a', 30, 5, 20, 10), box('b', 60, 40, 10, 10)];
  const out = alignBoxes(bs, ['a'], 'left', 'canvas');
  expect(byId(out, 'b')).toBe(bs[1]!); // sin tocar (misma referencia)
  expect(bs[0]!.x).toBe(30); // el original no cambia
});

/* ---------- Distribución ---------- */

test('distributeBoxes horizontal: extremos fijos, hueco uniforme', () => {
  const bs = [box('a', 0, 0, 10, 10), box('b', 30, 0, 20, 10), box('c', 80, 0, 10, 10)];
  const out = distributeBoxes(bs, ['a', 'b', 'c'], 'h');
  expect(byId(out, 'a').x).toBe(0); // extremo izq. fijo
  expect(byId(out, 'c').x).toBe(80); // extremo der. fijo
  expect(byId(out, 'b').x).toBe(35); // hueco (90-40)/2 = 25 → 0+10+25
});

test('distributeBoxes vertical usa y/h', () => {
  const bs = [box('a', 0, 0, 10, 10), box('b', 0, 30, 10, 20), box('c', 0, 80, 10, 10)];
  expect(byId(distributeBoxes(bs, ['a', 'b', 'c'], 'v'), 'b').y).toBe(35);
});

test('distributeBoxes no-op con menos de 3 cajas', () => {
  const bs = [box('a', 0, 0, 10, 10), box('b', 80, 0, 10, 10)];
  expect(distributeBoxes(bs, ['a', 'b'], 'h')).toBe(bs);
});

/* ---------- Grupos ---------- */

const grouped = (): SlideBox[] => [
  box('a', 0, 0, 10, 10, 'g1'),
  box('b', 0, 0, 10, 10, 'g1'),
  box('c', 0, 0, 10, 10),
  box('d', 0, 0, 10, 10, 'g2'),
  box('e', 0, 0, 10, 10, 'g2'),
];

test('expandGroups incluye a todos los miembros del grupo tocado', () => {
  expect(expandGroups(grouped(), ['a'])).toEqual(['a', 'b']);
  expect(expandGroups(grouped(), ['c'])).toEqual(['c']); // suelta
  expect(expandGroups(grouped(), ['a', 'd'])).toEqual(['a', 'b', 'd', 'e']);
  expect(expandGroups(grouped(), ['c', 'zzz'])).toEqual(['c']); // ids inexistentes se descartan
});

test('groupBoxes etiqueta con groupId común y compacta a bloque contiguo', () => {
  const out = groupBoxes(abcd(), ['b', 'd'], 'G');
  expect(ids(out)).toEqual(['a', 'c', 'b', 'd']); // contiguas donde estaba la más al frente (d)
  expect(byId(out, 'b').groupId).toBe('G');
  expect(byId(out, 'd').groupId).toBe('G');
  expect(byId(out, 'a').groupId).toBeUndefined();
});

test('groupBoxes no-op con menos de 2 cajas', () => {
  const bs = abcd();
  expect(groupBoxes(bs, ['b'], 'G')).toBe(bs);
});

test('ungroupBoxes quita el groupId de todo el grupo tocado', () => {
  const out = ungroupBoxes(grouped(), ['a']); // toca g1 (a,b)
  expect(byId(out, 'a').groupId).toBeUndefined();
  expect(byId(out, 'b').groupId).toBeUndefined();
  expect(byId(out, 'd').groupId).toBe('g2'); // otros grupos intactos
});

test('agrupar y luego expandir: seleccionar un miembro trae a todos', () => {
  const g = groupBoxes(abcd(), ['a', 'c'], 'G');
  expect(new Set(expandGroups(g, ['a']))).toEqual(new Set(['a', 'c']));
});
