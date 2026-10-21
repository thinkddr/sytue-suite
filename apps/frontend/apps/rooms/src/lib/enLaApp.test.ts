/**
 * Tests de enLaApp — cuándo se prueba el esquema de la app sin preguntar. Lo que puede
 * salir mal es probarlo en un navegador que enseña un error si la app no está.
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { esMovil } from './enLaApp.js';

test('en el móvil no se prueba: ahí abren la app los App Links y los Universal Links', () => {
  expect(esMovil('Mozilla/5.0 (Linux; Android 15; Pixel 9)')).toBe(true);
  expect(esMovil('Mozilla/5.0 (iPhone; CPU iPhone OS 19_0 like Mac OS X)')).toBe(true);
  expect(esMovil('Mozilla/5.0 (Macintosh; Intel Mac OS X 15_6) AppleWebKit/605.1.15')).toBe(false);
  expect(esMovil('Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/140.0')).toBe(false);
});
