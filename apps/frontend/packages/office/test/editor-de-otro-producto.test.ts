/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Un documento abierto en la app de otro producto se va a la del suyo (Docs se separó
 * en cuatro apps el 2026-10-12). Visto en rojo devolviendo siempre null.
 */
import { expect, test } from 'bun:test';
import { appUrl } from '@sytue/config';
import { editorDeOtroProducto } from '../src/lib/producto';

const ID = '3f2b6d1c-8a4e-4c1b-9d2e-5a6b7c8d9e0f';

test('una presentación abierta en Calcus se va al editor de Diapos, con su org y su vuelta', () => {
  expect(
    editorDeOtroProducto('spreadsheet', 'presentation', {
      id: ID,
      volver: '/Informes',
      org: 'acme',
    }),
  ).toBe(appUrl('slides', `/edit?id=${ID}&volver=%2FInformes`, 'acme'));
});

test('lo suyo, lo de la app conjunta y lo de tipo desconocido se abre donde está', () => {
  const sin = { id: ID, volver: null, org: null };
  expect(editorDeOtroProducto('spreadsheet', 'spreadsheet', sin)).toBeNull();
  expect(editorDeOtroProducto(undefined, 'presentation', sin)).toBeNull();
  expect(editorDeOtroProducto('document', null, sin)).toBeNull();
});
