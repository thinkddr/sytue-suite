/**
 * Tests de tokenTieneOrganizacion.
 *
 * Lo que se guarda: un token sin `tenant` se reconoce (para poder avisar), y un
 * token que no se entiende NO se toma por uno sin organización. Equivocarse en
 * esa segunda dirección pondría un aviso de «te falta la suscripción» a clientes
 * que la tienen pagada, en las diez apps a la vez.
 *
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { tokenTieneOrganizacion } from './tenantClaim.js';

function jwt(payload: Record<string, unknown>): string {
  const b64url = (o: unknown) =>
    btoa(JSON.stringify(o)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `${b64url({ alg: 'RS256' })}.${b64url(payload)}.firma`;
}

test('reconoce un token CON organización', () => {
  expect(tokenTieneOrganizacion(jwt({ sub: 'u1', tenant: 'corporate' }))).toBe(true);
});

test('reconoce un token SIN organización (modo bootstrap)', () => {
  expect(tokenTieneOrganizacion(jwt({ sub: 'u1' }))).toBe(false);
  expect(tokenTieneOrganizacion(jwt({ sub: 'u1', tenant: '' }))).toBe(false);
  expect(tokenTieneOrganizacion(jwt({ sub: 'u1', tenant: null }))).toBe(false);
});

test('sin token no hay organización que enseñar', () => {
  expect(tokenTieneOrganizacion(undefined)).toBe(false);
  expect(tokenTieneOrganizacion('')).toBe(false);
});

test('un token ilegible NO se toma por uno sin organización', () => {
  // Ante la duda se calla: avisar de más es peor que no avisar, porque le diría
  // a un cliente que paga que no tiene suscripción.
  expect(tokenTieneOrganizacion('esto-no-es-un-jwt')).toBe(true);
  expect(tokenTieneOrganizacion('a.b.c')).toBe(true);
  expect(tokenTieneOrganizacion('a.@@@.c')).toBe(true);
});

test('el payload con relleno incompleto se decodifica igual', () => {
  // Los JWT van sin `=` de relleno; sin reponerlo, `atob` lanza y el token se
  // daría por ilegible. Se prueban varias longitudes para cubrir los tres restos.
  for (const nombre of ['a', 'ab', 'abc', 'abcd', 'abcde']) {
    expect(tokenTieneOrganizacion(jwt({ tenant: 'corporate', pad: nombre }))).toBe(true);
    expect(tokenTieneOrganizacion(jwt({ pad: nombre }))).toBe(false);
  }
});

test('acepta el alfabeto base64url con - y _', () => {
  // `tenant` es un slug ASCII, pero otros claims (nombres, correos) traen bytes
  // que sí producen `-` y `_` al codificar. Con el alfabeto sin convertir, esos
  // tokens se daban por ilegibles justo para los usuarios con acentos.
  const t = jwt({ tenant: 'corporate', name: 'Añoños ÿÿÿ', mail: 'a+b/c@sytue.com' });
  expect(t).toMatch(/[-_]/);
  expect(tokenTieneOrganizacion(t)).toBe(true);
});
