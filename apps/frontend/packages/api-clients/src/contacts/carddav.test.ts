/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';

import { parseVcard } from './carddav.js';

test('proyecta los valores después de parámetros citados con dos puntos', () => {
  const card = [
    'BEGIN:VCARD',
    'VERSION:4.0',
    'UID;X-LABEL="id:externo":urn:uuid:123',
    'FN;X-LABEL="a:b":Ana Ruiz',
    'EMAIL;X-LABEL="trabajo:uno";TYPE=work:ana@example.test',
    'TEL;X-LABEL="central:uno":tel:+34123456789',
    'ORG;X-LABEL="departamento:uno":Sytue;Equipo',
    'END:VCARD',
  ].join('\r\n');

  expect(parseVcard(card)).toEqual({
    uid: 'urn:uuid:123',
    fullName: 'Ana Ruiz',
    emails: ['ana@example.test'],
    phones: ['tel:+34123456789'],
    organization: 'Sytue Equipo',
  });
});

test('despliega parámetros y valores antes de buscar el separador', () => {
  const card = [
    'BEGIN:VCARD',
    'FN;X-LABEL="etiqueta:',
    ' larga":Ana ',
    '\tRuiz',
    'EMAIL;X-LABEL="uno:dos":ana@',
    ' example.test',
    'END:VCARD',
  ].join('\r\n');

  expect(parseVcard(card).fullName).toBe('Ana Ruiz');
  expect(parseVcard(card).emails).toEqual(['ana@example.test']);
});

test('un FN vacío con parámetro citado conserva el marcador sin nombre', () => {
  expect(parseVcard('BEGIN:VCARD\nFN;X-LABEL="a:b":\nEND:VCARD').fullName).toBe('(Sin nombre)');
});

test('ignora líneas sin separador válido y conserva dos puntos en el valor', () => {
  const card = [
    'FN;X-LABEL="sin cerrar:Ana',
    'EMAIL',
    'FN:Ana: Ruiz',
    'EMAIL:ana@example.test',
  ].join('\n');

  expect(parseVcard(card).fullName).toBe('Ana: Ruiz');
  expect(parseVcard(card).emails).toEqual(['ana@example.test']);
});
