/** Chequeo de buildVcard: usa la proyección nueva pero conserva los campos no proyectados de la base. */
import { expect, test } from 'bun:test';
import { buildVcard, newUid, preservedLines, parseVcardDetails } from './vcard.js';

const BASE =
  [
    'BEGIN:VCARD',
    'VERSION:4.0',
    'UID:old-uid',
    'FN:Ada Vieja',
    'N:Vieja;Ada;;;',
    'EMAIL;TYPE=home:old@x.eu',
    'TEL;TYPE=home:+34 000',
    'ORG:OrgVieja',
    'NICKNAME:Adita',
    'BDAY:1815-12-10',
    'item1.ADR;TYPE=home:;;Calle 1;Madrid;;28001;ES',
    'item1.X-ABLabel:Casa',
    'X-CUSTOM:algo',
    'REV:20200101T000000Z',
    'END:VCARD',
  ].join('\r\n') + '\r\n';

test('newUid genera un identificador aleatorio UUID', () => {
  expect(newUid()).toMatch(/^[0-9a-f-]{36}@sytue\.eu$/i);
});

test('preservedLines: conserva no proyectadas (BDAY/NICKNAME/ADR/X-*), descarta las gestionadas', () => {
  expect(preservedLines(BASE)).toEqual([
    'NICKNAME:Adita',
    'BDAY:1815-12-10',
    'item1.ADR;TYPE=home:;;Calle 1;Madrid;;28001;ES',
    'item1.X-ABLabel:Casa',
    'X-CUSTOM:algo',
  ]);
});

test('buildVcard con base: proyección nueva + extras conservados, sin datos viejos gestionados', () => {
  const out = buildVcard({
    uid: 'new-uid',
    fullName: 'Ada Lovelace',
    emails: ['ada@sytue.eu'],
    phones: [],
    baseVcard: BASE,
  });
  // Proyección nueva (gestionada):
  expect(out).toContain('UID:new-uid');
  expect(out).toContain('FN:Ada Lovelace');
  expect(out).toContain('EMAIL;TYPE=work:ada@sytue.eu');
  // Extras conservados de la base:
  expect(out).toContain('BDAY:1815-12-10');
  expect(out).toContain('NICKNAME:Adita');
  expect(out).toContain('item1.ADR;TYPE=home:;;Calle 1;Madrid;;28001;ES');
  expect(out).toContain('X-CUSTOM:algo');
  // Los valores viejos de campos gestionados NO se cuelan:
  expect(out).not.toContain('old-uid');
  expect(out).not.toContain('old@x.eu');
  expect(out).not.toContain('Ada Vieja');
  expect(out).not.toContain('REV:20200101');
});

test('buildVcard sin base: solo la proyección (crear/importar)', () => {
  const out = buildVcard({ uid: 'u', fullName: 'X', emails: [], phones: [] });
  expect(out).not.toContain('BDAY');
  expect(out.startsWith('BEGIN:VCARD')).toBe(true);
  expect(out.endsWith('END:VCARD\r\n')).toBe(true);
});

test('preservedLines: desdobla una PHOTO plegada (data URI) en una sola línea', () => {
  const folded =
    'BEGIN:VCARD\r\nVERSION:4.0\r\nFN:X\r\nPHOTO:data:image/png;base64,AAAA\r\n BBBB\r\n CCCC\r\nEND:VCARD\r\n';
  expect(preservedLines(folded)).toEqual(['PHOTO:data:image/png;base64,AAAABBBBCCCC']);
});

test('buildVcard: emite EMAIL/TEL tipados con principal (PREF), ADR y BDAY', () => {
  const out = buildVcard({
    uid: 'u1',
    fullName: 'Grace Hopper',
    firstName: 'Grace',
    lastName: 'Hopper',
    emails: [],
    phones: [],
    emailsDetailed: [
      { value: 'grace@work.eu', type: 'work' },
      { value: 'grace@home.eu', type: 'home', primary: true },
    ],
    phonesDetailed: [{ value: '+34600000001', type: 'cell', primary: true }],
    addresses: [
      {
        type: 'work',
        street: '1 Trafalgar Sq',
        city: 'London',
        postalCode: 'WC2N',
        country: 'UK',
        primary: true,
      },
    ],
    birthday: '1906-12-09',
  });
  expect(out).toContain('EMAIL;TYPE=work:grace@work.eu');
  expect(out).toContain('EMAIL;TYPE=home;PREF=1:grace@home.eu');
  expect(out).toContain('TEL;TYPE=cell;PREF=1:+34600000001');
  expect(out).toContain('ADR;TYPE=work;PREF=1:;;1 Trafalgar Sq;London;;WC2N;UK');
  expect(out).toContain('BDAY:1906-12-09');
});

