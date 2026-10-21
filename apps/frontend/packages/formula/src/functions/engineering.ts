/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Funciones de INGENIERÍA (semántica OpenFormula / ECMA-376, código original
 * clean-room): conversión de base DEC2BIN/BIN2DEC/DEC2HEX/HEX2DEC/DEC2OCT/
 * OCT2DEC, operaciones de bits BITAND/BITOR/BITXOR/BITLSHIFT/BITRSHIFT, y
 * CONVERT (unidades de masa/longitud/tiempo/temperatura con prefijos SI).
 *
 * Las conversiones de base usan complemento a dos del ancho canónico de Excel
 * (binario 10 bits, octal 30, hex 40). NO edites functions/index.ts.
 */
import type { Arg, FnTable } from '../registry.js';
import { coerceNumber, coerceText, ERR, FormulaError, isError } from '../values.js';
import type { CellValue } from '../values.js';
import { erf, erfc } from './erf.js';

/** Guard local sobre uniones que no son CellValue (p. ej. bigint | FormulaError). */
function isErr(v: unknown): v is FormulaError {
  return v instanceof FormulaError;
}

/* ---------- Ayudas ---------- */

function intArg(a: Arg | undefined): number | FormulaError {
  if (!a) return ERR.VALUE;
  const n = coerceNumber(a.value());
  return isError(n) ? n : Math.trunc(n);
}

/** Núcleo DEC2X: entero → texto en base `radix`, complemento a dos de `bits` si
 *  es negativo, con relleno opcional `places` (solo para no negativos). */
function formatBase(
  n: number,
  bits: number,
  radix: number,
  min: number,
  max: number,
  places: number | undefined,
): string | FormulaError {
  if (n < min || n > max) return ERR.NUM;
  const width = Math.pow(2, bits);
  let s = (n < 0 ? n + width : n).toString(radix).toUpperCase();
  if (n < 0) return s; // negativos: ancho completo, `places` se ignora (como Excel)
  if (places !== undefined) {
    if (places < 0 || places > bits) return ERR.NUM;
    if (places < s.length) return ERR.NUM; // no cabe en `places` dígitos
    s = s.padStart(places, '0');
  }
  return s;
}

/** Núcleo X2DEC: texto en base `radix` (≤ `maxLen` dígitos) → entero con signo
 *  por complemento a dos de `bits`. */
function parseBase(
  t: string,
  bits: number,
  radix: number,
  maxLen: number,
  valid: RegExp,
): number | FormulaError {
  const s = t.trim().toUpperCase();
  if (s === '' || s.length > maxLen || !valid.test(s)) return ERR.NUM;
  const value = parseInt(s, radix);
  if (!Number.isFinite(value)) return ERR.NUM;
  const half = Math.pow(2, bits - 1);
  // El bit de signo solo se alcanza con el ancho completo, así que `>= half` basta.
  return value >= half ? value - Math.pow(2, bits) : value;
}

/** Lee el `places` opcional (2º argumento) de una conversión de base. */
function placesArg(args: Arg[]): number | undefined | FormulaError {
  if (args.length < 2) return undefined;
  const p = intArg(args[1]);
  if (isError(p)) return p;
  return p;
}

/** DEC2X: entero (1er argumento) → base destino, con `places` opcional. */
function decToBase(
  args: Arg[],
  bits: number,
  radix: number,
  min: number,
  max: number,
): string | FormulaError {
  if (args.length < 1 || args.length > 2) return ERR.VALUE;
  const n = intArg(args[0]);
  if (isError(n)) return n;
  const places = placesArg(args);
  if (isErr(places)) return places;
  return formatBase(n, bits, radix, min, max, places);
}

/** X2DEC: texto en base origen (1er argumento) → entero con signo. */
function baseToDec(
  args: Arg[],
  bits: number,
  radix: number,
  maxLen: number,
  valid: RegExp,
): number | FormulaError {
  if (args.length !== 1) return ERR.VALUE;
  const t = coerceText(args[0]!.value());
  if (isError(t)) return t;
  return parseBase(t, bits, radix, maxLen, valid);
}

/** X2Y: texto en base origen → entero con signo → texto en base destino,
 *  con `places` opcional. Compone parseBase (origen) y formatBase (destino). */
function convBase(
  args: Arg[],
  src: [bits: number, radix: number, maxLen: number, valid: RegExp],
  dst: [bits: number, radix: number, min: number, max: number],
): string | FormulaError {
  if (args.length < 1 || args.length > 2) return ERR.VALUE;
  const t = coerceText(args[0]!.value());
  if (isError(t)) return t;
  const dec = parseBase(t, src[0], src[1], src[2], src[3]);
  if (isError(dec)) return dec;
  const places = placesArg(args);
  if (isErr(places)) return places;
  return formatBase(dec, dst[0], dst[1], dst[2], dst[3], places);
}

/** Bases canónicas de Excel: [bits, radix, maxLen, regex-validador]. */
const BIN_SRC: [number, number, number, RegExp] = [10, 2, 10, /^[01]+$/];
const OCT_SRC: [number, number, number, RegExp] = [30, 8, 10, /^[0-7]+$/];
const HEX_SRC: [number, number, number, RegExp] = [40, 16, 10, /^[0-9A-F]+$/];
/** Destinos: [bits, radix, min, max] (rango con signo del ancho canónico). */
const BIN_DST: [number, number, number, number] = [10, 2, -512, 511];
const OCT_DST: [number, number, number, number] = [30, 8, -(2 ** 29), 2 ** 29 - 1];
const HEX_DST: [number, number, number, number] = [40, 16, -(2 ** 39), 2 ** 39 - 1];

