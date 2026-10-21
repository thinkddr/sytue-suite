/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los ayudantes PUROS del documento (canvas/exec):
 *  - pageFieldText: número de página «Page X of Y» con clamps.
 *  - rectFromCells: rectángulo sólido de celdas (condición de fusión), rechazando
 *    huecos y solapes — es la guarda que evita corromper la tabla al combinar.
 *  - captionLabel: etiqueta de leyenda «Tabla N» / «Figura N».
 * Sin DOM (bun test corre sin navegador): la lógica de mutación que usa estas
 * puras vive en exec.ts y se ejercita en el editor.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import { captionLabel, pageFieldText, rectFromCells } from '../src/editor/canvas/exec';

// Sin DOM no hay `<html lang>`, así que el catálogo cae al inglés (fuente de
// verdad), igual que captionLabel más abajo.
test('pageFieldText: formatea en el idioma activo y aplica clamps (≥1, X≤Y)', () => {
  expect(pageFieldText(2, 5)).toBe('Page 2 of 5');
  expect(pageFieldText(1, 1)).toBe('Page 1 of 1');
  // X nunca supera Y; ambos ≥ 1.
  expect(pageFieldText(9, 3)).toBe('Page 3 of 3');
  expect(pageFieldText(0, 0)).toBe('Page 1 of 1');
  expect(pageFieldText(-4, 7)).toBe('Page 1 of 7');
});

test('rectFromCells: un rectángulo sólido devuelve sus límites', () => {
  // 2×2 de celdas 1×1 → rectángulo [0..1]×[0..1].
  expect(
    rectFromCells([
      { r: 0, c: 0, rs: 1, cs: 1 },
      { r: 0, c: 1, rs: 1, cs: 1 },
      { r: 1, c: 0, rs: 1, cs: 1 },
      { r: 1, c: 1, rs: 1, cs: 1 },
    ]),
  ).toEqual({ r0: 0, c0: 0, r1: 1, c1: 1 });
  // Fila con una celda que ya abarca 2 columnas (colSpan) + otra: sigue siendo sólido.
  expect(
    rectFromCells([
      { r: 0, c: 0, rs: 1, cs: 2 },
      { r: 0, c: 2, rs: 1, cs: 1 },
    ]),
  ).toEqual({ r0: 0, c0: 0, r1: 0, c1: 2 });
});

test('rectFromCells: huecos, solapes y conjunto vacío → null (no se fusiona)', () => {
  expect(rectFromCells([])).toBeNull();
  // Hueco: falta la casilla (1,1) del rectángulo 2×2.
  expect(
    rectFromCells([
      { r: 0, c: 0, rs: 1, cs: 1 },
      { r: 0, c: 1, rs: 1, cs: 1 },
      { r: 1, c: 0, rs: 1, cs: 1 },
    ]),
  ).toBeNull();
  // Solape: dos celdas pisan la casilla (0,1).
  expect(
    rectFromCells([
      { r: 0, c: 0, rs: 1, cs: 2 },
      { r: 0, c: 1, rs: 1, cs: 1 },
    ]),
  ).toBeNull();
  // Forma en L (no rectángulo): tres 1×1 en (0,0),(1,0),(1,1) dejan el área < rect.
  expect(
    rectFromCells([
      { r: 0, c: 0, rs: 1, cs: 1 },
      { r: 1, c: 0, rs: 1, cs: 1 },
      { r: 1, c: 1, rs: 1, cs: 1 },
    ]),
  ).toBeNull();
});

// Sin `document` (Node), `editorT()` cae al idioma base, que es el inglés.
test('captionLabel: etiqueta por tipo con número saneado', () => {
  expect(captionLabel('tabla', 1)).toBe('Table 1');
  expect(captionLabel('figura', 3)).toBe('Figure 3');
  expect(captionLabel('tabla', 0)).toBe('Table 1'); // clamp ≥ 1
});
