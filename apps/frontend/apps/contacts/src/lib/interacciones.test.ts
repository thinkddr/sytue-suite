/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import type { SearchResult } from '@sytue/api-clients';
import {
  MAX_INTERACCIONES,
  correosABuscar,
  respuestaValida,
  unirInteracciones,
} from './interacciones.js';

const r = (source: string, id: string, timestamp?: number): SearchResult =>
  ({ source, id, title: id, url: `/${source}/${id}`, timestamp }) as SearchResult;

test('las direcciones se limpian, sin repetidos y como mucho tres', () => {
  expect(
    correosABuscar([' Ana@X.eu ', 'ana@x.eu', 'no es correo', 'b@x.eu', 'c@x.eu', 'd@x.eu']),
  ).toEqual(['ana@x.eu', 'b@x.eu', 'c@x.eu']);
});

test('solo correo y calendario, sin repetidos, lo más reciente primero', () => {
  const out = unirInteracciones([
    [r('mail', '1', 100), r('drive', 'f', 999), r('calendar', 'e', 300)],
    [r('mail', '1', 100), r('mail', '2'), r('mail', '3', 200)],
  ]);
  expect(out.map((x) => `${x.source}:${x.id}`)).toEqual([
    'calendar:e',
    'mail:3',
    'mail:1',
    'mail:2',
  ]);
});

test('se recorta a las que caben en la ficha', () => {
  const muchas = Array.from({ length: 12 }, (_, i) => r('mail', String(i), i));
  expect(unirInteracciones([muchas])).toHaveLength(MAX_INTERACCIONES);
});

test('con correo y calendario caídos dentro del Hub, la búsqueda no vale (no es «ninguno»)', () => {
  const caido = (source: string) => ({ source, error: 'x' }) as never;
  expect(respuestaValida({ results: [], fallidos: [caido('mail'), caido('calendar')] })).toBe(
    false,
  );
  expect(respuestaValida({ results: [], fallidos: [caido('mail')] })).toBe(true);
  expect(respuestaValida({ results: [], fallidos: [] })).toBe(true);
  expect(
    respuestaValida({ results: [r('mail', '1')], fallidos: [caido('mail'), caido('calendar')] }),
  ).toBe(true);
});
