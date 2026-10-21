// Checks de qué se puede soltar dónde. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { carpetaDe, esDescendiente, movibles, rutaDestino, sePuedeSoltar } from './arrastrar.js';

test('una carpeta NO se puede soltar dentro de sí misma ni de lo que contiene', () => {
  // Es el fallo que no avisa: el árbol se referencia a sí mismo y la carpeta
  // desaparece con todo lo que llevaba dentro.
  expect(sePuedeSoltar('/informes', '/informes')).toBe(false);
  expect(sePuedeSoltar('/informes', '/informes/2025')).toBe(false);
  expect(sePuedeSoltar('/informes', '/informes/2025/enero')).toBe(false);
});

test('una carpeta vecina de nombre parecido NO cuenta como descendiente', () => {
  // Sin comparar con la barra puesta, `/informes` «contendría» a
  // `/informes-2025` y el movimiento quedaría prohibido sin ningún motivo.
  expect(esDescendiente('/informes-2025', '/informes')).toBe(false);
  expect(sePuedeSoltar('/informes', '/informes-2025')).toBe(true);
});

test('soltar donde ya está no es un movimiento', () => {
  // No es un error, pero hacer la llamada gastaría una escritura y un refresco
  // para dejar todo exactamente igual.
  expect(sePuedeSoltar('/informes/mayo.pdf', '/informes')).toBe(false);
  expect(sePuedeSoltar('/mayo.pdf', '')).toBe(false); // ya en la raíz
});

test('lo normal sí se mueve', () => {
  expect(sePuedeSoltar('/mayo.pdf', '/informes')).toBe(true);
  expect(sePuedeSoltar('/informes/mayo.pdf', '/facturas')).toBe(true);
  expect(sePuedeSoltar('/informes/mayo.pdf', '')).toBe(true); // sacar a la raíz
});

test('de una selección solo viajan los que de verdad cambian de sitio', () => {
  const sel = ['/a.pdf', '/informes/b.pdf', '/informes', '/c.pdf'];
  // `/informes/b.pdf` ya está en el destino y `/informes` es el destino: fuera.
  expect(movibles(sel, '/informes')).toEqual(['/a.pdf', '/c.pdf']);
});

test('la ruta de destino conserva el nombre, también hacia la raíz', () => {
  expect(rutaDestino('/informes/mayo.pdf', '/facturas')).toBe('/facturas/mayo.pdf');
  expect(rutaDestino('/informes/mayo.pdf', '')).toBe('/mayo.pdf');
  expect(carpetaDe('/informes/mayo.pdf')).toBe('/informes');
  expect(carpetaDe('/mayo.pdf')).toBe('');
});
