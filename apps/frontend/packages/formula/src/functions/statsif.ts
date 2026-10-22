/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Agregados condicionales (COUNTIF, COUNTIFS, SUMIF, SUMIFS, AVERAGEIF,
 * AVERAGEIFS, MINIFS, MAXIFS) y el PARSER DE CRITERIOS de hoja de cálculo.
 * Semántica OpenFormula / ECMA-376, código original (clean-room).
 *
 * Un criterio es un valor escalar: número/booleano (igualdad directa) o texto
 * con un operador de comparación opcional al inicio (">=", "<=", "<>", ">",
 * "<", "=") y un operando. El operando es numérico si parsea como número; si no
 * es texto y, para «=»/«<>», admite comodines «*» (cualquier secuencia), «?»
 * (un carácter) y «~» de escape. El texto compara sin distinguir mayúsculas.
 *
 * NO edites functions/index.ts: se fusiona automáticamente.
 */
import type { Arg, FnCtx, FnTable } from '../registry.js';
import type { CellValue } from '../values.js';
import { coerceNumber, compareValues, ERR, FormulaError } from '../values.js';

/**
 * Guard local: el `isError` del núcleo solo acepta `CellValue`, pero aquí
 * comprobamos también uniones con error (Grid, Criterion, matriz). Es una línea.
 * ponytail: local a propósito; reutilizar `isError` obligaría a castear en cada sitio.
 */
function isErr(v: unknown): v is FormulaError {
  return v instanceof FormulaError;
}

type Op = '=' | '<>' | '>' | '<' | '>=' | '<=';

/** Criterio ya parseado, listo para aplicar a cada celda de un rango. */
interface Criterion {
  op: Op;
  kind: 'num' | 'text' | 'empty' | 'bool';
  n: number; // válido si kind === 'num'
  b: boolean; // válido si kind === 'bool'
  text: string; // operando textual (kind === 'text', para orden con compareValues)
  regex: RegExp | undefined; // patrón de comodines, solo para «=»/«<>» textual
}

/** Escapa los metacaracteres de una regex (para las partes literales del comodín). */
function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/**
 * Compila un patrón con comodines de hoja a RegExp insensible a mayúsculas:
 * «*»→cualquier secuencia, «?»→un carácter, «~*»/«~?»/«~~»→literal.
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
      re += escapeRegex('~'); // «~» suelto = literal
      continue;
    }
    if (ch === '*') {
      re += '.*';
      continue;
    }
    if (ch === '?') {
      re += '.';
      continue;
    }
    re += escapeRegex(ch);
  }
  return new RegExp(re + '$', 'i');
}

/** Parsea el operando a número reutilizando la coacción del núcleo. Vacío = no numérico. */
function operandNumber(s: string): number | undefined {
  if (s.trim() === '') return undefined;
  const n = coerceNumber(s);
  return isErr(n) ? undefined : n;
}

/** Parsea un valor-criterio a `Criterion`. Propaga el error si el criterio lo es.
 *  Exportado para reutilizarlo en las funciones de base de datos (database.ts). */
export function parseCriterion(v: CellValue): Criterion | FormulaError {
  if (isErr(v)) return v;
  if (typeof v === 'number')
    return { op: '=', kind: 'num', n: v, b: false, text: '', regex: undefined };
  if (typeof v === 'boolean')
    return { op: '=', kind: 'bool', n: 0, b: v, text: '', regex: undefined };

  let op: Op = '=';
  let rest = v;
  const two = v.slice(0, 2);
  if (two === '>=' || two === '<=' || two === '<>') {
    op = two;
    rest = v.slice(2);
  } else {
    const one = v[0];
    if (one === '>' || one === '<' || one === '=') {
      op = one;
      rest = v.slice(1);
    }
  }

  if (rest === '') return { op, kind: 'empty', n: 0, b: false, text: '', regex: undefined };

  const num = operandNumber(rest);
  if (num !== undefined) return { op, kind: 'num', n: num, b: false, text: rest, regex: undefined };

  if (op === '=' || op === '<>') {
    const upper = rest.toUpperCase();
    if (upper === 'TRUE') return { op, kind: 'bool', n: 0, b: true, text: rest, regex: undefined };
    if (upper === 'FALSE')
      return { op, kind: 'bool', n: 0, b: false, text: rest, regex: undefined };
    return { op, kind: 'text', n: 0, b: false, text: rest, regex: wildcardToRegex(rest) };
  }
  return { op, kind: 'text', n: 0, b: false, text: rest, regex: undefined };
}

