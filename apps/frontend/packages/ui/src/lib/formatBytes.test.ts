/**
 * `formatBytes` — su contrato: el separador decimal lo pone el IDIOMA.
 *
 * Es el defecto que traían cuatro de las siete copias que había repartidas: usaban
 * `toFixed`, que siempre escribe el punto inglés, así que «1.5 MB» se leía como
 * mil quinientos megas en los cuatro idiomas que separan con coma.
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { formatBytes } from './formatBytes.js';

test('el separador decimal es el del idioma', () => {
  expect(formatBytes('es', 1.5 * 1024 * 1024)).toBe('1,5 MB');
  expect(formatBytes('en', 1.5 * 1024 * 1024)).toBe('1.5 MB');
});

test('escala hasta TB y deja de dar decimales por encima de 10', () => {
  expect(formatBytes('es', 512)).toBe('512 B');
  expect(formatBytes('es', 512 * 1024)).toBe('512 KB');
  expect(formatBytes('es', 15 * 1024 ** 3)).toBe('15 GB');
  expect(formatBytes('es', 3 * 1024 ** 4)).toBe('3,0 TB');
});

test('una cuota que el backend no supo calcular se dice, no se inventa', () => {
  expect(formatBytes('es', Number.NaN)).toBe('—');
  expect(formatBytes('es', -1)).toBe('—');
});
