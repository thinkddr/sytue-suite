import { expect, test } from 'bun:test';
import { interpretarRespuesta, MAX_ETIQUETAS } from './respuestaIA.js';

test('el JSON pelado', () => {
  const r = interpretarRespuesta(
    '{"descripcion":"Un perro en la playa","etiquetas":["perro","playa"]}',
  );
  expect(r).toEqual({ descripcion: 'Un perro en la playa', etiquetas: ['perro', 'playa'] });
});

// Los tres envoltorios que de verdad manda un modelo.
test('envuelto en un bloque de código', () => {
  const r = interpretarRespuesta('```json\n{"descripcion":"Una playa","etiquetas":[]}\n```');
  expect(r?.descripcion).toBe('Una playa');
});

test('con una frase de cortesía delante y detrás', () => {
  const r = interpretarRespuesta(
    'Claro, aquí tienes:\n{"descripcion":"Un gato","etiquetas":["gato"]}\nEspero que sirva.',
  );
  expect(r?.descripcion).toBe('Un gato');
});

test('una llave dentro del texto no corta el objeto', () => {
  const r = interpretarRespuesta('{"descripcion":"Un cartel que pone {abierto}","etiquetas":[]}');
  expect(r?.descripcion).toBe('Un cartel que pone {abierto}');
});

test('un objeto anidado no corta en la primera llave de cierre', () => {
  const r = interpretarRespuesta('{"extra":{"a":1},"descripcion":"Algo","etiquetas":["x"]}');
  expect(r?.descripcion).toBe('Algo');
});

// Las etiquetas se normalizan: sin repetidas, sin vacías, en minúscula y con tope.
test('etiquetas normalizadas', () => {
  const r = interpretarRespuesta(
    '{"descripcion":"x","etiquetas":["Perro"," perro ","","playa",' +
      Array.from({ length: 20 }, (_, i) => `"e${i}"`).join(',') +
      ']}',
  );
  expect(r?.etiquetas[0]).toBe('perro');
  expect(r?.etiquetas.filter((e) => e === 'perro')).toHaveLength(1);
  expect(r?.etiquetas.length).toBeLessThanOrEqual(MAX_ETIQUETAS);
});

// Lo que NO se acepta: antes de inventar una descripción, no hay descripción.
test('lo que no es una descripción usable devuelve null', () => {
  for (const caso of [
    '',
    'No puedo ayudarte con eso.',
    '{"etiquetas":["perro"]}',
    '{"descripcion":"   "}',
    '{"descripcion":123}',
    '["descripcion"]',
    '{"descripcion":"sin cerrar"',
  ]) {
    expect(interpretarRespuesta(caso), caso).toBeNull();
  }
});
