/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Funciones de BASE DE DATOS (semántica OpenFormula / ECMA-376, código original
 * clean-room): DSUM, DCOUNT, DCOUNTA, DAVERAGE, DMAX, DMIN, DPRODUCT, DGET.
 *
 * Firma común: DFUN(base, campo, criterios). `base` es un rango cuya 1ª fila son
 * las cabeceras. `campo` selecciona la columna (nº 1-based, o nombre de cabecera).
 * `criterios` es un rango con cabeceras y filas de condiciones: un registro
 * coincide si cumple TODAS las condiciones de ALGUNA fila (Y dentro de fila, O
 * entre filas). Reutiliza el parser de criterios de statsif.ts.
 *
 * NO edites functions/index.ts: se fusiona automáticamente.
 */
import type { Arg, FnTable } from '../registry.js';
import type { CellValue } from '../values.js';
import { coerceText, ERR, FormulaError } from '../values.js';
import { matches, parseCriterion } from './statsif.js';

/** Guard local sobre uniones que no son CellValue (Criterion, number[], …). */
function isErr(v: unknown): v is FormulaError {
  return v instanceof FormulaError;
}

/** Compara dos cabeceras (texto, sin distinguir mayúsculas ni espacios extremos). */
function headerEq(a: CellValue, b: string): boolean {
  const t = coerceText(a);
  return !isErr(t) && t.trim().toUpperCase() === b.trim().toUpperCase();
}

/** Resuelve el argumento `campo` a un índice de columna 0-based de la base. */
function resolveField(headers: CellValue[], fieldArg: Arg): number | FormulaError {
  const v = fieldArg.value();
  if (isErr(v)) return v;
  if (typeof v === 'number') {
    const i = Math.trunc(v);
    if (i < 1 || i > headers.length) return ERR.VALUE;
    return i - 1;
  }
  const t = coerceText(v);
  if (isErr(t)) return t;
  const idx = headers.findIndex((h) => headerEq(h, t));
  return idx < 0 ? ERR.VALUE : idx;
}

/**
 * Índices de los registros (filas 1..n de la base) que casan con los criterios.
 * Empareja cada columna de criterios con la columna homónima de la base.
 */
function matchingRows(db: CellValue[][], crit: CellValue[][]): number[] | FormulaError {
  const dbHeaders = db[0] ?? [];
  const critHeaders = crit[0] ?? [];
  // Precompila, por cada columna de criterios, su columna destino en la base.
  const colMap: number[] = critHeaders.map((h) =>
    dbHeaders.findIndex((d) => {
      const t = coerceText(h);
      return !isErr(t) && headerEq(d, t);
    }),
  );

  const out: number[] = [];
  for (let r = 1; r < db.length; r++) {
    const record = db[r]!;
    let anyRow = false;
    for (let cr = 1; cr < crit.length && !anyRow; cr++) {
      let allCols = true;
      for (let c = 0; c < critHeaders.length; c++) {
        const cell = crit[cr]![c] ?? '';
        if (cell === '') continue; // condición vacía: se ignora
        const target = colMap[c]!;
        if (target < 0) {
          allCols = false;
          break;
        } // cabecera de criterio sin columna
        const parsed = parseCriterion(cell);
        if (isErr(parsed)) return parsed;
        if (!matches(record[target] ?? '', parsed)) {
          allCols = false;
          break;
        }
      }
      if (allCols) anyRow = true;
    }
    if (anyRow) out.push(r);
  }
  return out;
}

/** Reúne los valores de la columna `field` de los registros que casan. */
function collect(args: Arg[]): { vals: CellValue[] } | FormulaError {
  if (args.length !== 3) return ERR.VALUE;
  const db = args[0]!.matrix();
  if (db.length < 1) return ERR.VALUE;
  const field = resolveField(db[0]!, args[1]!);
  if (isErr(field)) return field;
  const crit = args[2]!.matrix();
  const rows = matchingRows(db, crit);
  if (isErr(rows)) return rows;
  return { vals: rows.map((r) => db[r]![field] ?? '') };
}

