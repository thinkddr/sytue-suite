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
  celdasCubiertas,
  combinar,
  combinarHoja,
  combinarPierdeDatos,
  fusionARango,
  fusionEn,
  fusionesEnVentana,
  moverPorFusiones,
  parseFusiones,
  separar,
} from '../src/editor/sheets/fusiones';
import { PROTECT_CONTENT_CMDS } from '../src/editor/sheets/SheetCanvas';

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

/** B2:D2 en coordenadas 0-based. */
const B2D2 = { c1: 1, r1: 1, c2: 3, r2: 1 };

test('combinar: añade la combinada y deja una sola celda (no concatena)', () => {
  expect(fusionARango(B2D2)).toBe('B2:D2');
  const r = combinar(undefined, B2D2);
  expect(r?.merges).toEqual(['B2:D2']);
  expect(r?.fusion).toEqual(B2D2);
  // Una sola celda no se combina.
  expect(combinar(['A1:B1'], { c1: 4, r1: 4, c2: 4, r2: 4 })).toBeNull();
});

test('combinar: las combinadas solapadas se sustituyen y la selección se agranda (Excel)', () => {
  // C2:C3 asoma por debajo de B2:D2: la nueva es B2:D3 y la vieja desaparece.
  const r = combinar(['A10:B10', 'C2:C3', 'basura'], B2D2);
  expect(r?.merges).toEqual(['A10:B10', 'basura', 'B2:D3']);
  // Cadena: al agrandar se alcanza otra combinada, que también se engloba.
  const c = combinar(['D2:E2', 'E3:F4'], { c1: 1, r1: 1, c2: 3, r2: 2 });
  expect(c?.merges).toEqual(['B2:F4']);
  // Lo que resulta lo pinta el lienzo tal cual: sin solapes.
  expect(parseFusiones(c?.merges)).toHaveLength(1);
});

test('separar: quita la combinada que toca la selección; null si no había', () => {
  const merges = ['A1:C1', 'B3:D4', 'F1:G1'];
  // Basta con el ancla seleccionada.
  expect(separar(merges, { c1: 1, r1: 2, c2: 1, r2: 2 })).toEqual(['A1:C1', 'F1:G1']);
  // Una selección que toca dos las quita las dos.
  expect(separar(merges, { c1: 0, r1: 0, c2: 5, r2: 0 })).toEqual(['B3:D4']);
  expect(separar(merges, { c1: 9, r1: 9, c2: 9, r2: 9 })).toBeNull();
  expect(separar(undefined, B2D2)).toBeNull();
  // Combinar y separar es volver a la lista de antes.
  const ida = combinar(['A1:C1'], B2D2)!;
  expect(separar(ida.merges, B2D2)).toEqual(['A1:C1']);
});

test('combinarHoja: conserva B2, vacía C2:D2, centra el ancla y deja el formato', () => {
  const cells = { B2: 'Total', C2: 'x', D2: '<b>42</b>', E2: 'fuera', B3: 'debajo' };
  expect(celdasCubiertas(cells, B2D2).sort()).toEqual(['C2', 'D2']);
  expect(combinarPierdeDatos(cells, B2D2)).toBe(true);
  const r = combinarHoja({ cells, merges: [] }, B2D2, true)!;
  expect(r.merges).toEqual(['B2:D2']);
  expect(Object.keys(r.cells).sort()).toEqual(['B2', 'B3', 'E2']);
  expect(r.cells.B2).toContain('Total');
  expect(r.cells.B2).toContain('center');
  // La hoja de partida no se toca: deshacer vuelve a ella.
  expect(cells.C2).toBe('x');
  // Sin centrar, el ancla queda como estaba.
  const sin = combinarHoja({ cells, merges: [] }, B2D2, false)!;
  expect(sin.cells.B2).toBe('Total');
});

test('combinarPierdeDatos: no avisa si lo cubierto está vacío o solo tiene etiquetas', () => {
  expect(combinarPierdeDatos({ B2: 'Total', C2: '', D2: '<b></b>' }, B2D2)).toBe(false);
  expect(combinarPierdeDatos({ A1: 'x', E2: 'y' }, B2D2)).toBe(false);
});

test('combinar sigue vetado con la hoja protegida', () => {
  expect(PROTECT_CONTENT_CMDS.has('merge')).toBe(true);
});

test('combinar: con la hoja en el tope de combinadas no combina (no vaciaría celdas en balde)', () => {
  // MAX_FUSIONES combinadas de dos celdas, una por fila, en A:B.
  const llenas = Array.from({ length: MAX_FUSIONES }, (_, i) => `A${i + 1}:B${i + 1}`);
  expect(combinar(llenas, { c1: 3, r1: 0, c2: 4, r2: 0 })).toBeNull();
  expect(
    combinarHoja(
      { cells: { D1: 'a', E1: 'b' }, merges: llenas },
      { c1: 3, r1: 0, c2: 4, r2: 0 },
      true,
    ),
  ).toBeNull();
  // Sustituir una que ya está sí cabe: la lista no crece.
  expect(combinar(llenas, { c1: 0, r1: 0, c2: 2, r2: 0 })?.merges).toHaveLength(MAX_FUSIONES);
});
