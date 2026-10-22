/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las celdas combinadas (sheets/fusiones.ts).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import { moverObjetos } from '../src/editor/sheets/refshift';
import {
  MAX_FUSIONES,
  fusionEn,
  fusionesEnVentana,
  moverPorFusiones,
  parseFusiones,
} from '../src/editor/sheets/fusiones';

test('parseFusiones: solo rangos de más de una celda y sin solaparse (la primera gana)', () => {
  const fs = parseFusiones(['A1', 'A1:B2', 'B2:C3', 'C3:D4', 'basura', 'D1:C1']);
  expect(fs).toEqual([
    { c1: 0, r1: 0, c2: 1, r2: 1 },
    { c1: 2, r1: 2, c2: 3, r2: 3 },
    { c1: 2, r1: 0, c2: 3, r2: 0 },
  ]);
  expect(parseFusiones(undefined)).toEqual([]);
});

test('parseFusiones: no crece sin medida con una entrada hostil', () => {
  const muchas = Array.from(
    { length: MAX_FUSIONES + 500 },
    (_, i) => `A${i * 2 + 1}:B${i * 2 + 2}`,
  );
  expect(parseFusiones(muchas).length).toBe(MAX_FUSIONES);
});

test('fusionEn y fusionesEnVentana', () => {
  const fs = parseFusiones(['A1:C1', 'A3:A5', 'E10:F11']);
  expect(fusionEn(fs, 2, 0)?.c1).toBe(0);
  expect(fusionEn(fs, 0, 4)?.r1).toBe(2);
  expect(fusionEn(fs, 1, 2)).toBeNull();
  expect(fusionesEnVentana(fs, 0, 0, 10, 5)).toHaveLength(2);
  expect(fusionesEnVentana(fs, 0, 6, 3, 9)).toHaveLength(0);
  expect(fusionesEnVentana(fs, 0, 0, 100, 100, 1)).toHaveLength(1);
});

test('moverPorFusiones: salta la combinada entera y cae en el ancla', () => {
  const fs = parseFusiones(['B2:D3']);
  // Desde el ancla a la derecha: la celda siguiente a la combinada.
  expect(moverPorFusiones(fs, { c: 1, r: 1 }, 1, 0, 100, 100)).toEqual({ c: 4, r: 1 });
  // Hacia abajo: la fila siguiente.
  expect(moverPorFusiones(fs, { c: 1, r: 1 }, 0, 1, 100, 100)).toEqual({ c: 1, r: 3 });
  // Hacia la izquierda desde dentro: la columna anterior.
  expect(moverPorFusiones(fs, { c: 3, r: 2 }, -1, 0, 100, 100)).toEqual({ c: 0, r: 2 });
  // Entrando en la combinada: se queda en su ancla.
  expect(moverPorFusiones(fs, { c: 0, r: 1 }, 1, 0, 100, 100)).toEqual({ c: 1, r: 1 });
  expect(moverPorFusiones(fs, { c: 3, r: 3 }, 0, -1, 100, 100)).toEqual({ c: 1, r: 1 });
  // En el borde de la hoja no se sale.
  expect(moverPorFusiones(fs, { c: 0, r: 0 }, -1, -1, 100, 100)).toEqual({ c: 0, r: 0 });
});

test('moverObjetos: las combinadas y las medidas siguen a sus celdas al insertar y borrar', () => {
  const hoja = {
    name: 'H',
    cells: {},
    merges: ['A1:C1', 'B3:B4', 'E5:F5'],
    colWidths: { '0': 150, '2': 200 },
    rowHeights: { '1': 40, '5': 60 },
  };
  // Insertar una fila antes de la 3: B3:B4 pasa a B4:B5, y las alturas de las filas 2.. bajan una.
  const ins = moverObjetos(hoja, { eje: 'fila', desde: 2, cantidad: 1, tope: 1000 });
  expect(ins.merges).toEqual(['A1:C1', 'B4:B5', 'E6:F6']);
  expect(ins.rowHeights).toEqual({ '1': 40, '6': 60 });
  // Borrar la columna B: A1:C1 encoge a A1:B1; B3:B4 desaparece; E5:F5 se desplaza.
  const del = moverObjetos(hoja, { eje: 'columna', desde: 1, cantidad: -1, tope: 1000 });
  expect(del.merges).toEqual(['A1:B1', 'D5:E5']);
  expect(del.colWidths).toEqual({ '0': 150, '1': 200 });
  // Borrar las dos columnas de una combinada de dos celdas la quita; una de una sola celda no queda.
  const dos = moverObjetos(
    { name: 'H', cells: {}, merges: ['B1:C1'] },
    { eje: 'columna', desde: 1, cantidad: -1, tope: 1000 },
  );
  expect(dos.merges).toBeUndefined();
});
