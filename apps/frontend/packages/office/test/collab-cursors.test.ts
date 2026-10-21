/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lógica pura de los cursores remotos: color estable por peer, validación de la
 * posición opaca por tipo, y mapeo posición → píxeles/cajas. Sin DOM.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/collab-cursors.test.ts
 */
import { expect, test } from 'bun:test';
import {
  cursorPosKey,
  parseCursorPos,
  parsePeerCursor,
  peerColor,
  peerName,
  sheetPeerBox,
  slidePeerBoxes,
} from '../src/editor/state/cursors';
import type { SlideBox } from '../src/editor/types';

test('peerColor: rango 1..6, determinista y con reparto (no todo el mismo tono)', () => {
  for (const id of ['', 'a', 'user-42', 'sub|google|x', '☺']) {
    const c = peerColor(id);
    expect(c).toMatch(/^var\(--sytue-avatar-[1-6]\)$/);
    expect(peerColor(id)).toBe(c); // determinista
  }
  // Reparto: 6 subs distintos no deben colapsar a un único color.
  const tones = new Set(['s0', 's1', 's2', 's3', 's4', 's5'].map(peerColor));
  expect(tones.size).toBeGreaterThan(1);
});

test('parseCursorPos: sheet válido y rechazo de campos no numéricos', () => {
  expect(parseCursorPos({ kind: 'sheet', sheet: 0, c1: 1, r1: 2, c2: 3, r2: 4 })).toEqual({
    kind: 'sheet',
    sheet: 0,
    c1: 1,
    r1: 2,
    c2: 3,
    r2: 4,
  });
  expect(parseCursorPos({ kind: 'sheet', sheet: 0, c1: 1, r1: 2, c2: 3 })).toBeNull(); // falta r2
  expect(parseCursorPos({ kind: 'sheet', sheet: 0, c1: '1', r1: 2, c2: 3, r2: 4 })).toBeNull();
  expect(parseCursorPos({ kind: 'sheet', sheet: 0, c1: NaN, r1: 2, c2: 3, r2: 4 })).toBeNull();
});

test('parseCursorPos: doc y slide; slide filtra ids no-string; kind desconocido → null', () => {
  expect(parseCursorPos({ kind: 'doc', anchor: 5, focus: 9 })).toEqual({
    kind: 'doc',
    anchor: 5,
    focus: 9,
  });
  expect(parseCursorPos({ kind: 'slide', slide: 2, boxes: ['a', 3, 'b', null] })).toEqual({
    kind: 'slide',
    slide: 2,
    boxes: ['a', 'b'],
  });
  expect(parseCursorPos({ kind: 'slide', slide: 1 })).toEqual({
    kind: 'slide',
    slide: 1,
    boxes: [],
  });
  expect(parseCursorPos({ kind: 'zzz' })).toBeNull();
  expect(parseCursorPos(null)).toBeNull();
  expect(parseCursorPos('nope')).toBeNull();
});

test('parsePeerCursor: exige sub + posición válida; color derivado del sub', () => {
  const pc = parsePeerCursor({
    sub: 'u1',
    name: 'Ada',
    cursor: { kind: 'doc', anchor: 0, focus: 3 },
  });
  expect(pc).toEqual({
    id: 'u1',
    name: 'Ada',
    color: peerColor('u1'),
    pos: { kind: 'doc', anchor: 0, focus: 3 },
  });
  // Roster sin cursor (join/leave) o sub ausente → no es un cursor de peer.
  expect(parsePeerCursor({ sub: 'u1' })).toBeNull();
  expect(parsePeerCursor({ name: 'Ada', cursor: { kind: 'doc', anchor: 0, focus: 0 } })).toBeNull();
  // name ausente → cadena vacía (el render pondrá la reserva).
  expect(parsePeerCursor({ sub: 'u2', cursor: { kind: 'doc', anchor: 1, focus: 1 } })?.name).toBe(
    '',
  );
});

test('peerName: reserva cuando viene vacío', () => {
  expect(peerName('  ')).toBe('Colaborador');
  expect(peerName('Ada Lovelace')).toBe('Ada Lovelace');
});

test('cursorPosKey: cambia al moverse, estable si no', () => {
  const a = cursorPosKey({ kind: 'sheet', sheet: 0, c1: 1, r1: 1, c2: 1, r2: 1 });
  const b = cursorPosKey({ kind: 'sheet', sheet: 0, c1: 2, r1: 1, c2: 2, r2: 1 });
  expect(a).not.toBe(b);
  expect(cursorPosKey({ kind: 'slide', slide: 1, boxes: ['x', 'y'] })).toBe('p1:x,y');
});

test('sheetPeerBox: rango → caja en px con la geometría de la rejilla', () => {
  // colX: columnas de 96px desde x=40 (HEAD_W), sin canalón; 3 columnas + borde.
  const colX = [40, 136, 232, 328];
  const visRows = [0, 1, 2, 3, 4];
  const geom = { colX, visRows, oy: 24, cellH: 24 };
  // Rango B2:C3 (c1=1,r1=1,c2=2,r2=2).
  expect(sheetPeerBox({ kind: 'sheet', sheet: 0, c1: 1, r1: 1, c2: 2, r2: 2 }, geom)).toEqual({
    left: 136,
    top: 24 + 24, // OY + vi(1)*cellH
    width: 328 - 136, // colX[3] - colX[1]
    height: 48, // dos filas
  });
});

test('sheetPeerBox: fila oculta (colapsada/filtrada) → null; clamp de columnas fuera de rango', () => {
  const colX = [40, 136, 232, 328];
  const geom = { colX, visRows: [0, 2, 3], oy: 24, cellH: 24 };
  // Fila 1 no está en visRows → null.
  expect(sheetPeerBox({ kind: 'sheet', sheet: 0, c1: 0, r1: 1, c2: 0, r2: 1 }, geom)).toBeNull();
  // Columna 99 se recorta a la última (índice 2 → colX[3] borde derecho).
  const box = sheetPeerBox({ kind: 'sheet', sheet: 0, c1: 0, r1: 0, c2: 99, r2: 0 }, geom);
  expect(box).toEqual({ left: 40, top: 24, width: 288, height: 24 });
});

test('slidePeerBoxes: cruza ids con cajas presentes, ignora inexistentes, conserva orden', () => {
  const boxes: SlideBox[] = [
    { id: 'a', x: 0, y: 0, w: 10, h: 10, html: '' },
    { id: 'b', x: 20, y: 30, w: 40, h: 50, html: '' },
    { id: 'c', x: 5, y: 5, w: 5, h: 5, html: '' },
  ];
  expect(slidePeerBoxes(boxes, ['b', 'zzz', 'a'])).toEqual([
    { id: 'a', x: 0, y: 0, w: 10, h: 10 },
    { id: 'b', x: 20, y: 30, w: 40, h: 50 },
  ]);
  expect(slidePeerBoxes(boxes, [])).toEqual([]);
});
