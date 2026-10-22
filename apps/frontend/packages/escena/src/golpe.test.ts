/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Hit-testing: qué hay bajo un punto y qué toca un marco.
 */
import { expect, test } from 'bun:test';
import {
  agregarCapa,
  agregarNodo,
  agrupar,
  ajustarCapa,
  escenaVacia,
  girarNodos,
  golpear,
  nodosEnRect,
  puntosDeRuta,
} from './index.js';
import type { Escena, Nodo } from './index.js';

const forma = (
  id: string,
  f: 'rect' | 'ellipse' | 'triangle' | 'star',
  x: number,
  y: number,
  w: number,
  h: number,
): Nodo => ({
  id,
  tipo: 'forma',
  forma: f,
  x,
  y,
  w,
  h,
});

function escena(...nodos: Nodo[]): Escena {
  let e = escenaVacia();
  for (const n of nodos) e = agregarNodo(e, 'capa1', n);
  return e;
}

test('rect: dentro y fuera, con tolerancia', () => {
  const e = escena(forma('a', 'rect', 100, 100, 50, 50));
  expect(golpear(e, 120, 120)?.id).toBe('a');
  expect(golpear(e, 99, 120, 0)).toBeUndefined();
  expect(golpear(e, 98, 120, 3)?.id).toBe('a');
  expect(golpear(e, 10, 10)).toBeUndefined();
});

test('elipse: la esquina de la caja queda fuera', () => {
  const e = escena(forma('o', 'ellipse', 0, 0, 100, 100));
  expect(golpear(e, 50, 50, 0)?.id).toBe('o');
  expect(golpear(e, 3, 3, 0)).toBeUndefined();
});

test('triángulo: la punta sí y el hueco junto a ella no', () => {
  const e = escena(forma('t', 'triangle', 0, 0, 100, 100));
  expect(golpear(e, 50, 60, 0)?.id).toBe('t');
  expect(golpear(e, 10, 10, 0)).toBeUndefined();
});

test('gana el nodo de más arriba y las capas ocultas o bloqueadas no se tocan', () => {
  let e = escena(forma('abajo', 'rect', 0, 0, 100, 100), forma('arriba', 'rect', 0, 0, 100, 100));
  expect(golpear(e, 50, 50)?.id).toBe('arriba');
  e = agregarCapa(e, { id: 'c2', nombre: 'Encima' });
  e = agregarNodo(e, 'c2', forma('tapa', 'rect', 0, 0, 100, 100));
  expect(golpear(e, 50, 50)).toEqual({ id: 'tapa', capaId: 'c2' });
  expect(golpear(ajustarCapa(e, 'c2', { bloqueada: true }), 50, 50)?.id).toBe('arriba');
  expect(golpear(ajustarCapa(e, 'c2', { visible: false }), 50, 50)?.id).toBe('arriba');
});

test('un nodo girado se toca donde está dibujado, no en su caja sin girar', () => {
  const e = girarNodos(escena(forma('a', 'rect', 0, 0, 200, 20)), ['a'], 90);
  // Centro (100, 10): tras girar 90º es una barra vertical de 20 × 200.
  expect(golpear(e, 100, 100, 0)?.id).toBe('a');
  expect(golpear(e, 190, 10, 0)).toBeUndefined();
});

test('línea, trazo y conector se tocan por su trazo', () => {
  const e = escena(
    { id: 'l', tipo: 'linea', x1: 0, y1: 0, x2: 100, y2: 0 },
    { id: 'p', tipo: 'trazo', puntos: [0, 50, 100, 50, 100, 90] },
    forma('a', 'rect', 0, 200, 20, 20),
    forma('b', 'rect', 200, 200, 20, 20),
    { id: 'k', tipo: 'conector', trazado: 'straight', desde: { nodo: 'a' }, hasta: { nodo: 'b' } },
  );
  expect(golpear(e, 50, 1)?.id).toBe('l');
  expect(golpear(e, 50, 20, 0)).toBeUndefined();
  expect(golpear(e, 100, 70)?.id).toBe('p');
  expect(golpear(e, 110, 210, 1)?.id).toBe('k'); // entre las dos formas
});

test('un clic en el hijo de un grupo devuelve el grupo', () => {
  const e = agrupar(
    escena(forma('a', 'rect', 0, 0, 10, 10), forma('b', 'rect', 100, 100, 10, 10)),
    ['a', 'b'],
    'g',
  );
  expect(golpear(e, 105, 105)?.id).toBe('g');
  expect(golpear(e, 50, 50, 0)).toBeUndefined(); // el hueco entre hijos no es del grupo
});

test('nodosEnRect: contiene o toca', () => {
  const e = escena(forma('a', 'rect', 0, 0, 10, 10), forma('b', 'rect', 95, 0, 20, 10));
  const marco = { x: -5, y: -5, w: 110, h: 30 };
  expect(nodosEnRect(e, marco, 'dentro')).toEqual(['a']);
  expect(nodosEnRect(e, marco, 'toca')).toEqual(['a', 'b']);
});

test('puntosDeRuta aplana líneas y Bézier', () => {
  expect(puntosDeRuta('M 0 0 L 10 0 L 10 10')).toEqual([
    { x: 0, y: 0 },
    { x: 10, y: 0 },
    { x: 10, y: 10 },
  ]);
  const c = puntosDeRuta('M 0 0 C 0 10 10 10 10 0');
  expect(c).toHaveLength(17);
  expect(c[16]).toEqual({ x: 10, y: 0 });
});
