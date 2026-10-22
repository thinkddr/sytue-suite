/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Financieras (PMT, PV, FV, NPV, IRR, RATE, NPER, IPMT, PPMT, SLN, SYD, DB, DDB,
 * XNPV, XIRR, CUMIPMT, CUMPRINC). Semántica OpenFormula / ECMA-376, código
 * original (clean-room). NO edites functions/index.ts.
 *
 * Núcleo del valor temporal del dinero (TVM). Para rate ≠ 0 se cumple:
 *   pv·(1+rate)^nper + pmt·(1 + rate·type)·((1+rate)^nper − 1)/rate + fv = 0
 * con type 0 (pago a fin de periodo) o 1 (a principio). Para rate = 0:
 *   pv + pmt·nper + fv = 0.
 * De ahí se despejan PMT/PV/FV/NPER. IPMT/PPMT usan el saldo pendiente al
 * inicio del periodo (vía FV parcial). IRR/RATE/XIRR resuelven por Newton con
 * derivada numérica y bisección de reserva; #NUM! si no convergen.
 */
import type { Arg, FnTable } from '../registry.js';
import { coerceNumber, ERR, FormulaError, isError } from '../values.js';
import { dayCount, isMonthEnd, shiftMonth, yearFrac } from './datetime.js';

/* ---------- Lectura de argumentos ---------- */

/**
 * Lee `required` números obligatorios seguidos de opcionales con `defaults`.
 * Un obligatorio ausente → #VALUE!; un opcional ausente → su default. Un
 * argumento presente se coacciona a número (celda en blanco = 0) y propaga el
 * primer #error. Devuelve el vector de números o el error.
 */
/**
 * Depreciación 0→`hasta` con cambio a lineal, para VDB.
 *
 * Va periodo a periodo porque el cambio depende del valor que quede por amortizar
 * en ese momento: no hay forma cerrada una vez que puede cambiar. El cambio es
 * DEFINITIVO —una vez en lineal no se vuelve—, como en Excel y LibreOffice.
 *
 * `vidaLineal` es la vida que usa la rama lineal y NO siempre es `life`: cuando
 * VDB arranca en un periodo posterior, el tramo se calcula sobre la vida que
 * queda. Restar dos acumuladas 0→end y 0→start da un número distinto (y menor),
 * porque decide el cambio con la vida completa.
 */
function vdbInter(
  cost: number,
  salvage: number,
  life: number,
  vidaLineal: number,
  hasta: number,
  factor: number,
): number {
  let total = 0;
  let porAmortizar = cost - salvage;
  let lineal = 0;
  let enLineal = false;
  const fin = Math.ceil(hasta);
  for (let i = 1; i <= fin; i++) {
    let term: number;
    if (!enLineal) {
      const dec = ddbPeriodo(cost, salvage, life, i, factor);
      lineal = porAmortizar / (vidaLineal - (i - 1));
      if (lineal > dec) {
        term = lineal;
        enLineal = true;
      } else {
        term = dec;
        porAmortizar -= dec;
      }
    } else {
      term = lineal;
    }
    // El último periodo puede entrar solo en parte (medio mes, 0,875 de año).
    if (i === fin) term *= hasta + 1 - fin;
    total += term;
  }
  return total;
}

/** DDB de UN periodo, forma cerrada. Es lo que calcula la función DDB de abajo;
 *  vive aparte para que VDB la use sin pasar por la capa de argumentos. */
function ddbPeriodo(
  cost: number,
  salvage: number,
  life: number,
  period: number,
  factor: number,
): number {
  const f = factor / life;
  if (f >= 1) return period <= 1 ? Math.max(cost - salvage, 0) : 0;
  const ini = cost * Math.pow(1 - f, period - 1);
  const fin = cost * Math.pow(1 - f, period);
  return fin < salvage ? Math.max(ini - salvage, 0) : ini - fin;
}

function nums(args: Arg[], required: number, defaults: number[] = []): number[] | FormulaError {
  const out: number[] = [];
  const total = required + defaults.length;
  for (let i = 0; i < total; i++) {
    const a = args[i];
    if (!a) {
      if (i < required) return ERR.VALUE;
      out.push(defaults[i - required]!);
      continue;
    }
    const n = coerceNumber(a.value());
    if (isError(n)) return n;
    out.push(n);
  }
  return out;
}

/** Números de un ÚNICO argumento en orden (fila a fila): solo cuentan los
 *  números; texto, blancos y booleanos se ignoran (regla NPV/IRR); el primer
 *  #error se propaga. El orden importa (posición → periodo). */
function orderedNums(a: Arg): number[] | FormulaError {
  const out: number[] = [];
  for (const row of a.matrix()) {
    for (const v of row) {
      if (isError(v)) return v;
      if (typeof v === 'number') out.push(v);
    }
  }
  return out;
}

/** Todas las celdas de un argumento coaccionadas a número (blanco = 0), en
 *  orden, para emparejar valores↔fechas en XNPV/XIRR. Propaga #errores. */
function flatNums(a: Arg): number[] | FormulaError {
  const out: number[] = [];
  for (const row of a.matrix()) {
    for (const v of row) {
      const n = coerceNumber(v);
      if (isError(n)) return n;
      out.push(n);
    }
  }
  return out;
}

/* ---------- Núcleo TVM ---------- */

function pmtCore(rate: number, nper: number, pv: number, fv: number, type: number): number {
  if (rate === 0) return -(pv + fv) / nper;
  const p = Math.pow(1 + rate, nper);
  return (-(pv * p + fv) * rate) / ((1 + rate * type) * (p - 1));
}

function fvCore(rate: number, nper: number, pmt: number, pv: number, type: number): number {
  if (rate === 0) return -(pv + pmt * nper);
  const p = Math.pow(1 + rate, nper);
  return -(pv * p + (pmt * (1 + rate * type) * (p - 1)) / rate);
}

function pvCore(rate: number, nper: number, pmt: number, fv: number, type: number): number {
  if (rate === 0) return -(pmt * nper + fv);
  const p = Math.pow(1 + rate, nper);
  return -((pmt * (1 + rate * type) * (p - 1)) / rate + fv) / p;
}

/** Interés del periodo `per` (signo Excel), dado el pago ya calculado. type=0:
 *  interés sobre el saldo al inicio de `per` = FV parcial de (per−1) periodos.
 *  type=1: el primer pago es todo principal (0); a partir de ahí, el interés
 *  acumulado del periodo anterior. */
function ipmtAt(rate: number, per: number, pv: number, type: number, pmt: number): number {
  if (type === 1) return per <= 1 ? 0 : (fvCore(rate, per - 2, pmt, pv, 1) - pmt) * rate;
  return fvCore(rate, per - 1, pmt, pv, 0) * rate;
}

/** Normaliza el tipo de pago a 0/1. ponytail: cualquier valor ≠0 cuenta como
 *  «principio de periodo» (como Google Sheets/LibreOffice), sin #NUM!. */
function payType(type: number): number {
  return type !== 0 ? 1 : 0;
}

/* ---------- Solucionador de tasa (Newton + bisección) ---------- */

/** Newton con derivada numérica; se mantiene en el dominio rate > −1.
 *  Devuelve la raíz o undefined si no converge en 80 iteraciones. */
function newton(f: (x: number) => number, guess: number): number | undefined {
  let x = Number.isFinite(guess) ? guess : 0.1;
  for (let i = 0; i < 80; i++) {
    const fx = f(x);
    if (!Number.isFinite(fx)) return undefined;
    if (Math.abs(fx) < 1e-10) return x;
    const h = 1e-6 * Math.max(1, Math.abs(x));
    const d = (f(x + h) - fx) / h;
    if (!Number.isFinite(d) || d === 0) return undefined;
    let nx = x - fx / d;
    if (!Number.isFinite(nx)) return undefined;
    if (nx <= -1) nx = (x - 1) / 2; // no salir del dominio
    if (Math.abs(nx - x) < 1e-12) {
      // Paso ínfimo: aceptar solo si es raíz real (f normalizada ≈ 0). Evita el
      // falso «convergió» cuando el clamp se estanca asintóticamente en rate=−1.
      const fn = f(nx);
      return Number.isFinite(fn) && Math.abs(fn) < 1e-7 ? nx : undefined;
    }
    x = nx;
  }
  return undefined;
}

