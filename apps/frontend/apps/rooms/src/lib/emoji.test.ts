// Checks del catálogo buscable de emoji del picker. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { searchEmoji, EMOJI_ENTRIES } from './emoji.js';

test('query vacía devuelve el catálogo completo', () => {
  expect(searchEmoji('')).toBe(EMOJI_ENTRIES);
  expect(searchEmoji('   ')).toBe(EMOJI_ENTRIES);
  expect(EMOJI_ENTRIES.length).toBeGreaterThan(50);
});

test('busca por nombre/shortcode (subcadena, sin distinción de mayúsculas)', () => {
  const fire = searchEmoji('fire');
  expect(fire.some((e) => e.glyph === '🔥')).toBe(true);
  expect(searchEmoji('FIRE').some((e) => e.glyph === '🔥')).toBe(true);
});

test('un glyph con varios shortcodes aparece una sola vez y se encuentra por cualquiera', () => {
  // 👍 mapea a :thumbsup: y :+1: en el mapa EMOJI.
  const up = EMOJI_ENTRIES.filter((e) => e.glyph === '👍');
  expect(up).toHaveLength(1);
  expect(searchEmoji('thumbsup').some((e) => e.glyph === '👍')).toBe(true);
  expect(searchEmoji('+1').some((e) => e.glyph === '👍')).toBe(true);
});

test('sin coincidencias → lista vacía', () => {
  expect(searchEmoji('zzxqwvunlikely')).toEqual([]);
});