const MAX48 = Math.pow(2, 48) - 1;

/** Valida un operando de bits: entero en [0, 2^48-1]. */
function bitArg(a: Arg | undefined): bigint | FormulaError {
  const n = intArg(a);
  if (isError(n)) return n;
  if (n < 0 || n > MAX48) return ERR.NUM;
  return BigInt(n);
}

function bitwise(args: Arg[], op: (a: bigint, b: bigint) => bigint): number | FormulaError {
  if (args.length !== 2) return ERR.VALUE;
  const a = bitArg(args[0]);
  if (isErr(a)) return a;
  const b = bitArg(args[1]);
  if (isErr(b)) return b;
  return Number(op(a, b));
}

/* ---------- CONVERT ---------- */
// ponytail: subconjunto útil (masa/longitud/tiempo/temperatura) con prefijos SI.
// Amplía la tabla UNITS/PREFIX si se piden volumen/energía/presión/velocidad.

type Cat = 'mass' | 'dist' | 'time';
/** unidad → [categoría, factor a la unidad base de su categoría]. Base: g, m, s. */
const UNITS: Record<string, [Cat, number]> = {
  // masa (base: gramo)
  g: ['mass', 1],
  u: ['mass', 1.66053886e-24],
  grain: ['mass', 0.06479891],
  ozm: ['mass', 28.349523125],
  lbm: ['mass', 453.59237],
  stone: ['mass', 6350.29318],
  sg: ['mass', 14593.9029372064],
  ton: ['mass', 907184.74], // ton corta (US)
  // longitud (base: metro)
  m: ['dist', 1],
  mi: ['dist', 1609.344],
  Nmi: ['dist', 1852],
  in: ['dist', 0.0254],
  ft: ['dist', 0.3048],
  yd: ['dist', 0.9144],
  ang: ['dist', 1e-10],
  ly: ['dist', 9.4607304725808e15],
  pica: ['dist', 0.0254 / 6],
  // tiempo (base: segundo)
  sec: ['time', 1],
  s: ['time', 1],
  min: ['time', 60],
  mn: ['time', 60],
  hr: ['time', 3600],
  day: ['time', 86400],
  d: ['time', 86400],
  yr: ['time', 31557600],
};
/** Unidades que admiten prefijo SI (métricas). */
const PREFIXABLE = new Set(['g', 'm', 'sec', 's']);
const PREFIX: Record<string, number> = {
  Y: 1e24,
  Z: 1e21,
  E: 1e18,
  P: 1e15,
  T: 1e12,
  G: 1e9,
  M: 1e6,
  k: 1e3,
  h: 1e2,
  da: 1e1,
  d: 1e-1,
  c: 1e-2,
  m: 1e-3,
  u: 1e-6,
  µ: 1e-6,
  n: 1e-9,
  p: 1e-12,
  f: 1e-15,
  a: 1e-18,
  z: 1e-21,
  y: 1e-24,
};

/** Resuelve una unidad (con prefijo SI opcional) a [categoría, factor]. */
function resolveUnit(u: string): [Cat, number] | undefined {
  const direct = UNITS[u];
  if (direct) return direct;
  for (const p of Object.keys(PREFIX)) {
    if (u.length > p.length && u.startsWith(p)) {
      const rest = u.slice(p.length);
      if (PREFIXABLE.has(rest) && UNITS[rest]) {
        const [cat, f] = UNITS[rest];
        return [cat, f * PREFIX[p]!];
      }
    }
  }
  return undefined;
}

/** Temperatura → grados Celsius (unidad intermedia). undefined si no es de temp. */
function toCelsius(n: number, u: string): number | undefined {
  switch (u) {
    case 'C':
    case 'cel':
      return n;
    case 'F':
    case 'fah':
      return ((n - 32) * 5) / 9;
    case 'K':
    case 'kel':
      return n - 273.15;
    default:
      return undefined;
  }
}
function fromCelsius(c: number, u: string): number | undefined {
  switch (u) {
    case 'C':
    case 'cel':
      return c;
    case 'F':
    case 'fah':
      return (c * 9) / 5 + 32;
    case 'K':
    case 'kel':
      return c + 273.15;
    default:
      return undefined;
  }
}

/* ---------- Números complejos ----------
 * En una hoja de cálculo un complejo NO es un tipo: es TEXTO con forma «a+bi». Todas
 * las IM* reciben y devuelven ese texto, así que el parser y el formateador son la
 * función de verdad — las operaciones en sí son aritmética de dos números.
 */

interface Complejo {
  re: number;
  im: number;
  /** Sufijo con el que venía escrito: se conserva para no convertir «i» en «j». */
  suf: 'i' | 'j';
}

/**
 * Localiza el signo que separa la parte real de la imaginaria.
 *
 * Se busca de DERECHA a IZQUIERDA y saltando el signo que sigue a una `e`/`E`, que es
 * el de un exponente y no un separador: en «1.5E+3+2i» el `+` bueno es el segundo, y un
 * parser que coja el primero lee 1.5E como real y +3+2i como imaginaria. También se
 * salta la posición 0, donde el signo es el del propio número («-3+4i»).
 */
