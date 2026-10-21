/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los helpers puros de la hoja (sheets/format.ts): formatos de
 * número es-ES sobre el valor crudo, evaluación de reglas de formato
 * condicional (pertenencia al rango + predicado), comparador de ordenación y
 * shiftRefs del autorelleno (desplazamiento de referencias vía AST).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import { ERR } from '@sytue/formula';
import type { CellValue } from '@sytue/formula';
import {
  applyNumFmt,
  compileCondRules,
  condFill,
  condMatches,
  formatCellValue,
  parseRangeRef,
  sanitizeChartSpec,
  sanitizeCondRule,
  sanitizeNumFmt,
  selectionStats,
  shiftRefs,
  sortCellCompare,
} from '../src/editor/sheets/format';
import {
  extractChartData,
  extractXY,
  niceTicks,
  polarPoint,
  project,
} from '../src/editor/sheets/Chart';
import type { CondRule } from '../src/editor/types';

/** es-ES usa NBSP/espacio fino antes de € y %; se normaliza para comparar. */
const plain = (s: string): string => s.replace(/[\u00A0\u202F]/g, ' ');

test('applyNumFmt: presets es-ES sobre el valor crudo', () => {
  expect(plain(applyNumFmt({ id: 'currency' }, 1234.5))).toBe('1.234,50 €');
  expect(plain(applyNumFmt({ id: 'percent' }, 0.5))).toBe('50 %');
  expect(applyNumFmt({ id: 'number' }, 1234.5)).toBe('1234,50');
  expect(applyNumFmt({ id: 'thousands' }, 1234.5)).toBe('1.234,50');
  expect(applyNumFmt({ id: 'number', decimals: 0 }, 1234.4)).toBe('1234');
  expect(plain(applyNumFmt({ id: 'percent', decimals: 1 }, 0.1234))).toBe('12,3 %');
  expect(applyNumFmt({ id: 'general' }, 1234.5)).toBe('1234.5'); // General = el del motor
  // custom: delega en la máscara (numfmt-mask); sin máscara → General del motor.
  expect(applyNumFmt({ id: 'custom', mask: '#,##0.00;(#,##0.00)' }, -1234.5)).toBe('(1.234,50)');
  expect(applyNumFmt({ id: 'custom' }, 1234.5)).toBe('1234.5');
});

test('applyNumFmt fecha: sistema 1900 del motor (serial 1, fantasma 60, ≥61)', () => {
  expect(applyNumFmt({ id: 'date' }, 1)).toBe('1/1/1900');
  expect(applyNumFmt({ id: 'date' }, 60)).toBe('29/2/1900'); // día fantasma del bug 1900
  expect(applyNumFmt({ id: 'date' }, 61)).toBe('1/3/1900');
  expect(applyNumFmt({ id: 'date' }, 45292)).toBe('1/1/2024');
  expect(applyNumFmt({ id: 'date' }, -3)).toBe('-3'); // fuera del sistema → General
});

test('formatCellValue: los no-números pasan de largo (error/booleano/texto)', () => {
  expect(formatCellValue(ERR.DIV0, { id: 'currency' })).toBe('#DIV/0!');
  expect(formatCellValue(true, { id: 'number' })).toBe('TRUE');
  expect(formatCellValue('hola', { id: 'currency' })).toBe('hola');
  expect(formatCellValue(2)).toBe('2');
});

test('parseRangeRef: celda, rango, $, minúsculas, normalización y basura', () => {
  expect(parseRangeRef('B2')).toEqual({ c1: 1, r1: 1, c2: 1, r2: 1 });
  expect(parseRangeRef('a1:C3')).toEqual({ c1: 0, r1: 0, c2: 2, r2: 2 });
  expect(parseRangeRef('C3:A1')).toEqual({ c1: 0, r1: 0, c2: 2, r2: 2 }); // invertido
  expect(parseRangeRef('$A$1:$B$2')).toEqual({ c1: 0, r1: 0, c2: 1, r2: 1 });
  expect(parseRangeRef('1A')).toBeNull();
  expect(parseRangeRef('A1:B')).toBeNull();
  expect(parseRangeRef('')).toBeNull();
});

const rule = (patch: Partial<CondRule>): CondRule => ({
  range: 'A1',
  op: 'gt',
  value: '100',
  fill: '#FEF08A',
  ...patch,
});