test('buildVcard: al gestionar ADR/BDAY reemplaza los de la base sin duplicar', () => {
  const base =
    [
      'BEGIN:VCARD',
      'VERSION:4.0',
      'FN:Vieja',
      'ADR;TYPE=home:;;Vieja;Nowhere;;00000;XX',
      'BDAY:1999-01-01',
      'X-CUSTOM:algo',
      'END:VCARD',
    ].join('\r\n') + '\r\n';
  const out = buildVcard({
    uid: 'u1',
    fullName: 'Grace',
    emails: [],
    phones: [],
    addresses: [{ type: 'work', city: 'Madrid' }],
    birthday: '2000-02-02',
    baseVcard: base,
  });
  // Una sola ADR y un solo BDAY (los de la base, gestionados, no se cuelan).
  expect(out.match(/^ADR/gm)?.length ?? 0).toBe(1);
  expect(out.match(/^BDAY/gm)?.length ?? 0).toBe(1);
  expect(out).toContain('ADR;TYPE=work:;;;Madrid;;;');
  expect(out).toContain('BDAY:2000-02-02');
  expect(out).not.toContain('Nowhere');
  expect(out).not.toContain('1999-01-01');
  // Lo no gestionado de la base se conserva.
  expect(out).toContain('X-CUSTOM:algo');
});

test('buildVcard: sin gestionar ADR/BDAY (import/fusión) los conserva de la base', () => {
  const base =
    'BEGIN:VCARD\r\nVERSION:4.0\r\nFN:X\r\nADR;TYPE=home:;;Calle 1;Madrid;;28001;ES\r\nBDAY:--12-10\r\nEND:VCARD\r\n';
  // Sin `addresses`/`birthday` en el input: no se tocan.
  const out = buildVcard({ uid: 'u', fullName: 'X', emails: [], phones: [], baseVcard: base });
  expect(out).toContain('ADR;TYPE=home:;;Calle 1;Madrid;;28001;ES');
  expect(out).toContain('BDAY:--12-10');
});

test('parseVcardDetails: lee tipos, principal, ADR, BDAY y nota; round-trip con buildVcard', () => {
  const card = buildVcard({
    uid: 'u1',
    fullName: 'Grace Hopper',
    firstName: 'Grace',
    lastName: 'Hopper',
    emails: [],
    phones: [],
    emailsDetailed: [
      { value: 'grace@work.eu', type: 'work' },
      { value: 'grace@home.eu', type: 'home', primary: true },
    ],
    phonesDetailed: [{ value: '+34600000001', type: 'fax' }],
    addresses: [
      {
        type: 'work',
        street: 'Calle, con coma',
        city: 'Madrid',
        postalCode: '28001',
        country: 'ES',
      },
    ],
    birthday: '1906-12-09',
    note: 'Con; punto y coma',
  });
  const d = parseVcardDetails(card);
  expect(d.firstName).toBe('Grace');
  expect(d.lastName).toBe('Hopper');
  expect(d.emails).toEqual([
    { value: 'grace@work.eu', type: 'work', primary: false },
    { value: 'grace@home.eu', type: 'home', primary: true },
  ]);
  expect(d.phones[0]).toEqual({ value: '+34600000001', type: 'fax', primary: false });
  expect(d.addresses).toEqual([
    {
      type: 'work',
      street: 'Calle, con coma',
      city: 'Madrid',
      region: '',
      postalCode: '28001',
      country: 'ES',
      primary: false,
    },
  ]);
  expect(d.birthday).toBe('1906-12-09');
  expect(d.birthdayManaged).toBe(true);
  expect(d.note).toBe('Con; punto y coma');
});

test('parseVcardDetails: BDAY sin año no es gestionable (se conservará verbatim)', () => {
  const card = 'BEGIN:VCARD\r\nVERSION:4.0\r\nFN:X\r\nBDAY:--12-10\r\nEND:VCARD\r\n';
  const d = parseVcardDetails(card);
  expect(d.birthday).toBe('');
  expect(d.birthdayManaged).toBe(false);
});
