/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Dónde escribe el asistente su fórmula en una hoja.
 */
import { expect, test } from 'bun:test';
import {
  celdasARellenar,
  cleanFormula,
  columnasCitadas,
  esFormulaMultiCelda,
  esSpillFormula,
  hastaDondeLleganLosDatos,
  tieneRangoMultiFila,
} from '../src/editor/sheets/aifill.js';

/** Desplazador de juguete: suma `dr` al número de fila de cada referencia A1. */
const desplazar = (f: string, dc: number, dr: number): string =>
  f.replace(/([A-Z]{1,2})(\d{1,3})/g, (_m, col: string, fila: string) => {
    const c = col.charCodeAt(0) - 65 + dc;
    return `${String.fromCharCode(65 + c)}${Number(fila) + dr}`;
  });

test('columnasCitadas saca las columnas de una fórmula', () => {
  expect(columnasCitadas('=SUMA(A1:B1)').sort((a, b) => a - b)).toEqual([0, 1]);
  expect(columnasCitadas('=C4*$D$9')).toContain(2);
  expect(columnasCitadas('=AA3')).toEqual([26]);
  expect(columnasCitadas('=HOY()')).toEqual([]);
});

// El caso del informe: una sola celda seleccionada y datos en A1:A8.
test('con una celda seleccionada se extiende hasta donde llegan los datos', () => {
  const tieneDato = (col: number, fila: number): boolean => col <= 1 && fila <= 7;
  expect(hastaDondeLleganLosDatos('=SUMA(A1:B1)', 0, 100, tieneDato)).toBe(7);
});

test('sin datos alrededor, una celda sigue siendo una celda', () => {
  expect(hastaDondeLleganLosDatos('=SUMA(A1:B1)', 0, 100, () => false)).toBe(0);
});

test('rellena la columna adaptando las referencias', () => {
  const celdas = celdasARellenar({
    formula: '=SUMA(A1:B1)',
    rect: { c1: 2, c2: 2, r1: 0, r2: 0 },
    filas: 100,
    tieneDato: (col, fila) => col <= 1 && fila <= 3,
    desplazar,
  });
  expect(celdas).toHaveLength(4);
  expect(celdas[0]?.formula).toBe('=SUMA(A1:B1)');
  expect(celdas[3]?.formula).toBe('=SUMA(A4:B4)');
  // Todas en la misma columna que la selección.
  expect(celdas.every((c) => c.dc === 0)).toBe(true);
});

// Con un rango marcado a mano manda el rango, no los datos: quien selecciona
// C1:C2 quiere dos celdas aunque haya diez filas de datos al lado.
test('con un rango seleccionado se respeta el rango', () => {
  const celdas = celdasARellenar({
    formula: '=SUMA(A1:B1)',
    rect: { c1: 2, c2: 2, r1: 0, r2: 1 },
    filas: 100,
    tieneDato: () => true,
    desplazar,
  });
  expect(celdas).toHaveLength(2);
});

test('una fórmula vacía no escribe nada', () => {
  expect(
    celdasARellenar({
      formula: '   ',
      rect: { c1: 0, c2: 0, r1: 0, r2: 5 },
      filas: 100,
      tieneDato: () => true,
      desplazar,
    }),
  ).toEqual([]);
});

// El desplazador devuelve `#…` cuando la referencia se sale de la rejilla: esa
// celda se salta en vez de quedarse con un #¡REF! escrito.
test('las celdas cuya referencia se sale se saltan', () => {
  const celdas = celdasARellenar({
    formula: '=A1',
    rect: { c1: 0, c2: 0, r1: 0, r2: 2 },
    filas: 100,
    tieneDato: () => true,
    desplazar: (f, _dc, dr) => (dr > 1 ? '#¡REF!' : desplazar(f, _dc, dr)),
  });
  expect(celdas).toHaveLength(2);
});

