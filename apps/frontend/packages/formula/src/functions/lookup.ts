/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Búsqueda y referencia: VLOOKUP, HLOOKUP, XLOOKUP, INDEX, MATCH, XMATCH,
 * LOOKUP, CHOOSE, OFFSET, INDIRECT, ROW, COLUMN, ROWS, COLUMNS, TRANSPOSE,
 * ADDRESS. Semántica OpenFormula / ECMA-376, código original (clean-room).
 *
 * Notas de contrato:
 *  - Los rangos llegan como `arg.matrix()` (CellValue[][], fila × columna). Las
 *    funciones que sólo necesitan la referencia (ROW/COLUMN/ROWS/COLUMNS/OFFSET)
 *    usan `arg.ref` para no evaluar celdas.
 *  - OFFSET/INDIRECT construyen un RefRange y lo leen con `ctx.resolve`; ROW/
 *    COLUMN sin argumentos usan `ctx.here`.
 *  - Los errores se propagan devolviendo el FormulaError que sale de
 *    value()/matrix()/coacciones. CHOOSE y el «si_no_encontrado» de XLOOKUP son
 *    perezosos (sólo evalúan la rama elegida).
 *
 * NO edites functions/index.ts: se fusiona automáticamente.
 */
import type { Arg, FnTable } from '../registry.js';
import type { CellValue, FormulaError } from '../values.js';
import { coerceBoolean, coerceNumber, coerceText, compareValues, ERR, isError } from '../values.js';
import { numToCol } from '../parser.js';

/* ---------- Utilidades de vector/búsqueda ---------- */

/** Aplana una matriz a un vector 1D en orden de lectura (fila a fila). */
function flatten(m: CellValue[][]): CellValue[] {
  const out: CellValue[] = [];
  for (const row of m) for (const v of row) out.push(v);
  return out;
}

/** Escapa los metacaracteres de RegExp de una cadena literal. */
function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/**
 * Compila un patrón con comodines de hoja a RegExp anclado e insensible a
 * mayúsculas: «*»→cualquier secuencia, «?»→un carácter, «~*»/«~?»/«~~»→literal.
 */
function wildcardToRegex(pattern: string): RegExp {
  let re = '^';
  for (let i = 0; i < pattern.length; i++) {
    const ch = pattern[i]!;
    if (ch === '~') {
      const nx = pattern[i + 1];
      if (nx === '*' || nx === '?' || nx === '~') {
        re += escapeRegex(nx);
        i++;
        continue;
      }
      re += escapeRegex('~');
      continue;
    }
    if (ch === '*') re += '.*';
    else if (ch === '?') re += '.';
    else re += escapeRegex(ch);
  }
  return new RegExp(re + '$', 'i');
}

/**
 * Si `lookup` es texto que contiene comodines de hoja, devuelve su RegExp; si no
 * (número, booleano o texto sin «*»/«?»), undefined → usar igualdad normal.
 */
function wildRegex(lookup: CellValue): RegExp | undefined {
  return typeof lookup === 'string' && /[*?]/.test(lookup) ? wildcardToRegex(lookup) : undefined;
}

/**
 * Coincidencia EXACTA: primer índice cuyo valor iguala a `lookup` (texto sin
 * distinguir mayúsculas, vía compareValues). Las celdas de error no casan. Si
 * `lookup` es texto con comodines «*»/«?» (Excel: VLOOKUP/HLOOKUP FALSE, MATCH 0),
 * casa por patrón contra las celdas de texto.
 */
function exactIndex(vec: CellValue[], lookup: CellValue): number {
  const rx = wildRegex(lookup);
  for (let i = 0; i < vec.length; i++) {
    if (rx) {
      const v = vec[i]!;
      if (typeof v === 'string' && rx.test(v)) return i;
      continue;
    }
    const c = compareValues(vec[i]!, lookup);
    if (!isError(c) && c === 0) return i;
  }
  return -1;
}

/**
 * Coincidencia APROXIMADA ascendente (MATCH 1 / VLOOKUP TRUE / LOOKUP): el mayor
 * valor ≤ lookup, asumiendo el vector ordenado ascendente; para en el primer
 * valor mayor. Las celdas de error se saltan. -1 si todos son mayores.
 */
function approxAsc(vec: CellValue[], lookup: CellValue): number {
  let cand = -1;
  for (let i = 0; i < vec.length; i++) {
    const c = compareValues(vec[i]!, lookup);
    if (isError(c)) continue;
    if (c <= 0) cand = i;
    else break;
  }
  return cand;
}