function cortePartes(t: string): number {
  for (let i = t.length - 1; i > 0; i -= 1) {
    const c = t[i]!;
    if (c !== '+' && c !== '-') continue;
    const prev = t[i - 1]!;
    if (prev === 'e' || prev === 'E') continue; // exponente, no separador
    return i;
  }
  return -1;
}

/** Interpreta el coeficiente de la parte imaginaria: «» y «+» valen 1, «-» vale −1. */
function coefImaginario(t: string): number | undefined {
  if (t === '' || t === '+') return 1;
  if (t === '-') return -1;
  const n = Number(t);
  return Number.isFinite(n) ? n : undefined;
}

/**
 * Convierte a complejo un valor de celda: texto «a+bi», o un número (imaginaria 0).
 *
 * El parámetro es `CellValue` y no `unknown` a propósito: con `unknown`, un #error que
 * llegara aquí se convertía en el texto «[object Object]» y de ahí salía un #NUM! —el
 * error original se perdía por el camino y el usuario veía el equivocado—. Tipado así,
 * el compilador obliga a decidir qué pasa con cada forma.
 */
function aComplejo(v: CellValue): Complejo | FormulaError {
  if (isError(v)) return v; // se propaga el error de origen, no se traduce a otro
  if (typeof v === 'number') return { re: v, im: 0, suf: 'i' };
  if (typeof v === 'boolean') return ERR.VALUE;
  const t = v.trim();
  if (t === '') return { re: 0, im: 0, suf: 'i' };

  const ultima = t[t.length - 1];
  if (ultima !== 'i' && ultima !== 'j') {
    const n = Number(t);
    return Number.isFinite(n) ? { re: n, im: 0, suf: 'i' } : ERR.NUM;
  }
  const suf = ultima;
  const cuerpo = t.slice(0, -1);
  const corte = cortePartes(cuerpo);
  if (corte < 0) {
    // Solo parte imaginaria: «4i», «i», «-i».
    const im = coefImaginario(cuerpo);
    return im === undefined ? ERR.NUM : { re: 0, im, suf };
  }
  const re = Number(cuerpo.slice(0, corte));
  const im = coefImaginario(cuerpo.slice(corte));
  if (!Number.isFinite(re) || im === undefined) return ERR.NUM;
  return { re, im, suf };
}

/**
 * Texto canónico de un complejo, con las omisiones que hace una hoja real: la parte
 * nula desaparece y el coeficiente 1 no se escribe. «3+1i» no es lo que devuelve Excel;
 * devuelve «3+i», y quien compare cadenas lo nota.
 */
function deComplejo(c: Complejo): string {
  const { re, im, suf } = c;
  if (im === 0) return String(re);
  const coef = im === 1 ? '' : im === -1 ? '-' : String(im);
  if (re === 0) return `${coef}${suf}`;
  const signo = im < 0 ? '' : '+'; // el negativo ya trae su signo en `coef`
  return `${re}${signo}${coef}${suf}`;
}

/** Lee el argumento `i` como complejo; #VALUE! si no existe. */
function cplx(args: Arg[], i: number): Complejo | FormulaError {
  const a = args[i];
  return a ? aComplejo(a.value()) : ERR.VALUE;
}

/**
 * Combina dos complejos exigiendo el MISMO sufijo. Mezclar «i» y «j» es #VALUE! en una
 * hoja real: son la misma unidad imaginaria, pero el resultado no puede llevar los dos
 * y elegir uno en silencio sorprende a quien lee la celda.
 */
function conSufijoComun(a: Complejo, b: Complejo): 'i' | 'j' | FormulaError {
  return a.suf === b.suf ? a.suf : ERR.VALUE;
}

/** Estrecha la unión de las ayudas de complejos, que `isError` no cubre (espera CellValue). */
function esErr(v: unknown): v is FormulaError {
  return v instanceof FormulaError;
}

/** Aplica a un complejo una función que devuelve otro complejo. */
function map1c(args: Arg[], f: (c: Complejo) => Complejo | FormulaError): string | FormulaError {
  const c = cplx(args, 0);
  if (esErr(c)) return c;
  if (args.length !== 1) return ERR.VALUE;
  const r = f(c);
  if (r instanceof FormulaError) return r;
  if (!Number.isFinite(r.re) || !Number.isFinite(r.im)) return ERR.NUM;
  return deComplejo(r);
}

/** Logaritmo en otra base: se divide el natural por ln(base), parte a parte. */
function escalaLog(c: Complejo, ln: number): Complejo | FormulaError {
  if (c.re === 0 && c.im === 0) return ERR.NUM;
  return {
    re: Math.log(Math.hypot(c.re, c.im)) / ln,
    im: Math.atan2(c.im, c.re) / ln,
    suf: c.suf,
  };
}

/** Producto de dos complejos (las partes se calculan ANTES de asignar, o se pisan). */
function mulc(a: Complejo, b: Complejo): Complejo {
  return { re: a.re * b.re - a.im * b.im, im: a.re * b.im + a.im * b.re, suf: a.suf };
}

/**
 * Potencia de exponente entero por cuadrados sucesivos: exacta y en log₂(n) productos,
 * así que un exponente grande tampoco cuesta. El negativo se resuelve invirtiendo al
 * final, no multiplicando inversas por el camino (menos operaciones, menos error).
 */
