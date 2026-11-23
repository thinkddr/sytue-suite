/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpError } from '@sytue/api-clients';
import { esUnidadCifrada } from './unidadCifrada';

const sobre = (code: string): string => JSON.stringify({ error: { code, message: 'x' } });

test('el 409 de la unidad cifrada se reconoce por su código', () => {
  const e = new HttpError(409, 'Conflict', '/files', sobre('private_storage_migration_required'));
  expect(esUnidadCifrada(e)).toBe(true);
});

test('otro 409, otro estado o algo que no es un HttpError no lo son', () => {
  expect(esUnidadCifrada(new HttpError(409, 'Conflict', '/files', sobre('name_taken')))).toBe(
    false,
  );
  expect(esUnidadCifrada(new HttpError(409, 'Conflict', '/files', 'no es json'))).toBe(false);
  expect(
    esUnidadCifrada(
      new HttpError(403, 'Forbidden', '/files', sobre('private_storage_migration_required')),
    ),
  ).toBe(false);
  expect(esUnidadCifrada(new Error('private_storage_migration_required'))).toBe(false);
  expect(esUnidadCifrada(undefined)).toBe(false);
});
