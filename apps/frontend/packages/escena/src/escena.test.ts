/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Operaciones de la escena: nodos, conectores, capas, orden Z, alinear, grupos.
 */
import { expect, test } from 'bun:test';
import {
  actualizarNodo,
  agregarCapa,
  agregarNodo,
  agrupar,
  ajustarCapa,
  alinearNodos,
  buscar,
  cajaMundo,
  desagrupar,
  distribuirNodos,
  eliminarCapa,
  eliminarNodos,
  enviarACapa,
  escenaVacia,
  girarNodos,
  moverNodos,
  nuevoId,
  ordenarCapa,
  ordenarZ,
  redimensionar,
  renombrarCapa,
  rutaDe,
} from './index.js';
import type { Escena, Nodo, NodoConector } from './index.js';

const caja = (id: string, x: number, y: number, w = 100, h = 50): Nodo => ({
  id,
  tipo: 'forma',
  forma: 'rect',
  x,
  y,
  w,
  h,
});

const conectar = (id: string, desde: string, hasta: string): NodoConector => ({
  id,
  tipo: 'conector',
  trazado: 'straight',
  desde: { nodo: desde },
  hasta: { nodo: hasta },
  flechaFin: true,
});

function base(): Escena {
  let e = escenaVacia({ ancho: 800, alto: 600 });
  e = agregarNodo(e, 'capa1', caja('a', 0, 0));
  e = agregarNodo(e, 'capa1', caja('b', 200, 0));
  e = agregarNodo(e, 'capa1', caja('c', 400, 100));
  return e;
}
const ids = (e: Escena): string[] => e.capas.flatMap((c) => c.nodos.map((n) => n.id));

test('escenaVacia lleva una capa con nombre', () => {
  const e = escenaVacia({ nombreCapa: 'Fondo' });
  expect(e.capas).toHaveLength(1);
  expect(e.capas[0]!.nombre).toBe('Fondo');
});

test('nuevoId no repite ids de nodos ni de capas', () => {
  const e = base();
  const vistos = new Set<string>(['a', 'b', 'c', 'capa1']);
  for (let i = 0; i < 200; i++) {
    const id = nuevoId(e);
    expect(vistos.has(id)).toBe(false);
    vistos.add(id);
  }
});

test('mover es inmutable y devuelve la misma escena si no hay efecto', () => {
  const e = base();
  const m = moverNodos(e, ['a'], 10, 5);
  expect(buscar(e, 'a')!.nodo).toMatchObject({ x: 0, y: 0 });
  expect(buscar(m, 'a')!.nodo).toMatchObject({ x: 10, y: 5 });
  expect(moverNodos(e, ['a'], 0, 0)).toBe(e);
  expect(moverNodos(e, ['no-existe'], 3, 3)).toBe(e);
});

test('el conector sigue a las formas ancladas y sale por el borde que mira al otro', () => {
  let e = base();
  e = agregarNodo(e, 'capa1', conectar('k', 'a', 'b'));
  const r1 = rutaDe(e, buscar(e, 'k')!.nodo as NodoConector);
  expect(r1.start.x).toBeCloseTo(100); // borde derecho de a
  expect(r1.end.x).toBeCloseTo(200); // borde izquierdo de b
  e = moverNodos(e, ['b'], 0, 300);
  const r2 = rutaDe(e, buscar(e, 'k')!.nodo as NodoConector);
  expect(r2.end.y).toBeLessThan(350); // se recalculó hacia la nueva posición
  expect(r2.end.y).toBeGreaterThan(250);
});

test('borrar una forma borra sus conectores y los de su grupo', () => {
  let e = base();
  e = agregarNodo(e, 'capa1', conectar('k', 'a', 'b'));
  e = agregarNodo(e, 'capa1', conectar('k2', 'b', 'c'));
  const sin = eliminarNodos(e, ['a']);
  expect(ids(sin)).toEqual(['b', 'c', 'k2']);
  const sinGrupo = eliminarNodos(agrupar(e, ['a', 'c'], 'g'), ['g']);
  expect(ids(sinGrupo)).toEqual(['b']);
  expect(eliminarNodos(e, ['zzz'])).toBe(e);
});

test('orden Z: al frente, al fondo y un paso', () => {
  const e = base();
  expect(ids(ordenarZ(e, ['a'], 'front'))).toEqual(['b', 'c', 'a']);
  expect(ids(ordenarZ(e, ['c'], 'back'))).toEqual(['c', 'a', 'b']);
  expect(ids(ordenarZ(e, ['a'], 'forward'))).toEqual(['b', 'a', 'c']);
  expect(ordenarZ(e, ['c'], 'front')).toBe(e); // ya está arriba
});

