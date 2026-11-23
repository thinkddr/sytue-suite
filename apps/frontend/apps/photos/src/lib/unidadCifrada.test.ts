/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { CODIGO_UNIDAD_CIFRADA, codigoDeError, esUnidadCifrada } from './unidadCifrada';

const sobre = (code: string): string => JSON.stringify({ error: { code, message: 'x' } });

test('el 409 de la unidad cifrada se reconoce por su código, y solo ese', () => {
  expect(esUnidadCifrada(409, codigoDeError(sobre(CODIGO_UNIDAD_CIFRADA)))).toBe(true);
  expect(esUnidadCifrada(409, codigoDeError(sobre('name_conflict')))).toBe(false);
  expect(esUnidadCifrada(403, codigoDeError(sobre(CODIGO_UNIDAD_CIFRADA)))).toBe(false);
});

test('un cuerpo que no es el sobre de error de la casa no tiene código', () => {
  for (const cuerpo of ['', 'no es json', 'null', '{"error":"plano"}', '{"error":{"code":7}}'])
    expect(codigoDeError(cuerpo)).toBeUndefined();
});
