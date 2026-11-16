/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cursor que comparten j/k y los botones «Más reciente»/«Más antigua» del lector
 * (MAIL-11). Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { pasoDelCursor, vecinasDeLaAbierta } from './cursorConversaciones.js';

const base = { total: 3, cursor: 0, hayMas: false };

test('sin nada abierto se retoma donde se quedó el cursor', () => {
  expect(pasoDelCursor({ ...base, actual: -1, cursor: 5, siguiente: true })).toEqual({
    tipo: 'ir',
    indice: 2,
  });
  expect(pasoDelCursor({ ...base, total: 0, actual: -1, siguiente: true })).toBeNull();
});

test('baja y sube una fila, y en los bordes se queda', () => {
  expect(pasoDelCursor({ ...base, actual: 0, siguiente: true })).toEqual({ tipo: 'ir', indice: 1 });
  expect(pasoDelCursor({ ...base, actual: 1, siguiente: false })).toEqual({
    tipo: 'ir',
    indice: 0,
  });
  expect(pasoDelCursor({ ...base, actual: 0, siguiente: false })).toEqual({
    tipo: 'ir',
    indice: 0,
  });
  expect(pasoDelCursor({ ...base, actual: 2, siguiente: true })).toEqual({ tipo: 'ir', indice: 2 });
});

test('en la última cargada, si quedan páginas, bajar pide la siguiente', () => {
  expect(pasoDelCursor({ ...base, hayMas: true, actual: 2, siguiente: true })).toEqual({
    tipo: 'cargar',
  });
  // Subir no pide nada aunque queden páginas: estas van por abajo.
  expect(pasoDelCursor({ ...base, hayMas: true, actual: 2, siguiente: false })).toEqual({
    tipo: 'ir',
    indice: 1,
  });
});

test('los botones del lector se apagan en los extremos', () => {
  expect(vecinasDeLaAbierta(3, 0, false)).toEqual({ anterior: false, siguiente: true });
  expect(vecinasDeLaAbierta(3, 1, false)).toEqual({ anterior: true, siguiente: true });
  expect(vecinasDeLaAbierta(3, 2, false)).toEqual({ anterior: true, siguiente: false });
  // Con más páginas, la última cargada todavía tiene algo debajo.
  expect(vecinasDeLaAbierta(3, 2, true)).toEqual({ anterior: true, siguiente: true });
  // Una conversación que no está en la lista no tiene vecinas.
  expect(vecinasDeLaAbierta(3, -1, true)).toEqual({ anterior: false, siguiente: false });
});
