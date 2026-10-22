/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Arrays dinámicos (semántica OpenFormula / Excel 365, código original
 * clean-room): SEQUENCE, RANDARRAY, UNIQUE, SORT, SORTBY, FILTER. Devuelven una
 * MATRIZ que el motor DERRAMA (spill) a las celdas contiguas; en contexto
 * escalar el motor la colapsa a su esquina superior-izquierda. LET vive en el
 * motor (necesita el AST de sus argumentos), no aquí.
 *
 * NO edites functions/index.ts: se fusiona automáticamente.
 */
import type { Arg, FnTable } from '../registry.js';
import type { CellValue } from '../values.js';
import {
  coerceBoolean,
  coerceNumber,
  compareValues,
  ERR,
  FormulaError,
  isError,
} from '../values.js';

/** Lee un argumento opcional como entero truncado; `def` si no está presente. */
function optInt(args: Arg[], i: number, def: number): number | FormulaError {
  if (i >= args.length) return def;
  const n = coerceNumber(args[i]!.value());
  return isError(n) ? n : Math.trunc(n);
}

/** Lee un argumento opcional como número; `def` si no está presente. */
function optNum(args: Arg[], i: number, def: number): number | FormulaError {
  return i >= args.length ? def : coerceNumber(args[i]!.value());
}

/** Clave estable de un valor (para igualdad de UNIQUE): tipo + contenido. */
function valKey(v: CellValue): string {
  if (isError(v)) return 'e:' + v.code;
  if (typeof v === 'number') return 'n:' + v;
  if (typeof v === 'boolean') return 'b:' + v;
  return 's:' + v.toUpperCase(); // texto insensible a mayúsculas (como la hoja)
}

/** Clave de una fila/columna entera. */
function rowKey(row: CellValue[]): string {
  return row.map(valKey).join('\u0001');
}

/**
 * Comparador de orden de hoja con los errores AL FINAL (Excel ordena los errores
 * después de todo). `dir` = 1 asc, -1 desc (los errores quedan al final en ambos).
 */
function cmp(a: CellValue, b: CellValue, dir: number): number {
  const ea = isError(a);
  const eb = isError(b);
  if (ea || eb) return ea && eb ? 0 : ea ? 1 : -1; // el error va detrás
  const c = compareValues(a, b);
  return (isError(c) ? 0 : c) * dir;
}

/** Transpone una matriz (para tratar «por columnas» como «por filas»). */
function transpose(m: CellValue[][]): CellValue[][] {
  const R = m.length;
  const C = m[0]?.length ?? 0;
  const out: CellValue[][] = [];
  for (let c = 0; c < C; c++) {
    const row: CellValue[] = [];
    for (let r = 0; r < R; r++) row.push(m[r]![c] ?? '');
    out.push(row);
  }
  return out;
}

/* ---------- Álgebra matricial (MMULT/MINVERSE/MDETERM/MUNIT) ---------- */

/** Matriz de un argumento coaccionada a números (blanco→0; texto→#VALUE!). */
function numMatrix(a: Arg): number[][] | FormulaError {
  const out: number[][] = [];
  for (const row of a.matrix()) {
    const r: number[] = [];
    for (const v of row) {
      if (isError(v)) return v;
      if (typeof v === 'number') r.push(v);
      else {
        const n = coerceNumber(v);
        if (isError(n)) return ERR.VALUE;
        r.push(n);
      }
    }
    out.push(r);
  }
  return out;
}

/** ¿La matriz es cuadrada y no vacía? */
function isSquare(m: number[][]): boolean {
  return m.length > 0 && m.every((r) => r.length === m.length);
}

/** Pares (x,y) alineados por posición; longitudes distintas → #N/A; se descarta
 *  el par si alguna celda no es número (regla SUMX2MY2/…). */
function pairedNums(ax: Arg, ay: Arg): { xs: number[]; ys: number[] } | FormulaError {
  const fx: CellValue[] = [];
  const fy: CellValue[] = [];
  for (const row of ax.matrix()) for (const v of row) fx.push(v);
  for (const row of ay.matrix()) for (const v of row) fy.push(v);
  if (fx.length !== fy.length) return ERR.NA;
  const xs: number[] = [];
  const ys: number[] = [];
  for (let i = 0; i < fx.length; i++) {
    const xv = fx[i]!;
    const yv = fy[i]!;
    if (isError(xv)) return xv;
    if (isError(yv)) return yv;
    if (typeof xv === 'number' && typeof yv === 'number') {
      xs.push(xv);
      ys.push(yv);
    }
  }
  return { xs, ys };
}

