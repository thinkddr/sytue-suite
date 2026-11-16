/** Sitios web, personas relacionadas y aniversario: leer, editar y conservar lo oculto. */
import { expect, test } from 'bun:test';
import { buildVcard, normalizeWebsite, parseVcardDetails } from './vcard.js';

const BASE = [
  'BEGIN:VCARD',
  'VERSION:4.0',
  'UID:x',
  'FN:Ada',
  'URL;TYPE=work:https://acme.example',
  'URL:ftp://viejo.example',
  'RELATED;TYPE=spouse;VALUE=text:William\\, conde',
  'RELATED:urn:uuid:1234',
  'ANNIVERSARY:--0609',
  'END:VCARD',
].join('\r\n');

test('la ficha lee los sitios web http(s), las personas de texto y el aniversario', () => {
  const d = parseVcardDetails(BASE);
  expect(d.urls).toEqual([{ value: 'https://acme.example', type: 'work' }]);
  expect(d.related).toEqual([{ value: 'William, conde', type: 'spouse' }]);
  expect(d.anniversary).toBe('');
  expect(d.anniversaryManaged).toBe(false);
  const completo = parseVcardDetails('BEGIN:VCARD\r\nANNIVERSARY:20120609\r\nEND:VCARD');
  expect(completo.anniversary).toBe('2012-06-09');
  expect(completo.anniversaryManaged).toBe(true);
});

test('editar sustituye lo que la ficha enseña y conserva lo oculto', () => {
  const out = buildVcard({
    uid: 'x',
    fullName: 'Ada',
    emails: [],
    phones: [],
    urls: [
      { value: 'ada.example', type: 'home' },
      { value: 'javascript:alert(1)', type: 'other' },
    ],
    related: [{ value: 'Ana, la de Lugo', type: 'sibling' }],
    baseVcard: BASE,
  });
  expect(out).toContain('URL;TYPE=home:https://ada.example');
  expect(out).toContain('URL:ftp://viejo.example');
  expect(out).not.toContain('acme.example');
  expect(out).not.toContain('javascript:');
  expect(out).toContain('RELATED;VALUE=text;TYPE=sibling:Ana\\, la de Lugo');
  expect(out).toContain('RELATED:urn:uuid:1234');
  expect(out).not.toContain('William');
  // Sin gestionar el aniversario, el parcial se conserva; con fecha, se sustituye.
  expect(out).toContain('ANNIVERSARY:--0609');
  const conFecha = buildVcard({
    uid: 'x',
    fullName: 'Ada',
    emails: [],
    phones: [],
    anniversary: '2012-06-09',
    baseVcard: BASE,
  });
  expect(conFecha).toContain('ANNIVERSARY:2012-06-09');
  expect(conFecha).not.toContain('--0609');
  // Sin gestionar URL ni RELATED (fusión/importación), la base se queda tal cual.
  expect(conFecha).toContain('URL;TYPE=work:https://acme.example');
  expect(conFecha).toContain('William');
});

test('normalizeWebsite completa el esquema y rechaza lo que no es un sitio web', () => {
  expect(normalizeWebsite(' ejemplo.com ')).toBe('https://ejemplo.com');
  expect(normalizeWebsite('ejemplo.com:8080/x')).toBe('https://ejemplo.com:8080/x');
  expect(normalizeWebsite('http://a.example')).toBe('http://a.example');
  for (const malo of [
    '',
    'javascript:alert(1)',
    'mailto:a@b.c',
    'a b.com',
    'ftp://x.example',
    'https://banco.example@otro.example',
  ]) {
    expect(normalizeWebsite(malo)).toBeNull();
  }
});
