/** Chequeo de la detección de duplicados (agrupación por correo/teléfono/nombre + transitividad). */
import { expect, test } from 'bun:test';
import type { Contact } from '@sytue/api-clients';
import { findDuplicateGroups } from './duplicates.js';

function contact(partial: Partial<Contact>): Contact {
  return { href: '/x.vcf', uid: 'u', fullName: 'X', emails: [], phones: [], vcard: '', ...partial };
}

test('agrupa por correo compartido pese a mayúsculas/espacios', () => {
  const a = contact({ uid: 'a', href: '/a', fullName: 'Ana', emails: ['Ana@Sytue.EU '] });
  const b = contact({ uid: 'b', href: '/b', fullName: 'Ana López', emails: ['ana@sytue.eu'] });
  const solo = contact({ uid: 's', href: '/s', fullName: 'Otro', emails: ['otro@x.eu'] });
  const groups = findDuplicateGroups([a, b, solo]);
  expect(groups.length).toBe(1);
  expect(groups[0]!.map((x) => x.uid).sort()).toEqual(['a', 'b']);
});

test('agrupa por teléfono ignorando el formato', () => {
  const a = contact({ uid: 'a', href: '/a', fullName: 'Sin datos', phones: ['+34 600 102 030'] });
  const b = contact({ uid: 'b', href: '/b', fullName: 'Otra grafía', phones: ['34-600-102-030'] });
  expect(findDuplicateGroups([a, b]).length).toBe(1);
});

test('transitividad: A~B por correo y B~C por teléfono forman un solo grupo', () => {
  const a = contact({ uid: 'a', href: '/a', emails: ['x@y.eu'] });
  const b = contact({ uid: 'b', href: '/b', emails: ['x@y.eu'], phones: ['600102030'] });
  const c = contact({ uid: 'c', href: '/c', phones: ['600 10 20 30'] });
  const groups = findDuplicateGroups([a, b, c]);
  expect(groups.length).toBe(1);
  expect(groups[0]!.length).toBe(3);
});

test('no agrupa sin coincidencias ni por el placeholder «(Sin nombre)»', () => {
  const a = contact({ uid: 'a', href: '/a', fullName: '(Sin nombre)' });
  const b = contact({ uid: 'b', href: '/b', fullName: '(Sin nombre)' });
  const c = contact({ uid: 'c', href: '/c', fullName: 'Único', emails: ['solo@x.eu'] });
  expect(findDuplicateGroups([a, b, c])).toEqual([]);
});

test('teléfonos demasiado cortos no agrupan (extensiones)', () => {
  const a = contact({ uid: 'a', href: '/a', fullName: 'A', phones: ['123'] });
  const b = contact({ uid: 'b', href: '/b', fullName: 'B', phones: ['123'] });
  expect(findDuplicateGroups([a, b])).toEqual([]);
});