function potenciaEntera(c: Complejo, n: number): Complejo {
  const e = Math.abs(n);
  let base = c;
  let acc: Complejo = { re: 1, im: 0, suf: c.suf };
  for (let k = e; k > 0; k = Math.floor(k / 2)) {
    if (k % 2 === 1) acc = mulc(acc, base);
    base = mulc(base, base);
  }
  if (n >= 0) return acc;
  const d = acc.re * acc.re + acc.im * acc.im;
  return { re: acc.re / d, im: -acc.im / d, suf: c.suf };
}

/** Potencia de exponente fraccionario: r^n · e^(inθ), rama principal. */
function potenciaPolar(c: Complejo, n: number): Complejo {
  const r = Math.pow(Math.hypot(c.re, c.im), n);
  const t = Math.atan2(c.im, c.re) * n;
  return { re: r * Math.cos(t), im: r * Math.sin(t), suf: c.suf };
}

/** Recíproco 1/z. El origen no tiene inverso → #NUM!, el criterio que ya usa
 *  IMDIV con denominador nulo. */
function invc(c: Complejo): Complejo | FormulaError {
  const d = c.re * c.re + c.im * c.im;
  if (d === 0) return ERR.NUM;
  return { re: c.re / d, im: -c.im / d, suf: c.suf };
}

/** Cociente de dos complejos, con el polo del denominador como #NUM!. */
function divc(a: Complejo, b: Complejo): Complejo | FormulaError {
  const inv = invc(b);
  return esErr(inv) ? inv : mulc(a, inv);
}

/** sen(a+bi) = sen(a)·cosh(b) + i·cos(a)·senh(b). */
function senoc(c: Complejo): Complejo {
  return { re: Math.sin(c.re) * Math.cosh(c.im), im: Math.cos(c.re) * Math.sinh(c.im), suf: c.suf };
}
/** cos(a+bi) = cos(a)·cosh(b) − i·sen(a)·senh(b). El signo del término
 *  imaginario NO es simétrico al del seno; invertirlo es el error habitual al
 *  copiar la fórmula, y solo se nota con parte imaginaria distinta de cero. */
function cosenoc(c: Complejo): Complejo {
  return {
    re: Math.cos(c.re) * Math.cosh(c.im),
    im: -Math.sin(c.re) * Math.sinh(c.im),
    suf: c.suf,
  };
}
/** senh(a+bi) = senh(a)·cos(b) + i·cosh(a)·sen(b). Ojo: aquí el argumento REAL
 *  va en las hiperbólicas y el imaginario en las circulares, justo al revés que
 *  en senoc. */
function senohc(c: Complejo): Complejo {
  return { re: Math.sinh(c.re) * Math.cos(c.im), im: Math.cosh(c.re) * Math.sin(c.im), suf: c.suf };
}
/** cosh(a+bi) = cosh(a)·cos(b) + i·senh(a)·sen(b) (término imaginario POSITIVO,
 *  al contrario que en el coseno circular). */
function cosenohc(c: Complejo): Complejo {
  return { re: Math.cosh(c.re) * Math.cos(c.im), im: Math.sinh(c.re) * Math.sin(c.im), suf: c.suf };
}

/* ---------- Funciones de Bessel ----------
 * Ninguna serie sola cubre todo el dominio, así que cada una usa el método que
 * es exacto en su tramo. Los valores están contrastados con tablas publicadas y
 * con los wronskianos JₙYₙ₊₁ − Jₙ₊₁Yₙ = −2/(πx) e IₙKₙ₊₁ + Iₙ₊₁Kₙ = 1/x, que
 * son identidades exactas y no dependen de ninguna tabla.
 */

/**
 * Primer término de la serie, (x/2)ⁿ/n!, como producto acumulado. Calcularlo
 * como cociente de dos cantidades enormes desbordaría aunque el resultado quepa:
 * con n = 200 y x = 700 ni (x/2)ⁿ ni n! son un double, y su cociente sí.
 */
function besselTermino0(n: number, x: number): number {
  let t = 1;
  for (let i = 1; i <= n; i += 1) t *= x / 2 / i;
  return t;
}

/**
 * Serie de potencias Σ (∓1)ᵏ (x/2)^(n+2k) / (k!·(n+k)!), con signo alterno para
 * J y todo positivo para I. Cada término sale del anterior, así que no aparece
 * ningún factorial explícito.
 *
 * En I no hay restas: la serie es exacta para cualquier x. En J los términos
 * alternan y a partir de |x| ≈ 15 el término mayor supera al resultado en más
 * de 10⁵, así que ahí se cambia a cuadratura.
 */
function besselSerie(n: number, x: number, alterna: boolean): number {
  const q = (x * x) / 4;
  let term = besselTermino0(n, x);
  let suma = term;
  for (let k = 1; k <= 4000; k += 1) {
    term = ((alterna ? -term : term) * q) / (k * (n + k));
    suma += term;
    if (Math.abs(term) <= 1e-18 * Math.abs(suma)) break;
  }
  return suma;
}

