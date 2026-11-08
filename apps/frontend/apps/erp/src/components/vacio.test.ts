/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, it } from 'bun:test';
import { vacioDeLista } from './vacio.js';

// VISTO EN ROJO: mirando solo estado y búsqueda (lo de antes), la papelera vacía pedía
// «Crea el primero con Nuevo».
it('la papelera vacía tiene su propio estado, sin «Nuevo»', () => {
  expect(vacioDeLista({ papelera: true, estado: '', busqueda: '' })).toEqual({
    titulo: 'list.trashEmpty',
    pista: 'list.trashEmptyHint',
    icono: 'trash',
    accion: 'limpiar',
  });
  expect(vacioDeLista({ papelera: true, estado: '', busqueda: 'x' })).toEqual({
    titulo: 'list.trashEmpty',
    icono: 'trash',
    accion: 'limpiar',
  });
});

it('la lista sigue como estaba', () => {
  expect(vacioDeLista({ papelera: false, estado: '', busqueda: '' })).toEqual({
    titulo: 'list.empty',
    pista: 'list.emptyHint',
    icono: 'building',
    accion: 'nuevo',
  });
  expect(vacioDeLista({ papelera: false, estado: 'pagado', busqueda: '' }).accion).toBe('limpiar');
});