// Limpieza de formato devuelto por la IA
test('cleanFormula limpia backticks, tildes agudas y comillas tipográficas', () => {
  // El caso exacto reportado por el usuario: ``=SUM(A1:A2)´´
  expect(cleanFormula('``=SUM(A1:A2)´´')).toBe('=SUM(A1:A2)');
  expect(cleanFormula('`=SUM(A1:A2)`')).toBe('=SUM(A1:A2)');
  expect(cleanFormula('"=SUM(A1:A2)"')).toBe('=SUM(A1:A2)');
  expect(cleanFormula("'=SUM(A1:A2)'")).toBe('=SUM(A1:A2)');
  expect(cleanFormula('«=SUM(A1:A2)»')).toBe('=SUM(A1:A2)');
  expect(cleanFormula('“=SUM(A1:A2)”')).toBe('=SUM(A1:A2)');
  expect(cleanFormula('‘=SUM(A1:A2)’')).toBe('=SUM(A1:A2)');
  expect(cleanFormula('La fórmula es: ``=SUM(A1:A2)´´')).toBe('=SUM(A1:A2)');
  expect(cleanFormula('```excel\n=SUM(A1:A2)\n```')).toBe('=SUM(A1:A2)');
  expect(cleanFormula('```\n=SUM(A1:A2)\n```')).toBe('=SUM(A1:A2)');
  expect(cleanFormula('SUM(A1:A2)')).toBe('=SUM(A1:A2)');
  expect(cleanFormula('SUMA(A1:A2)')).toBe('=SUMA(A1:A2)');
});

// Detección de fórmulas multi-celda
test('detección de fórmulas multi-celda y spill', () => {
  expect(tieneRangoMultiFila('=SUM(A1:A2)')).toBe(true);
  expect(tieneRangoMultiFila('=PROMEDIO(B1:B10)')).toBe(true);
  expect(tieneRangoMultiFila('=SUMA(A1:B1)')).toBe(false); // misma fila
  expect(tieneRangoMultiFila('=A1+B1')).toBe(false);

  expect(esSpillFormula('=SEQUENCE(10)')).toBe(true);
  expect(esSpillFormula('=FILTER(A1:B10, A1:A10>0)')).toBe(true);
  expect(esSpillFormula('=SUM(A1:A10)')).toBe(false);

  expect(esFormulaMultiCelda('=SUM(A1:A2)')).toBe(true);
  expect(esFormulaMultiCelda('=SEQUENCE(10)')).toBe(true);
  expect(esFormulaMultiCelda('=A1*B1')).toBe(false);
});

// Fórmulas multi-celda no se extienden con una sola celda seleccionada
test('fórmula multi-celda con una sola celda seleccionada solo rellena esa celda', () => {
  const tieneDato = (col: number, fila: number): boolean => col === 0 && fila <= 5;
  const celdas = celdasARellenar({
    formula: '``=SUM(A1:A2)´´',
    rect: { c1: 0, c2: 0, r1: 6, r2: 6 },
    filas: 100,
    tieneDato,
    desplazar,
  });
  expect(celdas).toHaveLength(1);
  expect(celdas[0]?.formula).toBe('=SUM(A1:A2)');
  expect(celdas[0]?.dc).toBe(0);
  expect(celdas[0]?.dr).toBe(0);
});

// Fórmulas spill van exclusivamente en la celda ancla
test('fórmula spill solo se escribe en la celda ancla sin colisiones', () => {
  const celdas = celdasARellenar({
    formula: '=SEQUENCE(10)',
    rect: { c1: 0, c2: 1, r1: 0, r2: 5 },
    filas: 100,
    tieneDato: () => false,
    desplazar,
  });
  expect(celdas).toHaveLength(1);
  expect(celdas[0]?.formula).toBe('=SEQUENCE(10)');
  expect(celdas[0]?.dc).toBe(0);
  expect(celdas[0]?.dr).toBe(0);
});
