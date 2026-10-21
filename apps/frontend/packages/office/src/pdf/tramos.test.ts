// Checks de partir el documento por su índice. Ejecutar: `bun test`.
//
// Es aritmética de bordes, que es donde falla sola: el último capítulo llega al
// final del documento y no al siguiente marcador, porque no hay siguiente.
import { expect, test } from 'bun:test';
import { tramosDelIndice, type Marcador } from './api';

const m = (titulo: string, pagina: number, nivel = 0): Marcador => ({ titulo, pagina, nivel });

test('cada capítulo llega hasta el siguiente, y el último hasta el final', () => {
  const tramos = tramosDelIndice([m('Uno', 0), m('Dos', 4), m('Tres', 9)], 20);
  expect(tramos).toEqual([
    { titulo: 'Uno', rango: '1-4' },
    { titulo: 'Dos', rango: '5-9' },
    // Sin este caso, el último capítulo se quedaría en su propia página y las
    // once restantes no saldrían en ninguna parte.
    { titulo: 'Tres', rango: '10-20' },
  ]);
});

test('los subcapítulos no abren tramo y los que no navegan tampoco', () => {
  const tramos = tramosDelIndice(
    [m('Uno', 0), m('Uno.a', 1, 1), m('sin destino', -1), m('Dos', 5)],
    8,
  );
  // «Uno.a» partiría el capítulo por la mitad y «sin destino» no dice por dónde
  // cortar: ninguno de los dos puede abrir un tramo.
  expect(tramos).toEqual([
    { titulo: 'Uno', rango: '1-5' },
    { titulo: 'Dos', rango: '6-8' },
  ]);
});

test('un capítulo de una sola página se escribe sin rango', () => {
  // «3-3» es válido pero se lee como si faltara algo; el campo acepta «3».
  expect(tramosDelIndice([m('A', 0), m('B', 2), m('C', 3)], 4)).toEqual([
    { titulo: 'A', rango: '1-2' },
    { titulo: 'B', rango: '3' },
    { titulo: 'C', rango: '4' },
  ]);
});

test('sin índice no hay nada que partir', () => {
  expect(tramosDelIndice([], 10)).toEqual([]);
});
