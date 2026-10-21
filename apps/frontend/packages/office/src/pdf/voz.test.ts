// Checks de preparar una página para leerla en voz alta. Ejecutar: `bun test`.
//
// Lo que se comprueba es el ORDEN y las junturas: un PDF guarda trozos
// colocados, no párrafos, y leerlos en el orden en que aparecen en el fichero
// —o sin espacios entre ellos— produce algo que no se entiende.
import { expect, test } from 'bun:test';
import { textoParaLeer, type Fragmento } from './api';

const f = (texto: string, x: number, y: number, tamano = 10): Fragmento => ({
  texto,
  x,
  y,
  tamano,
});

test('los trozos de una línea se leen seguidos y en orden de izquierda a derecha', () => {
  // Llegan desordenados a propósito: en el flujo de un PDF el orden es el de
  // dibujo, no el de lectura.
  expect(textoParaLeer([f('cliente', 60, 700), f('Estimado', 10, 700)])).toBe('Estimado cliente');
});

test('se lee de arriba abajo, que es como está escrito', () => {
  // La `y` de un PDF crece hacia ARRIBA: ordenar ascendente leería el documento
  // empezando por el pie.
  const t = textoParaLeer([f('Segunda', 10, 600), f('Primera', 10, 700)]);
  expect(t.indexOf('Primera')).toBeLessThan(t.indexOf('Segunda'));
});

test('el umbral de línea va con el tamaño de letra, no fijo', () => {
  // Un título de 30 pt y su subtítulo separados 8 pt son líneas distintas si se
  // mira en absoluto, pero con letra grande esa distancia es la misma línea.
  expect(textoParaLeer([f('TÍTULO', 10, 700, 30), f('GRANDE', 90, 694, 30)])).toBe('TÍTULO GRANDE');
  // Con letra pequeña, esos mismos 6 pt sí son dos líneas.
  const dos = textoParaLeer([f('uno', 10, 700, 8), f('dos', 10, 694, 8)]);
  expect(dos).toBe('uno. dos');
});

test('los trozos vacíos no meten pausas ni espacios de más', () => {
  expect(textoParaLeer([f('Hola', 10, 700), f('   ', 40, 700), f('mundo', 60, 700)])).toBe(
    'Hola mundo',
  );
  expect(textoParaLeer([])).toBe('');
});
