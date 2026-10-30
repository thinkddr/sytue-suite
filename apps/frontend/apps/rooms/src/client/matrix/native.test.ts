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

test('native storage return follows the product fixed before authorization', () => {
  for (const [product, native] of [
    ['hub', 'one'],
    ['mail', 'mail'],
    ['drive', 'drive'],
    ['docs', 'docs'],
    ['sheets', 'sheets'],
    ['slides', 'slides'],
    ['pdf', 'pdf'],
    ['calendar', 'calendar'],
    ['contacts', 'contacts'],
    ['notes', 'notes'],
    ['photos', 'photos'],
    ['rooms', 'rooms'],
    ['cast', 'cast'],
    ['erp', 'erp'],
    ['bases', 'bases'],
  ]) {
    const redirect_url = `sytue-${native}://matrix/callback?state=${state}&ticket=${ticket}`;
    expect(matrixNativeReturn({ product, ticket, redirect_url }, state)).toBe(redirect_url);
    expect(() =>
      matrixNativeReturn({ product: 'unregistered', ticket, redirect_url }, state),
    ).toThrow();
    expect(() =>
      matrixNativeReturn(
        { product: native === 'drive' ? 'mail' : 'drive', ticket, redirect_url },
        state,
      ),
    ).toThrow();
  }
});
