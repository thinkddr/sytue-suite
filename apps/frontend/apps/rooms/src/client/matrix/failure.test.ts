/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { matrixFailureCode } from './failure.js';
import { MatrixSecurityError } from './types.js';

test('el código dice el paso y el motivo, nunca el mensaje', () => {
  expect(matrixFailureCode('open', new MatrixSecurityError('device'))).toBe('open:device');
  const matrix = Object.assign(new Error('https://matrix.example/secret?token=x'), {
    errcode: 'M_FORBIDDEN',
    httpStatus: 403,
  });
  expect(matrixFailureCode('setup', matrix)).toBe('setup:M_FORBIDDEN/403');
  expect(matrixFailureCode('connect', new TypeError('Failed to fetch'))).toBe('connect:TypeError');
  expect(matrixFailureCode('open', Object.assign(new Error('x'), { httpStatus: 502 }))).toBe(
    'open:502',
  );
  expect(matrixFailureCode('recover', 'texto')).toBe('recover:unknown');
  // Un errcode raro no se cuela tal cual en la pantalla.
  expect(matrixFailureCode('setup', { errcode: '<img src=x>' })).toBe('setup:unknown');
});
