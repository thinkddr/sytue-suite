/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la geometría PURA de los diagramas SmartArt (slides/smartart.ts):
 * reparto de nodos y conectores por disposición, con los casos 0/1/muchos items
 * y la construcción del árbol jerárquico (sangría, saltos y bosque).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import { smartArtLayout } from '../src/editor/slides/smartart';
import type { SmartArtItem } from '../src/editor/types';

const VW = 600;
const VH = 400;
const items = (n: number): SmartArtItem[] =>
  Array.from({ length: n }, (_, i) => ({ text: `Elemento ${i + 1}` }));

/** Todos los nodos caben dentro de la caja (con holgura de 0,5px por redondeos). */
function withinBox(geo: ReturnType<typeof smartArtLayout>): boolean {
  return geo.nodes.every(
    (nd) => nd.x >= -0.5 && nd.y >= -0.5 && nd.x + nd.w <= VW + 0.5 && nd.y + nd.h <= VH + 0.5,
  );
}

test('caja vacía o degenerada → geometría vacía (sin reventar)', () => {
  expect(smartArtLayout('list', [], VW, VH)).toEqual({ nodes: [], connectors: [] });
  expect(smartArtLayout('process', items(3), 0, VH).nodes).toHaveLength(0);
  expect(smartArtLayout('cycle', items(3), VW, 0).nodes).toHaveLength(0);
});

test('list: tarjetas apiladas, misma anchura, sin conectores, en orden vertical', () => {
  const geo = smartArtLayout('list', items(4), VW, VH);
  expect(geo.nodes).toHaveLength(4);
  expect(geo.connectors).toHaveLength(0);
  expect(geo.nodes.every((n) => n.shape === 'rect')).toBe(true);
  // Misma x y misma anchura; y estrictamente creciente (apiladas sin solaparse).
  const w0 = geo.nodes[0]!.w;
  for (let k = 1; k < geo.nodes.length; k++) {
    expect(geo.nodes[k]!.x).toBeCloseTo(geo.nodes[0]!.x, 6);
    expect(geo.nodes[k]!.w).toBeCloseTo(w0, 6);
    expect(geo.nodes[k]!.y).toBeGreaterThan(geo.nodes[k - 1]!.y + geo.nodes[k - 1]!.h - 0.001);
  }
  expect(withinBox(geo)).toBe(true);
});

test('list con 1 item: una sola tarjeta que llena el alto útil', () => {
  const geo = smartArtLayout('list', items(1), VW, VH);
  expect(geo.nodes).toHaveLength(1);
  expect(geo.nodes[0]!.h).toBeGreaterThan(VH * 0.7); // sin gaps, ocupa casi todo
  expect(withinBox(geo)).toBe(true);
});

test('process: cajas en fila con una flecha entre cada par contiguo', () => {
  const geo = smartArtLayout('process', items(3), VW, VH);
  expect(geo.nodes).toHaveLength(3);
  expect(geo.connectors).toHaveLength(2);
  expect(geo.connectors.every((c) => c.kind === 'arrow')).toBe(true);
  // Misma y/alto; x creciente; cada flecha nace en el borde derecho de una caja
  // y muere antes de la siguiente (a la misma altura, horizontal).
  for (let k = 1; k < geo.nodes.length; k++) {
    expect(geo.nodes[k]!.x).toBeGreaterThan(geo.nodes[k - 1]!.x);
    expect(geo.nodes[k]!.y).toBeCloseTo(geo.nodes[0]!.y, 6);
  }
  geo.connectors.forEach((c) => {
    expect(c.y1).toBeCloseTo(c.y2, 6);
    expect(c.x2).toBeGreaterThan(c.x1);
  });
  expect(withinBox(geo)).toBe(true);
});

test('cycle con 1 item: un solo nodo-elipse centrado y sin conectores', () => {
  const geo = smartArtLayout('cycle', items(1), VW, VH);
  expect(geo.nodes).toHaveLength(1);
  expect(geo.connectors).toHaveLength(0);
  const n = geo.nodes[0]!;
  expect(n.shape).toBe('ellipse');
  expect(n.x + n.w / 2).toBeCloseTo(VW / 2, 6);
  expect(n.y + n.h / 2).toBeCloseTo(VH / 2, 6);
});

