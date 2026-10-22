/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Estadística descriptiva sin condición (AVERAGE, COUNT, MAX, MIN, MEDIAN,
 * MODE, STDEV, VAR, LARGE, SMALL, RANK, PERCENTILE, QUARTILE, GEOMEAN, …).
 * Semántica OpenFormula / ECMA-376, código original (clean-room).
 *
 * Convención de recolección de números (igual que el SUM de referencia en
 * math.ts, para coherencia del paquete): en un RANGO/referencia solo cuentan
 * los números; texto y blancos se ignoran; los booleanos cuentan SOLO como
 * argumento escalar directo, no dentro de un rango. Los errores se propagan.
 * Las variantes «A» (AVERAGEA/MAXA/MINA/COUNTA) sí incluyen texto (=0) y
 * booleanos (TRUE=1/FALSE=0) también dentro de rangos.
 */
import type { Arg, FnTable } from '../registry.js';
import { erfc } from './erf.js';
import {
  coerceBoolean,
  coerceNumber,
  ERR,
  FormulaError,
  isError,
  numeroDeLiteral,
  type CellValue,
} from '../values.js';

/* ---------- Distribución normal (aproximaciones estándar) ---------- */

/** CDF normal estándar Φ(z). */
function normCdf(z: number): number {
  return 0.5 * erfc(-z / Math.SQRT2); // por erfc: las dos colas con todas sus cifras
}

/** PDF normal estándar φ(z). */
function normPdf(z: number): number {
  return Math.exp((-z * z) / 2) / Math.sqrt(2 * Math.PI);
}

/**
 * Inversa de la CDF normal estándar Φ⁻¹(p): el algoritmo racional de P. Acklam (error
 * relativo ≲ 1e-9) y un paso de Halley contra la Φ exacta, que lo lleva a doble precisión.
 * p en (0,1). Fuera → NaN (los llamadores dan #NUM!).
 */
export function normInv(p: number): number {
  if (p <= 0 || p >= 1) return NaN;
  const x = normInvAprox(p);
  const e = normCdf(x) - p;
  const u = e * Math.sqrt(2 * Math.PI) * Math.exp((x * x) / 2);
  return x - u / (1 + (x * u) / 2);
}

function normInvAprox(p: number): number {
  const a = [
    -3.969683028665376e1, 2.209460984245205e2, -2.759285104469687e2, 1.38357751867269e2,
    -3.066479806614716e1, 2.506628277459239,
  ];
  const b = [
    -5.447609879822406e1, 1.615858368580409e2, -1.556989798598866e2, 6.680131188771972e1,
    -1.328068155288572e1,
  ];
  const c = [
    -7.784894002430293e-3, -3.223964580411365e-1, -2.400758277161838, -2.549732539343734,
    4.374664141464968, 2.938163982698783,
  ];
  const d = [7.784695709041462e-3, 3.224671290700398e-1, 2.445134137142996, 3.754408661907416];
  const plow = 0.02425;
  const phigh = 1 - plow;
  if (p < plow) {
    const q = Math.sqrt(-2 * Math.log(p));
    return (
      (((((c[0]! * q + c[1]!) * q + c[2]!) * q + c[3]!) * q + c[4]!) * q + c[5]!) /
      ((((d[0]! * q + d[1]!) * q + d[2]!) * q + d[3]!) * q + 1)
    );
  }
  if (p <= phigh) {
    const q = p - 0.5;
    const r = q * q;
    return (
      ((((((a[0]! * r + a[1]!) * r + a[2]!) * r + a[3]!) * r + a[4]!) * r + a[5]!) * q) /
      (((((b[0]! * r + b[1]!) * r + b[2]!) * r + b[3]!) * r + b[4]!) * r + 1)
    );
  }
  const q = Math.sqrt(-2 * Math.log(1 - p));
  return (
    -(((((c[0]! * q + c[1]!) * q + c[2]!) * q + c[3]!) * q + c[4]!) * q + c[5]!) /
    ((((d[0]! * q + d[1]!) * q + d[2]!) * q + d[3]!) * q + 1)
  );
}

/**
 * Reúne los números de todos los argumentos (patrón SUM): números siempre;
 * booleanos solo si el argumento es un escalar directo; texto y blancos se
 * ignoran; el primer error se propaga.
 *
 * Un texto ESCRITO como argumento («=AVERAGE("5";3)» = 4) se convierte a número, como en
 * Excel, y da #VALUE! si no lo es; lo que devuelve una función no (ver Arg.literal).
 */
function collectNumbers(args: Arg[]): number[] | FormulaError {
  const out: number[] = [];
  for (const a of args) {
    const isRange = a.ref !== undefined;
    for (const row of a.matrix()) {
      for (const v of row) {
        if (isError(v)) return v;
        if (typeof v === 'number') out.push(v);
        else if (typeof v === 'boolean' && !isRange) out.push(v ? 1 : 0);
        else if (typeof v === 'string' && a.literal) {
          const n = numeroDeLiteral(v);
          if (isError(n)) return n;
          out.push(n);
        }
      }
    }
  }
  return out;
}

/**
 * Recolección para las variantes «A»: número→su valor, booleano→1/0, texto no
 * vacío→0 (cuenta), blanco («»)→se ignora, error→se propaga. Sin distinción
 * rango/escalar (los booleanos cuentan también en rangos).
 */
function collectA(args: Arg[]): number[] | FormulaError {
  const out: number[] = [];
  for (const a of args) {
    for (const row of a.matrix()) {
      for (const v of row) {
        if (isError(v)) return v;
        if (typeof v === 'number') out.push(v);
        else if (typeof v === 'boolean') out.push(v ? 1 : 0);
        else if (v !== '') out.push(0); // texto no vacío cuenta como 0
      }
    }
  }
  return out;
}

function sum(nums: number[]): number {
  let s = 0;
  for (const v of nums) s += v;
  return s;
}

/** Suma de cuadrados de las desviaciones respecto a la media. */
function sumSqDev(nums: number[]): number {
  const m = sum(nums) / nums.length;
  let ss = 0;
  for (const v of nums) {
    const d = v - m;
    ss += d * d;
  }
  return ss;
}

/** Percentil interpolado (método inclusivo, PERCENTILE.INC): p en [0,1]. */
function percentile(nums: number[], p: number): number {
  const s = [...nums].sort((a, b) => a - b);
  const rank = p * (s.length - 1);
  const lo = Math.floor(rank);
  const frac = rank - lo;
  const base = s[lo]!;
  const next = s[lo + 1] ?? base;
  return base + frac * (next - base);
}

/* ---------- Funciones especiales Γ/B y solucionadores (clean-room) ---------- */

/** Coacciona los primeros `k` argumentos a número; #VALUE! si faltan. */
function reqNums(args: Arg[], k: number): number[] | FormulaError {
  if (args.length < k) return ERR.VALUE;
  const out: number[] = [];
  for (let i = 0; i < k; i++) {
    const n = coerceNumber(args[i]!.value());
    if (isError(n)) return n;
    out.push(n);
  }
  return out;
}

/** Lee el argumento «acumulada» (TRUE = CDF, FALSE = densidad/masa). */
function cumArg(a: Arg): boolean | FormulaError {
  return coerceBoolean(a.value());
}

/** Número finito → él mismo; ±∞/NaN → #NUM! (dominio o desbordamiento). */
function ok(v: number): CellValue {
  return Number.isFinite(v) ? v : ERR.NUM;
}

/**
 * Argumento sintético con un valor constante, para que las funciones heredadas
 * completen la firma de su equivalente moderna (el «acumulada» que la vieja da
 * por supuesto). No tiene referencia: no procede de ninguna celda.
 */
function litArg(v: CellValue): Arg {
  return { value: () => v, matrix: () => [[v]], ref: undefined };
}

/** log Γ(x): aproximación de Lanczos (g=7), con reflexión para x < 0.5. */
function gammaln(x: number): number {
  const c = [
    0.99999999999980993, 676.5203681218851, -1259.1392167224028, 771.32342877765313,
    -176.61502916214059, 12.507343278686905, -0.13857109526572012, 9.9843695780195716e-6,
    1.5056327351493116e-7,
  ];
  if (x < 0.5) return Math.log(Math.PI / Math.sin(Math.PI * x)) - gammaln(1 - x);
  x -= 1;
  let a = c[0]!;
  const t = x + 7.5;
  for (let i = 1; i < 9; i++) a += c[i]! / (x + i);
  return 0.5 * Math.log(2 * Math.PI) + (x + 0.5) * Math.log(t) - t + Math.log(a);
}

/** Γ(x) con reflexión (signo correcto en negativos); ±∞ en los polos 0, −1, −2… */
function gammafn(x: number): number {
  if (x < 0.5) return Math.PI / (Math.sin(Math.PI * x) * gammafn(1 - x));
  return Math.exp(gammaln(x));
}

/** log de la función beta: lnΓ(a) + lnΓ(b) − lnΓ(a+b). */
function lbeta(a: number, b: number): number {
  return gammaln(a) + gammaln(b) - gammaln(a + b);
}

/** P(a,x): gamma incompleta inferior regularizada (serie si x<a+1; fracción
 *  continua de Lentz si no). Base de χ², Poisson y Gamma. */
function gammap(a: number, x: number): number {
  if (x <= 0) return 0;
  if (a <= 0) return 1;
  if (x < a + 1) {
    let ap = a;
    let del = 1 / a;
    let sum = del;
    for (let n = 0; n < 300; n++) {
      ap++;
      del *= x / ap;
      sum += del;
      if (Math.abs(del) < Math.abs(sum) * 1e-15) break;
    }
    return sum * Math.exp(-x + a * Math.log(x) - gammaln(a));
  }
  const tiny = 1e-300;
  let b = x + 1 - a;
  let c = 1 / tiny;
  let d = 1 / b;
  let h = d;
  for (let i = 1; i < 300; i++) {
    const an = -i * (i - a);
    b += 2;
    d = an * d + b;
    if (Math.abs(d) < tiny) d = tiny;
    c = b + an / c;
    if (Math.abs(c) < tiny) c = tiny;
    d = 1 / d;
    const del = d * c;
    h *= del;
    if (Math.abs(del - 1) < 1e-15) break;
  }
  return 1 - Math.exp(-x + a * Math.log(x) - gammaln(a)) * h;
}

