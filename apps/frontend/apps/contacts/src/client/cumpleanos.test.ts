import { expect, test } from 'bun:test';
import type { Contact } from '@sytue/api-clients';
import { cumpleanosDe, formatearCumpleanos, proximosCumpleanos } from './cumpleanos.js';

const contacto = (fullName: string, bday?: string): Contact => ({
  href: `/c/${fullName}`,
  uid: fullName,
  fullName,
  emails: [],
  phones: [],
  vcard: `BEGIN:VCARD\nFN:${fullName}\n${bday ? `BDAY:${bday}\n` : ''}END:VCARD`,
});

test('lee las cuatro formas del BDAY y descarta lo ilegible', () => {
  expect(cumpleanosDe('BDAY:19900215')).toEqual({ mes: 2, dia: 15, anyo: 1990 });
  expect(cumpleanosDe('BDAY:1990-02-15')).toEqual({ mes: 2, dia: 15, anyo: 1990 });
  expect(cumpleanosDe('BDAY:--0215')).toEqual({ mes: 2, dia: 15 });
  expect(cumpleanosDe('BDAY;VALUE=date:--02-15')).toEqual({ mes: 2, dia: 15 });
  expect(cumpleanosDe('BDAY:mañana')).toBeNull();
  expect(cumpleanosDe('FN:Sin fecha')).toBeNull();
  expect(cumpleanosDe('BDAY:19901315')).toBeNull();
});

test('respeta el grupo, el año omitido de Apple y los días que no existen', () => {
  expect(cumpleanosDe('FN:Ana\nitem1.BDAY:1990-02-15')).toEqual({ mes: 2, dia: 15, anyo: 1990 });
  expect(cumpleanosDe('BDAY;X-APPLE-OMIT-YEAR=1604:1604-02-15')).toEqual({ mes: 2, dia: 15 });
  expect(cumpleanosDe('BDAY;X-APPLE-OMIT-YEAR=1604:1990-02-15')).toEqual({
    mes: 2,
    dia: 15,
    anyo: 1990,
  });
  expect(cumpleanosDe('BDAY:--0231')).toBeNull();
  expect(cumpleanosDe('BDAY:19900229')).toBeNull();
  expect(cumpleanosDe('BDAY:--0229')).toEqual({ mes: 2, dia: 29 });
});

test('ordena del más cercano al más lejano, con hoy a cero y la edad que cumple', () => {
  const hoy = new Date(2026, 8, 9); // 9 de septiembre de 2026
  const lista = proximosCumpleanos(
    [
      contacto('Ada', '19951001'),
      contacto('Bea', '--0909'),
      contacto('Carl', '2000-09-08'),
      contacto('Dani'),
    ],
    hoy,
  );
  expect(lista.map((c) => [c.contact.fullName, c.dias, c.edad ?? null])).toEqual([
    ['Bea', 0, null],
    ['Ada', 22, 31],
    ['Carl', 364, 27],
  ]);
});

test('la ventana recorta y un 29 de febrero no rompe nada', () => {
  const hoy = new Date(2026, 8, 9);
  expect(proximosCumpleanos([contacto('Ada', '19951001')], hoy, 7)).toEqual([]);
  const bisiesto = proximosCumpleanos([contacto('Leo', '20000229')], hoy);
  expect(bisiesto).toHaveLength(1);
  expect(bisiesto[0]?.fecha.getFullYear()).toBe(2027);
});

test('formatea el cumpleaños con año y, sin año, solo el día y el mes', () => {
  const sinAnyo = cumpleanosDe('BDAY:--0215');
  expect(sinAnyo).not.toBeNull();
  expect(formatearCumpleanos(sinAnyo!, 'es-ES')).toBe('15 de febrero');
  expect(formatearCumpleanos(sinAnyo!, 'en-GB')).toBe('15 February');
  expect(formatearCumpleanos(cumpleanosDe('BDAY:1990-02-15')!, 'es-ES')).toBe(
    '15 de febrero de 1990',
  );
  // Un 29 de febrero sin año no se corre al 1 de marzo.
  expect(formatearCumpleanos(cumpleanosDe('BDAY:--0229')!, 'es-ES')).toBe('29 de febrero');
});