test('condMatches: numérico (umbral es-ES), contains, eq/ne texto, errores y vacías nunca', () => {
  expect(condMatches(rule({}), 150)).toBe(true);
  expect(condMatches(rule({}), 100)).toBe(false);
  expect(condMatches(rule({ op: 'ge' }), 100)).toBe(true);
  expect(condMatches(rule({ op: 'le', value: '1.000,5' }), 1000.5)).toBe(true); // miles+coma es-ES
  expect(condMatches(rule({}), 'texto')).toBe(false); // el orden es solo numérico
  expect(condMatches(rule({}), ERR.DIV0)).toBe(false); // errores nunca disparan
  expect(condMatches(rule({ op: 'ne', value: 'x' }), '')).toBe(false); // vacías nunca
  expect(condMatches(rule({ op: 'contains', value: 'HOLA' }), 'dijo hola ayer')).toBe(true);
  expect(condMatches(rule({ op: 'contains', value: 'adiós' }), 'dijo hola')).toBe(false);
  expect(condMatches(rule({ op: 'eq', value: 'sí' }), 'SÍ')).toBe(true);
  expect(condMatches(rule({ op: 'ne', value: 'sí' }), 'no')).toBe(true);
  expect(condMatches(rule({ op: 'eq', value: '5' }), 5)).toBe(true);
});

test('condFill: pertenencia al rango y prioridad (gana la primera regla que cumple)', () => {
  const compiled = compileCondRules([
    rule({ range: 'A1:B2', op: 'gt', value: '10', fill: '#FEF08A' }),
    rule({ range: 'B2:C3', op: 'gt', value: '0', fill: '#BBF7D0' }),
    rule({ range: 'no-parsea', fill: '#FBCFE8' }), // descartada al compilar
  ]);
  expect(compiled.length).toBe(2);
  expect(condFill(compiled, 0, 0, 50)).toBe('#FEF08A'); // A1: solo la 1ª
  expect(condFill(compiled, 2, 2, 50)).toBe('#BBF7D0'); // C3: solo la 2ª
  expect(condFill(compiled, 1, 1, 50)).toBe('#FEF08A'); // B2 cumple ambas → primera
  expect(condFill(compiled, 1, 1, 5)).toBe('#BBF7D0'); // B2: la 1ª no cumple, la 2ª sí
  expect(condFill(compiled, 5, 5, 50)).toBeNull(); // fuera de todo rango
  expect(condFill(compiled, 0, 0, 'texto')).toBeNull(); // gt no ordena texto
});

test('sanitize: reglas y formatos desde datos no confiables (persistencia)', () => {
  const ok = { range: 'A1:B2', op: 'gt', value: '1', fill: '#FEF08A' };
  expect(sanitizeCondRule(ok)).toEqual(ok as CondRule);
  expect(sanitizeCondRule({ ...ok, range: 'no' })).toBeNull();
  expect(sanitizeCondRule({ ...ok, fill: 'url(evil)' })).toBeNull(); // solo #RRGGBB
  expect(sanitizeCondRule({ ...ok, op: 'jajaja' })).toBeNull();
  expect(sanitizeCondRule('cadena')).toBeNull();
  expect(sanitizeNumFmt({ id: 'currency', decimals: 3.7 })).toEqual({
    id: 'currency',
    decimals: 3,
  });
  expect(sanitizeNumFmt({ id: 'number' })).toEqual({ id: 'number' });
  expect(sanitizeNumFmt({ id: 'lo-que-sea' })).toBeNull();
  expect(sanitizeNumFmt(7)).toBeNull();
  // custom: exige máscara no vacía; sin ella (o vacía) no es un formato válido.
  expect(sanitizeNumFmt({ id: 'custom', mask: 'dd/mm/yyyy' })).toEqual({
    id: 'custom',
    mask: 'dd/mm/yyyy',
  });
  expect(sanitizeNumFmt({ id: 'custom' })).toBeNull();
  expect(sanitizeNumFmt({ id: 'custom', mask: '   ' })).toBeNull();
});

test('sortCellCompare: número < texto < booleano < error; vacías SIEMPRE al final', () => {
  expect(sortCellCompare(1, 2, 1)).toBeLessThan(0);
  expect(sortCellCompare('a', 'B', 1)).toBeLessThan(0); // insensible a mayúsculas
  expect(sortCellCompare(99, 'a', 1)).toBeLessThan(0); // número antes que texto
  expect(sortCellCompare('z', true, 1)).toBeLessThan(0); // texto antes que booleano
  expect(sortCellCompare(true, ERR.DIV0, 1)).toBeLessThan(0); // booleano antes que error
  expect(sortCellCompare('', 5, 1)).toBeGreaterThan(0); // vacía al final (asc)
  expect(sortCellCompare(2, 1, -1)).toBeLessThan(0); // desc invierte los valores…
  expect(sortCellCompare('', 5, -1)).toBeGreaterThan(0); // …pero la vacía sigue al final
  expect(sortCellCompare('a', 99, -1)).toBeLessThan(0); // …y también el rango de tipos
});

