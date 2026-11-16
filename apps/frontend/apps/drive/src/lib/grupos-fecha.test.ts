/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los tramos de «Recientes»: hoy y ayer por día de calendario local, la semana desde el
 * lunes, y nada se reordena ni se pierde.
 */
import { expect, test } from 'bun:test';
import { agruparPorFecha, grupoDeFecha } from './grupos-fecha';

// Jueves 12 de noviembre de 2026 a las 10:30, hora local.
const JUEVES = new Date(2026, 10, 12, 10, 30);
const local = (dia: number, h = 12, m = 0) => new Date(2026, 10, dia, h, m).toISOString();

test('hoy y ayer son días de calendario, no ventanas de 24 horas', () => {
  expect(grupoDeFecha(local(12, 0, 1), JUEVES)).toBe('today');
  expect(grupoDeFecha(local(11, 23, 50), JUEVES)).toBe('yesterday');
  expect(grupoDeFecha(local(11, 0, 0), JUEVES)).toBe('yesterday');
  // Pasada la medianoche, lo de hace diez minutos ya es de ayer.
  expect(grupoDeFecha(local(11, 23, 59), new Date(2026, 10, 12, 0, 9))).toBe('yesterday');
});

test('«Esta semana» empieza el lunes y lo anterior es «Antes»', () => {
  expect(grupoDeFecha(local(10), JUEVES)).toBe('thisWeek'); // martes
  expect(grupoDeFecha(local(9, 0, 0), JUEVES)).toBe('thisWeek'); // lunes a las 00:00
  expect(grupoDeFecha(local(8, 23, 59), JUEVES)).toBe('earlier'); // domingo
  expect(grupoDeFecha(local(1), JUEVES)).toBe('earlier');
});

test('un lunes, lo del domingo es «Ayer» y «Esta semana» se queda sin nada', () => {
  const lunes = new Date(2026, 10, 9, 9, 0);
  expect(grupoDeFecha(local(8, 18), lunes)).toBe('yesterday');
  expect(grupoDeFecha(local(7, 18), lunes)).toBe('earlier');
  // Y un domingo, la semana es de lunes a viernes.
  const domingo = new Date(2026, 10, 15, 9, 0);
  expect(grupoDeFecha(local(9, 8), domingo)).toBe('thisWeek');
  expect(grupoDeFecha(local(8, 23), domingo)).toBe('earlier');
});

test('sin fecha o con una ilegible va a «Antes»; una futura cuenta como hoy', () => {
  expect(grupoDeFecha(undefined, JUEVES)).toBe('earlier');
  expect(grupoDeFecha('', JUEVES)).toBe('earlier');
  expect(grupoDeFecha('no es una fecha', JUEVES)).toBe('earlier');
  expect(grupoDeFecha(local(13, 9), JUEVES)).toBe('today');
});

test('agrupar conserva el orden, omite los tramos vacíos y junta los descolocados', () => {
  const f = (name: string, modifiedAt?: string) => ({ name, modifiedAt });
  const tramos = agruparPorFecha(
    [f('a', local(12, 9)), f('b', local(11)), f('c', local(1)), f('d', local(12, 8)), f('e')],
    JUEVES,
  );
  expect(tramos.map((t) => [t.grupo, t.items.map((i) => i.name)])).toEqual([
    ['today', ['a', 'd']],
    ['yesterday', ['b']],
    ['earlier', ['c', 'e']],
  ]);
  expect(agruparPorFecha([], JUEVES)).toEqual([]);
});
