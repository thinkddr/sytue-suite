/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { isOwnerWorkerMessage } from './worker-origin.js';

const propio = 'https://app.sytue.test';

test('acepta los mensajes del documento propietario (origen vacío o el propio)', () => {
  expect(isOwnerWorkerMessage(new MessageEvent('message', { data: {} }), propio)).toBe(true);
  expect(isOwnerWorkerMessage({ origin: propio }, propio)).toBe(true);
});

test('descarta mensajes de cualquier otro origen', () => {
  expect(isOwnerWorkerMessage({ origin: 'https://ajeno.test' }, propio)).toBe(false);
  expect(isOwnerWorkerMessage({ origin: 'null' }, propio)).toBe(false);
  expect(isOwnerWorkerMessage({ origin: 'https://app.sytue.test.ajeno.test' }, propio)).toBe(false);
});
