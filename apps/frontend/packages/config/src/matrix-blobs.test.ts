/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { decodeCipherBlobReceipt, maxCipherBlockBytes } from './matrix.js';
const receipt = () => ({
  id: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ',
  product: 'drive',
  size_bytes: 0,
  sha256: 'a'.repeat(64),
});
test('ciphertext receipts bind opaque ID, product, byte limit and canonical digest without private metadata', () => {
  const value = receipt();
  expect(
    decodeCipherBlobReceipt({ ...value, privateKey: 'must not be copied' }, 'drive', value.id),
  ).toEqual(value);
  expect(
    decodeCipherBlobReceipt({ ...value, size_bytes: maxCipherBlockBytes }, 'drive').size_bytes,
  ).toBe(maxCipherBlockBytes);
  for (const bad of [
    null,
    [],
    { ...value, product: 'mail' },
    { ...value, id: '../other' },
    { ...value, id: 'A'.repeat(25) },
    { ...value, id: 'A'.repeat(65) },
    { ...value, size_bytes: -1 },
    { ...value, size_bytes: 0.5 },
    { ...value, size_bytes: maxCipherBlockBytes + 1 },
    { ...value, sha256: 'A'.repeat(64) },
  ])
    expect(() => decodeCipherBlobReceipt(bad, 'drive')).toThrow('cipher_blob_invalid');
  expect(() => decodeCipherBlobReceipt(value, 'mail')).toThrow('cipher_blob_invalid');
  expect(() => decodeCipherBlobReceipt(value, 'drive', 'A'.repeat(26))).toThrow(
    'cipher_blob_invalid',
  );
});
