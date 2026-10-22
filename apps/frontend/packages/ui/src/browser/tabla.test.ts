// Checks de la edición de tablas del compositor. Ejecutar: `bun test`.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (!globalThis.document) GlobalRegistrator.register();

import { expect, test } from 'bun:test';
import {
  celdaDe,
  eliminarColumna,
  eliminarFila,
  eliminarTabla,
  insertarColumna,
  insertarFila,
} from './tabla.js';

/** Monta una tabla de filas×columnas y devuelve la tabla y una celda concreta. */
function tabla(filas: number, cols: number, f = 0, c = 0) {
  const raiz = document.createElement('div');
  const tds = '<td>x</td>'.repeat(cols);
  raiz.innerHTML = `<table><tbody>${`<tr>${tds}</tr>`.repeat(filas)}</tbody></table>`;
  const t = raiz.querySelector('table') as HTMLTableElement;
  const celda = t.querySelectorAll('tr')[f]?.children[c] as HTMLTableCellElement;
  return { raiz, t, celda };
}

const dimensiones = (t: HTMLElement) => {
  const filas = [...t.querySelectorAll('tr')];
  return [filas.length, filas[0]?.children.length ?? 0];
};

test('la celda se encuentra desde donde de verdad cae el cursor', () => {
  // La selección casi nunca está en el <td>: está en el nodo de texto de dentro,
  // o en un <b>. Sin subir por los padres, la barra de tabla no aparecería nunca.
  const { celda } = tabla(2, 2);
  celda.innerHTML = '<b>hola</b>';
  const texto = celda.querySelector('b')?.firstChild ?? null;
  expect(celdaDe(texto)).toBe(celda);
  expect(celdaDe(celda)).toBe(celda);
  // Fuera de una tabla no hay celda que valga.
  expect(celdaDe(document.createElement('p'))).toBeNull();
});

test('insertar fila y columna respeta el lado', () => {
  const a = tabla(2, 3, 0, 0);
  insertarFila(a.celda, 'despues');
  expect(dimensiones(a.t)).toEqual([3, 3]);
  insertarColumna(a.celda, 'antes');
  expect(dimensiones(a.t)).toEqual([3, 4]);
  // La columna nueva entra en TODAS las filas, también en la recién creada: si
  // no, la tabla queda dentada y el destinatario ve celdas desalineadas.
  for (const fila of a.t.querySelectorAll('tr')) expect(fila.children.length).toBe(4);
});

test('una columna se añade también a las filas más cortas', () => {
  // Pasa con tablas venidas de un pegado: si a esas filas se las saltara, la
  // columna quedaría desplazada para siempre.
  const { t, celda } = tabla(2, 3, 0, 2);
  t.querySelectorAll('tr')[1]?.children[2]?.remove();
  insertarColumna(celda, 'despues');
  for (const fila of t.querySelectorAll('tr')) expect(fila.children.length).toBe(4);
});

test('quitar la última fila se lleva la tabla, no deja un fantasma', () => {
  // Una <table> sin filas es invisible y el cursor no la alcanza: parecería
  // borrada y seguiría viajando dentro del correo.
  const { raiz, celda } = tabla(1, 3);
  eliminarFila(celda);
  expect(raiz.querySelector('table')).toBeNull();
});

test('quitar la última columna se lleva la tabla', () => {
  const { raiz, celda } = tabla(3, 1);
  eliminarColumna(celda);
  expect(raiz.querySelector('table')).toBeNull();
});

test('lo normal: se quita solo lo señalado', () => {
  const a = tabla(3, 3, 1, 1);
  eliminarFila(a.celda);
  expect(dimensiones(a.t)).toEqual([2, 3]);
  const b = tabla(3, 3, 1, 1);
  eliminarColumna(b.celda);
  expect(dimensiones(b.t)).toEqual([3, 2]);
  const c = tabla(3, 3, 1, 1);
  eliminarTabla(c.celda);
  expect(c.raiz.querySelector('table')).toBeNull();
});
