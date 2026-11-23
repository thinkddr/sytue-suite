// Tipos de campo con formato: URL, correo, teléfono, moneda y valoración. Lo mismo que
// prueba `TestValoresConFormato` en el servicio, del lado de la interfaz.
import { expect, test } from 'bun:test';
import type { BasesField } from '@sytue/api-clients/bases';
import { efectivo } from './formulas.js';
import { operadoresDe } from './vistas.js';
import { enlaceDeValor, estrellas, textoVisible, valorDeTexto } from './valores.js';

const campo = (kind: BasesField['kind'], extra = {}): BasesField => ({
  id: 'c',
  table_id: 't',
  name: 'C',
  kind,
  position: 0,
  ...extra,
});

test('una URL se completa con https y rechaza otros esquemas', () => {
  const url = campo('url');
  expect(valorDeTexto(url, 'sytue.eu/precios')).toBe('https://sytue.eu/precios');
  expect(valorDeTexto(url, 'https://Sytue.EU')).toBe('https://sytue.eu');
  expect(valorDeTexto(url, 'javascript:alert(1)')).toBeUndefined();
  expect(valorDeTexto(url, 'ftp://sytue.eu')).toBeUndefined();
  expect(valorDeTexto(url, 'https://u:p@sytue.eu')).toBeUndefined();
  expect(valorDeTexto(url, '  ')).toBeNull();
});

test('un correo exige dirección sola con dominio y lo pone en minúsculas', () => {
  const correo = campo('email');
  expect(valorDeTexto(correo, 'Ana@Ejemplo.ES')).toBe('Ana@ejemplo.es');
  expect(valorDeTexto(correo, 'Ana <ana@ejemplo.es>')).toBeUndefined();
  expect(valorDeTexto(correo, 'ana@localhost')).toBeUndefined();
});

test('un teléfono admite separadores y funde los espacios', () => {
  const tel = campo('phone');
  expect(valorDeTexto(tel, '+34  600 12 34 56')).toBe('+34 600 12 34 56');
  expect(valorDeTexto(tel, '12')).toBeUndefined();
  expect(valorDeTexto(tel, 'llámame')).toBeUndefined();
});

test('moneda en céntimos y con su símbolo; valoración de 0 a 5', () => {
  const eur = campo('currency', { currency: 'EUR' });
  expect(valorDeTexto(eur, '12,345')).toBe(12.35);
  expect(textoVisible(eur, 1234.5, 'es-ES').replace(/\s/gu, ' ')).toBe('1234,50 €');
  expect(textoVisible(campo('currency', { currency: 'USD' }), 3, 'en-US')).toBe('$3.00');
  const val = campo('rating');
  expect(valorDeTexto(val, '4')).toBe(4);
  expect(valorDeTexto(val, '0')).toBeNull();
  expect(valorDeTexto(val, '6')).toBeUndefined();
  expect(valorDeTexto(val, '2.5')).toBeUndefined();
  expect(textoVisible(val, 3)).toBe('★★★☆☆');
  expect(estrellas(9)).toBe('★★★★★');
});

test('los enlaces solo son http(s), mailto: y tel:', () => {
  expect(enlaceDeValor(campo('url'), 'https://sytue.eu')).toBe('https://sytue.eu');
  expect(enlaceDeValor(campo('url'), 'javascript:alert(1)')).toBeNull();
  expect(enlaceDeValor(campo('email'), 'ana@ejemplo.es')).toBe('mailto:ana@ejemplo.es');
  expect(enlaceDeValor(campo('phone'), '+34 600-12-34')).toBe('tel:+346001234');
  expect(enlaceDeValor(campo('text'), 'https://sytue.eu')).toBeNull();
});

test('el mailto: separa el dominio por la última @ y codifica cualquier otra', () => {
  // Antes solo se devolvía la primera `%40` a `@`: con dos, el dominio quedaba roto.
  expect(enlaceDeValor(campo('email'), '"a@b"@ejemplo.es')).toBe('mailto:%22a%40b%22@ejemplo.es');
  expect(enlaceDeValor(campo('email'), 'a@b@ejemplo.es')).toBe('mailto:a%40b@ejemplo.es');
  expect(enlaceDeValor(campo('email'), 'ana?cc=x@ejemplo.es')).toBe(
    'mailto:ana%3Fcc%3Dx@ejemplo.es',
  );
  expect(enlaceDeValor(campo('email'), 'sin-arroba')).toBe('mailto:sin-arroba');
});

test('se filtran como el número o el texto que guardan', () => {
  expect(efectivo(campo('currency')).kind).toBe('number');
  expect(efectivo(campo('rating')).kind).toBe('number');
  expect(efectivo(campo('email')).kind).toBe('text');
  expect(operadoresDe(campo('currency'))).toContain('gt');
  expect(operadoresDe(campo('url'))).toContain('contains');
});