test('sanitizeChartSpec: tipo/rango válidos, x/y clamp, id regenerado, basura fuera', () => {
  expect(sanitizeChartSpec({ id: 'c1', type: 'bar', range: 'A1:B4', x: 10.6, y: -5 })).toEqual({
    id: 'c1',
    type: 'bar',
    range: 'A1:B4',
    x: 11,
    y: 0,
  });
  expect(sanitizeChartSpec({ id: 'c1', type: 'pie', range: 'A1' })).toEqual({
    id: 'c1',
    type: 'pie',
    range: 'A1',
  });
  expect(sanitizeChartSpec({ type: 'line', range: 'A1:A3' })?.id).toBeTruthy(); // id regenerado
  // Tipos nuevos válidos + series saneadas (enteros ≥0 y por debajo de las 16.384 columnas de la hoja, únicos, ordenados; vacío se descarta).
  expect(
    sanitizeChartSpec({
      id: 'c',
      type: 'combo',
      range: 'A1:C4',
      lineSeries: [2, 2, -1, 99, 16384, 1.5],
      secondaryAxis: [],
    }),
  ).toEqual({ id: 'c', type: 'combo', range: 'A1:C4', lineSeries: [2, 99] });
  expect(sanitizeChartSpec({ id: 'c', type: 'donut', range: 'A1:B2' })).toEqual({
    id: 'c',
    type: 'donut',
    range: 'A1:B2',
  });
  expect(sanitizeChartSpec({ id: 'c', type: 'radar', range: 'A1:B2' })?.type).toBe('radar');
  expect(sanitizeChartSpec({ id: 'c', type: 'bogus', range: 'A1' })).toBeNull(); // tipo desconocido fuera
  expect(sanitizeChartSpec({ id: 'c', type: 'bar', range: 'no-parsea' })).toBeNull();
  expect(sanitizeChartSpec({ id: 'c', type: 'bar', range: 'A1', x: Infinity })).toEqual({
    id: 'c',
    type: 'bar',
    range: 'A1',
  });
  expect(sanitizeChartSpec('cadena')).toBeNull();
  expect(sanitizeChartSpec(null)).toBeNull();
});

test('extractChartData: 1.ª columna rótulos, cabecera solo si es toda texto, no numérico = 0', () => {
  // A1:'Mes' B1:'Ventas' · A2:'Ene' B2:10 · A3:'Feb' B3:'x' (texto → 0)
  const grid: Record<string, CellValue> = {
    '0,0': 'Mes',
    '1,0': 'Ventas',
    '0,1': 'Ene',
    '1,1': 10,
    '0,2': 'Feb',
    '1,2': 'x',
  };
  const get = (c: number, r: number): CellValue => grid[`${c},${r}`] ?? '';
  expect(extractChartData({ c1: 0, r1: 0, c2: 1, r2: 2 }, get)).toEqual({
    labels: ['Ene', 'Feb'],
    series: [{ name: 'Ventas', values: [10, 0] }],
  });
  // Sin cabecera (número en la fila 1) y una sola columna: fila y letra de columna.
  expect(extractChartData({ c1: 1, r1: 1, c2: 1, r2: 2 }, get)).toEqual({
    labels: ['2', '3'],
    series: [{ name: 'B', values: [10, 0] }],
  });
});

test('niceTicks: paso 1·2·5, incluye extremos y degenerados sin dividir por cero', () => {
  expect(niceTicks(0, 10)).toEqual({ min: 0, max: 10, ticks: [0, 5, 10] });
  expect(niceTicks(-3, 7)).toEqual({ min: -5, max: 10, ticks: [-5, 0, 5, 10] });
  expect(niceTicks(0, 0)).toEqual({ min: 0, max: 1, ticks: [0, 0.5, 1] });
  expect(niceTicks(4, 4)).toEqual({ min: 0, max: 4, ticks: [0, 1, 2, 3, 4] });
  expect(niceTicks(0, 0.07).ticks).toEqual([0, 0.02, 0.04, 0.06, 0.08]); // sin ruido flotante
});

test('project: mapeo lineal de ejes; dominio degenerado = punto medio', () => {
  expect(project(5, 0, 10, 0, 100)).toBe(50);
  expect(project(2, 0, 4, 0, 200)).toBe(100);
  expect(project(7, 0, 0, 10, 20)).toBe(15); // min===max → medio, sin dividir por cero
});

test('polarPoint: vértices de un cuadrado (i=0 arriba, horario) y fracción del radio', () => {
  const near = (p: { x: number; y: number }, x: number, y: number): void => {
    expect(p.x).toBeCloseTo(x, 6);
    expect(p.y).toBeCloseTo(y, 6);
  };
  near(polarPoint(0, 0, 10, 0, 4, 1), 0, -10); // arriba
  near(polarPoint(0, 0, 10, 1, 4, 1), 10, 0); // derecha
  near(polarPoint(0, 0, 10, 2, 4, 1), 0, 10); // abajo
  near(polarPoint(0, 0, 10, 3, 4, 1), -10, 0); // izquierda
  near(polarPoint(0, 0, 10, 1, 4, 0.5), 5, 0); // media distancia
});

