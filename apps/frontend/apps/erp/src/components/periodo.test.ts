/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import { PERIODOS, ROTULO_PERIODO, rangoDePeriodo } from './periodo.js';
import { en } from '../i18n/messages/en.js';

// 5 de noviembre de 2026, a mediodía LOCAL.
const hoy = new Date(2026, 10, 5, 12);

describe('rangoDePeriodo', () => {
  it('todo no acota', () => {
    expect(rangoDePeriodo('todo', hoy)).toEqual({});
  });
  it('el mes, con su último día', () => {
    expect(rangoDePeriodo('mes', hoy)).toEqual({ desde: '2026-11-01', hasta: '2026-11-30' });
    expect(rangoDePeriodo('mes', new Date(2028, 1, 10))).toEqual({
      desde: '2028-02-01',
      hasta: '2028-02-29',
    });
  });
  it('el mes anterior cruza el año en enero', () => {
    expect(rangoDePeriodo('mesAnterior', hoy)).toEqual({
      desde: '2026-10-01',
      hasta: '2026-10-31',
    });
    expect(rangoDePeriodo('mesAnterior', new Date(2027, 0, 15))).toEqual({
      desde: '2026-12-01',
      hasta: '2026-12-31',
    });
  });
  it('el trimestre natural', () => {
    expect(rangoDePeriodo('trimestre', hoy)).toEqual({ desde: '2026-10-01', hasta: '2026-12-31' });
    expect(rangoDePeriodo('trimestre', new Date(2026, 3, 30))).toEqual({
      desde: '2026-04-01',
      hasta: '2026-06-30',
    });
  });
  it('el año y el anterior', () => {
    expect(rangoDePeriodo('anio', hoy)).toEqual({ desde: '2026-01-01', hasta: '2026-12-31' });
    expect(rangoDePeriodo('anioAnterior', hoy)).toEqual({
      desde: '2025-01-01',
      hasta: '2025-12-31',
    });
  });
  it('cada período tiene su rótulo en el catálogo', () => {
    for (const p of PERIODOS) expect(en[ROTULO_PERIODO[p]]).toBeTruthy();
  });
});
