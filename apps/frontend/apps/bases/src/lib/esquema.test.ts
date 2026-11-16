/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import type { BasesField } from '@sytue/api-clients/bases';
import {
  esOpcionRetirada,
  listaDeOpciones,
  moverCampo,
  opcionesQueSeRetiran,
  vecino,
} from './esquema.js';

test('las opciones se leen una por línea, limpias y sin repetir', () => {
  expect(listaDeOpciones(' Alta \n\nBaja\nAlta\n Media')).toEqual(['Alta', 'Baja', 'Media']);
  expect(opcionesQueSeRetiran(['a', 'b', 'c'], ['c', 'a', 'd'])).toEqual(['b']);
});

test('una opción retirada es un valor de selección que ya no está entre las opciones', () => {
  const sel = { id: 's', kind: 'select', choices: ['a'] } as BasesField;
  expect(esOpcionRetirada(sel, 'a')).toBe(false);
  expect(esOpcionRetirada(sel, 'z')).toBe(true);
  expect(esOpcionRetirada({ ...sel, kind: 'text' }, 'z')).toBe(false);
});

test('mover un campo lo pone en el sitio del destino y corre los de en medio', () => {
  const ids = ['a', 'b', 'c', 'd'];
  expect(moverCampo(ids, 'a', 'c')).toEqual(['b', 'c', 'a', 'd']);
  expect(moverCampo(ids, 'd', 'b')).toEqual(['a', 'd', 'b', 'c']);
  expect(moverCampo(ids, 'b', 'b')).toEqual(ids);
  expect(moverCampo(ids, 'x', 'b')).toEqual(ids);
  expect(vecino(ids, 'a', -1)).toBeNull();
  expect(vecino(ids, 'a', 1)).toBe('b');
  expect(vecino(ids, 'd', 1)).toBeNull();
});
