// Checks de la lógica pura de Web Push (Ola 18). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { urlBase64ToUint8Array } from './push.js';

// La clave VAPID viaja en base64url SIN relleno; el navegador necesita el Uint8Array crudo.
test('urlBase64ToUint8Array decodifica base64url sin relleno al byte array correcto', () => {
  // "hello" = 68 65 6c 6c 6f; en base64url sin relleno: "aGVsbG8".
  const got = urlBase64ToUint8Array('aGVsbG8');
  expect(Array.from(got)).toEqual([0x68, 0x65, 0x6c, 0x6c, 0x6f]);
});

test('urlBase64ToUint8Array traduce el alfabeto url-safe (-_) al estándar (+/)', () => {
  // Bytes 0xFB 0xFF 0xBF → base64 estándar "+/+/" ; url-safe "-_-_".
  const std = urlBase64ToUint8Array('+/+/');
  const url = urlBase64ToUint8Array('-_-_');
  expect(Array.from(url)).toEqual(Array.from(std));
  expect(Array.from(std)).toEqual([0xfb, 0xff, 0xbf]);
});

test('urlBase64ToUint8Array repone el relleno para longitudes no múltiplo de 4', () => {
  // Una clave VAPID real (65 bytes) en base64url son 87 chars sin relleno.
  const key65 = 'B'.repeat(87);
  const got = urlBase64ToUint8Array(key65);
  expect(got.length).toBe(65);
});
