/**
 * La barra del editor en una fila: lo que no cabe va a «Más», empezando por lo que
 * menos se usa. Una cuenta que esconde de más deja la barra vacía con hueco; una que
 * esconde de menos la desborda, que es justo lo que se quería quitar.
 */
import { expect, test } from 'bun:test';
import { anchoDeFila, repartirBarra, type PiezaDeBarra } from './desborde.js';

const M = { hueco: 2, separador: 9, anchoMas: 28 };
const pieza = (grupo: string, prioridad: number, ancho = 28): PiezaDeBarra => ({
  ancho,
  grupo,
  prioridad,
});

// historia (2) | formato (3) | herramientas (2)
const BARRA = [
  pieza('h', 8),
  pieza('h', 6),
  pieza('f', 10),
  pieza('f', 10),
  pieza('f', 5),
  pieza('t', 2),
  pieza('t', 1),
];

test('la fila cuenta piezas, huecos y un separador entre grupos', () => {
  const todas = BARRA.map(() => true);
  // 7 piezas de 28, 6 huecos de 2 y 2 separadores con su hueco de más (9 + 2).
  expect(anchoDeFila(BARRA, todas, M)).toBe(7 * 28 + 6 * 2 + 2 * 11);
  // Sin el grupo del medio, las otras dos quedan pegadas con un solo separador.
  expect(anchoDeFila(BARRA, [true, true, false, false, false, true, true], M)).toBe(
    4 * 28 + 3 * 2 + 11,
  );
});

test('si cabe todo, no se esconde nada y no hace falta «Más»', () => {
  const ancho = anchoDeFila(
    BARRA,
    BARRA.map(() => true),
    M,
  );
  expect(repartirBarra(BARRA, { ...M, disponible: ancho })).toEqual(BARRA.map(() => true));
  // Sin medida (antes de maquetar, o en un DOM sin maquetación) tampoco.
  expect(repartirBarra(BARRA, { ...M, disponible: 0 })).toEqual(BARRA.map(() => true));
});

test('se esconde primero lo de menos prioridad, y la fila con «Más» cabe', () => {
  const ancho =
    anchoDeFila(
      BARRA,
      BARRA.map(() => true),
      M,
    ) - 1;
  const v = repartirBarra(BARRA, { ...M, disponible: ancho });
  // Lo primero que se va es lo de prioridad 1 y 2: las herramientas.
  expect(v).toEqual([true, true, true, true, true, false, false]);
  expect(anchoDeFila(BARRA, v, M) + M.hueco + M.anchoMas).toBeLessThanOrEqual(ancho);
});

test('en el teléfono se queda lo más usado aunque esté en medio', () => {
  // Cabe una fila de unas tres piezas y «Más».
  const v = repartirBarra(BARRA, { ...M, disponible: 3 * 28 + 2 * 2 + 11 + 2 + 28 });
  // Las dos de formato más usadas y deshacer, por delante de rehacer (6) y tachar (5).
  expect(v).toEqual([true, false, true, true, false, false, false]);
});

test('a igual prioridad se va antes la de la derecha', () => {
  const iguales = [pieza('a', 1), pieza('a', 1), pieza('a', 1)];
  // Las tres miden 88; con «Más» solo cabe una.
  const v = repartirBarra(iguales, { ...M, disponible: 80 });
  expect(v).toEqual([true, false, false]);
});

test('sin sitio para nada, todo va a «Más»', () => {
  expect(repartirBarra(BARRA, { ...M, disponible: 30 })).toEqual(BARRA.map(() => false));
});