/**
 * J por la integral de Bessel Jₙ(x) = (1/π)∫₀^π cos(nθ − x·senθ)dθ. El
 * integrando es periódico y sobre un periodo completo la regla del trapecio
 * converge EXPONENCIALMENTE, así que con N ≈ 4|x| nodos el error ya está en el
 * epsilon de la máquina y sin ninguna cancelación.
 *
 * ponytail: coste O(|x|) y suelo de ruido ABSOLUTO (~1e−16). Si el orden es tan
 * alto que Jₙ(x) cae por debajo (n ≳ 1,5·x, con |x| > 15) lo que sale es ruido y
 * no el valor exponencialmente pequeño; para ese rincón haría falta la
 * recurrencia descendente de Miller.
 */
function besselJCuadratura(n: number, x: number): number {
  const N = 64 + 4 * Math.ceil(Math.abs(x));
  let s = 0;
  for (let j = 0; j < N; j += 1) {
    const t = (2 * Math.PI * j) / N;
    s += Math.cos(n * t - x * Math.sin(t));
  }
  return s / N;
}

function besselJ(n: number, x: number): number {
  return Math.abs(x) <= 15 ? besselSerie(n, x, true) : besselJCuadratura(n, x);
}

/** Constante de Euler-Mascheroni, al double más cercano (γ = 0,5772156649015328606…). */
const GAMMA_EULER = 0.5772156649015329;

/** ψ(m) para m entero ≥ 1: −γ + Σ 1/i. */
function digamma(m: number): number {
  let s = -GAMMA_EULER;
  for (let i = 1; i < m; i += 1) s += 1 / i;
  return s;
}

/**
 * Y de orden 0 o 1 por su desarrollo en serie (A&S 9.1.11): el término
 * logarítmico es el que reproduce la divergencia en el origen, y el polo −2/(πx)
 * solo aparece en el orden 1.
 */
function besselYSerie(n: number, x: number): number {
  const q = (x * x) / 4;
  const polo = n === 0 ? 0 : -2 / (Math.PI * x);
  const logaritmico = (2 / Math.PI) * Math.log(x / 2) * besselJ(n, x);
  let term = besselTermino0(n, x);
  let suma = 0;
  for (let k = 0; k <= 4000; k += 1) {
    const add = (digamma(k + 1) + digamma(n + k + 1)) * term;
    suma += add;
    term = (-term * q) / ((k + 1) * (n + k + 1));
    if (k > 3 && Math.abs(add) <= 1e-18 * Math.abs(suma)) break;
  }
  return polo + logaritmico - suma / Math.PI;
}

/**
 * Módulos P y Q del desarrollo asintótico de Hankel, con los coeficientes
 * generados por recurrencia (no hay tabla que copiar). La serie es DIVERGENTE:
 * se corta en el término más pequeño, que es justo donde da su mejor precisión.
 */
function hankelPQ(n: number, x: number): [number, number] {
  const mu = 4 * n * n;
  let p = 1;
  let q = 0;
  let term = 1;
  let menor = Infinity;
  for (let k = 1; k <= 80; k += 1) {
    term = (term * (mu - (2 * k - 1) ** 2)) / (k * 8 * x);
    if (Math.abs(term) > menor) break;
    menor = Math.abs(term);
    if (k % 2 === 0) p += (k / 2) % 2 === 0 ? term : -term;
    else q += ((k - 1) / 2) % 2 === 0 ? term : -term;
  }
  return [p, q];
}

/**
 * Y de cualquier orden. Y₀ e Y₁ se calculan por serie cerca del origen y por el
 * asintótico de Hankel lejos; el corte está en x = 13, donde los dos métodos
 * coinciden en ~1e−12 (por debajo el asintótico aún no ha convergido, por
 * encima la serie ya ha perdido dígitos por cancelación).
 *
 * De ahí se sube con Yₙ₊₁ = (2n/x)Yₙ − Yₙ₋₁: la recurrencia es ESTABLE hacia
 * arriba porque Y crece con el orden (hacia abajo sería justo lo contrario).
 */
function besselY(n: number, x: number): number {
  const y = (m: number): number => (x <= 13 ? besselYSerie(m, x) : besselYAsintotico(m, x));
  let anterior = y(0);
  if (n === 0) return anterior;
  let actual = y(1);
  for (let k = 1; k < n; k += 1) {
    const siguiente = ((2 * k) / x) * actual - anterior;
    anterior = actual;
    actual = siguiente;
  }
  return actual;
}

function besselYAsintotico(n: number, x: number): number {
  const [p, q] = hankelPQ(n, x);
  const chi = x - (n / 2 + 0.25) * Math.PI;
  return Math.sqrt(2 / (Math.PI * x)) * (p * Math.sin(chi) + q * Math.cos(chi));
}

/**
 * K por la integral Kₙ(x) = ∫₀^∞ e^(−x·cosh t)·cosh(nt)dt. El integrando es PAR
 * y decae doblemente exponencial, o sea que el trapecio sobre la recta entera
 * converge exponencialmente (error ~ e^(−π²/h)); con h = 0,1 eso está muy por
 * debajo del epsilon de la máquina.
 *
 * Los cosenos hiperbólicos se separan en exponenciales a propósito: cosh(nt)
 * desbordaría por sí solo mucho antes de multiplicarlo por el e^(−x·cosh t) que
 * lo anula.
 */