/** Bisección de reserva: escanea buscando un cambio de signo y biseca el primer
 *  intervalo que lo tenga. ponytail: rejilla ~[-99%, +10000%]; raíces fuera de
 *  ese rango o entre nodos de una función no monótona no se hallan → #NUM!. */
function bisect(f: (x: number) => number): number | undefined {
  let prev = -0.9999;
  let fprev = f(prev);
  const pts: number[] = [];
  for (let r = -0.99; r <= 1.0001; r += 0.01) pts.push(r);
  for (let r = 2; r <= 100; r += 1) pts.push(r);
  for (const x of pts) {
    const fx = f(x);
    if (Number.isFinite(fprev) && Number.isFinite(fx) && fprev * fx < 0) {
      let lo = prev;
      let hi = x;
      let flo = fprev;
      for (let i = 0; i < 100; i++) {
        const mid = (lo + hi) / 2;
        const fm = f(mid);
        if (!Number.isFinite(fm)) return undefined;
        if (Math.abs(fm) < 1e-12 || hi - lo < 1e-13) return mid;
        if (flo * fm < 0) hi = mid;
        else {
          lo = mid;
          flo = fm;
        }
      }
      return (lo + hi) / 2;
    }
    prev = x;
    fprev = fx;
  }
  return undefined;
}

function findRate(f: (x: number) => number, guess: number): number | undefined {
  return newton(f, guess) ?? bisect(f);
}

/* ---------- Bono simple (PRICE / YIELD / DURATION) ---------- */

/** ¿Frecuencia de cupón válida (1 anual, 2 semestral, 4 trimestral)? */
function validFreq(freq: number): boolean {
  const f = Math.trunc(freq);
  return f === 1 || f === 2 || f === 4;
}

/* ---------- Calendario de cupones (COUP*) ----------
 * Un bono no devenga «tantos días entre dos fechas»: devenga dentro del periodo
 * de cupón que contiene la liquidación. Todo lo que sigue (y el devengado de
 * ACCRINT) depende de saber cuál es ese periodo, que NO se puede aproximar con
 * meses redondos sin equivocarse de un día en cada febrero.
 */

/**
 * Fecha del k-ésimo cupón contando hacia ATRÁS desde el vencimiento (k = 0 es
 * el vencimiento). El calendario se ancla siempre en el vencimiento y no se
 * encadena cupón a cupón: encadenando, el recorte de febrero se arrastra hacia
 * adelante y un bono que vence el 31-mar acabaría pagando el 30-mar el resto de
 * su vida (30-sep → 30-mar → 30-sep…), un día menos de devengado por año.
 */
function coupDate(maturity: number, k: number, freq: number): number {
  return shiftMonth(maturity, (-k * 12) / freq, isMonthEnd(maturity));
}

/**
 * Índice del último cupón anterior o igual a la liquidación. Es a la vez el
 * número de cupones que quedan por cobrar, o sea COUPNUM: si el cupón k cae en
 * la liquidación o antes, los que quedan son k−1, k−2, …, 0 (el vencimiento).
 * −1 si la liquidación queda absurdamente lejos del vencimiento.
 */
function coupIndex(settle: number, maturity: number, freq: number): number {
  for (let k = 0; k <= 12_000; k += 1) {
    if (coupDate(maturity, k, freq) <= settle) return k;
  }
  return -1; // >3000 años de cupones: la entrada no describe un bono
}

interface CoupArgs {
  settle: number;
  maturity: number;
  freq: number;
  basis: number;
  k: number;
}

/** Argumentos comunes de la familia COUP*: (liquidación, vencimiento,
 *  frecuencia, [base]) validados y con el índice de cupón ya resuelto. */
function coupArgs(args: Arg[]): CoupArgs | FormulaError {
  const a = nums(args, 3, [0]);
  if (a instanceof FormulaError) return a;
  const settle = Math.floor(a[0]!);
  const maturity = Math.floor(a[1]!);
  const freq = Math.trunc(a[2]!);
  const basis = Math.trunc(a[3]!);
  if (settle < 0 || maturity <= settle || !validFreq(freq)) return ERR.NUM;
  if (basis < 0 || basis > 4) return ERR.NUM;
  const k = coupIndex(settle, maturity, freq);
  if (k < 0) return ERR.NUM;
  return { settle, maturity, freq, basis, k };
}

/** Días del periodo de cupón completo. Solo la base 1 los cuenta de verdad; las
 *  demás usan la longitud NOMINAL del año dividida entre la frecuencia (de ahí
 *  el 182,5 de la base 3, que a propósito no cuadra con DAYBS+DAYSNC). */
function coupPeriodDays(c: CoupArgs): number {
  if (c.basis === 1)
    return coupDate(c.maturity, c.k - 1, c.freq) - coupDate(c.maturity, c.k, c.freq);
  return (c.basis === 3 ? 365 : 360) / c.freq;
}

/**
 * Devengo entre dos fechas medido en CUPONES (la Σ Aᵢ/NLᵢ de ECMA-376), que es
 * lo que multiplica ACCRINT.
 *
 * Con base 1 hay que recorrer los cuasi-cupones uno a uno: cada uno mide
 * distinto (29-feb→31-ago son 184 días, 31-ago→28-feb son 181) y repartir el
 * devengo sobre un periodo «medio» daría un número plausible y equivocado. Con
 * el resto de bases todos los periodos miden igual y basta una división.
 */
function accruedCoupons(
  start: number,
  settle: number,
  anchor: number,
  freq: number,
  basis: number,
): number | FormulaError {
  if (basis !== 1) {
    const d = dayCount(start, settle, basis);
    if (d instanceof FormulaError) return d;
    return (d * freq) / (basis === 3 ? 365 : 360);
  }
  const eom = isMonthEnd(anchor);
  const months = -12 / freq;
  let first = 0;
  while (shiftMonth(anchor, first * months, eom) > start) {
    first += 1;
    if (first > 12_000) return ERR.NUM;
  }
  let total = 0;
  for (let i = first; i >= -12_000; i -= 1) {
    const from = shiftMonth(anchor, i * months, eom);
    const to = shiftMonth(anchor, (i - 1) * months, eom);
    const lo = Math.max(from, start);
    const hi = Math.min(to, settle);
    if (hi > lo) total += (hi - lo) / (to - from);
    if (to >= settle) return total;
  }
  return ERR.NUM;
}

type Siete = [number, number, number, number, number, number, number];
type Seis = [number, number, number, number, number, number];

/** Los argumentos de un bono: las fechas sin hora y la frecuencia y la base enteras. Los
 *  números de por medio (tasas, precio, rescate) pasan tal cual. */
function bonoArgs(a: number[]): number[] {
  const out = [...a];
  out[0] = Math.floor(a[0]!);
  out[1] = Math.floor(a[1]!);
  const iFreq = a.length === 7 ? 5 : 4; // PRICE/YIELD llevan un valor más (rescate)
  out[iFreq] = Math.trunc(a[iFreq]!);
  out[iFreq + 1] = Math.trunc(a[iFreq + 1]!);
  return out;
}

/** El bono en la fecha de liquidación según su calendario de cupones: cupones por cobrar (`n`),
 *  días devengados desde el anterior (`a`), del periodo (`e`) y hasta el siguiente (`dsc`).
 *  Es lo que piden PRICE, YIELD, DURATION y MDURATION (ECMA-376), y por eso no se pueden
 *  calcular con «n periodos enteros»: la liquidación casi nunca cae en fecha de cupón. */
interface Bono {
  n: number;
  a: number;
  e: number;
  dsc: number;
}

function bono(settle: number, maturity: number, freq: number, basis: number): Bono | FormulaError {
  if (basis < 0 || basis > 4) return ERR.NUM;
  const k = coupIndex(settle, maturity, freq);
  if (k < 0) return ERR.NUM;
  const a = dayCount(coupDate(maturity, k, freq), settle, basis);
  if (a instanceof FormulaError) return a;
  const dsc = dayCount(settle, coupDate(maturity, k - 1, freq), basis);
  if (dsc instanceof FormulaError) return dsc;
  return { n: k, a, e: coupPeriodDays({ settle, maturity, freq, basis, k }), dsc };
}