test('extractXY (dispersión): 1.ª columna = X cruda numérica, resto Y; NaN si no es número', () => {
  const grid: Record<string, CellValue> = {
    '0,0': 'X',
    '1,0': 'Y',
    '0,1': 1,
    '1,1': 10,
    '0,2': 2,
    '1,2': 20,
    '0,3': 3,
    '1,3': 'x', // no numérico → NaN
  };
  const get = (c: number, r: number): CellValue => grid[`${c},${r}`] ?? '';
  expect(extractXY({ c1: 0, r1: 0, c2: 1, r2: 3 }, get)).toEqual({
    x: [1, 2, 3],
    series: [{ name: 'Y', values: [10, 20, NaN] }],
  });
  // Sin cabecera (número en la fila 1): X cruda y nombre = letra de columna.
  expect(extractXY({ c1: 0, r1: 1, c2: 1, r2: 2 }, get)).toEqual({
    x: [1, 2],
    series: [{ name: 'B', values: [10, 20] }],
  });
});

test('shiftRefs: relativas se desplazan, $ancladas no, strings y hojas intactas', () => {
  expect(shiftRefs('=A1+B$2*$C3', 1, 2)).toBe('=B3+C$2*$C5');
  expect(shiftRefs('=$A$1', 5, 5)).toBe('=$A$1');
  expect(shiftRefs('=SUMA(A1:B2;"A1:B2")', 0, 1)).toBe('=SUMA(A2:B3;"A1:B2")'); // la cadena no se toca
  expect(shiftRefs("='Hoja 2'!A1+Hoja3!B2", 1, 0)).toBe("='Hoja 2'!B1+Hoja3!C2");
  expect(shiftRefs('=(A1+B1)*C1', 0, 1)).toBe('=(A2+B2)*C2'); // paréntesis conservados
  expect(shiftRefs('=-A1%', 0, 1)).toBe('=-A2%'); // unario y porcentaje
  expect(shiftRefs('=SI(A1>2;"sí";B1&"x")', 2, 0)).toBe('=SI(C1>2;"sí";D1&"x")');
  expect(shiftRefs('=A1', 0, -1)).toBe('#REF!'); // fuera de la rejilla → literal
  expect(shiftRefs('=FOO(', 1, 1)).toBe('=FOO('); // no parsea → copia tal cual
});

test('shiftRefs: un argumento omitido se conserva, no se come el siguiente', () => {
  expect(shiftRefs('=GROUPBY(A1:A9;B1:B9;SUM;;0)', 1, 0)).toBe('=GROUPBY(B1:B9;C1:C9;SUM;;0)');
  expect(shiftRefs('=IF(A1,,B1)', 0, 1)).toBe('=IF(A2;;B2)');
});

test('shiftRefs: refs de eje entero desplazan SOLO su eje (sin #REF! fantasma)', () => {
  expect(shiftRefs('=SUM(A:A)', 2, 0)).toBe('=SUM(C:C)'); // columna entera → +2 columnas
  expect(shiftRefs('=SUM(A:A)', 0, 5)).toBe('=SUM(A:A)'); // desplazar filas no la mueve
  expect(shiftRefs('=SUM(A:C)', 1, 0)).toBe('=SUM(B:D)');
  expect(shiftRefs('=SUM($A:$A)', 2, 0)).toBe('=SUM($A:$A)'); // ancladas no se mueven
  expect(shiftRefs('=SUM(1:1)', 0, 2)).toBe('=SUM(3:3)'); // fila entera → +2 filas
  expect(shiftRefs('=SUM(3:5)', 0, -1)).toBe('=SUM(2:4)');
});

/* ---------- Resumen de la selección (barra de hojas) ---------- */

test('selectionStats: cuenta lo ocupado, suma y promedia solo los números', () => {
  // Recuento incluye el texto (criterio Excel); la aritmética, solo los números.
  expect(selectionStats([1, 2, 3, 'texto', ''])).toEqual({ count: 4, sum: 6, avg: 2 });
  // Un error o un booleano ocupan celda pero no entran en suma ni promedio.
  expect(selectionStats([10, ERR.DIV0, true, ''])).toEqual({ count: 3, sum: 10, avg: 10 });
  // Sin un solo número no hay nada que resumir: la barra no pinta el bloque.
  expect(selectionStats(['a', 'b'])).toBeNull();
  expect(selectionStats([])).toBeNull();
  // Promedio a seis decimales (1/3 no puede salir con quince dígitos en la barra).
  expect(selectionStats([1, 1, 0])?.avg).toBe(0.666667);
  // Números enormes: se dejan sin redondear en vez de salirse del entero seguro.
  expect(selectionStats([1e12, 1e12])?.avg).toBe(1e12);
});