/**
 * Coincidencia APROXIMADA descendente (MATCH -1): el menor valor ≥ lookup,
 * asumiendo el vector ordenado descendente; para en el primer valor menor.
 */
function approxDesc(vec: CellValue[], lookup: CellValue): number {
  let cand = -1;
  for (let i = 0; i < vec.length; i++) {
    const c = compareValues(vec[i]!, lookup);
    if (isError(c)) continue;
    if (c >= 0) cand = i;
    else break;
  }
  return cand;
}

/**
 * Orden de recorrido de índices para XLOOKUP/XMATCH: negativo = de último a
 * primero. ponytail: los modos binarios (±2) se tratan como lineales en su
 * sentido — con datos ordenados dan el mismo índice.
 */
function order(n: number, searchMode: number): number[] {
  const out: number[] = [];
  if (searchMode < 0) for (let i = n - 1; i >= 0; i--) out.push(i);
  else for (let i = 0; i < n; i++) out.push(i);
  return out;
}

/**
 * Búsqueda de XLOOKUP/XMATCH: exacta (modo 0), comodines (modo 2: «*»/«?» si el
 * valor buscado es texto y los contiene) o, si falla, el «siguiente menor»
 * (modo -1: mayor valor < lookup) o «siguiente mayor» (modo 1: menor valor >
 * lookup). El sentido de recorrido rompe empates. -1 si no hay ninguno.
 */
function xfind(vec: CellValue[], lookup: CellValue, matchMode: number, searchMode: number): number {
  const ord = order(vec.length, searchMode);
  const rx = matchMode === 2 ? wildRegex(lookup) : undefined;
  for (const i of ord) {
    if (rx) {
      const v = vec[i]!;
      if (typeof v === 'string' && rx.test(v)) return i;
      continue;
    }
    const c = compareValues(vec[i]!, lookup);
    if (!isError(c) && c === 0) return i;
  }
  if (matchMode === 0 || matchMode === 2) return -1;
  let best = -1;
  for (const i of ord) {
    const c = compareValues(vec[i]!, lookup);
    if (isError(c)) continue;
    if (matchMode === -1 && c < 0) {
      if (best < 0) best = i;
      else {
        const d = compareValues(vec[i]!, vec[best]!);
        if (!isError(d) && d > 0) best = i; // el mayor de los menores
      }
    } else if (matchMode === 1 && c > 0) {
      if (best < 0) best = i;
      else {
        const d = compareValues(vec[i]!, vec[best]!);
        if (!isError(d) && d < 0) best = i; // el menor de los mayores
      }
    }
  }
  return best;
}

/** Lee un argumento opcional como entero (trunca). `def` si no está presente. */
function optInt(args: Arg[], i: number, def: number): number | FormulaError {
  if (i >= args.length) return def;
  const n = coerceNumber(args[i]!.value());
  return isError(n) ? n : Math.trunc(n);
}

/** Cita el nombre de hoja para ADDRESS si contiene algo distinto de palabra. */
function quoteSheet(name: string): string {
  return /^[A-Za-z_][A-Za-z0-9_.]*$/.test(name) ? name : `'${name.replace(/'/g, "''")}'`;
}

/* ---------- Funciones ---------- */

