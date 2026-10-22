/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Funciones matemáticas básicas (SUM, PRODUCT, ABS, ROUND, …). Semántica
 * OpenFormula / ECMA-376, código original (clean-room). NO edites index.ts.
 */
import type { Arg, FnTable } from '../registry.js';
import type { CellValue, FormulaError } from '../values.js';
import { ERR, isError, coerceNumber, numeroDeLiteral } from '../values.js';

/* ---------- Ayudas internas ---------- */

/** Coacciona el argumento `i` a número; #VALUE! si el argumento no existe. */
function num(args: Arg[], i: number): number | FormulaError {
  const a = args[i];
  return a ? coerceNumber(a.value()) : ERR.VALUE;
}

/**
 * Recorre todos los números de los argumentos (escalares y rangos) y llama a
 * `cb` con cada uno. Criterio de Excel: el texto y las celdas en blanco de un rango (o de lo
 * que devuelve una función) se ignoran; un booleano cuenta solo como argumento escalar
 * directo, no dentro de un rango; un texto ESCRITO como argumento («2») se convierte a número
 * y, si no lo es, da #VALUE!; el primer #error corta y se devuelve.
 */
function eachNumber(args: Arg[], cb: (n: number) => void): FormulaError | undefined {
  for (const a of args) {
    const isRange = a.ref !== undefined;
    for (const row of a.matrix()) {
      for (const v of row) {
        if (isError(v)) return v;
        if (typeof v === 'number') cb(v);
        else if (typeof v === 'boolean' && !isRange) cb(v ? 1 : 0);
        else if (typeof v === 'string' && a.literal) {
          const n = numeroDeLiteral(v);
          if (isError(n)) return n;
          cb(n);
        }
      }
    }
  }
  return undefined;
}

/**
 * Recorre todos los valores de los argumentos coaccionándolos a número (blanco
 * ignorado, texto numérico convertido, texto no numérico → #VALUE!). Úsalo en
 * GCD/LCM, que sí coaccionan. `cb` puede devolver un #error para abortar.
 */
function eachCellNum(
  args: Arg[],
  cb: (n: number) => FormulaError | void,
): FormulaError | undefined {
  for (const a of args) {
    for (const row of a.matrix()) {
      for (const v of row) {
        if (v === '') continue; // blanco ≈ "" en el modelo de texto: se ignora
        const n = coerceNumber(v);
        if (isError(n)) return n;
        const r = cb(n);
        if (r) return r;
      }
    }
  }
  return undefined;
}

/**
 * Redondeo a `digits` decimales con corrección del ruido binario de f64: escala
 * moviendo la coma, limpia a 15 cifras significativas (como el formato General,
 * lo que arregla casos como ROUND(2.15;1)=2.2 o ROUND(1.005;2)=1.01) y aplica el
 * modo pedido sobre la MAGNITUD, reponiendo el signo. `round` = mitad hacia
 * afuera, `up` = hacia afuera, `down` = hacia cero (truncar).
 */
function scaled(x: number, digits: number, mode: 'round' | 'up' | 'down'): number {
  if (x === 0) return 0;
  if (!Number.isFinite(x)) return x;
  const sign = x < 0 ? -1 : 1;
  const p = Math.pow(10, digits);
  const s = Number((Math.abs(x) * p).toPrecision(15));
  const r = mode === 'up' ? Math.ceil(s) : mode === 'down' ? Math.floor(s) : Math.round(s);
  return sign * Number((r / p).toPrecision(15));
}

/** CEILING/FLOOR: redondea al múltiplo de `s` limpiando el ruido binario. */
function toMultiple(x: number, s: number, dir: 'ceil' | 'floor'): number {
  const q = Number((x / s).toPrecision(15));
  const r = dir === 'ceil' ? Math.ceil(q) : Math.floor(q);
  return Number((s * r).toPrecision(15));
}

