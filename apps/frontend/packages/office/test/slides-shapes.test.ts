/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la geometría PURA de las formas ampliadas (slides/shapes.ts):
 * vértices de los polígonos (triángulo, rombo, pentágono, hexágono, estrella,
 * bocadillo) y medidas/paths del cilindro.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/slides-shapes.test.ts
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import {
  cylinderBodyPath,
  cylinderDims,
  cylinderSeamPath,
  polygonAttr,
  polygonPoints,
} from '../src/editor/slides/shapes';

const W = 300;
const H = 200;

/** Todos los vértices caen dentro de la caja (con el margen de trazo). */
const within = (pts: readonly (readonly [number, number])[], w: number, h: number): boolean =>
  pts.every(([x, y]) => x >= 0 && x <= w && y >= 0 && y <= h);

test('polygonPoints: recuento de vértices por forma', () => {
  expect(polygonPoints('triangle', W, H)).toHaveLength(3);
  expect(polygonPoints('diamond', W, H)).toHaveLength(4);
  expect(polygonPoints('pentagon', W, H)).toHaveLength(5);
  expect(polygonPoints('hexagon', W, H)).toHaveLength(6);
  expect(polygonPoints('star', W, H)).toHaveLength(10); // 5 puntas × 2 (ext/int)
  expect(polygonPoints('callout', W, H)).toHaveLength(7);
});

test('polygonPoints: las formas del núcleo/cilindro no son polígonos ([])', () => {
  for (const s of ['rect', 'ellipse', 'line', 'arrow', 'cylinder'] as const)
    expect(polygonPoints(s, W, H)).toHaveLength(0);
});

test('polygonPoints: todos los vértices dentro de la caja', () => {
  for (const s of ['triangle', 'diamond', 'pentagon', 'hexagon', 'star', 'callout'] as const)
    expect(within(polygonPoints(s, W, H), W, H)).toBe(true);
});

test('triángulo: ápice centrado arriba, base abajo', () => {
  const [apex, br, bl] = polygonPoints('triangle', W, H);
  expect(apex![0]).toBeCloseTo(W / 2, 5); // ápice en el centro horizontal
  expect(apex![1]).toBeLessThan(bl![1]); // ápice por encima de la base
  expect(br![1]).toBeCloseTo(bl![1], 5); // base horizontal
  expect(br![0]).toBeGreaterThan(bl![0]);
});

test('rombo: cuatro puntas centradas en los ejes medios', () => {
  const [top, right, bottom, left] = polygonPoints('diamond', W, H);
  expect(top![0]).toBeCloseTo(W / 2, 5);
  expect(bottom![0]).toBeCloseTo(W / 2, 5);
  expect(right![1]).toBeCloseTo(H / 2, 5);
  expect(left![1]).toBeCloseTo(H / 2, 5);
  expect(top![1]).toBeLessThan(bottom![1]);
  expect(left![0]).toBeLessThan(right![0]);
});

test('estrella: radios alternos (punta > valle) respecto al centro', () => {
  const pts = polygonPoints('star', W, H);
  const cx = W / 2;
  const cy = H / 2;
  const rAt = (i: number): number => Math.hypot(pts[i]![0] - cx, pts[i]![1] - cy);
  // Índices pares = puntas (radio exterior), impares = valles (radio interior).
  expect(rAt(0)).toBeGreaterThan(rAt(1));
  expect(rAt(2)).toBeGreaterThan(rAt(1));
  expect(rAt(2)).toBeGreaterThan(rAt(3));
});

test('polygonAttr: pares x,y separados por espacio', () => {
  const attr = polygonAttr([
    [1, 2],
    [3.456, 4],
  ]);
  expect(attr).toBe('1,2 3.46,4'); // redondeo a centésima
});

test('cylinderDims: radios y tapas dentro de la caja (yt < yb)', () => {
  const d = cylinderDims(W, H);
  expect(d.cx).toBeCloseTo(W / 2, 5);
  expect(d.rx).toBeCloseTo(W / 2 - 2, 5);
  expect(d.ry).toBeGreaterThanOrEqual(3);
  expect(d.ry).toBeLessThanOrEqual(H / 2 - 2);
  expect(d.yt).toBeCloseTo(2 + d.ry, 5);
  expect(d.yb).toBeCloseTo(H - 2 - d.ry, 5);
  expect(d.yt).toBeLessThan(d.yb);
});

test('cylinderDims: caja muy plana no degenera (ry >= 3)', () => {
  const d = cylinderDims(120, 10);
  expect(d.ry).toBeGreaterThanOrEqual(3);
  expect(d.yt).toBeLessThanOrEqual(d.yb + 1e-9); // no se cruzan
});

test('cylinder paths: cuerpo cerrado con arcos; costura con arco', () => {
  const d = cylinderDims(W, H);
  const body = cylinderBodyPath(d);
  expect(body.startsWith('M')).toBe(true);
  expect(body).toContain('A'); // arcos de las tapas
  expect(body.trimEnd().endsWith('Z')).toBe(true); // silueta cerrada
  const seam = cylinderSeamPath(d);
  expect(seam.startsWith('M')).toBe(true);
  expect(seam).toContain('A');
  expect(seam).not.toContain('Z'); // solo trazo (no se rellena)
});
