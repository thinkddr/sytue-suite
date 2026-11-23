/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { decodeSsssRawSecret, encodeSsssRawSecret, trimBase64Padding } from './ssss-vault.js';

test('trimBase64Padding solo quita los `=` del final', () => {
  expect(trimBase64Padding('')).toBe('');
  expect(trimBase64Padding('====')).toBe('');
  expect(trimBase64Padding('QQ==')).toBe('QQ');
  expect(trimBase64Padding('QUI=')).toBe('QUI');
  expect(trimBase64Padding('QUJD')).toBe('QUJD');
  expect(trimBase64Padding('Q=Q=')).toBe('Q=Q');
});

test('el secreto SSSS sin relleno sigue decodificándose igual', () => {
  const bytes = new Uint8Array([0, 1, 2, 250, 251, 252, 253]);
  const encoded = encodeSsssRawSecret(bytes);
  expect(encoded).toBe('AAEC+vv8/Q');
  expect(decodeSsssRawSecret(encoded)).toEqual(bytes);
  expect(decodeSsssRawSecret(encoded + '==')).toEqual(bytes);
  expect(() => decodeSsssRawSecret('Q')).toThrow();
});

// Regresión ReDoS: con `/=+$/` esta entrada tardaba segundos (cuadrática en la
// racha de `=`). Ahora el rechazo es lineal y llega muy por debajo del límite.
test('una racha enorme de `=` que no acaba la cadena se rechaza en tiempo lineal', () => {
  const hostil = '='.repeat(100_000) + 'A';
  const inicio = performance.now();
  expect(() => decodeSsssRawSecret(hostil)).toThrow();
  expect(performance.now() - inicio).toBeLessThan(250);
});