/** Solo los números de la columna seleccionada (propaga #error). */
function numbersOf(args: Arg[]): number[] | FormulaError {
  const c = collect(args);
  if (isErr(c)) return c;
  const out: number[] = [];
  for (const v of c.vals) {
    if (isErr(v)) return v;
    if (typeof v === 'number') out.push(v);
  }
  return out;
}

/**
 * Varianza o desviación de una lista, muestral o poblacional.
 *
 * La muestral necesita AL MENOS DOS registros —con uno, n−1 es cero y no hay varianza
 * que estimar— y ahí devuelve #DIV/0!; la poblacional se conforma con uno (vale 0).
 * Esa asimetría es real y es lo que distingue las dos familias.
 */
function dispersión(ns: number[], muestral: boolean, raíz: boolean): number | FormulaError {
  const n = ns.length;
  if (n === 0 || (muestral && n < 2)) return ERR.DIV0;
  const media = ns.reduce((a, b) => a + b, 0) / n;
  let sq = 0;
  for (const v of ns) sq += (v - media) * (v - media);
  const v = sq / (muestral ? n - 1 : n);
  return raíz ? Math.sqrt(v) : v;
}

export const funcs: FnTable = {
  DSUM(_ctx, args) {
    const ns = numbersOf(args);
    if (isErr(ns)) return ns;
    return ns.reduce((a, b) => a + b, 0);
  },

  DPRODUCT(_ctx, args) {
    const ns = numbersOf(args);
    if (isErr(ns)) return ns;
    return ns.reduce((a, b) => a * b, 1);
  },

  DAVERAGE(_ctx, args) {
    const ns = numbersOf(args);
    if (isErr(ns)) return ns;
    if (ns.length === 0) return ERR.DIV0;
    return ns.reduce((a, b) => a + b, 0) / ns.length;
  },

  DMAX(_ctx, args) {
    const ns = numbersOf(args);
    if (isErr(ns)) return ns;
    if (ns.length === 0) return 0;
    return Math.max(...ns);
  },

  DMIN(_ctx, args) {
    const ns = numbersOf(args);
    if (isErr(ns)) return ns;
    if (ns.length === 0) return 0;
    return Math.min(...ns);
  },

  // DCOUNT: cuenta cuántos de los registros que casan tienen NÚMERO en el campo.
  DCOUNT(_ctx, args) {
    const ns = numbersOf(args);
    return isErr(ns) ? ns : ns.length;
  },

  // DCOUNTA: cuenta los registros que casan con valor NO vacío en el campo.
  DCOUNTA(_ctx, args) {
    const c = collect(args);
    if (isErr(c)) return c;
    return c.vals.filter((v) => v !== '').length;
  },

  // DGET: el único valor que casa. Ninguno → #VALUE!; más de uno → #NUM!.
  DGET(_ctx, args) {
    const c = collect(args);
    if (isErr(c)) return c;
    if (c.vals.length === 0) return ERR.VALUE;
    if (c.vals.length > 1) return ERR.NUM;
    return c.vals[0]!;
  },
  /* ---------- Dispersión sobre los registros que casan ----------
   * Mismo criterio que STDEV/VAR fuera del mundo «D»: la variante sin P es MUESTRAL
   * (divide por n−1) y la que lleva P es POBLACIONAL (divide por n). Confundirlas da un
   * resultado creíble y sistemáticamente sesgado, que es la peor forma de estar mal. */

  /** Desviación típica MUESTRAL de los registros que cumplen el criterio. */
  DSTDEV(_ctx, args) {
    const ns = numbersOf(args);
    if (isErr(ns)) return ns;
    return dispersión(ns, true, true);
  },
  /** Desviación típica POBLACIONAL: se supone que los registros SON toda la población. */
  DSTDEVP(_ctx, args) {
    const ns = numbersOf(args);
    if (isErr(ns)) return ns;
    return dispersión(ns, false, true);
  },
  /** Varianza MUESTRAL de los registros que cumplen el criterio. */
  DVAR(_ctx, args) {
    const ns = numbersOf(args);
    if (isErr(ns)) return ns;
    return dispersión(ns, true, false);
  },
  /** Varianza POBLACIONAL de los registros que cumplen el criterio. */
  DVARP(_ctx, args) {
    const ns = numbersOf(args);
    if (isErr(ns)) return ns;
    return dispersión(ns, false, false);
  },
};