export const funcs: FnTable = {
  // VLOOKUP(valor, tabla, col, [aprox=TRUE]): busca en la 1ª columna y devuelve
  // la celda de la columna `col` (1-based). Aprox = mayor ≤ valor (tabla asc).
  VLOOKUP(_ctx, args) {
    if (args.length < 3 || args.length > 4) return ERR.VALUE;
    const lookup = args[0]!.value();
    if (isError(lookup)) return lookup;
    const table = args[1]!.matrix();
    const cols = table[0]?.length ?? 0;
    const colIdx = optInt(args, 2, 1);
    if (isError(colIdx)) return colIdx;
    if (colIdx < 1) return ERR.VALUE;
    if (colIdx > cols) return ERR.REF;
    let approx = true;
    if (args.length === 4) {
      const b = coerceBoolean(args[3]!.value());
      if (isError(b)) return b;
      approx = b;
    }
    const firstCol = table.map((row) => row[0] ?? '');
    const idx = approx ? approxAsc(firstCol, lookup) : exactIndex(firstCol, lookup);
    if (idx < 0) return ERR.NA;
    return table[idx]![colIdx - 1] ?? '';
  },

  // HLOOKUP(valor, tabla, fila, [aprox=TRUE]): igual que VLOOKUP pero busca en
  // la 1ª fila y devuelve la celda de la fila `fila` (1-based).
  HLOOKUP(_ctx, args) {
    if (args.length < 3 || args.length > 4) return ERR.VALUE;
    const lookup = args[0]!.value();
    if (isError(lookup)) return lookup;
    const table = args[1]!.matrix();
    const rows = table.length;
    const rowIdx = optInt(args, 2, 1);
    if (isError(rowIdx)) return rowIdx;
    if (rowIdx < 1) return ERR.VALUE;
    if (rowIdx > rows) return ERR.REF;
    let approx = true;
    if (args.length === 4) {
      const b = coerceBoolean(args[3]!.value());
      if (isError(b)) return b;
      approx = b;
    }
    const firstRow = table[0] ?? [];
    const idx = approx ? approxAsc(firstRow, lookup) : exactIndex(firstRow, lookup);
    if (idx < 0) return ERR.NA;
    return table[rowIdx - 1]![idx] ?? '';
  },

  // XLOOKUP(valor, buscar_en, devolver, [si_no], [modo=0], [sentido=1]).
  // modo: 0 exacta, -1 exacta o siguiente menor, 1 exacta o siguiente mayor,
  // 2 comodín («*»/«?»). Devuelve la fila/columna correspondiente de `devolver`
  // (puede ser una matriz al hacer «spill»).
  XLOOKUP(_ctx, args) {
    if (args.length < 3 || args.length > 6) return ERR.VALUE;
    const lookup = args[0]!.value();
    if (isError(lookup)) return lookup;
    const la = args[1]!.matrix();
    const ra = args[2]!.matrix();
    const horizontal = la.length === 1 && (la[0]?.length ?? 0) > 1;
    const vec = flatten(la);
    const n = vec.length;
    const matchMode = optInt(args, 4, 0);
    if (isError(matchMode)) return matchMode;
    const searchMode = optInt(args, 5, 1);
    if (isError(searchMode)) return searchMode;
    const idx = xfind(vec, lookup, matchMode, searchMode);
    if (idx < 0) return args.length >= 4 ? args[3]!.value() : ERR.NA; // perezoso
    if (horizontal) {
      if ((ra[0]?.length ?? 0) !== n) return ERR.VALUE;
      return ra.map((row) => [row[idx] ?? '']); // columna idx → vector vertical
    }
    if (ra.length !== n) return ERR.VALUE;
    return [ra[idx]!.slice()]; // fila idx → vector horizontal
  },

  // INDEX(matriz, fila, [col]): celda en (fila, col) 1-based. fila/col = 0
  // devuelve la columna/fila entera. En una matriz de una sola fila/columna el
  // único índice recorre esa dimensión.
  INDEX(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const m = args[0]!.matrix();
    const R = m.length;
    const C = m[0]?.length ?? 0;
    if (R === 0 || C === 0) return ERR.REF;
    const riV = coerceNumber(args[1]!.value());
    if (isError(riV)) return riV;
    let ri = Math.trunc(riV);
    let ci: number;
    if (args.length === 3) {
      const ciV = coerceNumber(args[2]!.value());
      if (isError(ciV)) return ciV;
      ci = Math.trunc(ciV);
    } else if (R === 1) {
      ci = ri; // fila única: el índice es la columna
      ri = 1;
    } else if (C === 1) {
      ci = 1; // columna única: el índice es la fila
    } else {
      ci = 0; // 2D con columna omitida → fila entera
    }
    if (ri < 0 || ci < 0) return ERR.VALUE;
    if (ri > R || ci > C) return ERR.REF;
    if (ri === 0 && ci === 0) return m;
    if (ri === 0) return m.map((row) => [row[ci - 1] ?? '']); // columna entera
    if (ci === 0) return [m[ri - 1]!.slice()]; // fila entera
    return m[ri - 1]![ci - 1] ?? '';
  },

  // MATCH(valor, vector, [tipo=1]): posición 1-based. tipo 0 exacta, 1 mayor ≤
  // valor (asc), -1 menor ≥ valor (desc). #N/A si no hay coincidencia.
  MATCH(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const lookup = args[0]!.value();
    if (isError(lookup)) return lookup;
    const m = args[1]!.matrix();
    if (m.length > 1 && (m[0]?.length ?? 0) > 1) return ERR.NA; // debe ser 1D
    const vec = flatten(m);
    const type = optInt(args, 2, 1);
    if (isError(type)) return type;
    const idx =
      type === 0
        ? exactIndex(vec, lookup)
        : type > 0
          ? approxAsc(vec, lookup)
          : approxDesc(vec, lookup);
    return idx < 0 ? ERR.NA : idx + 1;
  },

  // XMATCH(valor, vector, [modo=0], [sentido=1]): posición 1-based con la
  // semántica de coincidencia de XLOOKUP. #N/A si no hay ninguna.
  XMATCH(_ctx, args) {
    if (args.length < 2 || args.length > 4) return ERR.VALUE;
    const lookup = args[0]!.value();
    if (isError(lookup)) return lookup;
    const m = args[1]!.matrix();
    if (m.length > 1 && (m[0]?.length ?? 0) > 1) return ERR.NA;
    const vec = flatten(m);
    const matchMode = optInt(args, 2, 0);
    if (isError(matchMode)) return matchMode;
    const searchMode = optInt(args, 3, 1);
    if (isError(searchMode)) return searchMode;
    const idx = xfind(vec, lookup, matchMode, searchMode);
    return idx < 0 ? ERR.NA : idx + 1;
  },

  // LOOKUP: forma vectorial LOOKUP(valor, buscar_en, [devolver]) o forma de
  // matriz LOOKUP(valor, matriz) que busca en la 1ª fila/columna (según sea más
  // ancha o alta) y devuelve la última. Siempre coincidencia aproximada asc.
  LOOKUP(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const lookup = args[0]!.value();
    if (isError(lookup)) return lookup;
    const m2 = args[1]!.matrix();
    if (args.length === 3) {
      const idx = approxAsc(flatten(m2), lookup);
      if (idx < 0) return ERR.NA;
      return flatten(args[2]!.matrix())[idx] ?? ERR.NA;
    }
    const R = m2.length;
    const C = m2[0]?.length ?? 0;
    if (C > R) {
      const idx = approxAsc(m2[0] ?? [], lookup); // horizontal
      if (idx < 0) return ERR.NA;
      return m2[R - 1]![idx] ?? '';
    }
    const idx = approxAsc(
      m2.map((row) => row[0] ?? ''),
      lookup,
    ); // vertical
    if (idx < 0) return ERR.NA;
    return m2[idx]![C - 1] ?? '';
  },

  // CHOOSE(indice, v1, v2, …): devuelve el valor en la posición `indice`
  // (1-based). Perezoso: sólo evalúa el elegido. Fuera de rango → #VALUE!.
  // Devuelve la matriz del elegido para poder pasar un rango entero (p. ej.
  // SUM(CHOOSE(1, A1:A4, B1:B4))); en contexto escalar el motor lo colapsa.
  CHOOSE(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const iV = coerceNumber(args[0]!.value());
    if (isError(iV)) return iV;
    const idx = Math.trunc(iV);
    if (idx < 1 || idx > args.length - 1) return ERR.VALUE;
    return args[idx]!.matrix();
  },

  // OFFSET(ref, filas, cols, [alto], [ancho]): rango desplazado desde `ref`. El
  // 1er argumento debe ser una referencia. Alto/ancho por defecto = los de ref.
  OFFSET(ctx, args) {
    if (args.length < 3 || args.length > 5) return ERR.VALUE;
    const base = args[0]!.ref;
    if (!base) return ERR.VALUE;
    const dr = optInt(args, 1, 0);
    if (isError(dr)) return dr;
    const dc = optInt(args, 2, 0);
    if (isError(dc)) return dc;
    const h = optInt(args, 3, base.r2 - base.r1 + 1);
    if (isError(h)) return h;
    const w = optInt(args, 4, base.c2 - base.c1 + 1);
    if (isError(w)) return w;
    if (h < 1 || w < 1) return ERR.REF;
    const r1 = base.r1 + dr;
    const c1 = base.c1 + dc;
    if (r1 < 0 || c1 < 0) return ERR.REF;
    return ctx.resolve({ sheet: base.sheet, c1, r1, c2: c1 + w - 1, r2: r1 + h - 1 });
  },

  // INDIRECT(texto, [a1=TRUE]): interpreta `texto` como referencia y la lee.
  // ponytail: sólo estilo A1; a1=FALSE (R1C1) → #REF!. Ojo: la dependencia es
  // dinámica y el grafo estático no la rastrea (puede requerir recalc()).
  INDIRECT(ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const t = coerceText(args[0]!.value());
    if (isError(t)) return t;
    if (args.length === 2) {
      const a1 = coerceBoolean(args[1]!.value());
      if (isError(a1)) return a1;
      if (!a1) return ERR.REF;
    }
    const rr = ctx.parseRef(t.trim());
    if (!rr) return ERR.REF;
    return ctx.resolve(rr);
  },

  // ROW([ref]): sin argumento, la fila de la celda actual (1-based). Con una
  // referencia, su(s) número(s) de fila (vector vertical si abarca varias).
  ROW(ctx, args) {
    if (args.length === 0) return ctx.here.row + 1;
    if (args.length !== 1) return ERR.VALUE;
    const ref = args[0]!.ref;
    if (!ref) return ERR.VALUE;
    if (ref.r1 === ref.r2) return ref.r1 + 1;
    const out: CellValue[][] = [];
    for (let r = ref.r1; r <= ref.r2; r++) out.push([r + 1]);
    return out;
  },

  // COLUMN([ref]): análogo a ROW pero para columnas (vector horizontal).
  COLUMN(ctx, args) {
    if (args.length === 0) return ctx.here.col + 1;
    if (args.length !== 1) return ERR.VALUE;
    const ref = args[0]!.ref;
    if (!ref) return ERR.VALUE;
    if (ref.c1 === ref.c2) return ref.c1 + 1;
    const row: CellValue[] = [];
    for (let c = ref.c1; c <= ref.c2; c++) row.push(c + 1);
    return [row];
  },

  // ROWS(matriz/ref): número de filas. Usa la referencia si la hay (no evalúa).
  ROWS(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const ref = args[0]!.ref;
    if (ref) return ref.r2 - ref.r1 + 1;
    const m = args[0]!.matrix();
    if (m.length === 1 && (m[0]?.length ?? 0) === 1 && isError(m[0]![0]!)) return m[0]![0];
    return m.length;
  },

  // COLUMNS(matriz/ref): número de columnas.
  COLUMNS(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const ref = args[0]!.ref;
    if (ref) return ref.c2 - ref.c1 + 1;
    const m = args[0]!.matrix();
    if (m.length === 1 && (m[0]?.length ?? 0) === 1 && isError(m[0]![0]!)) return m[0]![0];
    return m[0]?.length ?? 0;
  },

  // TRANSPOSE(matriz): intercambia filas y columnas. Conserva errores en su sitio.
  TRANSPOSE(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const m = args[0]!.matrix();
    const R = m.length;
    const C = m[0]?.length ?? 0;
    if (R === 0 || C === 0) return ERR.VALUE;
    const out: CellValue[][] = [];
    for (let c = 0; c < C; c++) {
      const row: CellValue[] = [];
      for (let r = 0; r < R; r++) row.push(m[r]![c] ?? '');
      out.push(row);
    }
    return out;
  },

  // ADDRESS(fila, col, [abs=1], [a1=TRUE], [hoja]): texto de la dirección.
  // abs: 1=$A$1, 2=A$1, 3=$A1, 4=A1. a1=FALSE → estilo R1C1.
  ADDRESS(_ctx, args) {
    if (args.length < 2 || args.length > 5) return ERR.VALUE;
    const rV = coerceNumber(args[0]!.value());
    if (isError(rV)) return rV;
    const cV = coerceNumber(args[1]!.value());
    if (isError(cV)) return cV;
    const r = Math.trunc(rV);
    const c = Math.trunc(cV);
    if (r < 1 || c < 1) return ERR.VALUE;
    const abs = optInt(args, 2, 1);
    if (isError(abs)) return abs;
    if (abs < 1 || abs > 4) return ERR.VALUE;
    let a1 = true;
    if (args.length >= 4) {
      const b = coerceBoolean(args[3]!.value());
      if (isError(b)) return b;
      a1 = b;
    }
    const rowAbs = abs === 1 || abs === 2;
    const colAbs = abs === 1 || abs === 3;
    let addr = a1
      ? `${colAbs ? '$' : ''}${numToCol(c - 1)}${rowAbs ? '$' : ''}${r}`
      : `${rowAbs ? 'R' + r : 'R[' + r + ']'}${colAbs ? 'C' + c : 'C[' + c + ']'}`;
    if (args.length === 5) {
      const st = coerceText(args[4]!.value());
      if (isError(st)) return st;
      if (st !== '') addr = `${quoteSheet(st)}!${addr}`;
    }
    return addr;
  },

  // HYPERLINK(destino, [nombre]): el VALOR de la celda es el `nombre` visible (o
  // el `destino` si se omite). El enlace clicable lo pinta el editor aparte.
  HYPERLINK(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const link = coerceText(args[0]!.value());
    if (isError(link)) return link;
    if (args.length === 2) {
      const name = coerceText(args[1]!.value());
      if (isError(name)) return name;
      return name === '' ? link : name;
    }
    return link;
  },
};