function besselK(n: number, x: number): number {
  const h = 0.1;
  let s = Math.exp(-x) / 2;
  for (let j = 1; j <= 20_000; j += 1) {
    const t = j * h;
    const a = x * Math.cosh(t);
    if (!(a <= 745)) break; // e^(−745) ya es cero en un double: el resto no suma
    s += (Math.exp(n * t - a) + Math.exp(-n * t - a)) / 2;
  }
  return s * h;
}

/** Lee (x, n) de una BESSEL*: el orden se trunca a entero y no puede ser
 *  negativo (Excel devuelve #NUM!, no la identidad J₋ₙ = (−1)ⁿJₙ). */
function besselArgs(args: Arg[]): [number, number] | FormulaError {
  if (args.length !== 2) return ERR.VALUE;
  const x = coerceNumber(args[0]!.value());
  if (isError(x)) return x;
  const n = coerceNumber(args[1]!.value());
  if (isError(n)) return n;
  const orden = Math.trunc(n);
  if (orden < 0) return ERR.NUM;
  return [x, orden];
}

/** Envuelve una Bessel: valida argumentos, exige x > 0 si la función diverge en
 *  el origen (Y y K) y convierte un desbordamiento en #NUM!. */
function bessel(
  args: Arg[],
  soloPositivos: boolean,
  f: (n: number, x: number) => number,
): number | FormulaError {
  const a = besselArgs(args);
  if (isErr(a)) return a;
  const [x, n] = a;
  if (soloPositivos && !(x > 0)) return ERR.NUM;
  const r = f(n, x);
  return Number.isFinite(r) ? r : ERR.NUM;
}