/** Precio por 100 de nominal: cupones y rescate descontados a `yld`, menos el cupón devengado. */
function precioBono(b: Bono, rate: number, yld: number, redemption: number, freq: number): number {
  const cpn = (100 * rate) / freq;
  const w = b.dsc / b.e;
  if (b.n === 1) return (redemption + cpn) / (1 + (w * yld) / freq) - cpn * (b.a / b.e);
  let pv = redemption / Math.pow(1 + yld / freq, b.n - 1 + w);
  for (let k = 1; k <= b.n; k++) pv += cpn / Math.pow(1 + yld / freq, k - 1 + w);
  return pv - cpn * (b.a / b.e);
}

/** Duración de Macaulay (años), con el precio sucio (sin restar el devengado). */
function duracionBono(b: Bono, rate: number, yld: number, freq: number): number {
  const cpn = (100 * rate) / freq;
  const w = b.dsc / b.e;
  let ponderado = 0;
  let pv = 0;
  for (let k = 1; k <= b.n; k++) {
    const cf = (k === b.n ? cpn + 100 : cpn) / Math.pow(1 + yld / freq, k - 1 + w);
    ponderado += ((k - 1 + w) / freq) * cf;
    pv += cf;
  }
  return ponderado / pv;
}

/* ---------- Cupones irregulares (ODDF* / ODDL*) ----------
 * Un periodo de cupón irregular (el primero o el último) se mide en «cuasi-cupones»: se
 * prolonga el calendario regular del bono hacia atrás (ODDF, anclado en el primer cupón) o
 * hacia delante (ODDL, anclado en el último interés) y cada tramo se pesa por lo que mide
 * dentro del periodo irregular entre lo que mediría uno regular (DCi/NLi de ECMA-376). Con un
 * solo cuasi-cupón es la fórmula de la documentación de Excel; con varios (periodo largo) es
 * su suma. Con la emisión (o el último interés) un periodo regular antes, da exactamente PRICE.
 */

/** Longitud de un periodo de cupón [a, b]: solo la base 1 la cuenta de verdad. */
function largoCupon(a: number, b: number, freq: number, basis: number): number {
  return basis === 1 ? b - a : (basis === 3 ? 365 : 360) / freq;
}

/** Días entre dos fechas por la base (la base ya está validada: no puede fallar). */
const diasBase = (a: number, b: number, basis: number): number => dayCount(a, b, basis) as number;

/** Lo que de un bono con primer cupón irregular entra en el precio, medido en periodos. */
interface OddF {
  /** Fracción de cupón que paga el primero (Σ DCi/NLi). */
  fc: number;
  /** Cupón devengado hasta la liquidación, en cupones. */
  acc: number;
  /** Periodos que faltan hasta el primer cupón. */
  tfc: number;
  /** Cupones regulares después del primero (el último es el vencimiento). */
  m: number;
}

/** Los argumentos de ODDF*: (liquidación, vencimiento, emisión, primer cupón, tasa, rend/precio,
 *  rescate, frecuencia, [base]). #NUM! si las fechas no cumplen vencimiento > primer cupón >
 *  liquidación > emisión, o si el primer cupón no es una fecha del calendario del vencimiento. */
function oddFlujo(
  a: number[],
): { f: OddF; rate: number; x: number; red: number; freq: number } | FormulaError {
  const [settle, maturity, issue, first] = [0, 1, 2, 3].map((i) => Math.floor(a[i]!)) as Seis4;
  const [rate, x, red] = [a[4]!, a[5]!, a[6]!];
  const [freq, basis] = [Math.trunc(a[7]!), Math.trunc(a[8]!)];
  if (!(maturity > first && first > settle && settle > issue) || issue < 0) return ERR.NUM;
  if (rate < 0 || x < 0 || red <= 0 || !validFreq(freq) || basis < 0 || basis > 4) return ERR.NUM;
  // El primer cupón cae en el calendario del vencimiento: m es cuántos pagos lo siguen.
  let m = -1;
  for (let k = 0; k <= 12_000; k++) {
    const d = coupDate(maturity, k, freq);
    if (d === first) {
      m = k;
      break;
    }
    if (d < first) break;
  }
  if (m < 0) return ERR.NUM;
  const q = [first];
  for (let k = 1; q[0]! > issue; k++) {
    if (k > 12_000) return ERR.NUM;
    q.unshift(coupDate(first, k, freq));
  }
  let fc = 0;
  let acc = 0;
  let tfc = 0;
  for (let i = 1; i < q.length; i++) {
    const [lo, hi] = [q[i - 1]!, q[i]!];
    const nl = largoCupon(lo, hi, freq, basis);
    const desde = Math.max(lo, issue);
    fc += diasBase(desde, hi, basis) / nl;
    const hasta = Math.min(hi, settle);
    if (hasta > desde) acc += diasBase(desde, hasta, basis) / nl;
    const s0 = Math.max(lo, settle);
    if (hi > s0) tfc += diasBase(s0, hi, basis) / nl;
  }
  return { f: { fc, acc, tfc, m }, rate, x, red, freq };
}

type Seis4 = [number, number, number, number];

/** Precio por 100 de un bono con primer cupón irregular, dado el rendimiento. */
function oddFPrecio(f: OddF, rate: number, yld: number, red: number, freq: number): number {
  const c = (100 * rate) / freq;
  const v = 1 + yld / freq;
  let pv = (c * f.fc) / Math.pow(v, f.tfc);
  for (let j = 1; j <= f.m; j++) pv += c / Math.pow(v, f.tfc + j);
  return pv + red / Math.pow(v, f.tfc + f.m) - c * f.acc;
}

/** Lo que de un bono con último cupón irregular entra en el precio, medido en periodos. */
interface OddL {
  /** Fracción de cupón que paga el último (Σ DCi/NLi). */
  fc: number;
  /** Cupón devengado hasta la liquidación, en cupones. */
  fa: number;
  /** Periodos que faltan hasta el vencimiento. */
  tm: number;
}

/** Los argumentos de ODDL*: (liquidación, vencimiento, último interés, tasa, rend/precio,
 *  rescate, frecuencia, [base]). #NUM! si no se cumple vencimiento > liquidación > último interés. */
function oddLFlujo(
  a: number[],
): { f: OddL; rate: number; x: number; red: number; freq: number } | FormulaError {
  const [settle, maturity, last] = [0, 1, 2].map((i) => Math.floor(a[i]!)) as [
    number,
    number,
    number,
  ];
  const [rate, x, red] = [a[3]!, a[4]!, a[5]!];
  const [freq, basis] = [Math.trunc(a[6]!), Math.trunc(a[7]!)];
  if (!(maturity > settle && settle > last) || last < 0) return ERR.NUM;
  if (rate < 0 || x < 0 || red <= 0 || !validFreq(freq) || basis < 0 || basis > 4) return ERR.NUM;
  const q = [last];
  for (let k = 1; q[q.length - 1]! < maturity; k++) {
    if (k > 12_000) return ERR.NUM;
    q.push(coupDate(last, -k, freq));
  }
  let fc = 0;
  let fa = 0;
  let tm = 0;
  for (let i = 1; i < q.length; i++) {
    const [lo, hi0] = [q[i - 1]!, q[i]!];
    const nl = largoCupon(lo, hi0, freq, basis);
    const hi = Math.min(hi0, maturity);
    fc += diasBase(lo, hi, basis) / nl;
    const hasta = Math.min(hi, settle);
    if (hasta > lo) fa += diasBase(lo, hasta, basis) / nl;
    const s0 = Math.max(lo, settle);
    if (hi > s0) tm += diasBase(s0, hi, basis) / nl;
  }
  return { f: { fc, fa, tm }, rate, x, red, freq };
}

/** Precio por 100 de un bono con último cupón irregular, dado el rendimiento. */
function oddLPrecio(f: OddL, rate: number, yld: number, red: number, freq: number): number {
  const c = (100 * rate) / freq;
  return (red + c * f.fc) / (1 + (f.tm * yld) / freq) - c * f.fa;
}

/* ---------- Registro ---------- */

/** Argumentos de una letra del Tesoro (liquidación, vencimiento y un valor: descuento o
 *  precio) y los días que le quedan. #NUM! si vence antes o a más de un año. */
