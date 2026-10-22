import { expect, test } from 'bun:test';
import { terminosDeRespuesta } from './terminos.js';

test('el array pelado', () => {
  expect(terminosDeRespuesta('["orilla","mar","arena"]')).toEqual(['orilla', 'mar', 'arena']);
});

test('envuelto en un bloque de código y con prólogo', () => {
  expect(terminosDeRespuesta('Claro:\n```json\n["orilla","mar"]\n```')).toEqual(['orilla', 'mar']);
});

test('normaliza: minúsculas, sin repetidos, sin vacíos, con tope', () => {
  const r = terminosDeRespuesta('["Orilla"," orilla ","","MAR","a","b","c","d","e","f","g"]');
  expect(r.filter((x) => x === 'orilla')).toHaveLength(1);
  expect(r.length).toBeLessThanOrEqual(6);
});

test('lo que no es una lista de palabras da lista vacía', () => {
  for (const caso of ['', 'no puedo ayudarte', '{"a":1}', '[1,2,3]', '[']) {
    expect(terminosDeRespuesta(caso), caso).toEqual([]);
  }
});

test('una palabra absurdamente larga se descarta', () => {
  expect(terminosDeRespuesta(`["${'x'.repeat(80)}","mar"]`)).toEqual(['mar']);
});
