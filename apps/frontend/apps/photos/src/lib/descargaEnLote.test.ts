/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import { expect, test } from 'bun:test';
import {
  planDeDescarga,
  programarDescarga,
  TOPE_DESCARGA,
  type RelojDeDescarga,
} from './descargaEnLote.js';

const ids = (n: number) => Array.from({ length: n }, (_, i) => `foto-${i}`);

test('por debajo del límite incluye toda la selección', () => {
  const seleccion = Object.freeze(['c', 'a', 'b']);
  expect(planDeDescarga(seleccion)).toEqual({ ahora: ['c', 'a', 'b'], fuera: 0 });
  expect(seleccion).toEqual(['c', 'a', 'b']);
});

test('el límite exacto no deja fotos fuera del lote', () => {
  const seleccion = ids(TOPE_DESCARGA);
  expect(planDeDescarga(seleccion)).toEqual({ ahora: seleccion, fuera: 0 });
});

test('el lote conserva las primeras fotos y cuenta las excluidas', () => {
  const seleccion = ids(TOPE_DESCARGA + 7).reverse();
  expect(planDeDescarga(seleccion)).toEqual({
    ahora: seleccion.slice(0, TOPE_DESCARGA),
    fuera: 7,
  });
});

test('los IDs vacíos se ignoran antes de aplicar el límite y contar el resto', () => {
  const seleccion = ['', '  ', ...ids(TOPE_DESCARGA), '\t', 'ultima', '\n'];
  expect(planDeDescarga(seleccion)).toEqual({ ahora: ids(TOPE_DESCARGA), fuera: 1 });
});

test('una selección vacía o solo con blancos no genera peticiones', () => {
  expect(planDeDescarga([])).toEqual({ ahora: [], fuera: 0 });
  expect(planDeDescarga(['', ' \t\n '])).toEqual({ ahora: [], fuera: 0 });
});

function relojManual() {
  let siguiente = 0;
  const pendientes = new Map<number, { accion: () => void; espera: number }>();
  const reloj: RelojDeDescarga = {
    programar: (accion, espera) => {
      const id = ++siguiente;
      pendientes.set(id, { accion, espera });
      return id;
    },
    cancelar: (id) => {
      pendientes.delete(id);
    },
  };
  return { reloj, pendientes };
}

test('inicia la primera petición en el gesto y espacia solo las demás', () => {
  const { reloj, pendientes } = relojManual();
  const iniciadas: string[] = [];
  programarDescarga(['a', 'b', 'c'], (id) => iniciadas.push(id), reloj);
  expect(iniciadas).toEqual(['a']);
  expect([...pendientes.values()].map((p) => p.espera)).toEqual([600, 1200]);
  for (const p of pendientes.values()) p.accion();
  expect(iniciadas).toEqual(['a', 'b', 'c']);
});

test('cancelar limpia la cola y también neutraliza callbacks que ya habían vencido', () => {
  const { reloj, pendientes } = relojManual();
  const iniciadas: string[] = [];
  const parar = programarDescarga(['a', 'b', 'c'], (id) => iniciadas.push(id), reloj);
  const vencidos = [...pendientes.values()];
  parar();
  parar();
  expect(pendientes.size).toBe(0);
  for (const p of vencidos) p.accion();
  expect(iniciadas).toEqual(['a']);
});

test('reemplazar un lote permite continuar las pendientes sin revivir la cola anterior', () => {
  const { reloj, pendientes } = relojManual();
  const seleccion = new Set(['a', 'b', 'c']);
  const iniciadas: string[] = [];
  const iniciar = (id: string) => {
    iniciadas.push(id);
    seleccion.delete(id);
  };
  const parar = programarDescarga([...seleccion], iniciar, reloj);
  const anteriores = [...pendientes.values()];
  expect([...seleccion]).toEqual(['b', 'c']);
  parar();
  programarDescarga([...seleccion], iniciar, reloj);
  for (const p of anteriores) p.accion();
  for (const p of pendientes.values()) p.accion();
  expect(iniciadas).toEqual(['a', 'b', 'c']);
  expect([...seleccion]).toEqual([]);
});

test('una cola vacía no inicia peticiones ni programa temporizadores', () => {
  const { reloj, pendientes } = relojManual();
  const iniciadas: string[] = [];
  const parar = programarDescarga([], (id) => iniciadas.push(id), reloj);
  parar();
  expect(iniciadas).toEqual([]);
  expect(pendientes.size).toBe(0);
});