test('girar un nodo cambia rot y conserva su centro', () => {
  const e = girarNodos(base(), ['a'], 90);
  const n = buscar(e, 'a')!.nodo as Extract<Nodo, { tipo: 'forma' }>;
  expect(n.rot).toBe(90);
  const c = cajaMundo(e, 'a')!; // 100×50 girado 90º: 50 de ancho × 100 de alto, mismo centro (50,25)
  expect(c.w).toBeCloseTo(50);
  expect(c.h).toBeCloseTo(100);
  expect(c.x + c.w / 2).toBeCloseTo(50);
  expect(c.y + c.h / 2).toBeCloseTo(25);
  expect((buscar(girarNodos(e, ['a'], 270), 'a')!.nodo as { rot?: number }).rot).toBeUndefined();
});

test('redimensionar respeta un mínimo y no toca grupos', () => {
  const e = base();
  const r = redimensionar(e, 'a', { x: 5, y: 5, w: -10, h: 40 });
  expect(buscar(r, 'a')!.nodo).toMatchObject({ x: 5, y: 5, w: 1, h: 40 });
  const g = agrupar(e, ['a', 'b'], 'g');
  expect(redimensionar(g, 'g', { x: 0, y: 0, w: 5, h: 5 })).toBe(g);
});

test('alinear al borde izquierdo de la selección y al lienzo', () => {
  const e = base();
  const izq = alinearNodos(e, ['b', 'c'], 'left', 'seleccion');
  expect(buscar(izq, 'b')!.nodo).toMatchObject({ x: 200 });
  expect(buscar(izq, 'c')!.nodo).toMatchObject({ x: 200 });
  const der = alinearNodos(e, ['a'], 'right', 'lienzo');
  expect(buscar(der, 'a')!.nodo).toMatchObject({ x: 700 });
});

test('distribuir reparte con huecos iguales', () => {
  let e = escenaVacia();
  e = agregarNodo(e, 'capa1', caja('a', 0, 0, 100, 10));
  e = agregarNodo(e, 'capa1', caja('b', 130, 0, 100, 10));
  e = agregarNodo(e, 'capa1', caja('c', 500, 0, 100, 10));
  const d = distribuirNodos(e, ['a', 'b', 'c'], 'h');
  expect(buscar(d, 'b')!.nodo).toMatchObject({ x: 250 }); // huecos de 150
});

test('agrupar y desagrupar conserva la posición de cada nodo', () => {
  const e = base();
  const g = agrupar(e, ['a', 'c'], 'g');
  expect(ids(g)).toEqual(['b', 'g']);
  expect(buscar(g, 'g')!.nodo).toMatchObject({ x: 0, y: 0, w: 500, h: 150 });
  expect(cajaMundo(g, 'c')).toMatchObject({ x: 400, y: 100 });
  const movido = moverNodos(g, ['g'], 20, 20);
  expect(cajaMundo(movido, 'c')).toMatchObject({ x: 420, y: 120 });
  const libre = desagrupar(movido, 'g');
  expect(buscar(libre, 'c')!.nodo).toMatchObject({ x: 420, y: 120 });
  expect(agrupar(e, ['a'], 'g')).toBe(e); // un grupo necesita 2
});

test('desagrupar un grupo girado compone el giro en los hijos', () => {
  const g = girarNodos(agrupar(base(), ['a', 'b'], 'g'), ['g'], 90);
  const antes = cajaMundo(g, 'b')!;
  const libre = desagrupar(g, 'g');
  const despues = cajaMundo(libre, 'b')!;
  expect(despues.x).toBeCloseTo(antes.x);
  expect(despues.y).toBeCloseTo(antes.y);
  expect(despues.w).toBeCloseTo(antes.w);
  expect((buscar(libre, 'b')!.nodo as { rot?: number }).rot).toBe(90);
});

test('capas: nombre, orden, ocultar, enviar a otra capa y borrar', () => {
  let e = base();
  e = agregarCapa(e, { id: 'c2', nombre: 'Notas' });
  expect(agregarCapa(e, { id: 'c2', nombre: 'otra' })).toBe(e);
  e = renombrarCapa(e, 'capa1', 'Diagrama');
  expect(e.capas.map((c) => c.nombre)).toEqual(['Diagrama', 'Notas']);
  expect(ordenarCapa(e, 'c2', 'back').capas.map((c) => c.id)).toEqual(['c2', 'capa1']);
  expect(ajustarCapa(e, 'c2', { visible: false }).capas[1]!.visible).toBe(false);
  e = enviarACapa(e, ['a'], 'c2');
  expect(e.capas[1]!.nodos.map((n) => n.id)).toEqual(['a']);
  expect(e.capas[0]!.nodos.map((n) => n.id)).toEqual(['b', 'c']);
  e = eliminarCapa(e, 'c2');
  expect(ids(e)).toEqual(['b', 'c']);
  expect(eliminarCapa(e, 'capa1')).toBe(e); // siempre queda una
});

test('actualizarNodo sustituye solo el nodo pedido', () => {
  const e = base();
  const r = actualizarNodo(e, 'b', (n) => ({ ...n, nombre: 'B' }));
  expect(buscar(r, 'b')!.nodo.nombre).toBe('B');
  expect(r.capas[0]!.nodos[0]).toBe(e.capas[0]!.nodos[0]); // el resto comparte identidad
});
