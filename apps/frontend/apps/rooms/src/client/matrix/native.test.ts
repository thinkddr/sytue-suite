/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { matrixNativeReturn, opaqueMatrixNative } from './native.js';
const state = 'A'.repeat(43),
  ticket = 'B'.repeat(43);
test('native return contains only one-use opaque ticket and original state on the fixed app route', () => {
  const redirect_url = `sytue-rooms://matrix/callback?state=${state}&ticket=${ticket}`;
  expect(matrixNativeReturn({ ticket, redirect_url }, state)).toBe(redirect_url);
  for (const url of [
    redirect_url.replace('sytue-rooms:', 'https:'),
    redirect_url.replace('matrix/callback', 'evil/callback'),
    redirect_url + '&loginToken=private',
    redirect_url + '&state=' + state,
    redirect_url + '#secret',
    redirect_url.replace(state, 'C'.repeat(43)),
  ])
    expect(() => matrixNativeReturn({ ticket, redirect_url: url }, state)).toThrow();
  expect(opaqueMatrixNative('token')).toBe(false);
});