/**
 * Cuerpo común de CEILING.PRECISE / ISO.CEILING / FLOOR.PRECISE: redondea
 * respecto a la recta al múltiplo indicado, cuyo signo se ignora. Múltiplo 0 → 0
 * (no #DIV/0!: aquí Excel devuelve cero, a diferencia del FLOOR clásico).
 */
function preciseRound(args: Arg[], dir: 'ceil' | 'floor'): CellValue {
  const x = num(args, 0);
  if (isError(x)) return x;
  let s = 1;
  if (args.length >= 2) {
    const sv = num(args, 1);
    if (isError(sv)) return sv;
    s = sv;
  }
  if (s === 0) return 0;
  return toMultiple(x, Math.abs(s), dir);
}

/**
 * Cuerpo común de CEILING.MATH / FLOOR.MATH: como el anterior, más el `modo`
 * que solo actúa sobre negativos —distinto de cero invierte la dirección, o sea
 * redondea alejándose del cero, que es lo que hacen las funciones clásicas—.
 */
function mathRound(args: Arg[], dir: 'ceil' | 'floor'): CellValue {
  const x = num(args, 0);
  if (isError(x)) return x;
  let s = 1;
  if (args.length >= 2) {
    const sv = num(args, 1);
    if (isError(sv)) return sv;
    s = sv;
  }
  if (s === 0) return 0;
  let modo = 0;
  if (args.length >= 3) {
    const mv = num(args, 2);
    if (isError(mv)) return mv;
    modo = mv;
  }
  const invertir = x < 0 && modo !== 0;
  const efectiva = invertir ? (dir === 'ceil' ? 'floor' : 'ceil') : dir;
  return toMultiple(x, Math.abs(s), efectiva);
}

/** Máximo común divisor de dos enteros no negativos (Euclides). */
function gcd2(a: number, b: number): number {
  a = Math.abs(a);
  b = Math.abs(b);
  while (b) {
    const t = a % b;
    a = b;
    b = t;
  }
  return a;
}

/* ---------- Registro ---------- */

