/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { fichaDe } from './ai.js';

test('una firma de correo sale como ficha', () => {
  const f = fichaDe(
    '```json\n{"nombre":"Ana Ruiz","organizacion":"Acme S.L.","correos":["ana.ruiz@acme.es"],"telefonos":["+34 600 11 22 33"]}\n```',
  );
  expect(f?.nombre).toBe('Ana Ruiz');
  expect(f?.organizacion).toBe('Acme S.L.');
  expect(f?.correos).toEqual(['ana.ruiz@acme.es']);
  expect(f?.telefonos).toEqual(['+34 600 11 22 33']);
});

// El modelo cuela la web en la lista de correos más de lo que parece.
test('lo que no es correo ni teléfono se cae', () => {
  const f = fichaDe(
    '{"nombre":"X","correos":["www.acme.es","a@b.es"],"telefonos":["ext. 12","911 22 33 44"]}',
  );
  expect(f?.correos).toEqual(['a@b.es']);
  expect(f?.telefonos).toEqual(['911 22 33 44']);
});

test('sin nombre no hay ficha', () => {
  expect(fichaDe('{"correos":["a@b.es"]}')).toBeNull();
  expect(fichaDe('lo siento, no encuentro datos')).toBeNull();
});
