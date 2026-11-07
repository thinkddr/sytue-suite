/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { HttpError } from '@sytue/api-clients';
import { errorKey, returnErrorKey } from './externalMailErrors.js';

test('the provider return tells cancelled, missing permission, expired and repeated apart', () => {
  expect(returnErrorKey('denied')).toBe('externalMail.cancelled');
  expect(returnErrorKey('scope')).toBe('externalMail.scopeMissing');
  expect(returnErrorKey('state')).toBe('externalMail.timeout');
  expect(returnErrorKey('exists')).toBe('externalMail.exists');
  for (const code of ['exchange', 'provider', ''])
    expect(returnErrorKey(code)).toBe('externalMail.authError');
});

test('a mailbox already connected by hand is not a generic error', () => {
  const conflict = new HttpError(
    409,
    'Conflict',
    '/external-mail/accounts',
    JSON.stringify({ error: { code: 'mail_account_exists', params: { instance_id: 'a' } } }),
  );
  expect(errorKey(conflict)).toBe('externalMail.exists');
  expect(errorKey(new Error('offline'))).toBe('externalMail.error');
});
