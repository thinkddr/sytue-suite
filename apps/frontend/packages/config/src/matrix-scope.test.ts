/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { decodeStorageScopeHeader, encodeStorageScopeHeader } from './matrix.js';
test('scope headers interoperate with Go JSON without relaxing account limits or UTF-8', () => {
  const scope = { tenant: 'acme', subject: '💾 <owner>\u2028', product: 'drive' } as const;
  expect(decodeStorageScopeHeader(encodeStorageScopeHeader(scope))).toEqual(scope);
  const go = '{"tenant":"acme","subject":"💾 \\u003cowner\\u003e\\u2028","product":"drive"}';
  const header = 'v1.' + Buffer.from(go).toString('base64url');
  expect(decodeStorageScopeHeader(header)).toEqual(scope);
  for (const invalid of [
    '',
    'v1.invalid',
    header + '=',
    header + ',' + header,
    'v1.' + 'A'.repeat(4096),
    'v2.e30',
    'v1.e30',
  ])
    expect(() => decodeStorageScopeHeader(invalid)).toThrow('cipher_blob_invalid');
  for (const bytes of [
    Uint8Array.of(0xff),
    new TextEncoder().encode('{"tenant":"acme","subject":"person","product":"drive","extra":1}'),
  ])
    expect(() =>
      decodeStorageScopeHeader('v1.' + Buffer.from(bytes).toString('base64url')),
    ).toThrow('cipher_blob_invalid');
  for (const value of [
    { ...scope, tenant: 'é'.repeat(32) },
    { ...scope, subject: '\ud800' },
    { ...scope, subject: 'a\n' },
  ])
    expect(() => encodeStorageScopeHeader(value)).toThrow('cipher_blob_invalid');
});