/**
 * ¿La celda cumple el criterio? Reglas de hoja de cálculo: las celdas de error
 * y en blanco nunca cuentan (salvo el blanco con operando vacío en «=»). Las
 * comparaciones numéricas de orden solo aplican a números; los comodines solo a
 * texto; «<>» cuenta todo lo no-igual (incluidos tipos distintos), menos blanco.
 */
export function matches(cell: CellValue, c: Criterion): boolean {
  if (isErr(cell)) return false;
  const blank = cell === '';

  if (c.kind === 'empty') {
    if (c.op === '=') return blank;
    if (c.op === '<>') return !blank;
    return false; // orden con operando vacío: sin sentido → no cuenta
  }
  if (blank) return false; // el blanco solo cuenta con operando vacío en «=»

  if (c.kind === 'bool') {
    const eq = typeof cell === 'boolean' && cell === c.b;
    return c.op === '<>' ? !eq : eq;
  }

  if (c.kind === 'num') {
    if (c.op === '=') return typeof cell === 'number' && cell === c.n;
    if (c.op === '<>') return !(typeof cell === 'number' && cell === c.n);
    if (typeof cell !== 'number') return false;
    switch (c.op) {
      case '>':
        return cell > c.n;
      case '<':
        return cell < c.n;
      case '>=':
        return cell >= c.n;
      case '<=':
        return cell <= c.n;
    }
    return false;
  }

  // kind === 'text'
  if (c.op === '=' || c.op === '<>') {
    if (typeof cell !== 'string') return c.op === '<>'; // no-texto: nunca «=», siempre «<>»
    const m = c.regex!.test(cell);
    return c.op === '=' ? m : !m;
  }
  // Orden (>,<,>=,<=) con operando textual: Excel segrega por tipo, solo compara
  // contra celdas de texto (un número no es «<texto» ni un booleano «>texto»).
  if (typeof cell !== 'string') return false;
  const cmp = compareValues(cell, c.text);
  if (isErr(cmp)) return false;
  switch (c.op) {
    case '>':
      return cmp > 0;
    case '<':
      return cmp < 0;
    case '>=':
      return cmp >= 0;
    case '<=':
      return cmp <= 0;
  }
  return false;
}

/** Malla de pares criterio-rango ya resueltos (todos del mismo tamaño). */
interface Grid {
  crits: Criterion[];
  ranges: CellValue[][][];
  rows: number;
  cols: number;
}

/**
 * Parsea los pares (rango, criterio) desde `start` (paso 2). Exige que todos los
 * rangos tengan el mismo tamaño (si no, #VALUE!) y propaga el error de un criterio.
 */
function parseGrid(args: Arg[], start: number): Grid | FormulaError {
  const crits: Criterion[] = [];
  const ranges: CellValue[][][] = [];
  let rows = -1;
  let cols = -1;
  for (let i = start; i + 1 < args.length; i += 2) {
    const m = args[i]!.matrix();
    const r = m.length;
    const c = m[0]?.length ?? 0;
    if (rows === -1) {
      rows = r;
      cols = c;
    } else if (r !== rows || c !== cols) {
      return ERR.VALUE;
    }
    const parsed = parseCriterion(args[i + 1]!.value());
    if (isErr(parsed)) return parsed;
    ranges.push(m);
    crits.push(parsed);
  }
  return { crits, ranges, rows, cols };
}

/** ¿La posición (r,c) cumple TODOS los criterios de la malla? */
function allMatch(g: Grid, r: number, c: number): boolean {
  for (let i = 0; i < g.crits.length; i++) {
    if (!matches(g.ranges[i]![r]?.[c] ?? '', g.crits[i]!)) return false;
  }
  return true;
}

/** Cuenta las posiciones que cumplen todos los criterios. */
function countMatches(g: Grid): number {
  let n = 0;
  for (let r = 0; r < g.rows; r++) for (let c = 0; c < g.cols; c++) if (allMatch(g, r, c)) n++;
  return n;
}

/**
 * Alinea un rango de valores (suma/media) al tamaño del rango de criterios,
 * anclando en su esquina superior izquierda (quirk de SUMIF/AVERAGEIF: el rango
 * de valores no tiene por qué venir con la misma forma). Si ya coincide, se usa
 * tal cual; si no y es una referencia literal, se re-resuelve; si no, #VALUE!.
 */
function alignRange(
  ctx: FnCtx,
  arg: Arg,
  rows: number,
  cols: number,
): CellValue[][] | FormulaError {
  const m = arg.matrix();
  if (m.length === rows && (m[0]?.length ?? 0) === cols) return m;
  const ref = arg.ref;
  if (!ref) return ERR.VALUE;
  return ctx.resolve({
    sheet: ref.sheet,
    c1: ref.c1,
    r1: ref.r1,
    c2: ref.c1 + cols - 1,
    r2: ref.r1 + rows - 1,
  });
}