function letra(args: Arg[]): { dias: number; valor: number } | FormulaError {
  const a = nums(args, 3);
  if (a instanceof FormulaError) return a;
  const dias = Math.floor(a[1]!) - Math.floor(a[0]!);
  if (dias <= 0 || dias > 365) return ERR.NUM;
  return { dias, valor: a[2]! };
}

/** Argumentos comunes de AMORLINC y AMORDEGRC; `yf` es la fracción de año del primer periodo. */
function amort(
  args: Arg[],
): { cost: number; salvage: number; per: number; rate: number; yf: number } | FormulaError {
  const a = nums(args, 6, [0]);
  if (a instanceof FormulaError) return a;
  const [cost, bought, first, salvage, per, rate, basis] = a as [
    number,
    number,
    number,
    number,
    number,
    number,
    number,
  ];
  // La base 2 (real/360) no vale para estos dos: Excel da #NUM!.
  if (cost <= 0 || rate <= 0 || salvage < 0 || salvage > cost || per < 0 || Math.trunc(basis) === 2)
    return ERR.NUM;
  if (Math.floor(first) < Math.floor(bought)) return ERR.NUM;
  const yf = yearFrac(Math.floor(bought), Math.floor(first), Math.trunc(basis));
  if (yf instanceof FormulaError) return yf;
  return { cost, salvage, per: Math.trunc(per), rate, yf };
}