/* ---------- Remodelado de arrays (TAKE/DROP/WRAP/STACK/…) ---------- */

/** Dimensiones [filas, columnas] de una matriz (0×0 si está vacía). */
function dims(m: CellValue[][]): [number, number] {
  return [m.length, m[0]?.length ?? 0];
}

/** Índice 1-based (negativo = desde el final) → 0-based; -1 si está fuera de rango. */
function resolveIndex(k: number, total: number): number {
  if (k > 0 && k <= total) return k - 1;
  if (k < 0 && -k <= total) return total + k;
  return -1;
}

/** Rango [inicio, fin) de TAKE(n): n≥0 desde el principio, n<0 desde el final. */
function takeRange(n: number, total: number): [number, number] {
  return n >= 0 ? [0, Math.min(n, total)] : [Math.max(0, total + n), total];
}

/** Rango [inicio, fin) de DROP(n): descarta n del principio (n≥0) o del final (n<0). */
function dropRange(n: number, total: number): [number, number] {
  return n >= 0 ? [Math.min(n, total), total] : [0, Math.max(0, total + n)];
}

/** Sub-matriz [r0,r1)×[c0,c1); #CALC! si el recorte queda vacío. */
function slice2d(
  m: CellValue[][],
  r: [number, number],
  c: [number, number],
): CellValue[][] | FormulaError {
  if (r[1] <= r[0] || c[1] <= c[0]) return ERR.CALC;
  const out: CellValue[][] = [];
  for (let i = r[0]; i < r[1]; i++) {
    const row: CellValue[] = [];
    for (let j = c[0]; j < c[1]; j++) row.push(m[i]![j] ?? '');
    out.push(row);
  }
  return out;
}

/** Aplana una matriz a un vector aplicando `ignore` (1 blancos, 2 errores, 3 ambos)
 *  y el orden (por filas o por columnas). */
function flattenVals(m: CellValue[][], ignore: number, byCol: boolean): CellValue[] {
  const [R, C] = dims(m);
  const out: CellValue[] = [];
  const push = (v: CellValue): void => {
    if ((ignore === 1 || ignore === 3) && v === '') return;
    if ((ignore === 2 || ignore === 3) && isError(v)) return;
    out.push(v);
  };
  if (byCol) {
    for (let c = 0; c < C; c++) for (let r = 0; r < R; r++) push(m[r]![c] ?? '');
  } else {
    for (let r = 0; r < R; r++) for (let c = 0; c < C; c++) push(m[r]![c] ?? '');
  }
  return out;
}

/** Núcleo de TOROW/TOCOL: aplana a una fila (o columna). */
function toLine(args: Arg[], asColumn: boolean): CellValue[][] | FormulaError {
  if (args.length < 1 || args.length > 3) return ERR.VALUE;
  const ignore = optInt(args, 1, 0);
  if (isError(ignore)) return ignore;
  if (ignore < 0 || ignore > 3) return ERR.VALUE;
  const byCol = args.length >= 3 ? coerceBoolean(args[2]!.value()) : false;
  if (isError(byCol)) return byCol;
  const vals = flattenVals(args[0]!.matrix(), ignore, byCol);
  if (vals.length === 0) return ERR.CALC;
  return asColumn ? vals.map((v) => [v]) : [vals];
}

/** Núcleo de WRAPROWS/WRAPCOLS: envuelve un vector en líneas de `wrap_count`. */
function wrap(args: Arg[], byRow: boolean): CellValue[][] | FormulaError {
  if (args.length < 2 || args.length > 3) return ERR.VALUE;
  const m = args[0]!.matrix();
  const [R, C] = dims(m);
  if (R !== 1 && C !== 1) return ERR.VALUE; // debe ser un vector (fila o columna)
  const count = optInt(args, 1, 1);
  if (isError(count)) return count;
  if (count < 1) return ERR.NUM;
  const pad: CellValue = args.length >= 3 ? args[2]!.value() : ERR.NA;
  if (isError(pad) && args.length >= 3) return pad;
  const vals: CellValue[] = [];
  for (const row of m) for (const v of row) vals.push(v);
  const n = vals.length;
  const lines = Math.ceil(n / count);
  const rows: CellValue[][] = [];
  for (let i = 0; i < lines; i++) {
    const chunk: CellValue[] = [];
    for (let j = 0; j < count; j++) {
      const idx = i * count + j;
      chunk.push(idx < n ? vals[idx]! : pad);
    }
    rows.push(chunk);
  }
  return byRow ? rows : transpose(rows);
}

