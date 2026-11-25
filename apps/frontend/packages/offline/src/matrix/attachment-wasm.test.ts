/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { matrixAttachments } from './attachment-wasm.js';

test('the lazy Matrix attachment module is initialised once and round-trips a block', async () => {
  const [first, second] = await Promise.all([matrixAttachments(), matrixAttachments()]);
  expect(second).toBe(first);
  expect(await matrixAttachments()).toBe(first);
  const { Attachment, EncryptedAttachment } = first;
  const plain = new TextEncoder().encode('private block');
  const encrypted = Attachment.encrypt(plain.slice());
  const info = encrypted.mediaEncryptionInfo!;
  const ciphertext = encrypted.encryptedData.slice();
  expect([...ciphertext]).not.toEqual([...plain]);
  const opened = new EncryptedAttachment(ciphertext, info);
  try {
    expect(new TextDecoder().decode(Attachment.decrypt(opened))).toBe('private block');
  } finally {
    opened.free();
  }
});