export const funcs: FnTable = {
  // Suma los números de todos los argumentos (escalares y rangos). Ver eachNumber.
  SUM(_ctx, args) {
    let acc = 0;
    const err = eachNumber(args, (n) => {
      acc += n;
    });
    return err ?? acc;
  },

  // PERCENTOF(subconjunto, todos): la parte que el subconjunto es del total, como razón
  // (0,25 = 25 %): SUMA(subconjunto) / SUMA(todos). Es el agregador que se le pasa a
  // GROUPBY y PIVOTBY para ver cada grupo como porcentaje del total; suelta también vale.
  PERCENTOF(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    let parte = 0;
    let todo = 0;
    const e1 = eachNumber([args[0]!], (n) => {
      parte += n;
    });
    if (e1) return e1;
    const e2 = eachNumber([args[1]!], (n) => {
      todo += n;
    });
    if (e2) return e2;
    return todo === 0 ? ERR.DIV0 : parte / todo;
  },

  // Producto de los números. Sin ningún número → 0 (como una hoja real).
  PRODUCT(_ctx, args) {
    let acc = 1;
    let count = 0;
    const err = eachNumber(args, (n) => {
      acc *= n;
      count++;
    });
    return err ?? (count === 0 ? 0 : acc);
  },

  // Suma de los cuadrados de los números.
  SUMSQ(_ctx, args) {
    let acc = 0;
    const err = eachNumber(args, (n) => {
      acc += n * n;
    });
    return err ?? acc;
  },

  // Suma de los productos, componente a componente, de matrices del mismo tamaño.
  // Los no-números cuentan como 0; tamaños distintos → #VALUE!; #error se propaga.
  SUMPRODUCT(_ctx, args) {
    const mats = args.map((a) => a.matrix());
    const first = mats[0];
    if (!first) return ERR.VALUE;
    const R = first.length;
    const C = first[0]?.length ?? 0;
    for (const m of mats) {
      if (m.length !== R) return ERR.VALUE;
      for (const row of m) if (row.length !== C) return ERR.VALUE;
    }
    let sum = 0;
    for (let r = 0; r < R; r++) {
      const rows = mats.map((m) => m[r]!);
      for (let c = 0; c < C; c++) {
        let prod = 1;
        for (const row of rows) {
          const v = row[c]!;
          if (isError(v)) return v;
          prod *= typeof v === 'number' ? v : 0;
        }
        sum += prod;
      }
    }
    return sum;
  },

  ABS(_ctx, args) {
    const n = num(args, 0);
    return isError(n) ? n : Math.abs(n);
  },

  SIGN(_ctx, args) {
    const n = num(args, 0);
    if (isError(n)) return n;
    return n > 0 ? 1 : n < 0 ? -1 : 0;
  },

  SQRT(_ctx, args) {
    const n = num(args, 0);
    if (isError(n)) return n;
    return n < 0 ? ERR.NUM : Math.sqrt(n);
  },

  POWER(_ctx, args) {
    const b = num(args, 0);
    if (isError(b)) return b;
    const e = num(args, 1);
    if (isError(e)) return e;
    const p = Math.pow(b, e);
    // Igual que el operador «^» del motor: resultado no finito → #NUM!.
    return Number.isFinite(p) ? p : ERR.NUM;
  },

  // Resto con el signo del divisor: n − d·FLOOR(n/d). d = 0 → #DIV/0!.
  MOD(_ctx, args) {
    const n = num(args, 0);
    if (isError(n)) return n;
    const d = num(args, 1);
    if (isError(d)) return d;
    if (d === 0) return ERR.DIV0;
    return n - d * Math.floor(n / d);
  },

  // Parte entera de la división, truncada hacia cero. d = 0 → #DIV/0!.
  QUOTIENT(_ctx, args) {
    const n = num(args, 0);
    if (isError(n)) return n;
    const d = num(args, 1);
    if (isError(d)) return d;
    if (d === 0) return ERR.DIV0;
    return Math.trunc(n / d);
  },

  // Redondea hacia abajo (hacia −∞) al entero inferior. INT(−8,9) = −9.
  INT(_ctx, args) {
    const n = num(args, 0);
    return isError(n) ? n : Math.floor(n);
  },

  // Trunca hacia cero a `digits` decimales (0 por defecto). TRUNC(−8,9) = −8.
  TRUNC(_ctx, args) {
    const n = num(args, 0);
    if (isError(n)) return n;
    let d = 0;
    if (args.length >= 2) {
      const dv = num(args, 1);
      if (isError(dv)) return dv;
      d = dv;
    }
    return scaled(n, Math.trunc(d), 'down');
  },

  ROUND(_ctx, args) {
    const n = num(args, 0);
    if (isError(n)) return n;
    const d = num(args, 1);
    if (isError(d)) return d;
    return scaled(n, Math.trunc(d), 'round');
  },

  ROUNDUP(_ctx, args) {
    const n = num(args, 0);
    if (isError(n)) return n;
    const d = num(args, 1);
    if (isError(d)) return d;
    return scaled(n, Math.trunc(d), 'up');
  },

  ROUNDDOWN(_ctx, args) {
    const n = num(args, 0);
    if (isError(n)) return n;
    const d = num(args, 1);
    if (isError(d)) return d;
    return scaled(n, Math.trunc(d), 'down');
  },

  // Redondea al múltiplo de `multiple` más cercano (mitad hacia afuera).
  // multiple = 0 → 0; number y multiple con signos opuestos → #NUM!.
  MROUND(_ctx, args) {
    const n = num(args, 0);
    if (isError(n)) return n;
    const m = num(args, 1);
    if (isError(m)) return m;
    if (m === 0 || n === 0) return 0;
    if (n < 0 !== m < 0) return ERR.NUM; // signos opuestos (relacional liga antes que !==)
    return m * scaled(n / m, 0, 'round');
  },

  // Redondea hacia afuera (arriba en magnitud) al múltiplo de `significance`.
  CEILING(_ctx, args) {
    const x = num(args, 0);
    if (isError(x)) return x;
    let s = 1;
    if (args.length >= 2) {
      const sv = num(args, 1);
      if (isError(sv)) return sv;
      s = sv;
    }
    if (s === 0) return 0;
    if (x > 0 && s < 0) return ERR.NUM; // número positivo con múltiplo negativo
    return toMultiple(x, s, 'ceil');
  },

  // Redondea hacia dentro (abajo en magnitud) al múltiplo de `significance`.
  FLOOR(_ctx, args) {
    const x = num(args, 0);
    if (isError(x)) return x;
    let s = 1;
    if (args.length >= 2) {
      const sv = num(args, 1);
      if (isError(sv)) return sv;
      s = sv;
    }
    if (s === 0) return ERR.DIV0; // ponytail: como Excel/ECMA; Google Sheets daría 0
    if (x > 0 && s < 0) return ERR.NUM;
    return toMultiple(x, s, 'floor');
  },

  /* ---- Familia moderna de CEILING/FLOOR (Excel 2010/2013) ----------------
   *
   * Se distinguen de las clásicas de arriba en dos cosas, y ambas importan al
   * redondear NEGATIVOS —con positivos las cinco coinciden con CEILING/FLOOR—:
   *
   *   · El signo del múltiplo NO cambia el resultado ni provoca #NUM!: se toma
   *     su valor absoluto. `CEILING(-4,-1)` da error y `CEILING.MATH(-4,-1)` no.
   *   · Redondean respecto a la RECTA (arriba = hacia +∞), no respecto al cero
   *     como hacen las clásicas «hacia afuera en magnitud». De ahí que
   *     `CEILING.MATH(-4,5; 1)` sea -4 y no -5.
   *
   * El tercer argumento `modo` de las .MATH invierte esa regla solo para los
   * negativos: distinto de cero → alejarse del cero, que es el comportamiento
   * clásico. Con positivos no hace nada.
   */

  // CEILING.MATH(x, [múltiplo], [modo]): hacia +∞; con modo≠0 y x<0, hacia -∞.
  'CEILING.MATH'(_ctx, args) {
    return mathRound(args, 'ceil');
  },

  // FLOOR.MATH(x, [múltiplo], [modo]): hacia -∞; con modo≠0 y x<0, hacia +∞.
  'FLOOR.MATH'(_ctx, args) {
    return mathRound(args, 'floor');
  },

  // CEILING.PRECISE(x, [múltiplo]): siempre hacia +∞, sin modo.
  'CEILING.PRECISE'(_ctx, args) {
    return preciseRound(args, 'ceil');
  },

  // ISO.CEILING(x, [múltiplo]): el nombre ISO de CEILING.PRECISE, misma cuenta.
  'ISO.CEILING'(_ctx, args) {
    return preciseRound(args, 'ceil');
  },

  // FLOOR.PRECISE(x, [múltiplo]): siempre hacia -∞, sin modo.
  'FLOOR.PRECISE'(_ctx, args) {
    return preciseRound(args, 'floor');
  },

  // Máximo común divisor. Valores truncados a entero; negativo → #NUM!.
  GCD(_ctx, args) {
    let g = 0;
    const err = eachCellNum(args, (n) => {
      const t = Math.trunc(n);
      if (t < 0) return ERR.NUM;
      g = gcd2(g, t);
    });
    return err ?? g;
  },

  // Mínimo común múltiplo. Cualquier operando 0 → 0; negativo → #NUM!.
  LCM(_ctx, args) {
    let l = 1;
    let zero = false;
    const err = eachCellNum(args, (n) => {
      const t = Math.trunc(n);
      if (t < 0) return ERR.NUM;
      if (t === 0) {
        zero = true;
        return;
      }
      l = (l / gcd2(l, t)) * t;
    });
    if (err) return err;
    if (zero) return 0;
    return Number.isFinite(l) ? l : ERR.NUM;
  },
};
