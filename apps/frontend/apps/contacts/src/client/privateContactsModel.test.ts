/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import {
  privateBook,
  privateContact,
  type PrivateContactSnapshot,
} from './privateContactsModel.js';

const id = '11111111-1111-4111-8111-111111111111';
const bookId = '22222222-2222-4222-8222-222222222222';
const revision = '33333333-3333-4333-8333-333333333333';
const card = [
  'BEGIN:VCARD',
  'VERSION:4.0',
  'UID:urn:uuid:existing-contact',
  'FN:Ada Lovelace',
  'N:Lovelace;Ada;;;',
  'BDAY:--1210',
  'EMAIL;TYPE=work;PREF=1:ada@example.invalid',
  'PHOTO:data:image/png;base64,iVBORw0KGgo=',
  'X-KEEP:private opaque extension',
  'END:VCARD',
  '',
].join('\r\n');

function fixture(): PrivateContactSnapshot {
  return {
    revision,
    vcard: card,
    contact: {
      id,
      address_book_id: bookId,
      uid: 'existing-contact',
      etag: revision,
      formatted_name: 'Ada Lovelace',
      given_name: 'Ada',
      family_name: 'Lovelace',
      nickname: 'Enchantress',
      has_photo: true,
      photo_url: 'https://example.invalid/private-external-photo',
      emails: [{ email: 'ada@example.invalid', type: 'work', is_primary: true }],
      phones: [],
      addresses: [{ po_box: '42', extended: 'Floor 3', street: 'Private street' }],
    },
  };
}

test('private presentation retains IDs and exact card without creating a URL or href', () => {
  const detail = privateContact(fixture());
  expect(detail.id).toBe(id);
  expect(detail.bookId).toBe(bookId);
  expect(detail.uid).toBe('existing-contact');
  expect(detail.revision).toBe(revision);
  expect(detail.vcard).toBe(card);
  expect(detail.vcardDetails.birthdayManaged).toBe(false);
  expect(detail.emails).toEqual(['ada@example.invalid']);
  expect(detail.hasPhoto).toBe(true);
  expect(detail.addresses[0]?.poBox).toBe('42');
  expect(detail.addresses[0]?.extended).toBe('Floor 3');
  expect('href' in detail).toBe(false);
  expect('photo_url' in detail).toBe(false);
  expect(
    'url' in
      privateBook({ id: bookId, name: 'private', display_name: 'Private', is_default: true }),
  ).toBe(false);
});

test('edit describes only requested changes against original ID and revision', () => {
  const original = fixture();
  const detail = privateContact(original);
  // A later mutation of the caller's projection must not retarget the edit.
  original.contact.id = bookId;
  original.contact.etag = bookId;
  const planned = detail.planUpdate({ note: 'Changed' });
  expect(planned).toEqual({
    id,
    etag: revision,
    changes: { note: 'Changed' },
  });
  expect('nickname' in planned.changes).toBe(false);
  expect('birthday' in planned.changes).toBe(false);
  expect('addresses' in planned.changes).toBe(false);
  expect('photo_url' in planned.changes).toBe(false);
  expect(detail.vcard).toContain('X-KEEP:private opaque extension');
});

test('rejects revisions that disagree and keeps partial birthdays intact', () => {
  const mismatched = fixture();
  mismatched.revision = bookId;
  expect(() => privateContact(mismatched)).toThrow('private_contacts_invalid_snapshot');
  const nilId = fixture();
  nilId.contact.id = '00000000-0000-0000-0000-000000000000';
  expect(() => privateContact(nilId)).toThrow();
  // The repository fixture in contacts/views_test.go intentionally carries a
  // complete normalized birthday next to an original yearless BDAY.
  const partial = fixture();
  partial.contact.birthday = '1815-12-10T00:00:00Z';
  const mixed = privateContact(partial);
  expect(mixed.editor.birthday).toBe('1815-12-10');
  expect(mixed.vcardDetails.birthday).toBe('');
  expect(mixed.vcardDetails.birthdayManaged).toBe(false);
  expect(mixed.vcard).toContain('BDAY:--1210');
  expect(mixed.planUpdate({ note: 'Only note' }).changes).toEqual({ note: 'Only note' });
  const detail = privateContact(fixture());
  expect(() => detail.planUpdate({ birthday: '' })).toThrow('private_contacts_invalid_snapshot');
  expect(detail.planUpdate({ birthday: '2000-02-29' }).changes).toEqual({ birthday: '2000-02-29' });
});

