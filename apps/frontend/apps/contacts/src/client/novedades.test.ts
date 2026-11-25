/**
 * Lo nuevo de la lista de Contacts (los mismos casos que `contactos/novedades_test.go`
 * en Fyne): la búsqueda por apodo, cargo, ciudad y nota; los aniversarios con los
 * cumpleaños; ordenar por apellido; copiar los correos sin tope; y «#» a la papelera
 * en la hoja de atajos.
 *
 * Ejecutar: `bun test`.
 */
import { describe, expect, test } from 'bun:test';
import type { Contact } from '@sytue/api-clients';
import { accionDeAtajo } from '../components/hojaDeAtajos.js';
import { apellidoDe, textoBuscable } from './busqueda.js';
import { proximosCumpleanos } from './cumpleanos.js';
import { claveDeOrden, ordenar } from './orden.js';
import {
  TOPE_DESTINATARIOS,
  correosDe,
  correosParaCopiar,
  enlaceRedactar,
  textoDeFicha,
} from './seleccion.js';

function contacto(uid: string, nombre: string, lineas: string[] = [], correo?: string): Contact {
  const vcard = [
    'BEGIN:VCARD',
    'VERSION:3.0',
    `UID:${uid}`,
    `FN:${nombre}`,
    ...lineas,
    'END:VCARD',
  ].join('\r\n');
  return {
    href: `/c/${uid}.vcf`,
    uid,
    fullName: nombre,
    emails: correo ? [correo] : [],
    phones: [],
    vcard,
  };
}

describe('búsqueda', () => {
  test('apodo, cargo, ciudad y nota cuentan', () => {
    const t = textoBuscable(
      contacto('a', 'Ana Ruiz', [
        'NICKNAME:Pili',
        'TITLE:Abogada',
        'ADR;TYPE=home:;;Calle Mayor 1;Zaragoza;;50001;España',
        'NOTE:Conocida en el congreso',
      ]).vcard,
    );
    for (const palabra of ['Pili', 'Abogada', 'Zaragoza', 'congreso']) expect(t).toContain(palabra);
  });
});

describe('aniversarios', () => {
  test('salen con los cumpleaños, marcados y sin edad', () => {
    const c = contacto('a', 'Ana', ['BDAY:1990-11-26', 'ANNIVERSARY:2015-11-27']);
    const l = proximosCumpleanos([c], new Date(2026, 10, 25), 30);
    expect(l.map((x) => [x.dias, x.aniversario ?? false, x.edad])).toEqual([
      [1, false, 36],
      [2, true, undefined],
    ]);
  });
});

describe('ordenar por apellido', () => {
  test('por el apellido y, sin él, por el nombre', () => {
    const zoe = contacto('z', 'Zoe Abad', ['N:Abad;Zoe;;;']);
    const ana = contacto('a', 'Ana Zubiri', ['N:Zubiri;Ana;;;']);
    const acme = contacto('c', 'Acme');
    const clave = (c: Contact): string => claveDeOrden(c.fullName, apellidoDe(c.vcard), 'apellido');
    expect(ordenar([ana, acme, zoe], clave, (c) => c.fullName, 'es').map((c) => c.uid)).toEqual([
      'z',
      'c',
      'a',
    ]);
    expect(claveDeOrden('Ana Zubiri', 'Zubiri', 'nombre')).toBe('Ana Zubiri');
  });
});

describe('copiar los correos', () => {
  test('sin repetir y sin el tope del enlace a Mail', () => {
    const cs = Array.from({ length: TOPE_DESTINATARIOS + 5 }, (_, i) =>
      contacto(`u${i}`, `P${i}`, [], `p${i}@sytue.eu`),
    );
    cs.push(contacto('dup', 'Dup', [], 'P0@SYTUE.EU'));
    expect(correosParaCopiar(cs)).toHaveLength(TOPE_DESTINATARIOS + 5);
    expect(correosDe(cs).correos).toHaveLength(TOPE_DESTINATARIOS);
  });
});

test('«#» manda a la papelera', () => {
  expect(accionDeAtajo('#')).toBe('papelera');
});

describe('compartir la ficha por correo', () => {
  test('el texto es el de Fyne y el enlace abre el redactor con asunto y cuerpo', () => {
    const texto = textoDeFicha(
      'Ana Ruiz',
      {
        title: 'Abogada',
        emails: [{ value: 'ana@ruiz.eu', type: 'work' }],
        phones: [{ value: '+34 600 000 000', type: 'cell' }],
        addresses: [
          {
            type: 'work',
            street: 'Calle Mayor 1',
            postalCode: '50001',
            city: 'Zaragoza',
            country: 'España',
          },
        ],
      },
      'Ruiz y Asociados',
    );
    expect(texto).toBe(
      'Ana Ruiz\nAbogada · Ruiz y Asociados\nana@ruiz.eu\n+34 600 000 000\nCalle Mayor 1, 50001 Zaragoza, España',
    );
    const url = new URL(
      enlaceRedactar('https://acme.sytue.eu/mail', { subject: 'Ana Ruiz', body: texto }),
    );
    expect(url.searchParams.get('action')).toBe('compose');
    expect(url.searchParams.get('subject')).toBe('Ana Ruiz');
    expect(url.searchParams.get('body')).toBe(texto);
    expect(url.searchParams.has('to')).toBe(false);
  });
});