/** Fracción continua de I_x(a,b) (Lentz, Numerical Recipes betacf). */
function betacf(a: number, b: number, x: number): number {
  const tiny = 1e-300;
  const qab = a + b;
  const qap = a + 1;
  const qam = a - 1;
  let c = 1;
  let d = 1 - (qab * x) / qap;
  if (Math.abs(d) < tiny) d = tiny;
  d = 1 / d;
  let h = d;
  for (let m = 1; m < 300; m++) {
    const m2 = 2 * m;
    let aa = (m * (b - m) * x) / ((qam + m2) * (a + m2));
    d = 1 + aa * d;
    if (Math.abs(d) < tiny) d = tiny;
    c = 1 + aa / c;
    if (Math.abs(c) < tiny) c = tiny;
    d = 1 / d;
    h *= d * c;
    aa = (-(a + m) * (qab + m) * x) / ((a + m2) * (qap + m2));
    d = 1 + aa * d;
    if (Math.abs(d) < tiny) d = tiny;
    c = 1 + aa / c;
    if (Math.abs(c) < tiny) c = tiny;
    d = 1 / d;
    const del = d * c;
    h *= del;
    if (Math.abs(del - 1) < 1e-15) break;
  }
  return h;
}

/** I_x(a,b): beta incompleta regularizada. Base de t, F, Beta y Binomial. */
function betai(a: number, b: number, x: number): number {
  if (x <= 0) return 0;
  if (x >= 1) return 1;
  const bt = Math.exp(a * Math.log(x) + b * Math.log(1 - x) - lbeta(a, b));
  return x < (a + 1) / (a + b + 2)
    ? (bt * betacf(a, b, x)) / a
    : 1 - (bt * betacf(b, a, 1 - x)) / b;
}

/** Masa binomial C(n,j)·pʲ·(1−p)ⁿ⁻ʲ, robusta en p = 0/1. */
function binomPmf(j: number, n: number, p: number): number {
  if (p <= 0) return j === 0 ? 1 : 0;
  if (p >= 1) return j === n ? 1 : 0;
  return Math.exp(
    gammaln(n + 1) -
      gammaln(j + 1) -
      gammaln(n - j + 1) +
      j * Math.log(p) +
      (n - j) * Math.log(1 - p),
  );
}

/** Bisección (200 pasos) de una CDF creciente f para f(x) = p en [lo,hi]. */
function solveInc(f: (x: number) => number, p: number, lo: number, hi: number): number {
  let a = lo;
  let b = hi;
  for (let i = 0; i < 200; i++) {
    const m = (a + b) / 2;
    if (f(m) < p) a = m;
    else b = m;
  }
  return (a + b) / 2;
}

/** Inversa de una CDF creciente en [0,∞): expande el techo y biseca. */
function solvePos(f: (x: number) => number, p: number): number {
  let hi = 1;
  while (f(hi) < p && hi < 1e12) hi *= 2;
  return solveInc(f, p, 0, hi);
}

/** Inversa de una CDF creciente en ℝ: expande ambos extremos y biseca. */
function solveReal(f: (x: number) => number, p: number): number {
  let hi = 1;
  while (f(hi) < p && hi < 1e12) hi *= 2;
  let lo = -1;
  while (f(lo) > p && lo > -1e12) lo *= 2;
  return solveInc(f, p, lo, hi);
}

/* ---------- CDF/PDF derivadas de Γ y B ---------- */

function tcdf(x: number, df: number): number {
  const tail = 0.5 * betai(df / 2, 0.5, df / (df + x * x));
  return x >= 0 ? 1 - tail : tail;
}
function tpdf(x: number, df: number): number {
  return (
    (Math.exp(gammaln((df + 1) / 2) - gammaln(df / 2)) / Math.sqrt(df * Math.PI)) *
    Math.pow(1 + (x * x) / df, -(df + 1) / 2)
  );
}
function fcdf(x: number, d1: number, d2: number): number {
  return x <= 0 ? 0 : betai(d1 / 2, d2 / 2, (d1 * x) / (d1 * x + d2));
}
function fpdf(x: number, d1: number, d2: number): number {
  if (x <= 0) return 0;
  return Math.exp(
    (d1 / 2) * Math.log(d1 / d2) +
      (d1 / 2 - 1) * Math.log(x) -
      ((d1 + d2) / 2) * Math.log(1 + (d1 * x) / d2) -
      lbeta(d1 / 2, d2 / 2),
  );
}
function chisqpdf(x: number, df: number): number {
  if (x < 0) return 0;
  if (x === 0) return df < 2 ? Infinity : df === 2 ? 0.5 : 0;
  return Math.exp((df / 2 - 1) * Math.log(x) - x / 2 - (df / 2) * Math.LN2 - gammaln(df / 2));
}

/** Recoge pares (y,x) alineados por posición; longitudes distintas → #N/A;
 *  descarta el par si alguna celda no es número; propaga el primer #error. */
function collectPairs(aY: Arg, aX: Arg): { ys: number[]; xs: number[] } | FormulaError {
  const fy: CellValue[] = [];
  const fx: CellValue[] = [];
  for (const row of aY.matrix()) for (const v of row) fy.push(v);
  for (const row of aX.matrix()) for (const v of row) fx.push(v);
  if (fy.length !== fx.length) return ERR.NA;
  const ys: number[] = [];
  const xs: number[] = [];
  for (let i = 0; i < fy.length; i++) {
    const yv = fy[i]!;
    const xv = fx[i]!;
    if (isError(yv)) return yv;
    if (isError(xv)) return xv;
    if (typeof yv === 'number' && typeof xv === 'number') {
      ys.push(yv);
      xs.push(xv);
    }
  }
  return { ys, xs };
}

/** Números de un argumento (blancos ignorados, texto no numérico → #error). */
function flatNums(a: Arg): number[] | FormulaError {
  const out: number[] = [];
  for (const row of a.matrix())
    for (const v of row) {
      if (v === '') continue;
      const nn = coerceNumber(v);
      if (isError(nn)) return nn;
      out.push(nn);
    }
  return out;
}

/** Momentos de una regresión lineal simple y = a + b·x. */
function regress(
  ys: number[],
  xs: number[],
): {
  n: number;
  mx: number;
  my: number;
  sxx: number;
  syy: number;
  sxy: number;
} {
  const n = xs.length;
  let sx = 0;
  let sy = 0;
  for (let i = 0; i < n; i++) {
    sx += xs[i]!;
    sy += ys[i]!;
  }
  const mx = sx / n;
  const my = sy / n;
  let sxx = 0;
  let syy = 0;
  let sxy = 0;
  for (let i = 0; i < n; i++) {
    const dx = xs[i]! - mx;
    const dy = ys[i]! - my;
    sxx += dx * dx;
    syy += dy * dy;
    sxy += dx * dy;
  }
  return { n, mx, my, sxx, syy, sxy };
}

/** Percentil por el método EXCLUSIVO (PERCENTILE.EXC): p en (0,1) con
 *  1/(n+1) ≤ p ≤ n/(n+1); fuera de rango → #NUM!. */
function percentileExc(nums: number[], p: number): number | FormulaError {
  const s = [...nums].sort((a, b) => a - b);
  const n = s.length;
  const rank = p * (n + 1); // posición 1-based
  if (rank < 1 || rank > n) return ERR.NUM;
  const lo = Math.floor(rank);
  const frac = rank - lo;
  const base = s[lo - 1]!;
  const next = s[lo] ?? base;
  return base + frac * (next - base);
}

/** Trunca `x` a `sig` cifras significativas (hacia cero), como el argumento
 *  `significancia` de PERCENTRANK. */
function truncSig(x: number, sig: number): number {
  if (x === 0) return 0;
  const m = Math.pow(10, sig - 1 - Math.floor(Math.log10(Math.abs(x))));
  return Math.trunc(x * m) / m;
}

/** Rango porcentual de `x` en `nums` (inclusivo o exclusivo). Los duplicados
 *  usan la posición de su PRIMERA aparición (como Excel). Fuera de rango → #N/A. */
function percentRank(
  nums: number[],
  x: number,
  sig: number,
  exclusive: boolean,
): number | FormulaError {
  const s = [...nums].sort((a, b) => a - b);
  const n = s.length;
  if (n === 0 || x < s[0]! || x > s[n - 1]!) return ERR.NA;
  let lo = 0;
  while (lo < n && s[lo]! < x) lo++; // nº de valores estrictamente menores que x
  let pos: number;
  if (lo < n && s[lo] === x) pos = lo;
  else {
    const a = s[lo - 1]!;
    const b = s[lo]!;
    pos = lo - 1 + (x - a) / (b - a);
  }
  const pr = exclusive ? (pos + 1) / (n + 1) : n === 1 ? 0 : pos / (n - 1);
  return truncSig(pr, sig);
}

/** Núcleo de PERCENTRANK(.INC/.EXC): array, x, [significancia=3]. */
function percentRankFn(args: Arg[], exclusive: boolean): CellValue {
  if (args.length < 2 || args.length > 3) return ERR.VALUE;
  const nums = collectNumbers([args[0]!]);
  if (nums instanceof FormulaError) return nums;
  const x = coerceNumber(args[1]!.value());
  if (isError(x)) return x;
  let sig = 3;
  if (args.length === 3) {
    const sv = coerceNumber(args[2]!.value());
    if (isError(sv)) return sv;
    sig = Math.trunc(sv);
    if (sig < 1) return ERR.NUM;
  }
  if (nums.length === 0) return ERR.NA;
  return percentRank(nums, x, sig, exclusive);
}

/**
 * Recoge (ys, xs) de los dos primeros argumentos. Si no hay xs, se usa 1,2,3…, que es
 * lo que hace una hoja cuando solo se le dan los valores conocidos.
 */
function pareja(args: Arg[]): { ys: number[]; xs: number[] } | FormulaError {
  const ys = flatNums(args[0]!);
  if (ys instanceof FormulaError) return ys;
  if (args.length < 2 || args[1] === undefined) {
    return { ys, xs: ys.map((_, i) => i + 1) };
  }
  // Sin `!`: el guardia de arriba ya estrechó el tipo, y la aserción sobraba.
  const xs = flatNums(args[1]);
  if (xs instanceof FormulaError) return xs;
  if (xs.length === 0) return { ys, xs: ys.map((_, i) => i + 1) };
  if (xs.length !== ys.length) return ERR.REF;
  return { ys, xs };
}

/** Lee un argumento booleano opcional; ausente → `porDefecto`. */
function optBool(args: Arg[], i: number, porDefecto: boolean): boolean | FormulaError {
  const a = args[i];
  if (a === undefined) return porDefecto;
  const v = a.value();
  if (v === '') return porDefecto;
  return coerceBoolean(v);
}