export const funcs: FnTable = {
  DEC2BIN: (_c, a) => decToBase(a, 10, 2, -512, 511),
  DEC2OCT: (_c, a) => decToBase(a, 30, 8, -(2 ** 29), 2 ** 29 - 1),
  DEC2HEX: (_c, a) => decToBase(a, 40, 16, -(2 ** 39), 2 ** 39 - 1),
  BIN2DEC: (_c, a) => baseToDec(a, 10, 2, 10, /^[01]+$/),
  OCT2DEC: (_c, a) => baseToDec(a, 30, 8, 10, /^[0-7]+$/),
  HEX2DEC: (_c, a) => baseToDec(a, 40, 16, 10, /^[0-9A-F]+$/),

  // Conversiones cruzadas: parsean el origen a decimal con signo y lo reformatean
  // en la base destino (2's complement del ancho destino en negativos).
  BIN2OCT: (_c, a) => convBase(a, BIN_SRC, OCT_DST),
  BIN2HEX: (_c, a) => convBase(a, BIN_SRC, HEX_DST),
  OCT2BIN: (_c, a) => convBase(a, OCT_SRC, BIN_DST),
  OCT2HEX: (_c, a) => convBase(a, OCT_SRC, HEX_DST),
  HEX2BIN: (_c, a) => convBase(a, HEX_SRC, BIN_DST),
  HEX2OCT: (_c, a) => convBase(a, HEX_SRC, OCT_DST),

  BITAND: (_c, a) => bitwise(a, (x, y) => x & y),
  BITOR: (_c, a) => bitwise(a, (x, y) => x | y),
  BITXOR: (_c, a) => bitwise(a, (x, y) => x ^ y),

  BITLSHIFT(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const n = intArg(args[0]);
    if (isError(n)) return n;
    if (n < 0 || n > MAX48) return ERR.NUM;
    const sh = intArg(args[1]);
    if (isError(sh)) return sh;
    if (Math.abs(sh) > 53) return ERR.NUM;
    // desplazamiento negativo = a la derecha (como Excel).
    return sh >= 0 ? Number(BigInt(n) << BigInt(sh)) : Math.floor(n / Math.pow(2, -sh));
  },

  BITRSHIFT(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const n = intArg(args[0]);
    if (isError(n)) return n;
    if (n < 0 || n > MAX48) return ERR.NUM;
    const sh = intArg(args[1]);
    if (isError(sh)) return sh;
    if (Math.abs(sh) > 53) return ERR.NUM;
    return sh >= 0 ? Math.floor(n / Math.pow(2, sh)) : Number(BigInt(n) << BigInt(-sh));
  },

  // DELTA(n1, [n2=0]): 1 si n1 = n2, 0 si no (delta de Kronecker).
  DELTA(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const n1 = coerceNumber(args[0]!.value());
    if (isError(n1)) return n1;
    let n2 = 0;
    if (args.length === 2) {
      const v = coerceNumber(args[1]!.value());
      if (isError(v)) return v;
      n2 = v;
    }
    return n1 === n2 ? 1 : 0;
  },

  // GESTEP(n, [step=0]): 1 si n ≥ step, 0 si no (función escalón).
  GESTEP(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    if (isError(n)) return n;
    let step = 0;
    if (args.length === 2) {
      const v = coerceNumber(args[1]!.value());
      if (isError(v)) return v;
      step = v;
    }
    return n >= step ? 1 : 0;
  },

  // ERF(inf, [sup]): función error. Con un argumento, erf(inf) (integral 0→inf);
  // con dos, erf(sup) − erf(inf).
  ERF(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const lo = coerceNumber(args[0]!.value());
    if (isError(lo)) return lo;
    if (args.length === 2) {
      const hi = coerceNumber(args[1]!.value());
      if (isError(hi)) return hi;
      return erf(hi) - erf(lo);
    }
    return erf(lo);
  },
  // ERF.PRECISE(x): función error de un solo límite (erf(x)).
  'ERF.PRECISE'(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    return erf(x);
  },
  // ERFC(x): función error complementaria 1 − erf(x).
  ERFC(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    return erfc(x);
  },
  // ERFC.PRECISE(x): idéntica a ERFC en este motor.
  'ERFC.PRECISE'(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    return erfc(x);
  },

  // CONVERT(numero, de, a): convierte entre unidades compatibles. #N/A si alguna
  // unidad es desconocida o si las categorías no coinciden.
  CONVERT(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    if (isError(n)) return n;
    const from = coerceText(args[1]!.value());
    if (isError(from)) return from;
    const to = coerceText(args[2]!.value());
    if (isError(to)) return to;

    // Temperatura: conversión afín vía Celsius.
    const c = toCelsius(n, from);
    if (c !== undefined) {
      const r = fromCelsius(c, to);
      return r === undefined ? ERR.NA : r;
    }
    if (toCelsius(0, to) !== undefined) return ERR.NA; // destino temp, origen no

    const uf = resolveUnit(from);
    const ut = resolveUnit(to);
    if (!uf || !ut || uf[0] !== ut[0]) return ERR.NA;
    return (n * uf[1]) / ut[1];
  },
  /* ---------- Números complejos (IM*) ---------- */

  /** Construye el texto «a+bi» a partir de las dos partes. Sufijo «i» por defecto. */
  COMPLEX(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const re = coerceNumber(args[0]!.value());
    if (isError(re)) return re;
    const im = coerceNumber(args[1]!.value());
    if (isError(im)) return im;
    let suf: 'i' | 'j' = 'i';
    if (args.length === 3) {
      const t = coerceText(args[2]!.value());
      if (isError(t)) return t;
      if (t !== 'i' && t !== 'j') return ERR.VALUE; // «I» mayúscula tampoco vale
      suf = t;
    }
    return deComplejo({ re, im, suf });
  },

  IMREAL(_ctx, args) {
    const c = cplx(args, 0);
    return esErr(c) ? c : c.re;
  },
  IMAGINARY(_ctx, args) {
    const c = cplx(args, 0);
    return esErr(c) ? c : c.im;
  },
  /** Módulo: √(re²+im²). Se usa `hypot` para no desbordar con componentes grandes. */
  IMABS(_ctx, args) {
    const c = cplx(args, 0);
    return esErr(c) ? c : Math.hypot(c.re, c.im);
  },
  /**
   * Argumento en radianes, rango (−π, π]. `atan2` da el cuadrante correcto; `atan(im/re)`
   * no lo daría y además dividiría por cero en el eje imaginario.
   * El origen no tiene argumento definido → #DIV/0!, como una hoja real.
   */
  IMARGUMENT(_ctx, args) {
    const c = cplx(args, 0);
    if (esErr(c)) return c;
    if (c.re === 0 && c.im === 0) return ERR.DIV0;
    return Math.atan2(c.im, c.re);
  },
  IMCONJUGATE(_ctx, args) {
    return map1c(args, (c) => ({ re: c.re, im: -c.im, suf: c.suf }));
  },

  /** Suma de N complejos (0 argumentos → «0»). */
  IMSUM(_ctx, args) {
    let acc: Complejo = { re: 0, im: 0, suf: 'i' };
    let visto = false;
    for (let i = 0; i < args.length; i += 1) {
      const c = cplx(args, i);
      if (esErr(c)) return c;
      if (visto) {
        const suf = conSufijoComun(acc, c);
        if (esErr(suf)) return suf;
      } else {
        acc.suf = c.suf;
        visto = true;
      }
      acc = { re: acc.re + c.re, im: acc.im + c.im, suf: acc.suf };
    }
    return deComplejo(acc);
  },
  IMSUB(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = cplx(args, 0);
    if (esErr(a)) return a;
    const b = cplx(args, 1);
    if (esErr(b)) return b;
    const suf = conSufijoComun(a, b);
    if (esErr(suf)) return suf;
    return deComplejo({ re: a.re - b.re, im: a.im - b.im, suf });
  },
  /** Producto de N complejos: (a+bi)(c+di) = (ac−bd) + (ad+bc)i. */
  IMPRODUCT(_ctx, args) {
    let acc: Complejo = { re: 1, im: 0, suf: 'i' };
    let visto = false;
    for (let i = 0; i < args.length; i += 1) {
      const c = cplx(args, i);
      if (esErr(c)) return c;
      if (visto) {
        const suf = conSufijoComun(acc, c);
        if (esErr(suf)) return suf;
      } else {
        acc.suf = c.suf;
        visto = true;
      }
      acc = { ...mulc(acc, c), suf: acc.suf };
    }
    return deComplejo(acc);
  },
  /** División. Denominador cero → #NUM! (es el criterio de Excel para las IM*). */
  IMDIV(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = cplx(args, 0);
    if (esErr(a)) return a;
    const b = cplx(args, 1);
    if (esErr(b)) return b;
    const suf = conSufijoComun(a, b);
    if (esErr(suf)) return suf;
    const d = b.re * b.re + b.im * b.im;
    if (d === 0) return ERR.NUM;
    return deComplejo({
      re: (a.re * b.re + a.im * b.im) / d,
      im: (a.im * b.re - a.re * b.im) / d,
      suf,
    });
  },

  /** Raíz principal, vía forma polar: √r · e^(iθ/2). */
  IMSQRT(_ctx, args) {
    return map1c(args, (c) => {
      const r = Math.sqrt(Math.hypot(c.re, c.im));
      const t = Math.atan2(c.im, c.re) / 2;
      return { re: r * Math.cos(t), im: r * Math.sin(t), suf: c.suf };
    });
  },
  /**
   * Potencia de exponente REAL (no complejo).
   *
   * Con exponente ENTERO se multiplica (exponenciación binaria), no se pasa por la forma
   * polar: (1+i)² vale 2i exacto, y por polar sale «1.2246e-16+2.0000000000000004i»
   * porque sen(π/2) no da 0 redondo. Ese residuo no es un decimal de más, es basura que
   * el usuario ve en la celda y arrastra a la siguiente fórmula.
   *
   * Para exponente fraccionario la polar es el único camino y ahí sí se usa.
   */
  IMPOWER(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const c = cplx(args, 0);
    if (esErr(c)) return c;
    const n = coerceNumber(args[1]!.value());
    if (isError(n)) return n;
    if (c.re === 0 && c.im === 0) {
      return n === 0 ? ERR.NUM : deComplejo({ re: 0, im: 0, suf: c.suf });
    }
    const out = Number.isInteger(n) ? potenciaEntera(c, n) : potenciaPolar(c, n);
    if (!Number.isFinite(out.re) || !Number.isFinite(out.im)) return ERR.NUM;
    return deComplejo(out);
  },
  /** Exponencial: e^a·(cos b + i·sen b). */
  IMEXP(_ctx, args) {
    return map1c(args, (c) => {
      const e = Math.exp(c.re);
      return { re: e * Math.cos(c.im), im: e * Math.sin(c.im), suf: c.suf };
    });
  },
  /** Logaritmo natural (rama principal): ln|z| + i·arg(z). ln(0) no existe → #NUM!. */
  IMLN(_ctx, args) {
    return map1c(args, (c) => {
      if (c.re === 0 && c.im === 0) return ERR.NUM;
      return { re: Math.log(Math.hypot(c.re, c.im)), im: Math.atan2(c.im, c.re), suf: c.suf };
    });
  },
  IMLOG10(_ctx, args) {
    return map1c(args, (c) => escalaLog(c, Math.LN10));
  },
  IMLOG2(_ctx, args) {
    return map1c(args, (c) => escalaLog(c, Math.LN2));
  },
  /** Seno: sen(a)·cosh(b) + i·cos(a)·senh(b). */
  IMSIN(_ctx, args) {
    return map1c(args, senoc);
  },
  /** Coseno: cos(a)·cosh(b) − i·sen(a)·senh(b). */
  IMCOS(_ctx, args) {
    return map1c(args, cosenoc);
  },
  /** Tangente = seno/coseno. En los ceros del coseno (z = π/2 + kπ real) hay polo
   *  y el resultado es #NUM!, no un número enorme. */
  IMTAN(_ctx, args) {
    return map1c(args, (c) => divc(senoc(c), cosenoc(c)));
  },
  /** Cotangente = coseno/seno. Polo en los ceros del seno (z = kπ real). */
  IMCOT(_ctx, args) {
    return map1c(args, (c) => divc(cosenoc(c), senoc(c)));
  },
  /** Secante = 1/coseno. */
  IMSEC(_ctx, args) {
    return map1c(args, (c) => invc(cosenoc(c)));
  },
  /** Cosecante = 1/seno. */
  IMCSC(_ctx, args) {
    return map1c(args, (c) => invc(senoc(c)));
  },
  /** Seno hiperbólico: senh(a)·cos(b) + i·cosh(a)·sen(b). */
  IMSINH(_ctx, args) {
    return map1c(args, senohc);
  },
  /** Coseno hiperbólico: cosh(a)·cos(b) + i·senh(a)·sen(b). */
  IMCOSH(_ctx, args) {
    return map1c(args, cosenohc);
  },
  /** Secante hiperbólica = 1/cosh. */
  IMSECH(_ctx, args) {
    return map1c(args, (c) => invc(cosenohc(c)));
  },
  /** Cosecante hiperbólica = 1/senh. Polo en el origen. */
  IMCSCH(_ctx, args) {
    return map1c(args, (c) => invc(senohc(c)));
  },

  /* ---------- Bessel ---------- */

  // BESSELJ(x, n): primera especie. Solución de la ecuación de Bessel que es
  // FINITA en el origen (modos de una membrana circular, difracción).
  BESSELJ(_ctx, args) {
    return bessel(args, false, besselJ);
  },
  // BESSELY(x, n): segunda especie (función de Neumann). Diverge
  // logarítmicamente en el origen, así que x ≤ 0 es #NUM!.
  BESSELY(_ctx, args) {
    return bessel(args, true, besselY);
  },
  // BESSELI(x, n): primera especie MODIFICADA, e^x en vez de oscilar.
  BESSELI(_ctx, args) {
    return bessel(args, false, (n, x) => besselSerie(n, x, false));
  },
  // BESSELK(x, n): segunda especie modificada, la que decae como e^(−x)
  // (conducción de calor radial, líneas de transmisión). x ≤ 0 es #NUM!.
  BESSELK(_ctx, args) {
    return bessel(args, true, besselK);
  },
};