test('collection changes remain blocked when element identity is unavailable', () => {
  const source = fixture();
  source.contact.emails = [
    { email: 'same@example.invalid', type: 'work', is_primary: true },
    { email: 'same@example.invalid', type: 'home', is_primary: false },
  ];
  source.vcard = source.vcard.replace(
    'EMAIL;TYPE=work;PREF=1:ada@example.invalid',
    'item1.EMAIL;TYPE=work;PREF=1:same@example.invalid\r\nitem1.X-ABLabel:Work\r\nitem2.EMAIL;TYPE=home:same@example.invalid\r\nitem2.X-ABLabel:Home',
  );
  const detail = privateContact(source);
  expect(detail.editor.emails).toEqual([
    { value: 'same@example.invalid', type: 'work', primary: true },
    { value: 'same@example.invalid', type: 'home', primary: false },
  ]);
  expect(() => detail.planUpdate({ emails: [detail.editor.emails[0]!] })).toThrow(
    'private_contacts_item_identity_unavailable',
  );
  expect(() => detail.planUpdate({ phones: [] })).toThrow(
    'private_contacts_item_identity_unavailable',
  );
  expect(() => detail.planUpdate({ addresses: detail.addresses })).toThrow(
    'private_contacts_item_identity_unavailable',
  );
  expect(detail.vcard).toContain('item2.X-ABLabel:Home');
});

test('editor uses normalized fields even when original vCard omits them', () => {
  const source = fixture();
  source.vcard = source.vcard
    .replace('FN:Ada Lovelace', 'FN:Old name')
    .replace('EMAIL;TYPE=work;PREF=1:ada@example.invalid\r\n', '');
  source.contact.note = 'Private note';
  source.contact.birthday = '1815-12-10T00:00:00Z';
  const detail = privateContact(source);
  expect(detail.fullName).toBe('Ada Lovelace');
  expect(detail.editor.note).toBe('Private note');
  expect(detail.editor.emails).toEqual([
    { value: 'ada@example.invalid', type: 'work', primary: true },
  ]);
  expect(detail.vcardDetails.emails).toEqual([]);
  expect(detail.planUpdate({ note: 'Changed' }).changes).toEqual({ note: 'Changed' });
  expect(detail.vcard).toContain('FN:Old name');
});

test('the update plan omits dates from vCard when no date was edited', () => {
  const full = fixture();
  full.vcard = full.vcard.replace('BDAY:--1210', 'BDAY:2000-02-29');
  const detail = privateContact(full);
  expect(detail.vcardDetails.birthday).toBe('2000-02-29');
  expect(detail.planUpdate({ note: '' }).changes).toEqual({ note: '' });
  full.contact.birthday = '2001-02-28T00:00:00Z';
  const mixed = privateContact(full);
  expect(mixed.editor.birthday).toBe('2001-02-28');
  expect(mixed.vcardDetails.birthday).toBe('2000-02-29');
});

test('no CardDAV or image request is made by model adaptation', () => {
  const original = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = (() => {
    calls++;
    throw new Error('unexpected network');
  }) as unknown as typeof fetch;
  try {
    const detail = privateContact(fixture());
    detail.planUpdate({ note: 'Local-only patch' });
    privateBook({ id: bookId, name: 'private', display_name: 'Private', is_default: false });
    expect(calls).toBe(0);
  } finally {
    globalThis.fetch = original;
  }
});
