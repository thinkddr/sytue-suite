// El navegador resuelve el reto con el mismo contrato que el servicio. Ejecutar: `bun test`.
import { createHash } from 'node:crypto';
import { expect, test } from 'bun:test';
import { resolverReto } from './altcha.js';

const sha = async (d: BufferSource) =>
  createHash('sha256')
    .update(new Uint8Array(d as ArrayBuffer))
    .digest().buffer as ArrayBuffer;

test('encuentra el número y devuelve la carga con firma', async () => {
  const salt = 'sal?expires=9999999999';
  const challenge = createHash('sha256').update(`${salt}${137}`).digest('hex');
  const carga = await resolverReto(
    { algorithm: 'SHA-256', challenge, maxnumber: 1000, salt, signature: 'firma' },
    sha,
  );
  expect(carga).not.toBeNull();
  expect(JSON.parse(atob(carga!))).toEqual({
    algorithm: 'SHA-256',
    challenge,
    number: 137,
    salt,
    signature: 'firma',
  });
});

test('un reto sin solución, o de otro algoritmo, no se resuelve', async () => {
  expect(
    await resolverReto(
      { algorithm: 'SHA-256', challenge: 'x', maxnumber: 50, salt: 's', signature: '' },
      sha,
    ),
  ).toBeNull();
  expect(
    await resolverReto(
      { algorithm: 'MD5', challenge: 'x', maxnumber: 5, salt: 's', signature: '' },
      sha,
    ),
  ).toBeNull();
});