export const funcs: FnTable = {
  // SEQUENCE(filas, [cols=1], [inicio=1], [paso=1]): matriz filas×cols numerada
  // por filas. filas/cols se truncan; < 1 → #VALUE!.
  SEQUENCE(_ctx, args) {
    if (args.length < 1 || args.length > 4) return ERR.VALUE;
    const rows = optInt(args, 0, 1);
    if (isError(rows)) return rows;
    const cols = optInt(args, 1, 1);
    if (isError(cols)) return cols;
    const start = optNum(args, 2, 1);
    if (isError(start)) return start;
    const step = optNum(args, 3, 1);
    if (isError(step)) return step;
    if (rows < 1 || cols < 1) return ERR.VALUE;
    if (rows * cols > 1_000_000) return ERR.NUM; // ponytail: tope defensivo de tamaño
    const out: CellValue[][] = [];
    let v = start;
    for (let r = 0; r < rows; r++) {
      const row: CellValue[] = [];
      for (let c = 0; c < cols; c++) {
        row.push(v);
        v += step;
      }
      out.push(row);
    }
    return out;
  },

  // RANDARRAY([filas=1], [cols=1], [min=0], [max=1], [entero=FALSE]).
  RANDARRAY(ctx, args) {
    if (args.length > 5) return ERR.VALUE;
    const rows = optInt(args, 0, 1);
    if (isError(rows)) return rows;
    const cols = optInt(args, 1, 1);
    if (isError(cols)) return cols;
    const min = optNum(args, 2, 0);
    if (isError(min)) return min;
    const max = optNum(args, 3, 1);
    if (isError(max)) return max;
    let integer = false;
    if (args.length >= 5) {
      const b = coerceBoolean(args[4]!.value());
      if (isError(b)) return b;
      integer = b;
    }
    if (rows < 1 || cols < 1 || min > max) return ERR.VALUE;
    if (rows * cols > 1_000_000) return ERR.NUM;
    const out: CellValue[][] = [];
    for (let r = 0; r < rows; r++) {
      const row: CellValue[] = [];
      for (let c = 0; c < cols; c++) {
        const u = ctx.random();
        row.push(integer ? Math.floor(u * (max - min + 1)) + min : min + u * (max - min));
      }
      out.push(row);
    }
    return out;
  },

  // UNIQUE(array, [por_col=FALSE], [una_vez=FALSE]): filas (o columnas) distintas,
  // en orden de aparición. `una_vez` deja solo las que aparecen exactamente 1 vez.
  UNIQUE(_ctx, args) {
    if (args.length < 1 || args.length > 3) return ERR.VALUE;
    let m = args[0]!.matrix();
    const byCol = args.length >= 2 ? coerceBoolean(args[1]!.value()) : false;
    if (isError(byCol)) return byCol;
    const once = args.length >= 3 ? coerceBoolean(args[2]!.value()) : false;
    if (isError(once)) return once;
    const transposed = byCol;
    if (transposed) m = transpose(m);
    const counts = new Map<string, number>();
    for (const row of m) counts.set(rowKey(row), (counts.get(rowKey(row)) ?? 0) + 1);
    const seen = new Set<string>();
    const out: CellValue[][] = [];
    for (const row of m) {
      const k = rowKey(row);
      if (seen.has(k)) continue;
      seen.add(k);
      if (once && counts.get(k) !== 1) continue;
      out.push(row.slice());
    }
    if (out.length === 0) return ERR.CALC;
    return transposed ? transpose(out) : out;
  },

  // SORT(array, [indice=1], [orden=1], [por_col=FALSE]): ordena filas por la
  // columna `indice` (1-based); orden 1 asc, -1 desc. `por_col` ordena columnas.
  SORT(_ctx, args) {
    if (args.length < 1 || args.length > 4) return ERR.VALUE;
    let m = args[0]!.matrix();
    const idx = optInt(args, 1, 1);
    if (isError(idx)) return idx;
    const order = optInt(args, 2, 1);
    if (isError(order)) return order;
    const byCol = args.length >= 4 ? coerceBoolean(args[3]!.value()) : false;
    if (isError(byCol)) return byCol;
    const dir = order < 0 ? -1 : 1;
    if (byCol) m = transpose(m);
    const width = m[0]?.length ?? 0;
    if (idx < 1 || idx > width) return ERR.VALUE;
    const rows = m.map((r) => r.slice());
    rows.sort((a, b) => cmp(a[idx - 1] ?? '', b[idx - 1] ?? '', dir));
    return byCol ? transpose(rows) : rows;
  },

  // SORTBY(array, por1, [orden1], [por2, orden2], …): ordena las filas de `array`
  // por los valores de cada vector `porN` (mismo nº de filas), con su `ordenN`.
  SORTBY(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const m = args[0]!.matrix();
    const n = m.length;
    const keys: { vec: CellValue[]; dir: number }[] = [];
    for (let i = 1; i < args.length; i += 2) {
      const by = args[i]!.matrix();
      // Acepta el vector como columna (n×1) o fila (1×n).
      const vec = by.length === n ? by.map((r) => r[0] ?? '') : (by[0] ?? []);
      if (vec.length !== n) return ERR.VALUE;
      let dir = 1;
      if (i + 1 < args.length) {
        const o = coerceNumber(args[i + 1]!.value());
        if (isError(o)) return o;
        dir = o < 0 ? -1 : 1;
      }
      keys.push({ vec, dir });
    }
    const idxs = m.map((_, i) => i);
    idxs.sort((a, b) => {
      for (const { vec, dir } of keys) {
        const c = cmp(vec[a]!, vec[b]!, dir);
        if (c !== 0) return c;
      }
      return 0;
    });
    return idxs.map((i) => m[i]!.slice());
  },

  // FILTER(array, incluir, [si_vacio]): conserva las filas (incluir = vector
  // columna) o columnas (incluir = vector fila) marcadas TRUE. Sin ninguna →
  // `si_vacio` (o #CALC! si se omite).
  FILTER(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const m = args[0]!.matrix();
    const R = m.length;
    const C = m[0]?.length ?? 0;
    const inc = args[1]!.matrix();
    const incR = inc.length;
    const incC = inc[0]?.length ?? 0;

    const keep = (v: CellValue): boolean | FormulaError => {
      if (isError(v)) return v;
      const b = coerceBoolean(v);
      return b;
    };

    let out: CellValue[][];
    if (incC === 1 && incR === R) {
      // filtra filas
      out = [];
      for (let r = 0; r < R; r++) {
        const k = keep(inc[r]![0] ?? '');
        if (isError(k)) return k;
        if (k) out.push(m[r]!.slice());
      }
    } else if (incR === 1 && incC === C) {
      // filtra columnas
      const cols: number[] = [];
      for (let c = 0; c < C; c++) {
        const k = keep(inc[0]![c] ?? '');
        if (isError(k)) return k;
        if (k) cols.push(c);
      }
      out = m.map((row) => cols.map((c) => row[c] ?? ''));
      if (cols.length === 0) out = [];
    } else {
      return ERR.VALUE; // dimensiones incompatibles
    }

    if (out.length === 0 || (out[0]?.length ?? 0) === 0) {
      return args.length === 3 ? args[2]!.value() : ERR.CALC;
    }
    return out;
  },

  // MMULT(a, b): producto matricial. columnas(a) debe igualar filas(b). Derrama.
  MMULT(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const A = numMatrix(args[0]!);
    if (A instanceof FormulaError) return A;
    const B = numMatrix(args[1]!);
    if (B instanceof FormulaError) return B;
    const ra = A.length;
    const ca = A[0]?.length ?? 0;
    const rb = B.length;
    const cb = B[0]?.length ?? 0;
    if (ca === 0 || ca !== rb) return ERR.VALUE;
    const out: CellValue[][] = [];
    for (let i = 0; i < ra; i++) {
      const row: CellValue[] = [];
      for (let j = 0; j < cb; j++) {
        let s = 0;
        for (let k = 0; k < ca; k++) s += A[i]![k]! * B[k]![j]!;
        row.push(Number.isFinite(s) ? s : ERR.NUM);
      }
      out.push(row);
    }
    return out;
  },

  // MDETERM(m): determinante (eliminación gaussiana con pivoteo parcial). Escalar.
  MDETERM(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const A = numMatrix(args[0]!);
    if (A instanceof FormulaError) return A;
    if (!isSquare(A)) return ERR.VALUE;
    const n = A.length;
    const m = A.map((r) => r.slice());
    let det = 1;
    for (let i = 0; i < n; i++) {
      let p = i;
      for (let r = i + 1; r < n; r++) if (Math.abs(m[r]![i]!) > Math.abs(m[p]![i]!)) p = r;
      if (m[p]![i]! === 0) return 0;
      if (p !== i) {
        const tmp = m[i]!;
        m[i] = m[p]!;
        m[p] = tmp;
        det = -det;
      }
      det *= m[i]![i]!;
      for (let r = i + 1; r < n; r++) {
        const f = m[r]![i]! / m[i]![i]!;
        for (let c = i; c < n; c++) m[r]![c]! -= f * m[i]![c]!;
      }
    }
    return Number.isFinite(det) ? det : ERR.NUM;
  },

  // MINVERSE(m): matriz inversa (Gauss-Jordan). Singular → #NUM!. Derrama.
  MINVERSE(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const A = numMatrix(args[0]!);
    if (A instanceof FormulaError) return A;
    if (!isSquare(A)) return ERR.VALUE;
    const n = A.length;
    // Aumenta [A | I] y reduce por filas.
    const m = A.map((r, i) => {
      const row = r.slice();
      for (let j = 0; j < n; j++) row.push(i === j ? 1 : 0);
      return row;
    });
    for (let i = 0; i < n; i++) {
      let p = i;
      for (let r = i + 1; r < n; r++) if (Math.abs(m[r]![i]!) > Math.abs(m[p]![i]!)) p = r;
      if (m[p]![i]! === 0) return ERR.NUM; // singular
      const tmp = m[i]!;
      m[i] = m[p]!;
      m[p] = tmp;
      const piv = m[i]![i]!;
      for (let c = 0; c < 2 * n; c++) m[i]![c]! /= piv;
      for (let r = 0; r < n; r++) {
        if (r === i) continue;
        const f = m[r]![i]!;
        for (let c = 0; c < 2 * n; c++) m[r]![c]! -= f * m[i]![c]!;
      }
    }
    const out: CellValue[][] = [];
    for (let i = 0; i < n; i++) {
      const row: CellValue[] = [];
      for (let j = n; j < 2 * n; j++) {
        const v = m[i]![j]!;
        if (!Number.isFinite(v)) return ERR.NUM;
        row.push(v);
      }
      out.push(row);
    }
    return out;
  },

  // MUNIT(n): matriz identidad n×n. Derrama.
  MUNIT(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const nv = coerceNumber(args[0]!.value());
    if (isError(nv)) return nv;
    const n = Math.trunc(nv);
    if (n < 1) return ERR.VALUE;
    if (n * n > 1_000_000) return ERR.NUM; // ponytail: tope defensivo de tamaño
    const out: CellValue[][] = [];
    for (let i = 0; i < n; i++) {
      const row: CellValue[] = [];
      for (let j = 0; j < n; j++) row.push(i === j ? 1 : 0);
      out.push(row);
    }
    return out;
  },

  // SUMX2MY2(array_x, array_y): Σ(xᵢ² − yᵢ²).
  SUMX2MY2(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = pairedNums(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    let s = 0;
    for (let i = 0; i < p.xs.length; i++) s += p.xs[i]! * p.xs[i]! - p.ys[i]! * p.ys[i]!;
    return s;
  },
  // SUMX2PY2(array_x, array_y): Σ(xᵢ² + yᵢ²).
  SUMX2PY2(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = pairedNums(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    let s = 0;
    for (let i = 0; i < p.xs.length; i++) s += p.xs[i]! * p.xs[i]! + p.ys[i]! * p.ys[i]!;
    return s;
  },
  // SUMXMY2(array_x, array_y): Σ(xᵢ − yᵢ)².
  SUMXMY2(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = pairedNums(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    let s = 0;
    for (let i = 0; i < p.xs.length; i++) {
      const d = p.xs[i]! - p.ys[i]!;
      s += d * d;
    }
    return s;
  },

  // HSTACK(a, b, …): apila matrices EN HORIZONTAL. Filas = máximo de las entradas;
  // los huecos de una matriz más baja se rellenan con #N/A.
  HSTACK(_ctx, args) {
    if (args.length < 1) return ERR.VALUE;
    const mats = args.map((a) => a.matrix());
    const R = Math.max(...mats.map((m) => m.length));
    const out: CellValue[][] = [];
    for (let r = 0; r < R; r++) {
      const row: CellValue[] = [];
      for (const m of mats) {
        const C = m[0]?.length ?? 0;
        const src = m[r];
        for (let c = 0; c < C; c++) row.push(src ? (src[c] ?? '') : ERR.NA);
      }
      out.push(row);
    }
    return out;
  },

  // VSTACK(a, b, …): apila matrices EN VERTICAL. Columnas = máximo de las entradas;
  // los huecos de una matriz más estrecha se rellenan con #N/A.
  VSTACK(_ctx, args) {
    if (args.length < 1) return ERR.VALUE;
    const mats = args.map((a) => a.matrix());
    const C = Math.max(...mats.map((m) => m[0]?.length ?? 0));
    const out: CellValue[][] = [];
    for (const m of mats) {
      for (const row of m) {
        const r: CellValue[] = [];
        for (let c = 0; c < C; c++) r.push(row[c] ?? ERR.NA);
        out.push(r);
      }
    }
    return out;
  },

  // TOROW(array, [ignorar=0], [por_columna=FALSE]): aplana a una única FILA.
  // ignorar: 1 blancos, 2 errores, 3 ambos.
  TOROW(_ctx, args) {
    return toLine(args, false);
  },
  // TOCOL(array, [ignorar=0], [por_columna=FALSE]): aplana a una única COLUMNA.
  TOCOL(_ctx, args) {
    return toLine(args, true);
  },

  // WRAPROWS(vector, wrap_count, [relleno=#N/A]): envuelve un vector en FILAS de
  // `wrap_count` columnas; la última se completa con `relleno`.
  WRAPROWS(_ctx, args) {
    return wrap(args, true);
  },
  // WRAPCOLS(vector, wrap_count, [relleno=#N/A]): envuelve un vector en COLUMNAS de
  // `wrap_count` filas.
  WRAPCOLS(_ctx, args) {
    return wrap(args, false);
  },

  // TRIMRANGE(rango, [recortar_filas=3], [recortar_columnas=3]): quita las filas y columnas
  // en blanco de los BORDES del rango (0 nada, 1 las iniciales, 2 las finales, 3 ambas).
  // Solo los bordes: una fila en blanco en medio se queda. Todo en blanco → #CALC!.
  TRIMRANGE(_ctx, args) {
    if (args.length < 1 || args.length > 3) return ERR.VALUE;
    const m = args[0]!.matrix();
    const modos: number[] = [];
    for (const i of [1, 2]) {
      const arg = args[i];
      const n = arg ? coerceNumber(arg.value()) : 3;
      if (isError(n)) return n;
      if (![0, 1, 2, 3].includes(Math.trunc(n))) return ERR.VALUE;
      modos.push(Math.trunc(n));
    }
    const [R, C] = dims(m);
    const vacia = (v: CellValue | undefined): boolean => v === undefined || v === '';
    const filaVacia = (r: number): boolean => m[r]!.every(vacia);
    const colVacia = (c: number): boolean => m.every((f) => vacia(f[c]));
    let r1 = 0;
    let r2 = R;
    let c1 = 0;
    let c2 = C;
    if (modos[0]! & 1) while (r1 < r2 && filaVacia(r1)) r1++;
    if (modos[0]! & 2) while (r2 > r1 && filaVacia(r2 - 1)) r2--;
    if (modos[1]! & 1) while (c1 < c2 && colVacia(c1)) c1++;
    if (modos[1]! & 2) while (c2 > c1 && colVacia(c2 - 1)) c2--;
    if (r1 >= r2 || c1 >= c2) return ERR.CALC;
    return slice2d(m, [r1, r2], [c1, c2]);
  },

  // TAKE(array, filas, [columnas]): conserva las primeras `filas`/`columnas`
  // (negativo = desde el final). Recorte vacío → #CALC!.
  TAKE(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const m = args[0]!.matrix();
    const [R, C] = dims(m);
    const rn = coerceNumber(args[1]!.value());
    if (isError(rn)) return rn;
    let cRange: [number, number] = [0, C];
    if (args.length === 3) {
      const cn = coerceNumber(args[2]!.value());
      if (isError(cn)) return cn;
      cRange = takeRange(Math.trunc(cn), C);
    }
    return slice2d(m, takeRange(Math.trunc(rn), R), cRange);
  },

  // DROP(array, filas, [columnas]): descarta las primeras `filas`/`columnas`
  // (negativo = desde el final).
  DROP(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const m = args[0]!.matrix();
    const [R, C] = dims(m);
    const rn = coerceNumber(args[1]!.value());
    if (isError(rn)) return rn;
    let cRange: [number, number] = [0, C];
    if (args.length === 3) {
      const cn = coerceNumber(args[2]!.value());
      if (isError(cn)) return cn;
      cRange = dropRange(Math.trunc(cn), C);
    }
    return slice2d(m, dropRange(Math.trunc(rn), R), cRange);
  },

  // EXPAND(array, filas, [columnas], [relleno=#N/A]): amplía la matriz hasta
  // filas×columnas rellenando con `relleno`. Encoger → #VALUE!.
  EXPAND(_ctx, args) {
    if (args.length < 2 || args.length > 4) return ERR.VALUE;
    const m = args[0]!.matrix();
    const [R, C] = dims(m);
    const rn = coerceNumber(args[1]!.value());
    if (isError(rn)) return rn;
    const targetR = Math.trunc(rn);
    let targetC = C;
    if (args.length >= 3) {
      const cn = coerceNumber(args[2]!.value());
      if (isError(cn)) return cn;
      targetC = Math.trunc(cn);
    }
    if (targetR < R || targetC < C || targetR < 1 || targetC < 1) return ERR.VALUE;
    const pad: CellValue = args.length >= 4 ? args[3]!.value() : ERR.NA;
    if (isError(pad) && args.length >= 4) return pad;
    const out: CellValue[][] = [];
    for (let r = 0; r < targetR; r++) {
      const row: CellValue[] = [];
      for (let c = 0; c < targetC; c++) row.push(r < R && c < C ? (m[r]![c] ?? '') : pad);
      out.push(row);
    }
    return out;
  },

  // CHOOSEROWS(array, fila1, [fila2], …): selecciona filas por índice 1-based
  // (negativo = desde el final). Índice fuera de rango → #VALUE!.
  CHOOSEROWS(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const m = args[0]!.matrix();
    const R = m.length;
    const out: CellValue[][] = [];
    for (let i = 1; i < args.length; i++) {
      for (const row of args[i]!.matrix()) {
        for (const v of row) {
          const n = coerceNumber(v);
          if (isError(n)) return n;
          const idx = resolveIndex(Math.trunc(n), R);
          if (idx < 0) return ERR.VALUE;
          out.push(m[idx]!.slice());
        }
      }
    }
    return out.length === 0 ? ERR.CALC : out;
  },

  // CHOOSECOLS(array, col1, [col2], …): selecciona columnas por índice 1-based
  // (negativo = desde el final).
  CHOOSECOLS(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const m = args[0]!.matrix();
    const C = m[0]?.length ?? 0;
    const cols: number[] = [];
    for (let i = 1; i < args.length; i++) {
      for (const row of args[i]!.matrix()) {
        for (const v of row) {
          const n = coerceNumber(v);
          if (isError(n)) return n;
          const idx = resolveIndex(Math.trunc(n), C);
          if (idx < 0) return ERR.VALUE;
          cols.push(idx);
        }
      }
    }
    if (cols.length === 0) return ERR.CALC;
    return m.map((row) => cols.map((c) => row[c] ?? ''));
  },
};