/**
 * Recta de mínimos cuadrados y = m·x + b.
 *
 * Con `conConstante` en falso la recta pasa por el origen y las sumas NO se centran en
 * la media: m = Σxy/Σx². Usar las sumas centradas y luego poner b a cero daría una
 * pendiente distinta —y equivocada—, que es el atajo que se cuela al implementarlo.
 */
function rectaMinimos(
  ys: number[],
  xs: number[],
  conConstante: boolean,
): { m: number; b: number } | undefined {
  if (!conConstante) {
    let sxy = 0;
    let sxx = 0;
    for (let i = 0; i < xs.length; i += 1) {
      sxy += xs[i]! * ys[i]!;
      sxx += xs[i]! * xs[i]!;
    }
    if (sxx === 0) return undefined;
    return { m: sxy / sxx, b: 0 };
  }
  const r = regress(ys, xs);
  if (r.sxx === 0) return undefined;
  const m = r.sxy / r.sxx;
  return { m, b: r.my - m * r.mx };
}

export const funcs: FnTable = {
  AVERAGE(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length === 0) return ERR.DIV0;
    return sum(nums) / nums.length;
  },

  AVERAGEA(_ctx, args) {
    const nums = collectA(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length === 0) return ERR.DIV0;
    return sum(nums) / nums.length;
  },

  // Cuenta números; en rangos solo números; los booleanos y el texto numérico escritos como
  // argumento cuentan. Ignora el resto del texto, los blancos y los errores (no los propaga).
  COUNT(_ctx, args) {
    let n = 0;
    for (const a of args) {
      const isRange = a.ref !== undefined;
      for (const row of a.matrix()) {
        for (const v of row) {
          if (typeof v === 'number') n++;
          else if (typeof v === 'boolean' && !isRange) n++;
          // Un texto numérico escrito como argumento cuenta (COUNT("1";1) = 2 en Excel).
          else if (typeof v === 'string' && a.literal && !isError(numeroDeLiteral(v))) n++;
        }
      }
    }
    return n;
  },

  // Cuenta valores no vacíos (números, texto, booleanos y errores).
  COUNTA(_ctx, args) {
    let n = 0;
    for (const a of args) {
      for (const row of a.matrix()) {
        for (const v of row) {
          if (v !== '') n++; // «» ≈ celda en blanco
        }
      }
    }
    return n;
  },

  // Cuenta celdas en blanco (≈ «»). Los ceros no son blancos.
  COUNTBLANK(_ctx, args) {
    let n = 0;
    for (const a of args) {
      for (const row of a.matrix()) {
        for (const v of row) {
          if (v === '') n++;
        }
      }
    }
    return n;
  },

  MAX(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length === 0) return 0;
    let m = nums[0]!;
    for (const v of nums) if (v > m) m = v;
    return m;
  },

  MAXA(_ctx, args) {
    const nums = collectA(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length === 0) return 0;
    let m = nums[0]!;
    for (const v of nums) if (v > m) m = v;
    return m;
  },

  MIN(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length === 0) return 0;
    let m = nums[0]!;
    for (const v of nums) if (v < m) m = v;
    return m;
  },

  MINA(_ctx, args) {
    const nums = collectA(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length === 0) return 0;
    let m = nums[0]!;
    for (const v of nums) if (v < m) m = v;
    return m;
  },

  MEDIAN(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    const n = nums.length;
    if (n === 0) return ERR.NUM;
    const s = [...nums].sort((a, b) => a - b);
    const mid = n >> 1;
    return n % 2 === 1 ? s[mid]! : (s[mid - 1]! + s[mid]!) / 2;
  },

  // Valor más frecuente; el primero en aparecer si hay empate; #N/A si ninguno
  // se repite.
  MODE(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    const counts = new Map<number, number>(); // preserva orden de inserción
    for (const v of nums) counts.set(v, (counts.get(v) ?? 0) + 1);
    let best = 0;
    let bestCount = 0;
    for (const [val, c] of counts) {
      if (c > bestCount) {
        bestCount = c;
        best = val;
      }
    }
    return bestCount < 2 ? ERR.NA : best;
  },

  STDEV(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 2) return ERR.DIV0;
    return Math.sqrt(sumSqDev(nums) / (nums.length - 1));
  },

  STDEVP(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 1) return ERR.DIV0;
    return Math.sqrt(sumSqDev(nums) / nums.length);
  },

  VAR(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 2) return ERR.DIV0;
    return sumSqDev(nums) / (nums.length - 1);
  },

  VARP(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 1) return ERR.DIV0;
    return sumSqDev(nums) / nums.length;
  },

  // LARGE(array, k): k-ésimo mayor. k se trunca a entero; fuera de [1,n] → #NUM!.
  LARGE(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const nums = collectNumbers([args[0]!]);
    if (nums instanceof FormulaError) return nums;
    const k = coerceNumber(args[1]!.value());
    if (isError(k)) return k;
    const n = nums.length;
    const r = Math.trunc(k);
    if (r < 1 || r > n) return ERR.NUM;
    const s = [...nums].sort((a, b) => b - a);
    return s[r - 1]!;
  },

  // SMALL(array, k): k-ésimo menor.
  SMALL(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const nums = collectNumbers([args[0]!]);
    if (nums instanceof FormulaError) return nums;
    const k = coerceNumber(args[1]!.value());
    if (isError(k)) return k;
    const n = nums.length;
    const r = Math.trunc(k);
    if (r < 1 || r > n) return ERR.NUM;
    const s = [...nums].sort((a, b) => a - b);
    return s[r - 1]!;
  },

  // RANK(valor, ref, [orden]): orden 0/omitido → descendente; ≠0 → ascendente.
  // Duplicados comparten rango; #N/A si el valor no está.
  RANK(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    const nums = collectNumbers([args[1]!]);
    if (nums instanceof FormulaError) return nums;
    let asc = false;
    if (args.length >= 3) {
      const o = coerceNumber(args[2]!.value());
      if (isError(o)) return o;
      asc = o !== 0;
    }
    if (!nums.includes(x)) return ERR.NA;
    let ahead = 0;
    for (const v of nums) if (asc ? v < x : v > x) ahead++;
    return ahead + 1;
  },

  // PERCENTILE(array, p): p en [0,1], interpolación inclusiva.
  PERCENTILE(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const nums = collectNumbers([args[0]!]);
    if (nums instanceof FormulaError) return nums;
    const p = coerceNumber(args[1]!.value());
    if (isError(p)) return p;
    if (nums.length === 0 || p < 0 || p > 1) return ERR.NUM;
    return percentile(nums, p);
  },

  // QUARTILE(array, quart): quart 0..4 → percentiles 0/0.25/0.5/0.75/1.
  QUARTILE(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const nums = collectNumbers([args[0]!]);
    if (nums instanceof FormulaError) return nums;
    const q = coerceNumber(args[1]!.value());
    if (isError(q)) return q;
    const qi = Math.trunc(q);
    if (nums.length === 0 || qi < 0 || qi > 4) return ERR.NUM;
    return percentile(nums, qi / 4);
  },

  // GEOMEAN: media geométrica. Todos los valores deben ser > 0, si no → #NUM!.
  GEOMEAN(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length === 0) return ERR.NUM;
    let prod = 1;
    for (const v of nums) {
      if (v <= 0) return ERR.NUM;
      prod *= v; // ponytail: producto directo; para n enorme, exp(Σln)/n evita overflow
    }
    return Math.pow(prod, 1 / nums.length);
  },

  // HARMEAN: media armónica. Todos los valores deben ser > 0, si no → #NUM!.
  HARMEAN(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length === 0) return ERR.NUM;
    let acc = 0;
    for (const v of nums) {
      if (v <= 0) return ERR.NUM;
      acc += 1 / v;
    }
    return nums.length / acc;
  },

  // Variantes modernas (idénticas a STDEV/STDEVP/VAR/VARP): «.S» muestral
  // (divide por n-1), «.P» poblacional (divide por n).
  'STDEV.S'(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 2) return ERR.DIV0;
    return Math.sqrt(sumSqDev(nums) / (nums.length - 1));
  },
  'STDEV.P'(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 1) return ERR.DIV0;
    return Math.sqrt(sumSqDev(nums) / nums.length);
  },
  'VAR.S'(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 2) return ERR.DIV0;
    return sumSqDev(nums) / (nums.length - 1);
  },
  'VAR.P'(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 1) return ERR.DIV0;
    return sumSqDev(nums) / nums.length;
  },

  // NORM.DIST(x, media, desv, acumulada): normal general. desv > 0. acumulada
  // TRUE → CDF; FALSE → densidad.
  'NORM.DIST'(_ctx, args) {
    if (args.length !== 4) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    const mean = coerceNumber(args[1]!.value());
    if (isError(mean)) return mean;
    const sd = coerceNumber(args[2]!.value());
    if (isError(sd)) return sd;
    if (sd <= 0) return ERR.NUM;
    const cum = coerceBoolean(args[3]!.value());
    if (isError(cum)) return cum;
    const z = (x - mean) / sd;
    return cum ? normCdf(z) : normPdf(z) / sd;
  },

  // NORM.S.DIST(z, acumulada): normal estándar.
  'NORM.S.DIST'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const z = coerceNumber(args[0]!.value());
    if (isError(z)) return z;
    const cum = coerceBoolean(args[1]!.value());
    if (isError(cum)) return cum;
    return cum ? normCdf(z) : normPdf(z);
  },

  // NORM.INV(prob, media, desv): inversa de la normal general. prob en (0,1).
  'NORM.INV'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const p = coerceNumber(args[0]!.value());
    if (isError(p)) return p;
    const mean = coerceNumber(args[1]!.value());
    if (isError(mean)) return mean;
    const sd = coerceNumber(args[2]!.value());
    if (isError(sd)) return sd;
    if (sd <= 0 || p <= 0 || p >= 1) return ERR.NUM;
    return mean + sd * normInv(p);
  },

  // NORM.S.INV(prob): inversa de la normal estándar. prob en (0,1).
  'NORM.S.INV'(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const p = coerceNumber(args[0]!.value());
    if (isError(p)) return p;
    if (p <= 0 || p >= 1) return ERR.NUM;
    return normInv(p);
  },

  /* ================= Funciones Γ ================= */

  // GAMMALN(x): logaritmo natural de Γ(x). x > 0.
  GAMMALN(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const a = reqNums(args, 1);
    if (a instanceof FormulaError) return a;
    if (a[0]! <= 0) return ERR.NUM;
    return ok(gammaln(a[0]!));
  },
  // GAMMALN.PRECISE: idéntica a GAMMALN en este motor (misma aproximación).
  'GAMMALN.PRECISE'(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const a = reqNums(args, 1);
    if (a instanceof FormulaError) return a;
    if (a[0]! <= 0) return ERR.NUM;
    return ok(gammaln(a[0]!));
  },
  // GAMMA(x): función gamma Γ(x). Polos (#NUM!) en 0 y enteros negativos.
  GAMMA(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const a = reqNums(args, 1);
    if (a instanceof FormulaError) return a;
    const x = a[0]!;
    if (x <= 0 && Number.isInteger(x)) return ERR.NUM;
    return ok(gammafn(x));
  },
  // GAMMA.DIST(x, alfa, beta, acumulada): distribución gamma. x≥0, alfa,beta>0.
  'GAMMA.DIST'(_ctx, args) {
    if (args.length !== 4) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const cum = cumArg(args[3]!);
    if (isError(cum)) return cum;
    const [x, al, be] = a as [number, number, number];
    if (x < 0 || al <= 0 || be <= 0) return ERR.NUM;
    if (cum) return ok(gammap(al, x / be));
    return ok(Math.exp((al - 1) * Math.log(x) - x / be - al * Math.log(be) - gammaln(al)));
  },
  // GAMMA.INV(prob, alfa, beta): inversa de la CDF gamma. prob en [0,1).
  'GAMMA.INV'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const [p, al, be] = a as [number, number, number];
    if (p < 0 || p >= 1 || al <= 0 || be <= 0) return ERR.NUM;
    return ok(be * solvePos((u) => gammap(al, u), p));
  },

  /* ================= Discretas y exponencial ================= */

  // POISSON.DIST(x, media, acumulada): x se trunca; x≥0, media≥0.
  'POISSON.DIST'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const cum = cumArg(args[2]!);
    if (isError(cum)) return cum;
    const k = Math.trunc(a[0]!);
    const mean = a[1]!;
    if (k < 0 || mean < 0) return ERR.NUM;
    if (cum) return ok(1 - gammap(k + 1, mean));
    if (mean === 0) return k === 0 ? 1 : 0;
    return ok(Math.exp(-mean + k * Math.log(mean) - gammaln(k + 1)));
  },
  // EXPON.DIST(x, lambda, acumulada): exponencial. x≥0, lambda>0.
  'EXPON.DIST'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const cum = cumArg(args[2]!);
    if (isError(cum)) return cum;
    const x = a[0]!;
    const lam = a[1]!;
    if (x < 0 || lam <= 0) return ERR.NUM;
    return ok(cum ? 1 - Math.exp(-lam * x) : lam * Math.exp(-lam * x));
  },
  // BINOM.DIST(éxitos, ensayos, prob, acumulada). éxitos,ensayos truncados.
  'BINOM.DIST'(_ctx, args) {
    if (args.length !== 4) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const cum = cumArg(args[3]!);
    if (isError(cum)) return cum;
    const k = Math.trunc(a[0]!);
    const n = Math.trunc(a[1]!);
    const p = a[2]!;
    if (k < 0 || k > n || p < 0 || p > 1) return ERR.NUM;
    if (!cum) return ok(binomPmf(k, n, p));
    let s = 0;
    for (let j = 0; j <= k; j++) s += binomPmf(j, n, p);
    return ok(s);
  },
  // BINOM.DIST.RANGE(ensayos, prob, éxitos, [éxitos2]): probabilidad de entre `éxitos` y
  // `éxitos2` éxitos (ambos incluidos); sin `éxitos2`, la de exactamente `éxitos`.
  'BINOM.DIST.RANGE'(_ctx, args) {
    if (args.length < 3 || args.length > 4) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    let s2 = a[2]!;
    if (args.length === 4) {
      const v = coerceNumber(args[3]!.value());
      if (isError(v)) return v;
      s2 = v;
    }
    const n = Math.trunc(a[0]!);
    const p = a[1]!;
    const s1 = Math.trunc(a[2]!);
    s2 = Math.trunc(s2);
    if (n < 0 || p < 0 || p > 1 || s1 < 0 || s1 > n || s2 < s1 || s2 > n) return ERR.NUM;
    let t = 0;
    for (let j = s1; j <= s2; j++) t += binomPmf(j, n, p);
    return ok(t);
  },
  // BINOMDIST: alias heredado de BINOM.DIST.
  BINOMDIST(ctx, args) {
    return funcs['BINOM.DIST']!(ctx, args);
  },
  // WEIBULL.DIST(x, alfa, beta, acumulada). x≥0, alfa,beta>0.
  'WEIBULL.DIST'(_ctx, args) {
    if (args.length !== 4) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const cum = cumArg(args[3]!);
    if (isError(cum)) return cum;
    const [x, al, be] = a as [number, number, number];
    if (x < 0 || al <= 0 || be <= 0) return ERR.NUM;
    const z = Math.pow(x / be, al);
    return ok(
      cum ? 1 - Math.exp(-z) : (al / Math.pow(be, al)) * Math.pow(x, al - 1) * Math.exp(-z),
    );
  },
  // WEIBULL: alias heredado.
  WEIBULL(ctx, args) {
    return funcs['WEIBULL.DIST']!(ctx, args);
  },

  /* ================= t de Student ================= */

  // T.DIST(x, gl, acumulada): CDF (izquierda) o densidad. gl≥1.
  'T.DIST'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const cum = cumArg(args[2]!);
    if (isError(cum)) return cum;
    const x = a[0]!;
    const df = Math.trunc(a[1]!);
    if (df < 1) return ERR.NUM;
    return ok(cum ? tcdf(x, df) : tpdf(x, df));
  },
  // T.DIST.2T(x, gl): probabilidad de dos colas. x ≥ 0.
  'T.DIST.2T'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const x = a[0]!;
    const df = Math.trunc(a[1]!);
    if (df < 1 || x < 0) return ERR.NUM;
    return ok(betai(df / 2, 0.5, df / (df + x * x)));
  },
  // T.DIST.RT(x, gl): cola derecha 1 − CDF.
  'T.DIST.RT'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const df = Math.trunc(a[1]!);
    if (df < 1) return ERR.NUM;
    return ok(1 - tcdf(a[0]!, df));
  },
  // T.INV(prob, gl): inversa de la CDF izquierda. prob en (0,1).
  'T.INV'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const p = a[0]!;
    const df = Math.trunc(a[1]!);
    if (df < 1 || p <= 0 || p >= 1) return ERR.NUM;
    return ok(solveReal((x) => tcdf(x, df), p));
  },
  // T.INV.2T(prob, gl): inversa de dos colas. prob en (0,1].
  'T.INV.2T'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const p = a[0]!;
    const df = Math.trunc(a[1]!);
    if (df < 1 || p <= 0 || p > 1) return ERR.NUM;
    return ok(solveReal((x) => tcdf(x, df), 1 - p / 2));
  },

  /* ================= χ² ================= */

  // CHISQ.DIST(x, gl, acumulada): CDF o densidad. x≥0, gl≥1.
  'CHISQ.DIST'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const cum = cumArg(args[2]!);
    if (isError(cum)) return cum;
    const x = a[0]!;
    const df = Math.trunc(a[1]!);
    if (x < 0 || df < 1) return ERR.NUM;
    return ok(cum ? gammap(df / 2, x / 2) : chisqpdf(x, df));
  },
  // CHISQ.DIST.RT(x, gl): cola derecha (la CHIDIST clásica). x≥0, gl≥1.
  'CHISQ.DIST.RT'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const x = a[0]!;
    const df = Math.trunc(a[1]!);
    if (x < 0 || df < 1) return ERR.NUM;
    return ok(1 - gammap(df / 2, x / 2));
  },
  // CHISQ.INV(prob, gl): inversa de la CDF izquierda. prob en [0,1).
  'CHISQ.INV'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const p = a[0]!;
    const df = Math.trunc(a[1]!);
    if (p < 0 || p >= 1 || df < 1) return ERR.NUM;
    return ok(solvePos((x) => gammap(df / 2, x / 2), p));
  },
  // CHISQ.INV.RT(prob, gl): inversa de la cola derecha. prob en (0,1].
  'CHISQ.INV.RT'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = reqNums(args, 2);
    if (a instanceof FormulaError) return a;
    const p = a[0]!;
    const df = Math.trunc(a[1]!);
    if (p <= 0 || p > 1 || df < 1) return ERR.NUM;
    return ok(solvePos((x) => gammap(df / 2, x / 2), 1 - p));
  },

  /* ================= F de Snedecor ================= */

  // F.DIST(x, gl1, gl2, acumulada): CDF o densidad. x≥0, gl≥1.
  'F.DIST'(_ctx, args) {
    if (args.length !== 4) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const cum = cumArg(args[3]!);
    if (isError(cum)) return cum;
    const x = a[0]!;
    const d1 = Math.trunc(a[1]!);
    const d2 = Math.trunc(a[2]!);
    if (x < 0 || d1 < 1 || d2 < 1) return ERR.NUM;
    return ok(cum ? fcdf(x, d1, d2) : fpdf(x, d1, d2));
  },
  // F.DIST.RT(x, gl1, gl2): cola derecha (la FDIST clásica).
  'F.DIST.RT'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const x = a[0]!;
    const d1 = Math.trunc(a[1]!);
    const d2 = Math.trunc(a[2]!);
    if (x < 0 || d1 < 1 || d2 < 1) return ERR.NUM;
    return ok(1 - fcdf(x, d1, d2));
  },
  // F.INV(prob, gl1, gl2): inversa de la CDF izquierda. prob en [0,1).
  'F.INV'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const p = a[0]!;
    const d1 = Math.trunc(a[1]!);
    const d2 = Math.trunc(a[2]!);
    if (p < 0 || p >= 1 || d1 < 1 || d2 < 1) return ERR.NUM;
    return ok(solvePos((x) => fcdf(x, d1, d2), p));
  },
  // F.INV.RT(prob, gl1, gl2): inversa de la cola derecha. prob en (0,1].
  'F.INV.RT'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const p = a[0]!;
    const d1 = Math.trunc(a[1]!);
    const d2 = Math.trunc(a[2]!);
    if (p <= 0 || p > 1 || d1 < 1 || d2 < 1) return ERR.NUM;
    return ok(solvePos((x) => fcdf(x, d1, d2), 1 - p));
  },

  /* ================= Beta ================= */

  // BETA.DIST(x, alfa, beta, acumulada, [A=0], [B=1]): distribución beta en [A,B].
  'BETA.DIST'(_ctx, args) {
    if (args.length < 4 || args.length > 6) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    const al = coerceNumber(args[1]!.value());
    if (isError(al)) return al;
    const be = coerceNumber(args[2]!.value());
    if (isError(be)) return be;
    const cum = cumArg(args[3]!);
    if (isError(cum)) return cum;
    const A = args.length >= 5 ? coerceNumber(args[4]!.value()) : 0;
    if (isError(A)) return A;
    const B = args.length >= 6 ? coerceNumber(args[5]!.value()) : 1;
    if (isError(B)) return B;
    if (al <= 0 || be <= 0 || B <= A) return ERR.NUM;
    const z = (x - A) / (B - A);
    if (z < 0 || z > 1) return ERR.NUM;
    if (cum) return ok(betai(al, be, z));
    return ok(
      Math.exp((al - 1) * Math.log(z) + (be - 1) * Math.log(1 - z) - lbeta(al, be)) / (B - A),
    );
  },
  // BETA.INV(prob, alfa, beta, [A=0], [B=1]): inversa de la CDF beta. prob en (0,1).
  'BETA.INV'(_ctx, args) {
    if (args.length < 3 || args.length > 5) return ERR.VALUE;
    const p = coerceNumber(args[0]!.value());
    if (isError(p)) return p;
    const al = coerceNumber(args[1]!.value());
    if (isError(al)) return al;
    const be = coerceNumber(args[2]!.value());
    if (isError(be)) return be;
    const A = args.length >= 4 ? coerceNumber(args[3]!.value()) : 0;
    if (isError(A)) return A;
    const B = args.length >= 5 ? coerceNumber(args[4]!.value()) : 1;
    if (isError(B)) return B;
    if (p <= 0 || p >= 1 || al <= 0 || be <= 0 || B <= A) return ERR.NUM;
    const z = solveInc((t) => betai(al, be, t), p, 0, 1);
    return ok(A + z * (B - A));
  },

  /* ================= Intervalos de confianza ================= */

  // CONFIDENCE.NORM(alfa, desv, tamaño): semiancho normal del IC de la media.
  'CONFIDENCE.NORM'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const [alpha, sd, size] = a as [number, number, number];
    const n = Math.trunc(size);
    if (alpha <= 0 || alpha >= 1 || sd <= 0 || n < 1) return ERR.NUM;
    return ok(normInv(1 - alpha / 2) * (sd / Math.sqrt(n)));
  },
  // CONFIDENCE: alias heredado de CONFIDENCE.NORM.
  CONFIDENCE(ctx, args) {
    return funcs['CONFIDENCE.NORM']!(ctx, args);
  },
  // CONFIDENCE.T(alfa, desv, tamaño): semiancho con la t de Student (n−1 gl).
  'CONFIDENCE.T'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const a = reqNums(args, 3);
    if (a instanceof FormulaError) return a;
    const [alpha, sd, size] = a as [number, number, number];
    const n = Math.trunc(size);
    if (alpha <= 0 || alpha >= 1 || sd <= 0 || n < 2) return ERR.NUM;
    const t = solveReal((x) => tcdf(x, n - 1), 1 - alpha / 2);
    return ok(t * (sd / Math.sqrt(n)));
  },

  /* ================= Agregadas descriptivas y regresión ================= */

  // DEVSQ: suma de cuadrados de las desviaciones respecto a la media.
  DEVSQ(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    return nums.length === 0 ? 0 : sumSqDev(nums);
  },
  // AVEDEV: desviación media absoluta respecto a la media.
  AVEDEV(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    const n = nums.length;
    if (n === 0) return ERR.NUM;
    const m = sum(nums) / n;
    let s = 0;
    for (const v of nums) s += Math.abs(v - m);
    return s / n;
  },
  // SKEW: asimetría muestral (n≥3).
  SKEW(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    const n = nums.length;
    if (n < 3) return ERR.DIV0;
    const m = sum(nums) / n;
    const sd = Math.sqrt(sumSqDev(nums) / (n - 1));
    if (sd === 0) return ERR.DIV0;
    let s = 0;
    for (const v of nums) s += ((v - m) / sd) ** 3;
    return (n / ((n - 1) * (n - 2))) * s;
  },
  // SKEW.P: asimetría poblacional.
  'SKEW.P'(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    const n = nums.length;
    if (n < 1) return ERR.DIV0;
    const m = sum(nums) / n;
    const sd = Math.sqrt(sumSqDev(nums) / n);
    if (sd === 0) return ERR.DIV0;
    let s = 0;
    for (const v of nums) s += ((v - m) / sd) ** 3;
    return s / n;
  },
  // KURT: curtosis muestral en exceso (n≥4).
  KURT(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    const n = nums.length;
    if (n < 4) return ERR.DIV0;
    const m = sum(nums) / n;
    const sd = Math.sqrt(sumSqDev(nums) / (n - 1));
    if (sd === 0) return ERR.DIV0;
    let s = 0;
    for (const v of nums) s += ((v - m) / sd) ** 4;
    return (
      (n * (n + 1) * s) / ((n - 1) * (n - 2) * (n - 3)) - (3 * (n - 1) ** 2) / ((n - 2) * (n - 3))
    );
  },
  // CORREL(array1, array2): coeficiente de correlación de Pearson.
  CORREL(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = collectPairs(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    if (p.xs.length < 1) return ERR.DIV0;
    const r = regress(p.ys, p.xs);
    const d = Math.sqrt(r.sxx * r.syy);
    return d === 0 ? ERR.DIV0 : r.sxy / d;
  },
  // PEARSON: alias de CORREL.
  PEARSON(ctx, args) {
    return funcs['CORREL']!(ctx, args);
  },
  // COVARIANCE.P(array1, array2): covarianza poblacional.
  'COVARIANCE.P'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = collectPairs(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    if (p.xs.length < 1) return ERR.DIV0;
    return regress(p.ys, p.xs).sxy / p.xs.length;
  },
  // COVAR: alias heredado de COVARIANCE.P.
  COVAR(ctx, args) {
    return funcs['COVARIANCE.P']!(ctx, args);
  },
  // COVARIANCE.S(array1, array2): covarianza muestral (n−1).
  'COVARIANCE.S'(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = collectPairs(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    if (p.xs.length < 2) return ERR.DIV0;
    return regress(p.ys, p.xs).sxy / (p.xs.length - 1);
  },
  // SLOPE(known_y, known_x): pendiente de la recta de mínimos cuadrados.
  SLOPE(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = collectPairs(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    if (p.xs.length < 1) return ERR.DIV0;
    const r = regress(p.ys, p.xs);
    return r.sxx === 0 ? ERR.DIV0 : r.sxy / r.sxx;
  },
  // INTERCEPT(known_y, known_x): ordenada en el origen de la recta.
  INTERCEPT(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = collectPairs(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    if (p.xs.length < 1) return ERR.DIV0;
    const r = regress(p.ys, p.xs);
    if (r.sxx === 0) return ERR.DIV0;
    return r.my - (r.sxy / r.sxx) * r.mx;
  },
  // RSQ(known_y, known_x): coeficiente de determinación r².
  RSQ(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = collectPairs(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    if (p.xs.length < 1) return ERR.DIV0;
    const r = regress(p.ys, p.xs);
    const d = r.sxx * r.syy;
    if (d === 0) return ERR.DIV0;
    const corr = r.sxy / Math.sqrt(d);
    return corr * corr;
  },
  // FORECAST.LINEAR(x, known_y, known_x): predicción lineal en x.
  'FORECAST.LINEAR'(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const xv = coerceNumber(args[0]!.value());
    if (isError(xv)) return xv;
    const p = collectPairs(args[1]!, args[2]!);
    if (p instanceof FormulaError) return p;
    if (p.xs.length < 1) return ERR.DIV0;
    const r = regress(p.ys, p.xs);
    if (r.sxx === 0) return ERR.DIV0;
    const slope = r.sxy / r.sxx;
    return ok(r.my - slope * r.mx + slope * xv);
  },
  // FORECAST: alias heredado de FORECAST.LINEAR.
  FORECAST(ctx, args) {
    return funcs['FORECAST.LINEAR']!(ctx, args);
  },
  // TREND(known_y, [known_x], [new_x], [const=TRUE]): valores ajustados por
  // mínimos cuadrados (una variable). Derrama sobre la forma de new_x.
  TREND(_ctx, args) {
    if (args.length < 1 || args.length > 4) return ERR.VALUE;
    const ys = flatNums(args[0]!);
    if (ys instanceof FormulaError) return ys;
    const n = ys.length;
    if (n === 0) return ERR.VALUE;
    let xs: number[];
    if (args.length >= 2) {
      const xv = flatNums(args[1]!);
      if (xv instanceof FormulaError) return xv;
      xs = xv;
    } else xs = ys.map((_, i) => i + 1);
    if (xs.length !== n) return ERR.REF;
    let useConst = true;
    if (args.length >= 4) {
      const b = coerceBoolean(args[3]!.value());
      if (isError(b)) return b;
      useConst = b;
    }
    let slope: number;
    let intercept: number;
    if (useConst) {
      const r = regress(ys, xs);
      if (r.sxx === 0) return ERR.DIV0;
      slope = r.sxy / r.sxx;
      intercept = r.my - slope * r.mx;
    } else {
      let sxx = 0;
      let sxy = 0;
      for (let i = 0; i < n; i++) {
        sxx += xs[i]! * xs[i]!;
        sxy += xs[i]! * ys[i]!;
      }
      if (sxx === 0) return ERR.DIV0;
      slope = sxy / sxx;
      intercept = 0;
    }
    const predict = (x: number): CellValue => ok(intercept + slope * x);
    if (args.length >= 3) {
      return args[2]!.matrix().map((row) =>
        row.map((v) => {
          const xn = coerceNumber(v);
          return isError(xn) ? xn : predict(xn);
        }),
      );
    }
    return xs.map((x) => [predict(x)]);
  },

  /* ================= Variantes modernas de percentil / rango ================= */

  // PERCENTILE.INC / QUARTILE.INC: idénticas a las clásicas (método inclusivo).
  'PERCENTILE.INC'(ctx, args) {
    return funcs['PERCENTILE']!(ctx, args);
  },
  'QUARTILE.INC'(ctx, args) {
    return funcs['QUARTILE']!(ctx, args);
  },
  // PERCENTILE.EXC(array, p): percentil por el método exclusivo.
  'PERCENTILE.EXC'(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const nums = collectNumbers([args[0]!]);
    if (nums instanceof FormulaError) return nums;
    const p = coerceNumber(args[1]!.value());
    if (isError(p)) return p;
    if (nums.length === 0) return ERR.NUM;
    return percentileExc(nums, p);
  },
  // QUARTILE.EXC(array, cuartil): cuartil 1..3 por el método exclusivo (0 y 4 → #NUM!).
  'QUARTILE.EXC'(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const nums = collectNumbers([args[0]!]);
    if (nums instanceof FormulaError) return nums;
    const q = coerceNumber(args[1]!.value());
    if (isError(q)) return q;
    const qi = Math.trunc(q);
    if (nums.length === 0 || qi < 1 || qi > 3) return ERR.NUM;
    return percentileExc(nums, qi / 4);
  },

  // RANK.EQ: idéntica a RANK (los empates comparten el rango superior).
  'RANK.EQ'(ctx, args) {
    return funcs['RANK']!(ctx, args);
  },
  // RANK.AVG(valor, ref, [orden]): los empates reciben el PROMEDIO de sus rangos.
  'RANK.AVG'(_ctx, args) {
    if (args.length < 2) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    const nums = collectNumbers([args[1]!]);
    if (nums instanceof FormulaError) return nums;
    let asc = false;
    if (args.length >= 3) {
      const o = coerceNumber(args[2]!.value());
      if (isError(o)) return o;
      asc = o !== 0;
    }
    let ahead = 0;
    let tie = 0;
    for (const v of nums) {
      if (asc ? v < x : v > x) ahead++;
      if (v === x) tie++;
    }
    if (tie === 0) return ERR.NA;
    return ahead + 1 + (tie - 1) / 2;
  },

  // PERCENTRANK.INC / PERCENTRANK (alias): rango porcentual inclusivo.
  'PERCENTRANK.INC'(_ctx, args) {
    return percentRankFn(args, false);
  },
  PERCENTRANK(_ctx, args) {
    return percentRankFn(args, false);
  },
  // PERCENTRANK.EXC: rango porcentual exclusivo.
  'PERCENTRANK.EXC'(_ctx, args) {
    return percentRankFn(args, true);
  },

  // TRIMMEAN(array, porcentaje): media tras recortar (porcentaje·n, redondeado a
  // par) valores repartidos entre los dos extremos. porcentaje en [0,1).
  TRIMMEAN(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const nums = collectNumbers([args[0]!]);
    if (nums instanceof FormulaError) return nums;
    const pct = coerceNumber(args[1]!.value());
    if (isError(pct)) return pct;
    if (pct < 0 || pct >= 1) return ERR.NUM;
    const n = nums.length;
    if (n === 0) return ERR.NUM;
    const k = Math.floor((n * pct) / 2); // de cada extremo
    const s = [...nums].sort((a, b) => a - b);
    const kept = s.slice(k, n - k);
    if (kept.length === 0) return ERR.NUM;
    return sum(kept) / kept.length;
  },

  // STEYX(known_y, known_x): error típico de la predicción y sobre x (n ≥ 3).
  STEYX(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const p = collectPairs(args[0]!, args[1]!);
    if (p instanceof FormulaError) return p;
    const n = p.xs.length;
    if (n < 3) return ERR.DIV0;
    const r = regress(p.ys, p.xs);
    if (r.sxx === 0) return ERR.DIV0;
    const v = (r.syy - (r.sxy * r.sxy) / r.sxx) / (n - 2);
    return ok(Math.sqrt(Math.max(0, v)));
  },

  /* ================= Moda múltiple y utilidades normales ================= */

  // MODE.SNGL: idéntica a MODE (la moda única).
  'MODE.SNGL'(ctx, args) {
    return funcs['MODE']!(ctx, args);
  },
  // MODE.MULT(array): TODAS las modas (empatadas), derramadas en columna, en
  // orden de aparición; #N/A si ninguna se repite.
  'MODE.MULT'(_ctx, args) {
    const nums = collectNumbers(args);
    if (nums instanceof FormulaError) return nums;
    const counts = new Map<number, number>();
    for (const v of nums) counts.set(v, (counts.get(v) ?? 0) + 1);
    let best = 0;
    for (const c of counts.values()) if (c > best) best = c;
    if (best < 2) return ERR.NA;
    const modes: CellValue[][] = [];
    for (const [val, c] of counts) if (c === best) modes.push([val]);
    return modes;
  },

  // STANDARDIZE(x, media, desv): valor normalizado (x − media)/desv. desv > 0.
  STANDARDIZE(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    const mean = coerceNumber(args[1]!.value());
    if (isError(mean)) return mean;
    const sd = coerceNumber(args[2]!.value());
    if (isError(sd)) return sd;
    if (sd <= 0) return ERR.NUM;
    return (x - mean) / sd;
  },

  // FISHER(x): transformación de Fisher artanh(x). x en (−1, 1).
  FISHER(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    if (x <= -1 || x >= 1) return ERR.NUM;
    return 0.5 * Math.log((1 + x) / (1 - x));
  },
  // FISHERINV(y): inversa de FISHER, tanh(y).
  FISHERINV(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const y = coerceNumber(args[0]!.value());
    if (isError(y)) return y;
    const e2 = Math.exp(2 * y);
    return ok((e2 - 1) / (e2 + 1));
  },
  // GAUSS(z): Φ(z) − 0.5, probabilidad entre la media y z de la normal estándar.
  GAUSS(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const z = coerceNumber(args[0]!.value());
    if (isError(z)) return z;
    return normCdf(z) - 0.5;
  },
  // PHI(x): densidad de la normal estándar φ(x).
  PHI(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    return normPdf(x);
  },
  /* ---------- Variantes «A»: el texto cuenta como cero ---------- */

  /**
   * Desviación típica muestral incluyendo texto y booleanos.
   *
   * La diferencia con STDEV no es de fórmula sino de QUÉ se cuenta: aquí el texto vale 0
   * y entra en la muestra, así que una columna con un «N/D» escrito a mano da un
   * resultado distinto —y ese es justamente el motivo de que existan las dos—.
   */
  STDEVA(_ctx, args) {
    const nums = collectA(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 2) return ERR.DIV0;
    return ok(Math.sqrt(sumSqDev(nums) / (nums.length - 1)));
  },
  /** Varianza muestral incluyendo texto y booleanos (mismo criterio que STDEVA). */
  VARA(_ctx, args) {
    const nums = collectA(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 2) return ERR.DIV0;
    return ok(sumSqDev(nums) / (nums.length - 1));
  },

  /** Desviación estándar de la POBLACIÓN incluyendo texto y booleanos (mismo criterio que STDEVA). */
  STDEVPA(_ctx, args) {
    const nums = collectA(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 1) return ERR.DIV0;
    return ok(Math.sqrt(sumSqDev(nums) / nums.length));
  },
  /** Varianza de la POBLACIÓN incluyendo texto y booleanos. */
  VARPA(_ctx, args) {
    const nums = collectA(args);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 1) return ERR.DIV0;
    return ok(sumSqDev(nums) / nums.length);
  },

  /* ---------- Combinatoria con repetición ---------- */

  /** Permutaciones CON repetición: n^k. PERMUT, sin repetición, es n!/(n−k)!. */
  PERMUTATIONA(_ctx, args) {
    const v = reqNums(args, 2);
    if (v instanceof FormulaError) return v;
    const n = Math.trunc(v[0]!);
    const k = Math.trunc(v[1]!);
    if (n < 0 || k < 0) return ERR.NUM;
    return ok(Math.pow(n, k));
  },

  /* ---------- Distribuciones ---------- */

  /**
   * Lognormal: X es lognormal si ln(X) es normal, así que se delega en la normal sobre
   * el logaritmo. x ≤ 0 está fuera del soporte (#NUM!), no vale 0: el logaritmo no existe.
   */
  'LOGNORM.DIST': (_ctx, args) => {
    if (args.length !== 4) return ERR.VALUE;
    const v = reqNums(args, 3);
    if (v instanceof FormulaError) return v;
    const [x, mu, sd] = v as [number, number, number];
    const cum = cumArg(args[3]!);
    if (isError(cum)) return cum;
    if (x <= 0 || sd <= 0) return ERR.NUM;
    const z = (Math.log(x) - mu) / sd;
    return ok(cum ? normCdf(z) : normPdf(z) / (x * sd));
  },
  /** Inversa de la lognormal: exp(μ + σ·Φ⁻¹(p)). */
  'LOGNORM.INV': (_ctx, args) => {
    const v = reqNums(args, 3);
    if (v instanceof FormulaError) return v;
    const [p, mu, sd] = v as [number, number, number];
    if (p <= 0 || p >= 1 || sd <= 0) return ERR.NUM;
    return ok(Math.exp(mu + sd * normInv(p)));
  },
  /**
   * Binomial negativa: probabilidad de f fracasos antes del s-ésimo éxito.
   * P = C(f+s−1, f) · pˢ · (1−p)ᶠ. La masa se calcula por logaritmos (gammaln) para no
   * desbordar con f grande, igual que binomPmf.
   */
  'NEGBINOM.DIST': (_ctx, args) => {
    if (args.length < 3 || args.length > 4) return ERR.VALUE;
    const v = reqNums(args, 3);
    if (v instanceof FormulaError) return v;
    const f = Math.trunc(v[0]!);
    const s = Math.trunc(v[1]!);
    const p = v[2]!;
    let cum = false;
    if (args.length === 4) {
      const c = cumArg(args[3]!);
      if (isError(c)) return c;
      cum = c;
    }
    if (f < 0 || s < 1 || p <= 0 || p > 1) return ERR.NUM;
    const masa = (k: number): number =>
      Math.exp(
        gammaln(k + s) - gammaln(k + 1) - gammaln(s) + s * Math.log(p) + k * Math.log(1 - p),
      );
    if (!cum) return ok(masa(f));
    let acc = 0;
    for (let k = 0; k <= f; k += 1) acc += masa(k);
    return ok(acc);
  },
  /**
   * Hipergeométrica: extraer sin reposición. P = C(K,k)·C(N−K,n−k)/C(N,n).
   * Se calcula con gammaln y NO con factoriales: una urna de unos pocos miles ya
   * desborda el factorial aunque la probabilidad sea perfectamente representable.
   */
  'HYPGEOM.DIST': (_ctx, args) => {
    if (args.length < 4 || args.length > 5) return ERR.VALUE;
    const v = reqNums(args, 4);
    if (v instanceof FormulaError) return v;
    const k = Math.trunc(v[0]!); // éxitos en la muestra
    const n = Math.trunc(v[1]!); // tamaño de la muestra
    const K = Math.trunc(v[2]!); // éxitos en la población
    const N = Math.trunc(v[3]!); // tamaño de la población
    let cum = false;
    if (args.length === 5) {
      const c = cumArg(args[4]!);
      if (isError(c)) return c;
      cum = c;
    }
    if (n <= 0 || n > N || K < 0 || K > N || k < 0 || k > n || k > K || n - k > N - K) {
      return ERR.NUM;
    }
    const lc = (a: number, b: number): number =>
      gammaln(a + 1) - gammaln(b + 1) - gammaln(a - b + 1);
    const masa = (j: number): number => Math.exp(lc(K, j) + lc(N - K, n - j) - lc(N, n));
    if (!cum) return ok(masa(k));
    let acc = 0;
    for (let j = Math.max(0, n - (N - K)); j <= k; j += 1) acc += masa(j);
    return ok(acc);
  },
  /**
   * Menor valor cuya CDF binomial alcanza el criterio: el cuantil discreto.
   * Se acumula desde 0 en vez de bisecar: el dominio es entero y pequeño, y una
   * bisección sobre una función escalonada devuelve el escalón equivocado en los saltos.
   */
  'BINOM.INV': (_ctx, args) => {
    const v = reqNums(args, 3);
    if (v instanceof FormulaError) return v;
    const n = Math.trunc(v[0]!);
    const p = v[1]!;
    const alpha = v[2]!;
    if (n < 0 || p < 0 || p > 1 || alpha <= 0 || alpha >= 1) return ERR.NUM;
    let acc = 0;
    for (let k = 0; k <= n; k += 1) {
      acc += binomPmf(k, n, p);
      if (acc >= alpha) return k;
    }
    return n;
  },
  /**
   * Valor P de una cola de la prueba z: 1 − Φ((x̄ − μ₀)/(σ/√n)).
   *
   * Sin sigma se usa la desviación MUESTRAL de los datos, que es lo que hace una hoja
   * real cuando se omite el argumento.
   */
  'Z.TEST': (_ctx, args) => {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const nums = collectNumbers([args[0]!]);
    if (nums instanceof FormulaError) return nums;
    if (nums.length < 1) return ERR.NA;
    const mu = coerceNumber(args[1]!.value());
    if (isError(mu)) return mu;
    let sd: number;
    if (args.length === 3) {
      const s = coerceNumber(args[2]!.value());
      if (isError(s)) return s;
      sd = s;
    } else {
      if (nums.length < 2) return ERR.DIV0;
      sd = Math.sqrt(sumSqDev(nums) / (nums.length - 1));
    }
    if (sd <= 0) return ERR.NUM;
    const media = sum(nums) / nums.length;
    return ok(1 - normCdf((media - mu) / (sd / Math.sqrt(nums.length))));
  },
  /**
   * Probabilidad de que un valor caiga en [lo, hi] dada una distribución discreta
   * declarada como dos rangos (valores y sus probabilidades).
   *
   * Se valida que las probabilidades sumen 1 (con tolerancia): una distribución que no
   * suma 1 no es una distribución, y devolver un número igualmente escondería el error
   * de datos que el usuario tiene en su hoja.
   */
  PROB(_ctx, args) {
    if (args.length < 3 || args.length > 4) return ERR.VALUE;
    const xs = flatNums(args[0]!);
    if (xs instanceof FormulaError) return xs;
    const ps = flatNums(args[1]!);
    if (ps instanceof FormulaError) return ps;
    if (xs.length !== ps.length || xs.length === 0) return ERR.NA;
    let total = 0;
    for (const q of ps) {
      if (q < 0 || q > 1) return ERR.NUM;
      total += q;
    }
    if (Math.abs(total - 1) > 1e-9) return ERR.NUM;
    const lo = coerceNumber(args[2]!.value());
    if (isError(lo)) return lo;
    let hi = lo;
    if (args.length === 4) {
      const h = coerceNumber(args[3]!.value());
      if (isError(h)) return h;
      hi = h;
    }
    let acc = 0;
    for (let i = 0; i < xs.length; i += 1) {
      const x = xs[i]!;
      if (x >= lo && x <= hi) acc += ps[i]!;
    }
    return ok(acc);
  },
  /* ---------- Pruebas de hipótesis ---------- */

  /**
   * Valor P de la prueba t de Student. `colas` 1 o 2; `tipo` 1 pareada, 2 dos muestras
   * con varianza IGUAL, 3 con varianza distinta (Welch).
   *
   * Los tres tipos NO son intercambiables: aplicar el pareado a muestras independientes
   * da un valor P sistemáticamente distinto y perfectamente creíble. Por eso el tipo es
   * obligatorio, sin valor por defecto que adivine la intención.
   */
  'T.TEST': (_ctx, args) => {
    if (args.length !== 4) return ERR.VALUE;
    const xs = flatNums(args[0]!);
    if (xs instanceof FormulaError) return xs;
    const ys = flatNums(args[1]!);
    if (ys instanceof FormulaError) return ys;
    const colas = coerceNumber(args[2]!.value());
    if (isError(colas)) return colas;
    const tipo = coerceNumber(args[3]!.value());
    if (isError(tipo)) return tipo;
    const c = Math.trunc(colas);
    const t = Math.trunc(tipo);
    if ((c !== 1 && c !== 2) || t < 1 || t > 3) return ERR.NUM;

    let stat: number;
    let df: number;
    if (t === 1) {
      // Pareada: se trabaja sobre las DIFERENCIAS, así que exige igual longitud.
      if (xs.length !== ys.length) return ERR.NA;
      const d: number[] = [];
      for (let i = 0; i < xs.length; i += 1) d.push(xs[i]! - ys[i]!);
      const n = d.length;
      if (n < 2) return ERR.DIV0;
      const md = sum(d) / n;
      const sd = Math.sqrt(sumSqDev(d) / (n - 1));
      if (sd === 0) return ERR.DIV0;
      stat = md / (sd / Math.sqrt(n));
      df = n - 1;
    } else {
      const n1 = xs.length;
      const n2 = ys.length;
      if (n1 < 2 || n2 < 2) return ERR.DIV0;
      const m1 = sum(xs) / n1;
      const m2 = sum(ys) / n2;
      const v1 = sumSqDev(xs) / (n1 - 1);
      const v2 = sumSqDev(ys) / (n2 - 1);
      if (t === 2) {
        // Varianza común ponderada por grados de libertad.
        const vp = ((n1 - 1) * v1 + (n2 - 1) * v2) / (n1 + n2 - 2);
        if (vp === 0) return ERR.DIV0;
        stat = (m1 - m2) / Math.sqrt(vp * (1 / n1 + 1 / n2));
        df = n1 + n2 - 2;
      } else {
        // Welch: los grados de libertad salen de Welch–Satterthwaite y NO son enteros.
        // Redondearlos es un atajo común que cambia el valor P en el tercer decimal.
        const a = v1 / n1;
        const b = v2 / n2;
        if (a + b === 0) return ERR.DIV0;
        stat = (m1 - m2) / Math.sqrt(a + b);
        df = ((a + b) * (a + b)) / ((a * a) / (n1 - 1) + (b * b) / (n2 - 1));
      }
    }
    const unaCola = 1 - tcdf(Math.abs(stat), df);
    return ok(c === 1 ? unaCola : 2 * unaCola);
  },

  /**
   * Valor P de la prueba F de igualdad de varianzas (DOS colas).
   *
   * Se pone la varianza mayor en el numerador y se dobla la cola: así el resultado no
   * depende del orden en que se pasen las muestras, que es lo que espera quien la usa.
   */
  'F.TEST': (_ctx, args) => {
    if (args.length !== 2) return ERR.VALUE;
    const xs = flatNums(args[0]!);
    if (xs instanceof FormulaError) return xs;
    const ys = flatNums(args[1]!);
    if (ys instanceof FormulaError) return ys;
    const n1 = xs.length;
    const n2 = ys.length;
    if (n1 < 2 || n2 < 2) return ERR.DIV0;
    const v1 = sumSqDev(xs) / (n1 - 1);
    const v2 = sumSqDev(ys) / (n2 - 1);
    if (v1 === 0 || v2 === 0) return ERR.DIV0;
    const [mayor, menor, dfM, dfm] = v1 >= v2 ? [v1, v2, n1 - 1, n2 - 1] : [v2, v1, n2 - 1, n1 - 1];
    return ok(2 * (1 - fcdf(mayor / menor, dfM, dfm)));
  },

  /**
   * Prueba χ² de independencia: valor P a partir de las frecuencias observadas y las
   * esperadas. Los grados de libertad salen de la FORMA de la tabla —(filas−1)(cols−1)
   * en una tabla de contingencia, y n−1 si es un vector—, no del número de celdas.
   */
  'CHISQ.TEST': (_ctx, args) => {
    if (args.length !== 2) return ERR.VALUE;
    const obs = args[0]!.matrix();
    const esp = args[1]!.matrix();
    const filas = obs.length;
    const cols = filas > 0 ? obs[0]!.length : 0;
    if (filas === 0 || cols === 0) return ERR.NA;
    if (esp.length !== filas || (esp[0]?.length ?? 0) !== cols) return ERR.NA;
    let chi = 0;
    for (let i = 0; i < filas; i += 1) {
      for (let j = 0; j < cols; j += 1) {
        // Una matriz irregular deja huecos `undefined`; se saltan igual que el texto.
        const o = obs[i]?.[j];
        const e = esp[i]?.[j];
        if (o === undefined || e === undefined) continue;
        if (isError(o)) return o;
        if (isError(e)) return e;
        if (typeof o !== 'number' || typeof e !== 'number') continue;
        if (e === 0) return ERR.DIV0; // dividir por una frecuencia esperada nula
        chi += ((o - e) * (o - e)) / e;
      }
    }
    const df = filas === 1 || cols === 1 ? filas * cols - 1 : (filas - 1) * (cols - 1);
    if (df < 1) return ERR.NA;
    return ok(1 - gammap(df / 2, chi / 2));
  },
  /* ---------- Resultados de MATRIZ ---------- */

  /**
   * Histograma: cuántos valores caen en cada intervalo.
   *
   * Devuelve SIEMPRE un elemento MÁS que límites, y ese último es el desbordamiento
   * (lo que supera el límite mayor). Omitirlo es el fallo clásico y hace que las cuentas
   * no sumen el total, cosa que nadie mira hasta que falla el cuadre.
   *
   * Los límites son SUPERIORES E INCLUSIVOS —un valor igual al límite cuenta en ese
   * intervalo, no en el siguiente— y se ordenan antes de contar: el usuario puede
   * escribirlos desordenados y espera el mismo histograma.
   */
  FREQUENCY(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const datos = flatNums(args[0]!);
    if (datos instanceof FormulaError) return datos;
    const limites = flatNums(args[1]!);
    if (limites instanceof FormulaError) return limites;
    if (limites.length === 0) return [[datos.length]];
    const orden = [...limites].sort((a, b) => a - b);
    const cuentas = new Array<number>(orden.length + 1).fill(0);
    for (const v of datos) {
      let i = 0;
      while (i < orden.length && v > orden[i]!) i += 1;
      cuentas[i] = (cuentas[i] ?? 0) + 1;
    }
    // Columna, no fila: el resultado se derrama hacia abajo junto a los límites.
    return cuentas.map((c) => [c]);
  },

  /**
   * Regresión lineal por mínimos cuadrados. Devuelve {pendiente, intercepto} EN ESE
   * ORDEN, que es el inverso del intuitivo y una fuente inagotable de confusión: la
   * primera celda del resultado es m, no b.
   *
   * Con `constante` en FALSO la recta pasa por el origen, y entonces las sumas NO se
   * centran en la media — es otra regresión, no la misma con b forzado a cero.
   */
  LINEST(_ctx, args) {
    if (args.length < 1 || args.length > 4) return ERR.VALUE;
    const par = pareja(args);
    if (par instanceof FormulaError) return par;
    const { ys, xs } = par;
    if (ys.length < 2) return ERR.DIV0;
    const conConstante = optBool(args, 2, true);
    if (isError(conConstante)) return conConstante;
    const r = rectaMinimos(ys, xs, conConstante);
    if (r === undefined) return ERR.DIV0;
    return [[r.m, r.b]];
  },

  /**
   * Regresión EXPONENCIAL y = b·mˣ. Devuelve {m, b}, igual orden que LINEST.
   *
   * Se ajusta una recta sobre ln(y), así que un y ≤ 0 no es un dato más: no tiene
   * logaritmo y la curva exponencial no puede pasar por él. Es #NUM!, no un punto que
   * se descarte en silencio.
   */
  LOGEST(_ctx, args) {
    if (args.length < 1 || args.length > 4) return ERR.VALUE;
    const par = pareja(args);
    if (par instanceof FormulaError) return par;
    const { ys, xs } = par;
    if (ys.length < 2) return ERR.DIV0;
    const conConstante = optBool(args, 2, true);
    if (isError(conConstante)) return conConstante;
    const logs: number[] = [];
    for (const y of ys) {
      if (y <= 0) return ERR.NUM;
      logs.push(Math.log(y));
    }
    const r = rectaMinimos(logs, xs, conConstante);
    if (r === undefined) return ERR.DIV0;
    return [[Math.exp(r.m), Math.exp(r.b)]];
  },

  /**
   * Predicción con el modelo exponencial de LOGEST: ŷ = b·mˣ para cada x nuevo.
   * Sin `nuevas x` se predice sobre las mismas x, que es lo que hace una hoja real.
   */
  GROWTH(_ctx, args) {
    if (args.length < 1 || args.length > 4) return ERR.VALUE;
    const par = pareja(args);
    if (par instanceof FormulaError) return par;
    const { ys, xs } = par;
    if (ys.length < 2) return ERR.DIV0;
    const conConstante = optBool(args, 3, true);
    if (isError(conConstante)) return conConstante;
    const logs: number[] = [];
    for (const y of ys) {
      if (y <= 0) return ERR.NUM;
      logs.push(Math.log(y));
    }
    const r = rectaMinimos(logs, xs, conConstante);
    if (r === undefined) return ERR.DIV0;
    let nuevas = xs;
    if (args.length >= 3) {
      const nx = flatNums(args[2]!);
      if (nx instanceof FormulaError) return nx;
      if (nx.length > 0) nuevas = nx;
    }
    return nuevas.map((x) => [ok(Math.exp(r.b + r.m * x))]);
  },

  /* ============ Compatibilidad: nombres anteriores a Excel 2010 ============
   *
   * Un .xlsx guardado con Excel 2007 o con LibreOffice trae estos nombres, y sin
   * ellos la celda importada muestra #NAME? — el fichero del cliente se ve roto
   * aunque el motor sepa hacer la cuenta. Por eso están, no por engordar la lista.
   *
   * NO son alias: varias cambiaron de semántica al renombrarse, y tratarlas como
   * sinónimos daría un número equivocado sin avisar, que es peor que el #NAME?.
   * Los tres cambios que hay que respetar:
   *   · Las de cola:  TDIST/FDIST/CHIDIST devuelven la cola DERECHA y TINV/FINV/
   *     CHIINV invierten sobre esa cola. Sus homónimas modernas sin sufijo son
   *     acumuladas por la izquierda, así que el destino lleva `.RT` (y `TINV` es
   *     de DOS colas: va a `T.INV.2T`).
   *   · Las que perdieron el argumento «acumulada»: la vieja fija un valor y la
   *     moderna lo pide (NORMSDIST y LOGNORMDIST son CDF; HYPGEOMDIST y
   *     NEGBINOMDIST, densidad; BETADIST, CDF).
   *   · Las meramente renombradas, que sí delegan tal cual.
   */

  // NORMDIST(x, media, desv, acumulada) — idéntica a NORM.DIST.
  NORMDIST(ctx, args) {
    return funcs['NORM.DIST']!(ctx, args);
  },
  // NORMINV(prob, media, desv) — idéntica a NORM.INV.
  NORMINV(ctx, args) {
    return funcs['NORM.INV']!(ctx, args);
  },
  // NORMSDIST(z) — solo CDF: la moderna exige el 2.º argumento.
  NORMSDIST(ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    return funcs['NORM.S.DIST']!(ctx, [args[0]!, litArg(true)]);
  },
  // NORMSINV(prob) — idéntica a NORM.S.INV.
  NORMSINV(ctx, args) {
    return funcs['NORM.S.INV']!(ctx, args);
  },
  // TDIST(x, gl, colas): x ≥ 0 y colas ∈ {1,2}. Cola derecha, no acumulada.
  TDIST(ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    const colas = coerceNumber(args[2]!.value());
    if (isError(colas)) return colas;
    const t = Math.trunc(colas);
    if (x < 0 || (t !== 1 && t !== 2)) return ERR.NUM;
    const dest = t === 1 ? 'T.DIST.RT' : 'T.DIST.2T';
    return funcs[dest]!(ctx, [args[0]!, args[1]!]);
  },
  // TINV(prob, gl) — DOS colas, no la inversa acumulada.
  TINV(ctx, args) {
    return funcs['T.INV.2T']!(ctx, args);
  },
  // FDIST(x, gl1, gl2) — cola derecha.
  FDIST(ctx, args) {
    return funcs['F.DIST.RT']!(ctx, args);
  },
  // FINV(prob, gl1, gl2) — inversa de la cola derecha.
  FINV(ctx, args) {
    return funcs['F.INV.RT']!(ctx, args);
  },
  // CHIDIST(x, gl) — cola derecha.
  CHIDIST(ctx, args) {
    return funcs['CHISQ.DIST.RT']!(ctx, args);
  },
  // CHIINV(prob, gl) — inversa de la cola derecha.
  CHIINV(ctx, args) {
    return funcs['CHISQ.INV.RT']!(ctx, args);
  },
  // CHITEST(observado, esperado) — renombrada.
  CHITEST(ctx, args) {
    return funcs['CHISQ.TEST']!(ctx, args);
  },
  // FTEST(matriz1, matriz2) — renombrada.
  FTEST(ctx, args) {
    return funcs['F.TEST']!(ctx, args);
  },
  // TTEST(matriz1, matriz2, colas, tipo) — renombrada.
  TTEST(ctx, args) {
    return funcs['T.TEST']!(ctx, args);
  },
  // ZTEST(matriz, x, [sigma]) — renombrada.
  ZTEST(ctx, args) {
    return funcs['Z.TEST']!(ctx, args);
  },
  // EXPONDIST(x, lambda, acumulada) — renombrada.
  EXPONDIST(ctx, args) {
    return funcs['EXPON.DIST']!(ctx, args);
  },
  // POISSON(x, media, acumulada) — renombrada.
  POISSON(ctx, args) {
    return funcs['POISSON.DIST']!(ctx, args);
  },
  // HYPGEOMDIST(x, n_muestra, éxitos, población) — solo densidad.
  HYPGEOMDIST(ctx, args) {
    if (args.length !== 4) return ERR.VALUE;
    return funcs['HYPGEOM.DIST']!(ctx, [...args, litArg(false)]);
  },
  // LOGNORMDIST(x, media, desv) — solo CDF.
  LOGNORMDIST(ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    return funcs['LOGNORM.DIST']!(ctx, [...args, litArg(true)]);
  },
  // LOGINV(prob, media, desv) — renombrada a LOGNORM.INV.
  LOGINV(ctx, args) {
    return funcs['LOGNORM.INV']!(ctx, args);
  },
  // BETADIST(x, alfa, beta, [A], [B]) — solo CDF; «acumulada» va en 4.ª posición
  // en la moderna, así que los límites opcionales se corren detrás.
  BETADIST(ctx, args) {
    if (args.length < 3 || args.length > 5) return ERR.VALUE;
    return funcs['BETA.DIST']!(ctx, [args[0]!, args[1]!, args[2]!, litArg(true), ...args.slice(3)]);
  },
  // BETAINV(prob, alfa, beta, [A], [B]) — idéntica a BETA.INV.
  BETAINV(ctx, args) {
    return funcs['BETA.INV']!(ctx, args);
  },
  // GAMMADIST(x, alfa, beta, acumulada) — idéntica a GAMMA.DIST.
  GAMMADIST(ctx, args) {
    return funcs['GAMMA.DIST']!(ctx, args);
  },
  // GAMMAINV(prob, alfa, beta) — idéntica a GAMMA.INV.
  GAMMAINV(ctx, args) {
    return funcs['GAMMA.INV']!(ctx, args);
  },
  // NEGBINOMDIST(fracasos, éxitos, prob) — solo densidad.
  NEGBINOMDIST(ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    return funcs['NEGBINOM.DIST']!(ctx, [...args, litArg(false)]);
  },
  // CRITBINOM(ensayos, prob, alfa) — renombrada a BINOM.INV.
  CRITBINOM(ctx, args) {
    return funcs['BINOM.INV']!(ctx, args);
  },
};
