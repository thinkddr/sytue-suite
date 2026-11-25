/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las funciones de fórmula llegan aparte del editor de hojas (`cargarFunciones` de
 * @sytue/formula/motor). Mientras no llegan, el motor del lienzo calcula lo que no las
 * usa y deja VACÍO (#BUSY!) lo que sí, nunca un valor equivocado ni un #NAME?; cuando
 * llegan, el lienzo construye otro motor (SheetCanvas: `conFunciones`) y lo calcula todo.
 */
import { expect, test } from 'bun:test';
import { buildRegistry } from '@sytue/formula';
import { instalarFunciones } from '../../formula/src/perezosas';
import { formatCellValue } from '../src/editor/sheets/format';
import { construirMotor, sincronizarMotor } from '../src/editor/sheets/motor-vivo';
import type { SheetModel } from '../src/editor/types';

const modelo = {
  sheets: [{ name: 'Hoja1', cells: { A1: '2', A2: '=A1*3', A3: '=SUM(A1,3)', A4: '=A3+1' } }],
} as unknown as SheetModel;

test('sin las funciones, la hoja pinta lo que no las usa y deja vacío lo que sí', () => {
  instalarFunciones(undefined as unknown as typeof buildRegistry);
  let enEspera;
  try {
    enEspera = construirMotor(modelo);
  } finally {
    instalarFunciones(buildRegistry);
  }
  const e = enEspera.engine;
  expect(e.esperaFunciones).toBe(true);
  expect(e.getValue('Hoja1', 'A2')).toBe(6);
  expect(formatCellValue(e.getValue('Hoja1', 'A3'))).toBe('');
  // Lo que depende de una que espera, también espera: nunca un número a medias.
  expect(formatCellValue(e.getValue('Hoja1', 'A4'))).toBe('');

  // Al llegar, el lienzo descarta el motor en espera y construye otro.
  const { vivo } = sincronizarMotor(null, modelo);
  expect(vivo.engine.esperaFunciones).toBe(false);
  expect(vivo.engine.getValue('Hoja1', 'A3')).toBe(5);
  expect(vivo.engine.getValue('Hoja1', 'A4')).toBe(6);
});
