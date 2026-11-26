/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Copiar el texto», «Ir a…» y «Ayer»/«Mañana» del diario: lo que deciden sin pintar.
 */
import { expect, test } from 'bun:test';
import { unirTitulo } from './copiarNota.js';
import { diaElegido, diaRelativo } from './diario.js';

test('el texto copiado: título, línea en blanco y cuerpo; sin uno de los dos, el otro', () => {
  expect(unirTitulo(' Compra ', '- pan\n- leche ')).toBe('Compra\n\n- pan\n- leche');
  expect(unirTitulo('', 'solo cuerpo')).toBe('solo cuerpo');
  expect(unirTitulo('Solo título', '   ')).toBe('Solo título');
});

test('«Ir a…» abre un día válido y distinto del abierto, y nada más', () => {
  expect(diaElegido('2026-03-01', '2026-11-25')).toBe('2026-03-01');
  expect(diaElegido('2026-11-25', '2026-11-25')).toBeNull();
  expect(diaElegido('', '2026-11-25')).toBeNull();
  // Un día que no existe no se abre (el 30 de febrero pasaría a marzo sin avisar).
  expect(diaElegido('2026-02-30', '2026-11-25')).toBeNull();
});

test('«Ayer» y «Mañana» respecto de hoy, también cruzando un mes', () => {
  expect(diaRelativo('2026-11-24', '2026-11-25')).toBe('daily.yesterday');
  expect(diaRelativo('2026-11-26', '2026-11-25')).toBe('daily.tomorrow');
  expect(diaRelativo('2026-11-30', '2026-12-01')).toBe('daily.yesterday');
  expect(diaRelativo('2026-11-27', '2026-11-25')).toBeNull();
  expect(diaRelativo('2026-11-25', '2026-11-25')).toBeNull();
});
