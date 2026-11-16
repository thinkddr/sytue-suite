/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las filas y columnas ocultas a mano dentro de la hoja: siguen a la estructura al insertar
 * o borrar, la rejilla se estira para llegar a lo que hay detrás de un bloque oculto, y la
 * orden pasa con la hoja protegida solo con el permiso de formato. Los vectores de los
 * helpers puros, compartidos con la app nativa, están en sheets-ocultas-vectores.test.ts.
 */
import { expect, test } from 'bun:test';
import { construirMotor, sincronizarMotor } from '../src/editor/sheets/motor-vivo';
import { extentPastHidden } from '../src/editor/sheets/ocultas';
import { moverObjetos } from '../src/editor/sheets/refshift';
import {
  PROTECT_CONTENT_CMDS,
  PROTECT_FORMAT_CMDS,
  PROTECT_PERMISSION_CMDS,
} from '../src/editor/sheets/SheetCanvas';
import type { SheetData, SheetModel } from '../src/editor/types';

const hoja: SheetData = {
  name: 'Hoja1',
  cells: {},
  hiddenRows: [{ start: 3, end: 4 }],
  hiddenCols: [{ start: 2, end: 2 }],
};

test('insertar una fila encima baja las filas ocultas; las columnas no se tocan', () => {
  const out = moverObjetos(hoja, { eje: 'fila', desde: 0, cantidad: 1, tope: 100 });
  expect(out.hiddenRows).toEqual([{ start: 4, end: 5 }]);
  expect(out.hiddenCols).toEqual([{ start: 2, end: 2 }]);
});

test('borrar la columna oculta la quita del modelo, sin dejar la clave vacía', () => {
  const out = moverObjetos(hoja, { eje: 'columna', desde: 1, cantidad: -2, tope: 26 });
  expect('hiddenCols' in out).toBe(false);
  expect(out.hiddenRows).toEqual([{ start: 3, end: 4 }]);
});

test('una hoja sin ocultas sale igual que entró', () => {
  const out = moverObjetos(
    { name: 'H', cells: {} },
    { eje: 'fila', desde: 0, cantidad: 1, tope: 100 },
  );
  expect('hiddenRows' in out).toBe(false);
  expect('hiddenCols' in out).toBe(false);
});

test('la extensión se estira por lo oculto para llegar a lo de detrás', () => {
  // Sin ocultas, la de siempre.
  expect(extentPastHidden(120, undefined, 1000)).toBe(120);
  expect(extentPastHidden(120, [], 1000)).toBe(120);
  // Cincuenta ocultas dentro: cincuenta más, para que se vean las mismas filas.
  expect(extentPastHidden(120, [{ start: 10, end: 59 }], 1000)).toBe(170);
  // Un bloque que cruza el borde se estira en pocas pasadas y no hasta el final de la hoja.
  const ext = extentPastHidden(120, [{ start: 100, end: 1_048_575 }], 1_048_576);
  expect(ext).toBeGreaterThan(120);
  expect(ext).toBeLessThanOrEqual(120 * 5);
  // Nunca pasa del tope de la rejilla.
  expect(extentPastHidden(90, [{ start: 0, end: 50 }], 100)).toBe(100);
});

test('ocultar con la hoja protegida va con el permiso de formato', () => {
  expect(PROTECT_FORMAT_CMDS.has('hide')).toBe(true);
  expect(PROTECT_CONTENT_CMDS.has('hide')).toBe(false);
  expect(PROTECT_PERMISSION_CMDS.has('hide')).toBe(false);
});

test('SUBTOTAL(109) se salta las filas ocultas a mano; SUBTOTAL(9) no', () => {
  const cells = {
    A1: '1',
    A2: '10',
    A3: '100',
    C1: '=SUBTOTAL(109,A1:A3)',
    C2: '=SUBTOTAL(9,A1:A3)',
  };
  const m0: SheetModel = { active: 0, sheets: [{ name: 'Hoja1', cells }] };
  const v0 = construirMotor(m0);
  expect(v0.engine.getValue('Hoja1', 'C1')).toBe(111);
  // Ocultar la fila 2 (índice 1): el mismo motor, ya sin ella en el 109.
  const m1: SheetModel = {
    ...m0,
    sheets: [{ ...m0.sheets[0]!, hiddenRows: [{ start: 1, end: 1 }] }],
  };
  const s1 = sincronizarMotor(v0, m1);
  expect(s1.vivo.engine.getValue('Hoja1', 'C1')).toBe(101);
  expect(s1.vivo.engine.getValue('Hoja1', 'C2')).toBe(111);
  // Volver a mostrarla.
  const s2 = sincronizarMotor(s1.vivo, m0);
  expect(s2.vivo.engine.getValue('Hoja1', 'C1')).toBe(111);
});