export const funcs: FnTable = {
  // Pago periódico de un préstamo/anualidad. PMT(rate, nper, pv, [fv], [type]).
  PMT(_ctx, args) {
    const a = nums(args, 3, [0, 0]);
    if (a instanceof FormulaError) return a;
    if (a[1] === 0) return ERR.DIV0; // nper = 0
    const r = pmtCore(a[0]!, a[1]!, a[2]!, a[3]!, payType(a[4]!));
    return Number.isFinite(r) ? r : ERR.NUM;
  },

  // Valor actual. PV(rate, nper, pmt, [fv], [type]).
  PV(_ctx, args) {
    const a = nums(args, 3, [0, 0]);
    if (a instanceof FormulaError) return a;
    const r = pvCore(a[0]!, a[1]!, a[2]!, a[3]!, payType(a[4]!));
    return Number.isFinite(r) ? r : ERR.NUM;
  },

  // Valor futuro. FV(rate, nper, pmt, [pv], [type]).
  FV(_ctx, args) {
    const a = nums(args, 3, [0, 0]);
    if (a instanceof FormulaError) return a;
    const r = fvCore(a[0]!, a[1]!, a[2]!, a[3]!, payType(a[4]!));
    return Number.isFinite(r) ? r : ERR.NUM;
  },

  // Número de periodos. NPER(rate, pmt, pv, [fv], [type]).
  NPER(_ctx, args) {
    const a = nums(args, 3, [0, 0]);
    if (a instanceof FormulaError) return a;
    const rate = a[0]!;
    const pmt = a[1]!;
    const pv = a[2]!;
    const fv = a[3]!;
    const type = payType(a[4]!);
    if (rate === 0) {
      if (pmt === 0) return ERR.DIV0;
      return -(pv + fv) / pmt;
    }
    const k = (pmt * (1 + rate * type)) / rate;
    const ratio = (k - fv) / (pv + k);
    if (!(ratio > 0)) return ERR.NUM; // log de valor no positivo → sin solución
    const n = Math.log(ratio) / Math.log(1 + rate);
    return Number.isFinite(n) ? n : ERR.NUM;
  },

  // Interés del pago `per`. IPMT(rate, per, nper, pv, [fv], [type]).
  IPMT(_ctx, args) {
    const a = nums(args, 4, [0, 0]);
    if (a instanceof FormulaError) return a;
    const rate = a[0]!;
    const per = a[1]!;
    const nper = a[2]!;
    const type = payType(a[5]!);
    if (per < 1 || per > nper) return ERR.NUM;
    const pmt = pmtCore(rate, nper, a[3]!, a[4]!, type);
    const r = ipmtAt(rate, per, a[3]!, type, pmt);
    return Number.isFinite(r) ? r : ERR.NUM;
  },

  // Principal del pago `per`. PPMT = pago − interés.
  PPMT(_ctx, args) {
    const a = nums(args, 4, [0, 0]);
    if (a instanceof FormulaError) return a;
    const rate = a[0]!;
    const per = a[1]!;
    const nper = a[2]!;
    const type = payType(a[5]!);
    if (per < 1 || per > nper) return ERR.NUM;
    const pmt = pmtCore(rate, nper, a[3]!, a[4]!, type);
    const r = pmt - ipmtAt(rate, per, a[3]!, type, pmt);
    return Number.isFinite(r) ? r : ERR.NUM;
  },

  // Tasa por periodo (iterativa). RATE(nper, pmt, pv, [fv], [type], [guess]).
  RATE(_ctx, args) {
    const a = nums(args, 3, [0, 0, 0.1]);
    if (a instanceof FormulaError) return a;
    const nper = a[0]!;
    const pmt = a[1]!;
    const pv = a[2]!;
    const fv = a[3]!;
    const type = payType(a[4]!);
    const scale = Math.abs(pv) + Math.abs(fv) + Math.abs(pmt) * Math.abs(nper) + 1;
    const f = (rate: number): number => {
      let g: number;
      if (Math.abs(rate) < 1e-12) g = pv + pmt * nper + fv;
      else {
        const p = Math.pow(1 + rate, nper);
        g = pv * p + (pmt * (1 + rate * type) * (p - 1)) / rate + fv;
      }
      return g / scale;
    };
    const root = findRate(f, a[5]!);
    return root === undefined ? ERR.NUM : root;
  },

  // Valor actual neto de flujos periódicos (el primero a UN periodo vista).
  // NPV(rate, valor1, [valor2], …). Texto/blanco/lógico en rangos se ignoran.
  // ponytail: se ignoran también en argumentos escalares literales (=NPV(0,TRUE,1)
  // da 1, no 2); Excel contaría un booleano/número-texto pasado directo. Caso
  // marginal; distinguir literal de rango por arg.ref si algún día importa.
  NPV(_ctx, args) {
    const first = args[0];
    if (!first) return ERR.VALUE;
    const rate = coerceNumber(first.value());
    if (isError(rate)) return rate;
    if (1 + rate === 0) return ERR.DIV0;
    let s = 0;
    let i = 0;
    for (let k = 1; k < args.length; k++) {
      const vals = orderedNums(args[k]!);
      if (vals instanceof FormulaError) return vals;
      for (const v of vals) s += v / Math.pow(1 + rate, ++i);
    }
    return s;
  },

  // Tasa interna de retorno (iterativa). IRR(valores, [estimación]). El primer
  // flujo es en t=0. Necesita al menos un flujo positivo y otro negativo.
  IRR(_ctx, args) {
    const v0 = args[0];
    if (!v0) return ERR.VALUE;
    const vals = orderedNums(v0);
    if (vals instanceof FormulaError) return vals;
    if (vals.length < 2) return ERR.NUM;
    let guess = 0.1;
    const g = args[1];
    if (g) {
      const gv = coerceNumber(g.value());
      if (isError(gv)) return gv;
      guess = gv;
    }
    const scale = vals.reduce((acc, v) => acc + Math.abs(v), 0) || 1;
    const f = (r: number): number => {
      let s = 0;
      for (let i = 0; i < vals.length; i++) s += vals[i]! / Math.pow(1 + r, i);
      return s / scale;
    };
    const root = findRate(f, guess);
    return root === undefined ? ERR.NUM : root;
  },

  // Depreciación lineal. SLN(cost, salvage, life).
  SLN(_ctx, args) {
    const a = nums(args, 3);
    if (a instanceof FormulaError) return a;
    if (a[2] === 0) return ERR.DIV0;
    return (a[0]! - a[1]!) / a[2]!;
  },

  // Depreciación por suma de dígitos de los años. SYD(cost, salvage, life, per).
  SYD(_ctx, args) {
    const a = nums(args, 4);
    if (a instanceof FormulaError) return a;
    const life = a[2]!;
    const per = a[3]!;
    if (life <= 0 || per < 1 || per > life) return ERR.NUM;
    return ((a[0]! - a[1]!) * (life - per + 1) * 2) / (life * (life + 1));
  },

  // Depreciación de saldo fijo decreciente. DB(cost, salvage, life, period, [month]).
  // La tasa se redondea a 3 decimales; el primer y (si month<12) último periodo
  // se prorratean por meses. Se acumula periodo a periodo (sin forma cerrada).
  DB(_ctx, args) {
    const a = nums(args, 4, [12]);
    if (a instanceof FormulaError) return a;
    const cost = a[0]!;
    const salvage = a[1]!;
    const life = a[2]!;
    const period = a[3]!;
    const month = a[4]!;
    if (cost <= 0 || salvage < 0 || life < 1 || period < 1 || month < 1 || month > 12)
      return ERR.NUM;
    if (period > life + 1) return ERR.NUM;
    const rate = Math.round((1 - Math.pow(salvage / cost, 1 / life)) * 1000) / 1000;
    let acc = 0;
    let dep = 0;
    for (let p = 1; p <= period; p++) {
      if (p === 1) dep = (cost * rate * month) / 12;
      else if (p === life + 1) dep = ((cost - acc) * rate * (12 - month)) / 12;
      else dep = (cost - acc) * rate;
      acc += dep;
    }
    return Number.isFinite(dep) ? dep : ERR.NUM;
  },

  // Depreciación de doble saldo decreciente (o factor). DDB(cost, salvage, life,
  // period, [factor=2]). Forma cerrada con tope en el valor de salvamento.
  DDB(_ctx, args) {
    const a = nums(args, 4, [2]);
    if (a instanceof FormulaError) return a;
    const cost = a[0]!;
    const salvage = a[1]!;
    const life = a[2]!;
    const period = a[3]!;
    const factor = a[4]!;
    if (cost < 0 || salvage < 0 || life <= 0 || period < 1 || factor <= 0) return ERR.NUM;
    const f = factor / life;
    // ponytail: si factor ≥ life el primer periodo agota la depreciación.
    if (f >= 1) return period <= 1 ? Math.max(cost - salvage, 0) : 0;
    const bookStart = cost * Math.pow(1 - f, period - 1);
    const bookEnd = cost * Math.pow(1 - f, period);
    const dep = bookEnd < salvage ? Math.max(bookStart - salvage, 0) : bookStart - bookEnd;
    return Number.isFinite(dep) ? dep : ERR.NUM;
  },

  // Amortización decreciente con cambio a lineal. VDB(cost, salvage, life,
  // start, end, [factor=2], [no_switch=FALSE]).
  //
  // Es DDB por tramos, con dos diferencias que son justo lo que la hace útil:
  // los periodos pueden ser FRACCIONARIOS (medio mes, 0,875 de año) y, salvo que
  // se pida lo contrario, cambia a lineal en cuanto la lineal del resto de vida
  // deprecia más que la decreciente —que es lo que hace Hacienda y lo que espera
  // cualquiera que venga de Excel—. Con `no_switch` se queda en decreciente.
  VDB(_ctx, args) {
    const a = nums(args, 5, [2, 0]);
    if (a instanceof FormulaError) return a;
    const cost = a[0]!;
    const salvage = a[1]!;
    const life = a[2]!;
    const start = a[3]!;
    const end = a[4]!;
    const factor = a[5]!;
    const noSwitch = a[6] !== 0;
    if (cost < 0 || salvage < 0 || life <= 0 || factor <= 0) return ERR.NUM;
    if (start < 0 || end < start || end > life) return ERR.NUM;
    let dep: number;
    if (noSwitch) {
      // Decreciente pura: se suman los periodos del tramo, recortando los extremos
      // si start o end son fraccionarios.
      dep = 0;
      const si = Math.floor(start);
      const ei = Math.ceil(end);
      for (let p = si + 1; p <= ei; p++) {
        let d = ddbPeriodo(cost, salvage, life, p, factor);
        if (p - 1 < start) d *= Math.min(1, p - start);
        else if (p > end) d *= end + 1 - p;
        dep += d;
      }
    } else {
      // Con cambio a lineal: primero se deprecia hasta `start`, y el tramo se
      // calcula sobre lo que queda y con la VIDA RESTANTE.
      const si = Math.floor(start);
      const restante = cost - vdbInter(cost, salvage, life, life, si, factor);
      dep = vdbInter(restante, salvage, life, life - si, end - si, factor);
    }
    return Number.isFinite(dep) ? dep : ERR.NUM;
  },

  // Valor actual neto con fechas irregulares. XNPV(rate, valores, fechas).
  // Descuenta cada flujo por (fecha − fecha0)/365 años; fecha0 = primera fecha.
  XNPV(_ctx, args) {
    const r0 = args[0];
    const va = args[1];
    const da = args[2];
    if (!r0 || !va || !da) return ERR.VALUE;
    const rate = coerceNumber(r0.value());
    if (isError(rate)) return rate;
    const vals = flatNums(va);
    if (vals instanceof FormulaError) return vals;
    const dates = flatNums(da);
    if (dates instanceof FormulaError) return dates;
    if (vals.length === 0 || vals.length !== dates.length) return ERR.NUM;
    if (1 + rate <= 0) return ERR.NUM;
    const base = Math.trunc(dates[0]!);
    let s = 0;
    for (let i = 0; i < vals.length; i++) {
      const di = Math.trunc(dates[i]!);
      if (di < base) return ERR.NUM; // fecha anterior a la inicial
      s += vals[i]! / Math.pow(1 + rate, (di - base) / 365);
    }
    return s;
  },

  // TIR con fechas irregulares (iterativa). XIRR(valores, fechas, [estimación]).
  XIRR(_ctx, args) {
    const va = args[0];
    const da = args[1];
    if (!va || !da) return ERR.VALUE;
    const vals = flatNums(va);
    if (vals instanceof FormulaError) return vals;
    const dates = flatNums(da);
    if (dates instanceof FormulaError) return dates;
    if (vals.length < 2 || vals.length !== dates.length) return ERR.NUM;
    let guess = 0.1;
    const g = args[2];
    if (g) {
      const gv = coerceNumber(g.value());
      if (isError(gv)) return gv;
      guess = gv;
    }
    const base = Math.trunc(dates[0]!);
    const days: number[] = [];
    for (let i = 0; i < dates.length; i++) {
      const di = Math.trunc(dates[i]!);
      if (di < base) return ERR.NUM;
      days.push(di - base);
    }
    const scale = vals.reduce((acc, v) => acc + Math.abs(v), 0) || 1;
    const f = (r: number): number => {
      let s = 0;
      for (let i = 0; i < vals.length; i++) s += vals[i]! / Math.pow(1 + r, days[i]! / 365);
      return s / scale;
    };
    const root = findRate(f, guess);
    return root === undefined ? ERR.NUM : root;
  },

  // Interés acumulado entre start y end (ambos inclusive), con fv = 0.
  // CUMIPMT(rate, nper, pv, start, end, type). type es obligatorio.
  CUMIPMT(_ctx, args) {
    const a = nums(args, 6);
    if (a instanceof FormulaError) return a;
    const rate = a[0]!;
    const nper = a[1]!;
    const pv = a[2]!;
    const start = Math.trunc(a[3]!);
    const end = Math.trunc(a[4]!);
    const type = payType(a[5]!);
    if (rate <= 0 || nper <= 0 || pv <= 0) return ERR.NUM;
    if (start < 1 || end < start || end > nper) return ERR.NUM;
    const pmt = pmtCore(rate, nper, pv, 0, type);
    let acc = 0;
    for (let per = start; per <= end; per++) acc += ipmtAt(rate, per, pv, type, pmt);
    return Number.isFinite(acc) ? acc : ERR.NUM;
  },

  // Principal acumulado entre start y end (ambos inclusive), con fv = 0.
  // CUMPRINC(rate, nper, pv, start, end, type). type es obligatorio.
  CUMPRINC(_ctx, args) {
    const a = nums(args, 6);
    if (a instanceof FormulaError) return a;
    const rate = a[0]!;
    const nper = a[1]!;
    const pv = a[2]!;
    const start = Math.trunc(a[3]!);
    const end = Math.trunc(a[4]!);
    const type = payType(a[5]!);
    if (rate <= 0 || nper <= 0 || pv <= 0) return ERR.NUM;
    if (start < 1 || end < start || end > nper) return ERR.NUM;
    const pmt = pmtCore(rate, nper, pv, 0, type);
    let acc = 0;
    for (let per = start; per <= end; per++) acc += pmt - ipmtAt(rate, per, pv, type, pmt);
    return Number.isFinite(acc) ? acc : ERR.NUM;
  },

  /* ---------- Tasas y conversiones ---------- */

  // Periodos para que una inversión alcance un valor. PDURATION(rate, pv, fv).
  PDURATION(_ctx, args) {
    const a = nums(args, 3);
    if (a instanceof FormulaError) return a;
    const [rate, pv, fv] = a as [number, number, number];
    if (rate <= 0 || pv <= 0 || fv <= 0) return ERR.NUM;
    return (Math.log(fv) - Math.log(pv)) / Math.log(1 + rate);
  },

  // Tasa de rendimiento equivalente de un crecimiento. RRI(nper, pv, fv).
  RRI(_ctx, args) {
    const a = nums(args, 3);
    if (a instanceof FormulaError) return a;
    const [nper, pv, fv] = a as [number, number, number];
    if (nper <= 0 || pv === 0) return ERR.NUM;
    const r = Math.pow(fv / pv, 1 / nper) - 1;
    return Number.isFinite(r) ? r : ERR.NUM;
  },

  // Tasa anual efectiva a partir de la nominal. EFFECT(nominal, nº_periodos).
  EFFECT(_ctx, args) {
    const a = nums(args, 2);
    if (a instanceof FormulaError) return a;
    const nom = a[0]!;
    const p = Math.trunc(a[1]!);
    if (nom <= 0 || p < 1) return ERR.NUM;
    return Math.pow(1 + nom / p, p) - 1;
  },

  // Tasa nominal a partir de la efectiva. NOMINAL(efectiva, nº_periodos).
  NOMINAL(_ctx, args) {
    const a = nums(args, 2);
    if (a instanceof FormulaError) return a;
    const eff = a[0]!;
    const p = Math.trunc(a[1]!);
    if (eff <= 0 || p < 1) return ERR.NUM;
    return p * (Math.pow(1 + eff, 1 / p) - 1);
  },

  // Notación fraccionaria → decimal. DOLLARDE(dólar_fracc, fracción).
  DOLLARDE(_ctx, args) {
    const a = nums(args, 2);
    if (a instanceof FormulaError) return a;
    const frac = Math.trunc(a[1]!);
    if (frac < 0) return ERR.NUM;
    if (frac === 0) return ERR.DIV0;
    const sign = a[0]! < 0 ? -1 : 1;
    const abs = Math.abs(a[0]!);
    const intPart = Math.floor(abs);
    const digits = Math.ceil(Math.log(frac) / Math.LN10);
    const dec = ((abs - intPart) * Math.pow(10, digits)) / frac;
    return sign * (intPart + dec);
  },

  // Decimal → notación fraccionaria. DOLLARFR(dólar_dec, fracción).
  DOLLARFR(_ctx, args) {
    const a = nums(args, 2);
    if (a instanceof FormulaError) return a;
    const frac = Math.trunc(a[1]!);
    if (frac < 0) return ERR.NUM;
    if (frac === 0) return ERR.DIV0;
    const sign = a[0]! < 0 ? -1 : 1;
    const abs = Math.abs(a[0]!);
    const intPart = Math.floor(abs);
    const digits = Math.ceil(Math.log(frac) / Math.LN10);
    const fr = ((abs - intPart) * frac) / Math.pow(10, digits);
    return sign * (intPart + fr);
  },

  /* ---------- Bono con cupones ---------- */

  // Precio por 100 de valor nominal. PRICE(liquid, vencim, cupón, rendim, rescate,
  // frecuencia, [base]).
  PRICE(_ctx, args) {
    const a = nums(args, 6, [0]);
    if (a instanceof FormulaError) return a;
    const [settle, maturity, rate, yld, redemption, freq, basis] = bonoArgs(a) as Siete;
    if (maturity <= settle || rate < 0 || yld < 0 || redemption <= 0 || !validFreq(freq))
      return ERR.NUM;
    const b = bono(settle, maturity, freq, basis);
    if (b instanceof FormulaError) return b;
    const p = precioBono(b, rate, yld, redemption, freq);
    return Number.isFinite(p) ? p : ERR.NUM;
  },

  // Rendimiento (iterativo) dado el precio. YIELD(liquid, vencim, cupón, precio,
  // rescate, frecuencia, [base]).
  YIELD(_ctx, args) {
    const a = nums(args, 6, [0]);
    if (a instanceof FormulaError) return a;
    const [settle, maturity, rate, pr, redemption, freq, basis] = bonoArgs(a) as Siete;
    if (maturity <= settle || rate < 0 || pr <= 0 || redemption <= 0 || !validFreq(freq))
      return ERR.NUM;
    const b = bono(settle, maturity, freq, basis);
    if (b instanceof FormulaError) return b;
    const root = findRate((y) => precioBono(b, rate, y, redemption, freq) - pr, 0.05);
    return root === undefined ? ERR.NUM : root;
  },

  // Duración de Macaulay (años). DURATION(liquid, vencim, cupón, rendim, frecuencia,
  // [base]). El rescate se asume a la par (100).
  DURATION(_ctx, args) {
    const a = nums(args, 5, [0]);
    if (a instanceof FormulaError) return a;
    const [settle, maturity, rate, yld, freq, basis] = bonoArgs(a) as Seis;
    if (maturity <= settle || rate < 0 || yld < 0 || !validFreq(freq)) return ERR.NUM;
    const b = bono(settle, maturity, freq, basis);
    if (b instanceof FormulaError) return b;
    const dur = duracionBono(b, rate, yld, freq);
    return Number.isFinite(dur) ? dur : ERR.NUM;
  },

  // Duración modificada (años). MDURATION(liquid, vencim, cupón, rendim, frecuencia,
  // [base]) = DURATION / (1 + rendim/frecuencia).
  MDURATION(_ctx, args) {
    const a = nums(args, 5, [0]);
    if (a instanceof FormulaError) return a;
    const [settle, maturity, rate, yld, freq, basis] = bonoArgs(a) as Seis;
    if (maturity <= settle || rate < 0 || yld < 0 || !validFreq(freq)) return ERR.NUM;
    const b = bono(settle, maturity, freq, basis);
    if (b instanceof FormulaError) return b;
    const md = duracionBono(b, rate, yld, freq) / (1 + yld / freq);
    return Number.isFinite(md) ? md : ERR.NUM;
  },

  // Tasa interna de retorno modificada. MIRR(valores, tasa_financ, tasa_reinv).
  // Los negativos se descuentan a `tasa_financ`; los positivos se capitalizan a
  // `tasa_reinv`. Necesita al menos un flujo positivo y otro negativo.
  MIRR(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const vals = orderedNums(args[0]!);
    if (vals instanceof FormulaError) return vals;
    const fr = coerceNumber(args[1]!.value());
    if (isError(fr)) return fr;
    const rr = coerceNumber(args[2]!.value());
    if (isError(rr)) return rr;
    const n = vals.length;
    if (n < 2) return ERR.DIV0;
    let pvNeg = 0;
    let fvPos = 0;
    let hasPos = false;
    let hasNeg = false;
    for (let i = 0; i < n; i++) {
      const v = vals[i]!;
      if (v < 0) {
        pvNeg += v / Math.pow(1 + fr, i);
        hasNeg = true;
      } else if (v > 0) {
        fvPos += v * Math.pow(1 + rr, n - 1 - i);
        hasPos = true;
      }
    }
    if (!hasPos || !hasNeg) return ERR.DIV0;
    const r = Math.pow(-fvPos / pvNeg, 1 / (n - 1)) - 1;
    return Number.isFinite(r) ? r : ERR.NUM;
  },

  // Tasa de descuento de un valor. DISC(liquid, vencim, precio, rescate, [base]).
  DISC(_ctx, args) {
    const a = nums(args, 4, [0]);
    if (a instanceof FormulaError) return a;
    const [settle, maturity, pr, redemption, basis] = a as [number, number, number, number, number];
    if (maturity <= settle || pr <= 0 || redemption <= 0) return ERR.NUM;
    const yf = yearFrac(Math.floor(settle), Math.floor(maturity), Math.trunc(basis));
    if (yf instanceof FormulaError) return yf;
    if (yf === 0) return ERR.NUM;
    return (redemption - pr) / redemption / yf;
  },

  // Tasa de interés de un valor totalmente invertido. INTRATE(liquid, vencim,
  // inversión, rescate, [base]).
  INTRATE(_ctx, args) {
    const a = nums(args, 4, [0]);
    if (a instanceof FormulaError) return a;
    const [settle, maturity, investment, redemption, basis] = a as [
      number,
      number,
      number,
      number,
      number,
    ];
    if (maturity <= settle || investment <= 0 || redemption <= 0) return ERR.NUM;
    const yf = yearFrac(Math.floor(settle), Math.floor(maturity), Math.trunc(basis));
    if (yf instanceof FormulaError) return yf;
    if (yf === 0) return ERR.NUM;
    return (redemption - investment) / investment / yf;
  },

  // Cantidad recibida al vencimiento de un valor con descuento. RECEIVED(liquid,
  // vencim, inversión, descuento, [base]).
  RECEIVED(_ctx, args) {
    const a = nums(args, 4, [0]);
    if (a instanceof FormulaError) return a;
    const [settle, maturity, investment, discount, basis] = a as [
      number,
      number,
      number,
      number,
      number,
    ];
    if (maturity <= settle || investment <= 0 || discount <= 0) return ERR.NUM;
    const yf = yearFrac(Math.floor(settle), Math.floor(maturity), Math.trunc(basis));
    if (yf instanceof FormulaError) return yf;
    const denom = 1 - discount * yf;
    if (denom <= 0) return ERR.NUM;
    return investment / denom;
  },

  // Precio por 100 de un valor con descuento (letra del Tesoro, pagaré…).
  // PRICEDISC(liquid, vencim, descuento, rescate, [base]).
  PRICEDISC(_ctx, args) {
    const a = nums(args, 4, [0]);
    if (a instanceof FormulaError) return a;
    const settle = Math.floor(a[0]!);
    const maturity = Math.floor(a[1]!);
    const [, , discount, redemption, basis] = a as [number, number, number, number, number];
    if (maturity <= settle || discount <= 0 || redemption <= 0) return ERR.NUM;
    const yf = yearFrac(settle, maturity, Math.trunc(basis));
    if (yf instanceof FormulaError) return yf;
    return redemption - discount * redemption * yf;
  },

  // Rendimiento anual de un valor con descuento.
  // YIELDDISC(liquid, vencim, precio, rescate, [base]).
  YIELDDISC(_ctx, args) {
    const a = nums(args, 4, [0]);
    if (a instanceof FormulaError) return a;
    const settle = Math.floor(a[0]!);
    const maturity = Math.floor(a[1]!);
    const [, , pr, redemption, basis] = a as [number, number, number, number, number];
    if (maturity <= settle || pr <= 0 || redemption <= 0) return ERR.NUM;
    const yf = yearFrac(settle, maturity, Math.trunc(basis));
    if (yf instanceof FormulaError) return yf;
    if (yf === 0) return ERR.NUM;
    return (redemption - pr) / pr / yf;
  },

  /* ---------- Fechas de cupón ---------- */

  // Fecha del cupón ANTERIOR a la liquidación. COUPPCD(liquid, vencim, frec, [base]).
  COUPPCD(_ctx, args) {
    const c = coupArgs(args);
    if (c instanceof FormulaError) return c;
    return coupDate(c.maturity, c.k, c.freq);
  },

  // Fecha del cupón SIGUIENTE a la liquidación. COUPNCD(liquid, vencim, frec, [base]).
  COUPNCD(_ctx, args) {
    const c = coupArgs(args);
    if (c instanceof FormulaError) return c;
    return coupDate(c.maturity, c.k - 1, c.freq);
  },

  // Cupones pendientes entre la liquidación y el vencimiento (ambos contados
  // como uno solo si la liquidación cae justo en un cupón).
  COUPNUM(_ctx, args) {
    const c = coupArgs(args);
    if (c instanceof FormulaError) return c;
    return c.k;
  },

  // Días del periodo de cupón que contiene la liquidación.
  COUPDAYS(_ctx, args) {
    const c = coupArgs(args);
    if (c instanceof FormulaError) return c;
    return coupPeriodDays(c);
  },

  // Días transcurridos desde el cupón anterior hasta la liquidación: es el
  // devengado que el comprador paga aparte del precio.
  COUPDAYBS(_ctx, args) {
    const c = coupArgs(args);
    if (c instanceof FormulaError) return c;
    return dayCount(coupDate(c.maturity, c.k, c.freq), c.settle, c.basis);
  },

  // Días desde la liquidación hasta el cupón siguiente.
  COUPDAYSNC(_ctx, args) {
    const c = coupArgs(args);
    if (c instanceof FormulaError) return c;
    return dayCount(c.settle, coupDate(c.maturity, c.k - 1, c.freq), c.basis);
  },

  /* ---------- Intereses devengados ---------- */

  // Interés devengado de un valor con pagos periódicos. ACCRINT(emisión,
  // primer_interés, liquid, tasa, nominal, frecuencia, [base], [método_calculo]).
  // `método_calculo` FALSO devenga desde el primer_interés en vez de desde la
  // emisión, y SOLO cuenta si la liquidación es posterior al primer interés
  // (con método FALSO y liquidación anterior, Excel devenga igualmente desde la
  // emisión: sin ese matiz el resultado sale a cero).
  ACCRINT(_ctx, args) {
    const a = nums(args, 6, [0, 1]);
    if (a instanceof FormulaError) return a;
    const issue = Math.floor(a[0]!);
    const firstInterest = Math.floor(a[1]!);
    const settle = Math.floor(a[2]!);
    const rate = a[3]!;
    const par = a[4]!;
    const freq = Math.trunc(a[5]!);
    const basis = Math.trunc(a[6]!);
    if (settle <= issue || rate <= 0 || par <= 0 || !validFreq(freq)) return ERR.NUM;
    if (basis < 0 || basis > 4) return ERR.NUM;
    const fromFirst = a[7]! === 0 && settle > firstInterest;
    const start = fromFirst ? firstInterest : issue;
    const coupons = accruedCoupons(start, settle, firstInterest, freq, basis);
    if (coupons instanceof FormulaError) return coupons;
    return (par * rate * coupons) / freq;
  },

  // Interés devengado de un valor que paga TODO al vencimiento (sin cupones, así
  // que no hay calendario: es la fracción de año pura).
  // ACCRINTM(emisión, liquid, tasa, nominal, [base]).
  ACCRINTM(_ctx, args) {
    const a = nums(args, 4, [0]);
    if (a instanceof FormulaError) return a;
    const issue = Math.floor(a[0]!);
    const settle = Math.floor(a[1]!);
    const [, , rate, par, basis] = a as [number, number, number, number, number];
    if (settle <= issue || rate <= 0 || par <= 0) return ERR.NUM;
    const yf = yearFrac(issue, settle, Math.trunc(basis));
    if (yf instanceof FormulaError) return yf;
    return par * rate * yf;
  },

  // Valor futuro de un capital con una SERIE de tasas de interés compuesto.
  // FVSCHEDULE(capital, tasas): capital × Π(1 + tasaᵢ). Un blanco es tasa 0; texto, #VALUE!.
  FVSCHEDULE(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const principal = coerceNumber(args[0]!.value());
    if (isError(principal)) return principal;
    let r = principal;
    for (const row of args[1]!.matrix()) {
      for (const v of row) {
        if (isError(v)) return v;
        if (v === '') continue;
        const t = coerceNumber(v);
        if (isError(t)) return t;
        r *= 1 + t;
      }
    }
    return Number.isFinite(r) ? r : ERR.NUM;
  },

  // Letras del Tesoro: los tres se definen con los días entre liquidación y vencimiento
  // (ni uno más de un año) sobre un año comercial de 360.
  // TBILLPRICE(liquid, vencim, descuento) = 100 × (1 − descuento × días / 360).
  TBILLPRICE(_ctx, args) {
    const t = letra(args);
    if (t instanceof FormulaError) return t;
    if (t.valor <= 0) return ERR.NUM;
    return 100 * (1 - (t.valor * t.dias) / 360);
  },
  // TBILLYIELD(liquid, vencim, precio) = (100 − precio) / precio × 360 / días.
  TBILLYIELD(_ctx, args) {
    const t = letra(args);
    if (t instanceof FormulaError) return t;
    if (t.valor <= 0) return ERR.NUM;
    return ((100 - t.valor) / t.valor) * (360 / t.dias);
  },
  // TBILLEQ(liquid, vencim, descuento): rendimiento equivalente a un bono (año de 365).
  // Hasta medio año es 365·d / (360 − d·días); más allá hay que despejar el rendimiento de
  // un cupón semestral implícito, que sale de una ecuación de segundo grado.
  TBILLEQ(_ctx, args) {
    const t = letra(args);
    if (t instanceof FormulaError) return t;
    const d = t.valor;
    if (d <= 0) return ERR.NUM;
    if (t.dias <= 182) {
      const den = 360 - d * t.dias;
      return den <= 0 ? ERR.NUM : (365 * d) / den;
    }
    const precio = 100 * (1 - (d * t.dias) / 360);
    if (precio <= 0) return ERR.NUM;
    const a = t.dias / 730 - 0.25;
    const b = t.dias / 365;
    const c = (precio - 100) / precio;
    const disc = b * b - 4 * a * c;
    if (disc < 0) return ERR.NUM;
    const y = (-b + Math.sqrt(disc)) / (2 * a);
    return Number.isFinite(y) ? y : ERR.NUM;
  },

  // Precio de un bono con primer cupón irregular (corto o largo).
  // ODDFPRICE(liquid, vencim, emisión, 1.er cupón, tasa, rend, rescate, frecuencia, [base]).
  ODDFPRICE(_ctx, args) {
    const a = nums(args, 8, [0]);
    if (a instanceof FormulaError) return a;
    const o = oddFlujo(a);
    if (o instanceof FormulaError) return o;
    const p = oddFPrecio(o.f, o.rate, o.x, o.red, o.freq);
    return Number.isFinite(p) ? p : ERR.NUM;
  },
  // Rendimiento de un bono con primer cupón irregular, dado su precio.
  ODDFYIELD(_ctx, args) {
    const a = nums(args, 8, [0]);
    if (a instanceof FormulaError) return a;
    const o = oddFlujo(a);
    if (o instanceof FormulaError) return o;
    if (o.x <= 0) return ERR.NUM;
    const root = findRate((y) => oddFPrecio(o.f, o.rate, y, o.red, o.freq) - o.x, o.rate || 0.05);
    return root === undefined ? ERR.NUM : root;
  },
  // Precio de un bono con último cupón irregular.
  // ODDLPRICE(liquid, vencim, último interés, tasa, rend, rescate, frecuencia, [base]).
  ODDLPRICE(_ctx, args) {
    const a = nums(args, 7, [0]);
    if (a instanceof FormulaError) return a;
    const o = oddLFlujo(a);
    if (o instanceof FormulaError) return o;
    const p = oddLPrecio(o.f, o.rate, o.x, o.red, o.freq);
    return Number.isFinite(p) ? p : ERR.NUM;
  },
  // Rendimiento de un bono con último cupón irregular, dado su precio. El precio es lineal en
  // 1/(1 + tm·y/f), así que el rendimiento sale en forma cerrada.
  ODDLYIELD(_ctx, args) {
    const a = nums(args, 7, [0]);
    if (a instanceof FormulaError) return a;
    const o = oddLFlujo(a);
    if (o instanceof FormulaError) return o;
    if (o.x <= 0) return ERR.NUM;
    const c = (100 * o.rate) / o.freq;
    const y = (((o.red + c * o.f.fc) / (o.x + c * o.f.fa) - 1) * o.freq) / o.f.tm;
    return Number.isFinite(y) ? y : ERR.NUM;
  },

  // Precio por 100 de un valor que paga TODO al vencimiento, con interés.
  // PRICEMAT(liquid, vencim, emisión, tasa, rend, [base]).
  PRICEMAT(_ctx, args) {
    const a = nums(args, 5, [0]);
    if (a instanceof FormulaError) return a;
    const [settle, maturity, issue] = [Math.floor(a[0]!), Math.floor(a[1]!), Math.floor(a[2]!)];
    const [rate, yld, basis] = [a[3]!, a[4]!, Math.trunc(a[5]!)];
    if (maturity <= settle || issue > settle || rate < 0 || yld < 0) return ERR.NUM;
    const fim = yearFrac(issue, maturity, basis);
    if (fim instanceof FormulaError) return fim;
    const fsm = yearFrac(settle, maturity, basis);
    if (fsm instanceof FormulaError) return fsm;
    const fis = yearFrac(issue, settle, basis);
    if (fis instanceof FormulaError) return fis;
    return (100 + fim * rate * 100) / (1 + fsm * yld) - fis * rate * 100;
  },
  // Rendimiento anual de un valor que paga todo al vencimiento, con interés.
  // YIELDMAT(liquid, vencim, emisión, tasa, precio, [base]).
  YIELDMAT(_ctx, args) {
    const a = nums(args, 5, [0]);
    if (a instanceof FormulaError) return a;
    const [settle, maturity, issue] = [Math.floor(a[0]!), Math.floor(a[1]!), Math.floor(a[2]!)];
    const [rate, pr, basis] = [a[3]!, a[4]!, Math.trunc(a[5]!)];
    if (maturity <= settle || issue > settle || rate < 0 || pr <= 0) return ERR.NUM;
    const fim = yearFrac(issue, maturity, basis);
    if (fim instanceof FormulaError) return fim;
    const fsm = yearFrac(settle, maturity, basis);
    if (fsm instanceof FormulaError) return fsm;
    const fis = yearFrac(issue, settle, basis);
    if (fis instanceof FormulaError) return fis;
    if (fsm === 0) return ERR.NUM;
    const pagado = pr / 100 + fis * rate;
    return (1 + fim * rate - pagado) / pagado / fsm;
  },

  // Depreciación LINEAL por periodo contable (primer periodo prorrateado, el último
  // con lo que quede). AMORLINC(coste, compra, fin 1er periodo, residual, periodo, tasa, [base]).
  AMORLINC(_ctx, args) {
    const a = amort(args);
    if (a instanceof FormulaError) return a;
    const { cost, salvage, per, rate, yf } = a;
    const anual = cost * rate;
    const primero = yf * anual;
    const completos = Math.floor((cost - salvage - primero) / anual);
    if (per === 0) return primero;
    if (per <= completos) return anual;
    if (per === completos + 1) return cost - salvage - anual * completos - primero;
    return 0;
  },
  // Depreciación DEGRESIVA por periodo contable, en importes enteros. La tasa se multiplica por
  // un coeficiente según la vida del bien (1/tasa): <3 años 1, <5 1,5, ≤6 2, más 2,5. Los dos
  // últimos periodos se reparten lo que queda: la mitad del coste en el penúltimo y 0 después.
  // AMORDEGRC(coste, compra, fin 1er periodo, residual, periodo, tasa, [base]).
  AMORDEGRC(_ctx, args) {
    const a = amort(args);
    if (a instanceof FormulaError) return a;
    const { salvage, per, yf } = a;
    let { cost } = a;
    const vida = 1 / a.rate;
    const coef = vida < 3 ? 1 : vida < 5 ? 1.5 : vida <= 6 ? 2 : 2.5;
    const rate = a.rate * coef;
    let cuota = Math.floor(yf * rate * cost + 0.5);
    cost -= cuota;
    let resto = cost - salvage;
    for (let n = 0; n < per; n++) {
      cuota = Math.floor(rate * cost + 0.5);
      resto -= cuota;
      if (resto < 0) return per - n === 1 ? Math.floor(cost * 0.5 + 0.5) : 0;
      cost -= cuota;
    }
    return cuota;
  },

  // Interés del periodo `per` en un préstamo de AMORTIZACIÓN CONSTANTE (el
  // principal se devuelve a partes iguales), al revés que IPMT, que asume cuota
  // constante. ISPMT(tasa, per, nper, va). Devuelve negativo: es un pago.
  ISPMT(_ctx, args) {
    const a = nums(args, 4);
    if (a instanceof FormulaError) return a;
    const [rate, per, nper, pv] = a as [number, number, number, number];
    if (nper === 0) return ERR.DIV0;
    return pv * rate * (per / nper - 1);
  },
};
