/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Valores de celda y errores canónicos de hoja de cálculo, más las reglas de
 * coacción (a número, texto, booleano) y comparación de la semántica
 * OpenFormula / ECMA-376. Código original (clean-room).
 */

/** Códigos de error canónicos de una hoja de cálculo. */
export type ErrorCode =
  | '#DIV/0!'
  | '#VALUE!'
  | '#NAME?'
  | '#REF!'
  | '#N/A'
  | '#NUM!'
  | '#NULL!'
  | '#SPILL!' // un array dinámico no cabe: choca con una celda ocupada
  | '#CALC!' // un array vacío (FILTER/UNIQUE sin resultados)
  | '#CIRC!'; // referencia circular (equivalente a la #REF! circular de Excel)

/** Un error de fórmula. Es un *valor* de celda: se propaga como tal. */
export class FormulaError {
  constructor(readonly code: ErrorCode) {}
  toString(): string {
    return this.code;
  }
}

/** Singletons de error: reutilízalos, no hace falta instanciar. */
export const ERR = {
  DIV0: new FormulaError('#DIV/0!'),
  VALUE: new FormulaError('#VALUE!'),
  NAME: new FormulaError('#NAME?'),
  REF: new FormulaError('#REF!'),
  NA: new FormulaError('#N/A'),
  NUM: new FormulaError('#NUM!'),
  NULL: new FormulaError('#NULL!'),
  SPILL: new FormulaError('#SPILL!'),
  CALC: new FormulaError('#CALC!'),
  CIRC: new FormulaError('#CIRC!'),
} as const;

/** El valor calculado de una celda o subexpresión. */
export type CellValue = number | string | boolean | FormulaError;

export function isError(v: CellValue): v is FormulaError {
  return v instanceof FormulaError;
}

/**
 * Formatea un número al «General» aproximado: enteros tal cual, decimales
 * redondeados a 15 cifras significativas (colapsa el ruido binario de f64,
 * p. ej. 0.1+0.2 → «0.3»).
 */
export function formatNumber(n: number): string {
  if (!Number.isFinite(n)) return n > 0 ? '#NUM!' : '#NUM!';
  if (Number.isInteger(n)) return String(n);
  return String(Number(n.toPrecision(15)));
}

/**
 * Coacción a número (semántica de hoja): número→él mismo, booleano→1/0,
 * texto→parseado (vacío = 0; no numérico = #VALUE!), error→se propaga.
 */
export function coerceNumber(v: CellValue): number | FormulaError {
  if (typeof v === 'number') return v;
  if (typeof v === 'boolean') return v ? 1 : 0;
  if (v instanceof FormulaError) return v;
  const t = v.trim();
  if (t === '') return 0; // celda en blanco ≈ "" en contexto numérico → 0
  let s = t;
  let pct = false;
  if (s.endsWith('%')) {
    pct = true;
    s = s.slice(0, -1).trim();
  }
  if (!/^[-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][-+]?\d+)?$/.test(s)) return ERR.VALUE;
  const n = Number(s);
  return pct ? n / 100 : n;
}

/**
 * El número de un texto escrito como argumento de una función que agrega (SUM("2";1) = 3 en
 * Excel): como coerceNumber, pero el texto vacío no es un cero sino #VALUE!.
 */
export function numeroDeLiteral(v: string): number | FormulaError {
  return v.trim() === '' ? ERR.VALUE : coerceNumber(v);
}

/**
 * Coacción a texto: texto→él mismo, número→formateado, booleano→«TRUE»/«FALSE»,
 * error→se propaga.
 */
export function coerceText(v: CellValue): string | FormulaError {
  if (typeof v === 'string') return v;
  if (typeof v === 'number') return formatNumber(v);
  if (typeof v === 'boolean') return v ? 'TRUE' : 'FALSE';
  return v;
}

/**
 * Coacción a booleano: booleano→él mismo, número→(≠0), texto→TRUE/FALSE
 * (insensible a mayúsculas; vacío = FALSE; otro = #VALUE!), error→se propaga.
 */
export function coerceBoolean(v: CellValue): boolean | FormulaError {
  if (typeof v === 'boolean') return v;
  if (typeof v === 'number') return v !== 0;
  if (v instanceof FormulaError) return v;
  const t = v.trim().toUpperCase();
  if (t === '' || t === 'FALSE') return false;
  if (t === 'TRUE') return true;
  return ERR.VALUE;
}

/** Rango de tipos para comparar mixtos: número < texto < booleano. */
function rank(v: number | string | boolean): 0 | 1 | 2 {
  return typeof v === 'number' ? 0 : typeof v === 'string' ? 1 : 2;
}

/**
 * Compara dos valores según la ordenación de hoja de cálculo. Devuelve
 * -1/0/1, o el error si alguno lo es. Texto insensible a mayúsculas; en
 * tipos distintos manda el rango número<texto<booleano.
 */
export function compareValues(a: CellValue, b: CellValue): number | FormulaError {
  if (a instanceof FormulaError) return a;
  if (b instanceof FormulaError) return b;
  const ra = rank(a);
  const rb = rank(b);
  if (ra !== rb) return ra < rb ? -1 : 1;
  if (typeof a === 'number' && typeof b === 'number') return a < b ? -1 : a > b ? 1 : 0;
  if (typeof a === 'boolean' && typeof b === 'boolean') {
    const x = a ? 1 : 0;
    const y = b ? 1 : 0;
    return x < y ? -1 : x > y ? 1 : 0;
  }
  const x = String(a).toUpperCase();
  const y = String(b).toUpperCase();
  return x < y ? -1 : x > y ? 1 : 0;
}
