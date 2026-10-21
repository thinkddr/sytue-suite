// Checks de los cursores anclados y de la presencia (./cursors.ts). Ejecuta: `bun test`.
import { expect, test } from 'bun:test';
import { anclarOffset, anclarPos, cambioDePresencia, quienCambia } from './cursors.js';

test('sin cambios el offset no se mueve', () => {
  expect(anclarOffset(3, 'hola mundo', 'hola mundo')).toBe(3);
});

test('alguien escribe ANTES del cursor: el cursor se desplaza con su texto', () => {
  // el peer está tras «hola» (4); otra persona antepone «¡» al texto
  expect(anclarOffset(4, 'hola mundo', '¡hola mundo')).toBe(5);
  // y borra «ho» del principio
  expect(anclarOffset(4, 'hola mundo', 'la mundo')).toBe(2);
});

test('la edición queda DESPUÉS del cursor: no se mueve', () => {
  expect(anclarOffset(4, 'hola mundo', 'hola mundo cruel')).toBe(4);
  expect(anclarOffset(0, 'hola', 'xhola')).toBe(0);
});

test('el cursor está justo donde se inserta: se queda antes de lo insertado', () => {
  expect(anclarOffset(4, 'hola mundo', 'hola, qué tal mundo')).toBe(4);
});

test('el cursor estaba dentro de lo borrado: al principio de la zona', () => {
  expect(anclarOffset(7, 'hola mundo cruel', 'hola cruel')).toBe(5);
});

test('texto repetido: el reparto prefijo/sufijo no se pasa del texto', () => {
  expect(anclarOffset(2, 'aaaa', 'aaaaa')).toBe(2);
  expect(anclarOffset(4, 'aaaa', 'aaaaa')).toBe(5);
  expect(anclarOffset(1, 'a', '')).toBe(0);
});

test('un caret al final del texto sigue al final cuando se añade detrás', () => {
  expect(anclarOffset(4, 'hola', 'hola!')).toBe(5);
  expect(anclarOffset(4, 'hola', 'hola mundo')).toBe(10);
  // a mitad de texto no se mueve: la edición cae detrás de él
  expect(anclarOffset(2, 'hola', 'hola!')).toBe(2);
});

test('anclarPos mueve ancla y foco de la selección', () => {
  expect(anclarPos({ kind: 'doc', anchor: 5, focus: 10 }, 'hola mundo', 'oh hola mundo')).toEqual({
    kind: 'doc',
    anchor: 8,
    focus: 13,
  });
});

test('presencia: entra, sale o nada', () => {
  expect(cambioDePresencia(['a'], ['a', 'b'])).toBe('entra');
  expect(cambioDePresencia(['a', 'b'], ['a'])).toBe('sale');
  expect(cambioDePresencia(['a', 'b'], ['b', 'a'])).toBeNull();
  expect(cambioDePresencia(['a', 'b'], ['a', 'c'])).toBe('entra');
  expect(cambioDePresencia([], [])).toBeNull();
});

test('quienCambia: quién entra y quién sale', () => {
  expect(quienCambia(['a', 'b'], ['b', 'c'])).toEqual({ entran: ['c'], salen: ['a'] });
  expect(quienCambia(['a'], ['a'])).toEqual({ entran: [], salen: [] });
});
