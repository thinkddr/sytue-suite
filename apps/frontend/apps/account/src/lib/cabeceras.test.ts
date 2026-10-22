/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { cabecerasConToken } from './cabeceras.js';

// El caso que motivó la función: `HeadersInit` admite un array de pares, y
// esparcirlo dentro de un objeto lo convierte en `{0: [...], 1: [...]}`. Las
// cabeceras de quien llama desaparecían sin error.
test('conserva las cabeceras en las tres formas de HeadersInit', () => {
  const esperado = 'application/json';
  for (const init of [
    { 'x-propia': 'sí' },
    [['x-propia', 'sí']] as [string, string][],
    new Headers({ 'x-propia': 'sí' }),
  ]) {
    const h = cabecerasConToken(init, 'tok');
    expect(h.get('x-propia')).toBe('sí');
    expect(h.get('authorization')).toBe('Bearer tok');
    expect(h.get('accept')).toBe(esperado);
  }
});

test('sin cabeceras propias, solo pone las suyas', () => {
  const h = cabecerasConToken(undefined, 'tok');
  expect(h.get('authorization')).toBe('Bearer tok');
  expect(h.get('accept')).toBe('application/json');
});

// El token del proxy MANDA: una petición no puede colar su propia autorización
// y hablar con Sytue ID en nombre de otro.
test('la autorización de quien llama no puede pisar la del proxy', () => {
  const h = cabecerasConToken({ authorization: 'Bearer robado' }, 'bueno');
  expect(h.get('authorization')).toBe('Bearer bueno');
});

// «Mi cuenta» se sirve en el origen de Sytue ID y el navegador le manda sus
// cookies. Riesgo aceptado a condición de no leerlas ni reenviarlas
// (docs/security.md#riesgo-aceptado-mi-cuenta-en-el-origen-del-idp): el proxy
// tira cualquier `cookie` que le llegue, y ningún fichero de la app las nombra.
test('el proxy hacia Sytue ID no reenvía cookies', () => {
  const h = cabecerasConToken({ cookie: '__Host-sytue_id_session=robada' }, 'tok');
  expect(h.get('cookie')).toBeNull();
  expect(h.get('authorization')).toBe('Bearer tok');
});

test('ningún fichero de la app lee las cookies del IdP', async () => {
  const { Glob } = await import('bun');
  const prohibidos = [
    'sytue_id_session',
    'sytue_device',
    'sytue_id_mfa',
    'sytue_id_fed',
    'sytue_id_saml',
    'deviceCookieHeader',
    'DEVICE_COOKIE',
  ];
  const raiz = new URL('../', import.meta.url).pathname;
  const culpables: string[] = [];
  for await (const f of new Glob('**/*.{ts,tsx,astro}').scan(raiz)) {
    if (f.endsWith('.test.ts')) continue;
    const texto = await Bun.file(raiz + f).text();
    for (const p of prohibidos) if (texto.includes(p)) culpables.push(`${f}: ${p}`);
  }
  expect(culpables).toEqual([]);
});