test('cycle con muchos: nodos-elipse en anillo y flechas en bucle (k→k+1, cierra)', () => {
  const geo = smartArtLayout('cycle', items(5), VW, VH);
  expect(geo.nodes).toHaveLength(5);
  expect(geo.nodes.every((n) => n.shape === 'ellipse')).toBe(true);
  // Bucle cerrado: tantas flechas como nodos.
  expect(geo.connectors).toHaveLength(5);
  expect(geo.connectors.every((c) => c.kind === 'arrow')).toBe(true);
  // Primer nodo arriba (ángulo -90°): centrado en x, por encima del centro.
  const first = geo.nodes[0]!;
  expect(first.x + first.w / 2).toBeCloseTo(VW / 2, 6);
  expect(first.y + first.h / 2).toBeLessThan(VH / 2);
  expect(withinBox(geo)).toBe(true);
});

test('hierarchy: raíz + 2 hijos → raíz centrada sobre los hijos, codos, 2 filas', () => {
  const tree: SmartArtItem[] = [
    { text: 'Dirección' },
    { text: 'Equipo A', level: 1 },
    { text: 'Equipo B', level: 1 },
  ];
  const geo = smartArtLayout('hierarchy', tree, VW, VH);
  expect(geo.nodes).toHaveLength(3);
  expect(geo.connectors).toHaveLength(2);
  expect(geo.connectors.every((c) => c.kind === 'elbow')).toBe(true);
  // Nodos ordenados por índice de item.
  const [root, a, b] = geo.nodes;
  const cx = (n: (typeof geo.nodes)[number]): number => n.x + n.w / 2;
  const cy = (n: (typeof geo.nodes)[number]): number => n.y + n.h / 2;
  // La raíz se centra entre sus dos hijos y va en una fila superior.
  expect(cx(root!)).toBeCloseTo((cx(a!) + cx(b!)) / 2, 6);
  expect(cy(root!)).toBeLessThan(cy(a!));
  expect(cy(a!)).toBeCloseTo(cy(b!), 6);
  // Los codos salen del borde inferior de la raíz hacia el borde superior del hijo.
  geo.connectors.forEach((c) => expect(c.y2).toBeGreaterThan(c.y1));
  expect(withinBox(geo)).toBe(true);
});

test('hierarchy: salto de sangría 0→2 se absorbe (profundidad = padre + 1)', () => {
  // El 2.º item declara level 2 pero su padre está a 0 → debe caer en la fila 1.
  const tree: SmartArtItem[] = [{ text: 'Raíz' }, { text: 'Hijo (salto)', level: 2 }];
  const geo = smartArtLayout('hierarchy', tree, VW, VH);
  expect(geo.nodes).toHaveLength(2);
  expect(geo.connectors).toHaveLength(1);
  const [root, child] = geo.nodes;
  // Dos filas exactas: el hijo justo debajo de la raíz, no dos niveles más abajo.
  expect(child!.y).toBeGreaterThan(root!.y);
  expect(child!.y + child!.h).toBeLessThanOrEqual(VH + 0.5);
});

test('hierarchy: varias raíces (bosque) → fila única sin conectores', () => {
  const geo = smartArtLayout('hierarchy', items(3), VW, VH); // todos level 0
  expect(geo.nodes).toHaveLength(3);
  expect(geo.connectors).toHaveLength(0); // ningún padre/hijo
  // En una sola fila (misma y) y repartidos por x sin solaparse.
  const [n0, n1, n2] = geo.nodes;
  expect(n0!.y).toBeCloseTo(n1!.y, 6);
  expect(n1!.y).toBeCloseTo(n2!.y, 6);
  expect(n0!.x + n0!.w).toBeLessThanOrEqual(n1!.x + 0.5);
  expect(n1!.x + n1!.w).toBeLessThanOrEqual(n2!.x + 0.5);
});
