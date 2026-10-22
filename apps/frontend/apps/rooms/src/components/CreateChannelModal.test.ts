// Checks de la pista de slug de «Crear canal»: debe coincidir con el domain.Slugify del
// servicio Go (services/chat/internal/domain/domain.go). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { channelSlug } from './CreateChannelModal.js';

test('minúsculas y espacios a guion', () => {
  expect(channelSlug('Lanzamiento Otono')).toBe('lanzamiento-otono');
});

test('colapsa separadores repetidos y recorta los de los extremos', () => {
  expect(channelSlug('  __hola  -- mundo._  ')).toBe('hola-mundo');
});

test('descarta acentos y signos (como validSlugRune del backend)', () => {
  expect(channelSlug('Diseño & Móvil!')).toBe('diseo-mvil');
});

test('los dígitos sobreviven; el guion no abre el slug', () => {
  expect(channelSlug('-2026 Q3-')).toBe('2026-q3');
});

test('sin caracteres válidos → cadena vacía (el botón queda deshabilitado)', () => {
  expect(channelSlug('¿¡ ***')).toBe('');
  expect(channelSlug('   ')).toBe('');
});

test('recorta a 64 y no deja guion final', () => {
  const long = channelSlug(`${'a'.repeat(64)} b`);
  expect(long).toBe('a'.repeat(64));
  // El corte cae justo sobre el guion: se recorta también.
  expect(channelSlug(`${'a'.repeat(63)} bcd`)).toBe('a'.repeat(63));
});
