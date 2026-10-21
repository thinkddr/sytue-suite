/** Chequeo mínimo de la fusión de contactos (unión + dedupe + elección). */
import { expect, test } from 'bun:test';
import type { Contact } from '@sytue/api-clients';
import { buildMergeInput, unionValues } from './merge.js';

function contact(partial: Partial<Contact>): Contact {
  return { href: '/c.vcf', uid: 'u', fullName: 'X', emails: [], phones: [], vcard: '', ...partial };
}

test('unionValues: trim + dedupe case-insensitive, conserva orden y primera grafía', () => {
  expect(unionValues(['Ana@sytue.eu ', '', '+34 600'], ['ana@SYTUE.eu', 'b@sytue.eu'])).toEqual([
    'Ana@sytue.eu',
    '+34 600',
    'b@sytue.eu',
  ]);
});

test('buildMergeInput: unión de correos/teléfonos, uid y nombre del superviviente por defecto', () => {
  const survivor = contact({
    uid: 'a',
    fullName: 'Ana',
    emails: ['a@x.eu'],
    phones: ['+34 600 1'],
    organization: 'ACME',
  });
  const loser = contact({
    uid: 'b',
    fullName: 'Ana López',
    emails: ['A@X.EU', 'b@x.eu'],
    phones: ['+34 600 2'],
  });
  expect(buildMergeInput(survivor, loser)).toEqual({
    uid: 'a',
    fullName: 'Ana',
    emails: ['a@x.eu', 'b@x.eu'],
    phones: ['+34 600 1', '+34 600 2'],
    organization: 'ACME',
  });
});

test('buildMergeInput: respeta la elección de nombre y hereda organización del perdedor', () => {
  const survivor = contact({ uid: 'a', fullName: 'Ana' });
  const loser = contact({ uid: 'b', fullName: 'Ana López', organization: 'ACME' });
  const merged = buildMergeInput(survivor, loser, { fullName: 'Ana López' });
  expect(merged.fullName).toBe('Ana López');
  expect(merged.organization).toBe('ACME');
});
