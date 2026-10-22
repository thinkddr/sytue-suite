import { expect, test } from 'bun:test';
import { tituloSugerido } from './tituloAlbum.js';

const f = (...tags: string[]) => ({ tags });

test('propone lo que comparten, no lo que sale en una sola', () => {
  expect(tituloSugerido([f('playa', 'sombrilla'), f('playa', 'gaviota')])).toBe('Playa');
});

test('hasta tres, de más repetida a menos', () => {
  const fotos = [
    f('playa', 'mar', 'arena', 'toalla'),
    f('playa', 'mar', 'arena', 'barco'),
    f('playa', 'mar', 'arena', 'nube'),
  ];
  expect(tituloSugerido(fotos)).toBe('Arena · mar · playa');
});

test('con una sola foto no hay nada en común que proponer', () => {
  expect(tituloSugerido([f('playa', 'mar')])).toBe('');
});

test('sin etiquetas o sin nada compartido, vacío y no un nombre inventado', () => {
  expect(tituloSugerido([f(), f()])).toBe('');
  expect(tituloSugerido([f('playa'), f('montaña'), f('ciudad'), f('bosque')])).toBe('');
});

test('el mismo conjunto da siempre el mismo nombre', () => {
  // Empate a dos: sin criterio alfabético el orden dependería del Map.
  const fotos = [f('mar', 'cielo'), f('mar', 'cielo')];
  expect(tituloSugerido(fotos)).toBe('Cielo · mar');
  expect(tituloSugerido([...fotos].reverse())).toBe('Cielo · mar');
});

test('una etiqueta repetida dentro de una foto no vota dos veces', () => {
  expect(tituloSugerido([f('playa', 'Playa', ' playa '), f('mar')])).toBe('');
});
