/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La franja de teselas y la lista son dos piezas en pantalla y una sola lista para lo
 * que cuenta posiciones. Si la selección por rango mirase la lista del servicio y no lo
 * que se ve, Mayús + clic entre una tesela y una fila marcaría otros elementos.
 */
import { expect, test } from 'bun:test';
import { idsDelRango, partirEnTeselas, posicionEnLista } from './teselas';

const el = (id: string, isDirectory = false) => ({ id, isDirectory });
// Como lo manda el servicio: carpetas primero y, dentro de cada grupo, por nombre.
const servicio = [el('c-a', true), el('c-b', true), el('f-a'), el('f-b'), el('f-c')];

test('con la franja inactiva no se parte nada: todo va a la lista, en su orden', () => {
  const p = partirEnTeselas(servicio, false);
  expect(p.carpetas).toEqual([]);
  expect(p.resto).toEqual(servicio);
  expect(p.enOrden).toBe(servicio);
  expect(posicionEnLista(p, 2)).toBe(2);
});

test('con la franja activa, las carpetas van arriba y cada parte conserva su orden', () => {
  const p = partirEnTeselas(servicio, true);
  expect(p.carpetas.map((x) => x.id)).toEqual(['c-a', 'c-b']);
  expect(p.resto.map((x) => x.id)).toEqual(['f-a', 'f-b', 'f-c']);
  expect(p.enOrden.map((x) => x.id)).toEqual(['c-a', 'c-b', 'f-a', 'f-b', 'f-c']);
});

test('una carpeta que llega entre ficheros sube a la franja sin desordenar el resto', () => {
  const p = partirEnTeselas([el('f-a'), el('c-z', true), el('f-b')], true);
  expect(p.enOrden.map((x) => x.id)).toEqual(['c-z', 'f-a', 'f-b']);
});

test('sin carpetas, o sin nada cargado todavía, no hay franja', () => {
  expect(partirEnTeselas([el('f-a')], true).carpetas).toEqual([]);
  expect(partirEnTeselas(null, true)).toEqual({ carpetas: [], resto: [], enOrden: [] });
});

test('Mayús + clic de una tesela a una fila marca el tramo que se VE entre las dos', () => {
  const p = partirEnTeselas(servicio, true);
  // Ancla en la segunda tesela (posición 1); se pulsa la segunda fila de la lista.
  const hasta = posicionEnLista(p, 1);
  expect(hasta).toBe(3);
  expect(idsDelRango(p.enOrden, 1, hasta)).toEqual(['c-b', 'f-a', 'f-b']);
  // Y al revés: de una fila hacia una tesela.
  expect(idsDelRango(p.enOrden, hasta, 0)).toEqual(['c-a', 'c-b', 'f-a', 'f-b']);
});

test('dentro de la lista, el rango no arrastra ninguna tesela', () => {
  const p = partirEnTeselas(servicio, true);
  expect(idsDelRango(p.enOrden, posicionEnLista(p, 0), posicionEnLista(p, 2))).toEqual([
    'f-a',
    'f-b',
    'f-c',
  ]);
});

test('lo que no tiene id no se selecciona', () => {
  expect(idsDelRango([{ id: 'a' }, {}, { id: 'c' }], 0, 2)).toEqual(['a', 'c']);
});