export const funcs: FnTable = {
  COUNTIF(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const g = parseGrid(args, 0);
    return isErr(g) ? g : countMatches(g);
  },

  COUNTIFS(_ctx, args) {
    if (args.length < 2 || args.length % 2 !== 0) return ERR.VALUE;
    const g = parseGrid(args, 0);
    return isErr(g) ? g : countMatches(g);
  },

  SUMIF(ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const range = args[0]!.matrix();
    const crit = parseCriterion(args[1]!.value());
    if (isErr(crit)) return crit;
    const rows = range.length;
    const cols = range[0]?.length ?? 0;
    const sum = alignRange(ctx, args.length === 3 ? args[2]! : args[0]!, rows, cols);
    if (isErr(sum)) return sum;
    let acc = 0;
    for (let r = 0; r < rows; r++)
      for (let c = 0; c < cols; c++) {
        if (!matches(range[r]?.[c] ?? '', crit)) continue;
        const v = sum[r]?.[c] ?? '';
        if (isErr(v)) return v;
        if (typeof v === 'number') acc += v;
      }
    return acc;
  },

  SUMIFS(_ctx, args) {
    if (args.length < 3 || args.length % 2 !== 1) return ERR.VALUE;
    const sum = args[0]!.matrix();
    const g = parseGrid(args, 1);
    if (isErr(g)) return g;
    if (sum.length !== g.rows || (sum[0]?.length ?? 0) !== g.cols) return ERR.VALUE;
    let acc = 0;
    for (let r = 0; r < g.rows; r++)
      for (let c = 0; c < g.cols; c++) {
        if (!allMatch(g, r, c)) continue;
        const v = sum[r]?.[c] ?? '';
        if (isErr(v)) return v;
        if (typeof v === 'number') acc += v;
      }
    return acc;
  },

  AVERAGEIF(ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const range = args[0]!.matrix();
    const crit = parseCriterion(args[1]!.value());
    if (isErr(crit)) return crit;
    const rows = range.length;
    const cols = range[0]?.length ?? 0;
    const avg = alignRange(ctx, args.length === 3 ? args[2]! : args[0]!, rows, cols);
    if (isErr(avg)) return avg;
    let acc = 0;
    let n = 0;
    for (let r = 0; r < rows; r++)
      for (let c = 0; c < cols; c++) {
        if (!matches(range[r]?.[c] ?? '', crit)) continue;
        const v = avg[r]?.[c] ?? '';
        if (isErr(v)) return v;
        if (typeof v === 'number') {
          acc += v;
          n++;
        }
      }
    return n === 0 ? ERR.DIV0 : acc / n;
  },

  AVERAGEIFS(_ctx, args) {
    if (args.length < 3 || args.length % 2 !== 1) return ERR.VALUE;
    const avg = args[0]!.matrix();
    const g = parseGrid(args, 1);
    if (isErr(g)) return g;
    if (avg.length !== g.rows || (avg[0]?.length ?? 0) !== g.cols) return ERR.VALUE;
    let acc = 0;
    let n = 0;
    for (let r = 0; r < g.rows; r++)
      for (let c = 0; c < g.cols; c++) {
        if (!allMatch(g, r, c)) continue;
        const v = avg[r]?.[c] ?? '';
        if (isErr(v)) return v;
        if (typeof v === 'number') {
          acc += v;
          n++;
        }
      }
    return n === 0 ? ERR.DIV0 : acc / n;
  },

  MINIFS(_ctx, args) {
    return minMaxIfs(args, true);
  },

  MAXIFS(_ctx, args) {
    return minMaxIfs(args, false);
  },
};

/** Núcleo común de MINIFS/MAXIFS. Sin coincidencias numéricas → 0 (como Excel). */
function minMaxIfs(args: Arg[], min: boolean): CellValue {
  if (args.length < 3 || args.length % 2 !== 1) return ERR.VALUE;
  const range = args[0]!.matrix();
  const g = parseGrid(args, 1);
  if (isErr(g)) return g;
  if (range.length !== g.rows || (range[0]?.length ?? 0) !== g.cols) return ERR.VALUE;
  let best: number | undefined;
  for (let r = 0; r < g.rows; r++)
    for (let c = 0; c < g.cols; c++) {
      if (!allMatch(g, r, c)) continue;
      const v = range[r]?.[c] ?? '';
      if (isErr(v)) return v;
      if (typeof v === 'number')
        best = best === undefined ? v : min ? Math.min(best, v) : Math.max(best, v);
    }
  return best ?? 0;
}
