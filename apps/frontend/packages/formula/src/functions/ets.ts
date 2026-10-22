/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Suavizado exponencial triple (Holt-Winters): FORECAST.ETS y su familia
 * (`.ADD`, `.MULT`, `.CONFINT` = `.PI.ADD`, `.PI.MULT`, `.SEASONALITY`, `.STAT`).
 * Implementación ORIGINAL escrita desde el algoritmo publicado (Hyndman et al., «Forecasting
 * with Exponential Smoothing», 2008) y las reglas de datos de la documentación de Microsoft;
 * se ha contrastado con LibreOffice (compat/ets-esperado-libreoffice.json) y NO comparte
 * código con él. Espejo en Go: apps/nucleo/formula/fn_ets.go; los dos dan lo mismo (vectores
 * comunes en packages/office/test/vectores/calcus.json).
 *
 * El modelo, con nivel l, tendencia b y estación s de longitud m (α, β, γ en [0, 1]):
 *   aditivo         l = α(y − s) + (1−α)(l+b)      s = γ(y − l − b) + (1−γ)s       ŷ = l + h·b + s
 *   multiplicativo  l = α(y / s) + (1−α)(l+b)      s = γ(y / (l+b)) + (1−γ)s       ŷ = (l + h·b)·s
 *   b = β(l − l_ant) + (1−β)b en los dos; sin estacionalidad (m = 1) es el de Holt.
 * Los parámetros minimizan la suma de los cuadrados de los errores a un paso: una rejilla
 * de 0,1 y una búsqueda por patrones que afina hasta 1e-4. Solo suma, resta, producto,
 * cociente y raíz cuadrada: el espejo de Go da los mismos números.
 *
 * Reglas de datos (Microsoft): la línea de tiempo no tiene por qué venir ordenada, pero sí
 * tener un paso constante (los huecos se completan); los instantes repetidos se agregan; el
 * objetivo ha de ser posterior al final del histórico.
 */
import type { Arg, FnTable } from '../registry.js';
import { coerceNumber, ERR, FormulaError, isError, type CellValue } from '../values.js';
import { ymdFromSerial } from './datetime.js';
import { normInv } from './stats.js';

/** Periodo estacional máximo que admite Excel. */
const MAX_PERIODO = 8760;
/** Puntos máximos de la serie regular (la línea de tiempo completada): tope de memoria. */
const MAX_PUNTOS = 1_000_000;

/** La serie ya regular: un valor por paso de la línea de tiempo. */
interface Serie {
  y: number[];
  /** Paso de la línea de tiempo (en meses si `mensual`). */
  paso: number;
  /** Origen: el primer instante (o su índice de mes si `mensual`). */
  origen: number;
  /** ¿La línea de tiempo avanza por meses (el día del mes es el mismo) y no por días? */
  mensual: boolean;
  /** Día del mes de todos los puntos (0 si todos son fin de mes). */
  dia: number;
  /** Instante del último punto (el serial del día en modo normal). */
  ultimo: number;
}

const esEntero = (x: number): boolean => Number.isInteger(x);

function mediana(v: number[]): number {
  const s = [...v].sort((a, b) => a - b);
  const m = s.length >> 1;
  return s.length % 2 ? s[m]! : (s[m - 1]! + s[m]!) / 2;
}

/** Agrega los valores de un mismo instante: 0 y 1 media, 2 y 3 cuenta, 4 máximo, 5 mediana,
 *  6 mínimo, 7 suma (el orden de LibreOffice; Microsoft solo da la lista sin numerar). */
function agregar(v: number[], modo: number): number {
  switch (modo) {
    case 2:
    case 3:
      return v.length;
    case 4:
      return Math.max(...v);
    case 5:
      return mediana(v);
    case 6:
      return Math.min(...v);
    case 7:
      return v.reduce((a, b) => a + b, 0);
    default:
      return v.reduce((a, b) => a + b, 0) / v.length;
  }
}

/** Índice de mes (años·12 + mes) de un serial entero, y si es el último día del mes. */
function mesDe(serial: number): { mes: number; dia: number; fin: boolean } {
  const f = ymdFromSerial(serial);
  const sig = ymdFromSerial(serial + 1);
  return { mes: f.y * 12 + f.m - 1, dia: f.d, fin: sig.d === 1 };
}

/**
 * Convierte (línea de tiempo, valores) en una serie regular. Devuelve un error de Excel si no
 * se puede: tamaños distintos (#N/A), texto (#VALUE!), menos de tres instantes o paso no
 * constante (#NUM!).
 */
function preparar(
  vals: CellValue[],
  tl: CellValue[],
  completar: boolean,
  agregacion: number,
): Serie | FormulaError {
  if (vals.length !== tl.length) return ERR.NA;
  const pares: [number, number][] = [];
  for (let i = 0; i < tl.length; i++) {
    const x = tl[i]!;
    const v = vals[i]!;
    if (isError(x)) return x;
    if (isError(v)) return v;
    if (typeof x !== 'number') return ERR.VALUE;
    if (v === '') continue; // celda vacía: punto que falta, lo completa data_completion
    if (typeof v !== 'number') return ERR.VALUE;
    pares.push([x, v]);
  }
  pares.sort((a, b) => a[0] - b[0]);
  // Los instantes repetidos se agregan.
  const xs: number[] = [];
  const ys: number[] = [];
  for (let i = 0; i < pares.length;) {
    let j = i;
    const grupo: number[] = [];
    while (j < pares.length && pares[j]![0] === pares[i]![0]) grupo.push(pares[j++]![1]);
    xs.push(pares[i]![0]);
    ys.push(agregar(grupo, agregacion));
    i = j;
  }
  if (xs.length < 3) return ERR.NUM;

  // El paso es la menor diferencia; el resto han de ser múltiplos enteros suyos.
  let paso = Infinity;
  for (let i = 1; i < xs.length; i++) paso = Math.min(paso, xs[i]! - xs[i - 1]!);
  const multiplos = (p: number, v: number[]): boolean =>
    v.every((d) => {
      const q = d / p;
      return Math.abs(q - Math.round(q)) <= 1e-6 * Math.max(1, q);
    });
  const difs: number[] = [];
  for (let i = 1; i < xs.length; i++) difs.push(xs[i]! - xs[i - 1]!);

  let origen = xs[0]!;
  let mensual = false;
  let dia = 0;
  let posiciones: number[];
  if (multiplos(paso, difs)) {
    posiciones = xs.map((x) => Math.round((x - origen) / paso));
  } else {
    // Pasos de un mes (o de varios): los días de cada mes difieren, pero el día del mes no.
    // ponytail: la línea mensual exige el mismo día del mes (o fin de mes) en todos los puntos.
    if (!xs.every(esEntero)) return ERR.NUM;
    const ms = xs.map(mesDe);
    const finDeMes = ms.every((m) => m.fin);
    if (!finDeMes && !ms.every((m) => m.dia === ms[0]!.dia)) return ERR.NUM;
    const idx = ms.map((m) => m.mes);
    const dm: number[] = [];
    for (let i = 1; i < idx.length; i++) dm.push(idx[i]! - idx[i - 1]!);
    paso = Math.min(...dm);
    if (paso < 1 || !multiplos(paso, dm)) return ERR.NUM;
    mensual = true;
    dia = finDeMes ? 0 : ms[0]!.dia;
    origen = idx[0]!;
    posiciones = idx.map((m) => Math.round((m - origen) / paso));
  }
  const n = posiciones[posiciones.length - 1]! + 1;
  if (n > MAX_PUNTOS) return ERR.NUM;
  const y = new Array<number>(n).fill(0);
  const hay = new Array<boolean>(n).fill(false);
  posiciones.forEach((p, i) => {
    y[p] = ys[i]!;
    hay[p] = true;
  });
  if (completar) {
    // Los huecos, por interpolación lineal entre los vecinos con dato.
    let ant = 0;
    for (let p = 1; p < n; p++) {
      if (!hay[p]) continue;
      for (let k = ant + 1; k < p; k++)
        y[k] = y[ant]! + ((y[p]! - y[ant]!) * (k - ant)) / (p - ant);
      ant = p;
    }
  }
  return { y, paso, origen, mensual, dia, ultimo: xs[xs.length - 1]! };
}

/** Pasos que separan el objetivo del último punto (puede ser fraccionario). */
function pasosHasta(s: Serie, objetivo: number): number {
  const n = s.y.length;
  if (!s.mensual) return (objetivo - s.ultimo) / s.paso;
  const ult = s.origen + (n - 1) * s.paso;
  const t = mesDe(Math.floor(objetivo));
  // El día del mes del objetivo desplaza fracciones de mes de 30 días.
  const fr = s.dia === 0 ? (t.fin ? 0 : (t.dia - 31) / 30) : (t.dia - s.dia) / 30;
  return (t.mes - ult + fr) / s.paso;
}

/* ---------- el modelo ---------- */

interface Ajuste {
  alfa: number;
  beta: number;
  gamma: number;
  /** Suma de cuadrados de los errores a un paso (los que quedan tras inicializar). */
  sse: number;
  /** Cuántos errores entran. */
  cuenta: number;
  mae: number;
  smape: number;
  nivel: number;
  tend: number;
  /** Últimos m coeficientes estacionales, en el orden en que se aplican (el primero, a h = 1). */
  est: number[];
}

/**
 * Pasa el modelo por la serie con (α, β, γ) y devuelve el SSE (Infinity si el multiplicativo
 * se sale del dominio). Con `completo` rellena también los coeficientes finales y las medidas.
 */
function recorrer(
  y: number[],
  m: number,
  mult: boolean,
  alfa: number,
  beta: number,
  gamma: number,
  completo: boolean,
): { sse: number; aj?: Ajuste } {
  const n = y.length;
  let nivel: number;
  let tend: number;
  const s = new Array<number>(Math.max(m, 1)).fill(0);
  let t0: number;
  if (m > 1) {
    let m1 = 0;
    let m2 = 0;
    for (let i = 0; i < m; i++) {
      m1 += y[i]!;
      m2 += y[m + i]!;
    }
    m1 /= m;
    m2 /= m;
    tend = (m2 - m1) / m;
    nivel = m1 - (tend * (m + 1)) / 2;
    // La estación inicial: la media de los dos primeros periodos, sin tendencia.
    for (let i = 0; i < m; i++) {
      const b1 = nivel + tend * (i + 1);
      const b2 = nivel + tend * (i + 1 + m);
      if (mult) {
        if (!(b1 > 0) || !(b2 > 0)) return { sse: Infinity };
        s[i] = (y[i]! / b1 + y[m + i]! / b2) / 2;
      } else s[i] = (y[i]! - b1 + (y[m + i]! - b2)) / 2;
    }
    t0 = 0;
  } else {
    const k = Math.min(n - 1, 4);
    nivel = y[0]!;
    tend = (y[k]! - y[0]!) / k;
    t0 = 1;
  }
  let sse = 0;
  let cuenta = 0;
  let sabs = 0;
  let ssmape = 0;
  const desde = m > 1 ? m : t0;
  for (let t = t0; t < n; t++) {
    const base = nivel + tend;
    const j = m > 1 ? t % m : 0;
    let pred: number;
    if (m > 1) pred = mult ? base * s[j]! : base + s[j]!;
    else pred = base;
    const e = y[t]! - pred;
    if (t >= desde) {
      sse += e * e;
      cuenta++;
      if (completo) {
        sabs += Math.abs(e);
        const d = (Math.abs(y[t]!) + Math.abs(pred)) / 2;
        if (d > 0) ssmape += Math.abs(e) / d;
      }
    }
    let nuevoNivel: number;
    if (m > 1) {
      if (mult) {
        if (!(s[j]! > 0) || !(base > 0)) return { sse: Infinity };
        nuevoNivel = alfa * (y[t]! / s[j]!) + (1 - alfa) * base;
        s[j] = gamma * (y[t]! / base) + (1 - gamma) * s[j]!;
      } else {
        nuevoNivel = alfa * (y[t]! - s[j]!) + (1 - alfa) * base;
        s[j] = gamma * (y[t]! - base) + (1 - gamma) * s[j]!;
      }
    } else nuevoNivel = alfa * y[t]! + (1 - alfa) * base;
    tend = beta * (nuevoNivel - nivel) + (1 - beta) * tend;
    nivel = nuevoNivel;
    if (!Number.isFinite(sse)) return { sse: Infinity };
  }
  if (!completo) return { sse };
  // Los coeficientes en el orden de uso: el que sigue a la última observación va primero.
  const est: number[] = [];
  if (m > 1) for (let i = 0; i < m; i++) est.push(s[(n + i) % m]!);
  return {
    sse,
    aj: {
      alfa,
      beta,
      gamma,
      sse,
      cuenta,
      mae: sabs / cuenta,
      smape: ssmape / cuenta,
      nivel,
      tend,
      est,
    },
  };
}

/** Rejilla de 0,1 y búsqueda por patrones: los parámetros de menor SSE. */
function ajustar(y: number[], m: number, mult: boolean): Ajuste | FormulaError {
  const dim = m > 1 ? 3 : 2;
  const p = [0, 0, 0];
  const costeDe = (q: number[]): number => recorrer(y, m, mult, q[0]!, q[1]!, q[2]!, false).sse;
  let mejor = Infinity;
  const q = [0, 0, 0];
  const pasos = 11;
  const total = pasos ** dim;
  for (let c = 0; c < total; c++) {
    q[0] = (c % pasos) / 10;
    q[1] = (Math.floor(c / pasos) % pasos) / 10;
    q[2] = dim === 3 ? Math.floor(c / (pasos * pasos)) / 10 : 0;
    const e = costeDe(q);
    if (e < mejor) {
      mejor = e;
      p[0] = q[0];
      p[1] = q[1];
      p[2] = q[2];
    }
  }
  if (!Number.isFinite(mejor)) return ERR.NUM;
  for (let paso = 0.05; paso >= 1e-4; paso /= 2) {
    for (let vuelta = 0; vuelta < 100; vuelta++) {
      let mejora = false;
      for (let k = 0; k < dim; k++) {
        for (const signo of [1, -1]) {
          const c = [p[0]!, p[1]!, p[2]!];
          c[k] = Math.min(1, Math.max(0, c[k]! + signo * paso));
          if (c[k] === p[k]) continue;
          const e = costeDe(c);
          if (e < mejor) {
            mejor = e;
            p[k] = c[k]!;
            mejora = true;
          }
        }
      }
      if (!mejora) break;
    }
  }
  const r = recorrer(y, m, mult, p[0]!, p[1]!, p[2]!, true);
  return r.aj ?? ERR.NUM;
}

/** El periodo estacional por autocorrelación de lo que queda tras quitar la recta de
 *  regresión: el primer máximo local, de al menos el 90 % del mayor, que supera la banda de
 *  confianza del ruido blanco (2/√n). 1 si no hay estacionalidad. */
function detectarPeriodo(y: number[]): number {
  const n = y.length;
  if (n < 6) return 1;
  let sx = 0;
  let sy = 0;
  for (let i = 0; i < n; i++) {
    sx += i;
    sy += y[i]!;
  }
  const mx = sx / n;
  const my = sy / n;
  let sxx = 0;
  let sxy = 0;
  for (let i = 0; i < n; i++) {
    sxx += (i - mx) * (i - mx);
    sxy += (i - mx) * (y[i]! - my);
  }
  const b = sxy / sxx;
  const r = y.map((v, i) => v - my - b * (i - mx));
  let e0 = 0;
  for (const v of r) e0 += v * v;
  if (!(e0 > 1e-12 * n * (Math.abs(my) + 1) ** 2)) return 1;
  const maxLag = n >> 1;
  const rho = new Array<number>(maxLag + 2).fill(0);
  for (let k = 1; k <= maxLag + 1; k++) {
    let c = 0;
    for (let t = 0; t + k < n; t++) c += r[t]! * r[t + k]!;
    rho[k] = c / (n - k) / (e0 / n);
  }
  const umbral = 2 / Math.sqrt(n);
  let maximo = -Infinity;
  const picos: number[] = [];
  for (let k = 2; k <= maxLag; k++) {
    if (rho[k]! > rho[k - 1]! && rho[k]! >= rho[k + 1]! && rho[k]! > umbral) {
      picos.push(k);
      maximo = Math.max(maximo, rho[k]!);
    }
  }
  for (const k of picos) if (rho[k]! >= 0.9 * maximo) return k;
  return 1;
}

/* ---------- argumentos ---------- */

interface Opciones {
  completar: boolean;
  agregacion: number;
}

function planos(a: Arg): CellValue[] {
  const out: CellValue[] = [];
  for (const fila of a.matrix()) for (const v of fila) out.push(v);
  return out;
}

/** Un argumento numérico opcional: `def` si falta, el error si no es número. */
function opc(args: Arg[], i: number, def: number): number | FormulaError {
  if (i >= args.length) return def;
  const v = args[i]!.value();
  // Un argumento omitido con coma («,,») llega vacío: el valor por defecto.
  if (v === '') return def;
  return coerceNumber(v);
}

/** data_completion (0/1) y aggregation (0..7): lo comparten las cinco funciones. */
function opciones(args: Arg[], iComp: number): Opciones | FormulaError {
  const c = opc(args, iComp, 1);
  if (isError(c)) return c;
  const g = opc(args, iComp + 1, 0);
  if (isError(g)) return g;
  const ci = Math.trunc(c);
  const gi = Math.trunc(g);
  if ((ci !== 0 && ci !== 1) || gi < 0 || gi > 7) return ERR.NUM;
  return { completar: ci === 1, agregacion: gi };
}

/** El periodo pedido (0 ninguno, 1 automático, 2.. fijo) → el que se usa. */
function periodo(y: number[], pedido: number, mult: boolean): number | FormulaError {
  const p = Math.trunc(pedido);
  if (p < 0 || p > MAX_PERIODO) return ERR.NUM;
  if (p === 0) return 1;
  const m = p === 1 ? detectarPeriodo(y) : p;
  // Con menos de dos periodos completos no hay de dónde sacar la estación.
  if (m > 1 && 2 * m > y.length) return ERR.NUM;
  if (m > 1 && mult && y.some((v) => !(v > 0))) return ERR.NUM;
  return m;
}

interface Contexto {
  serie: Serie;
  m: number;
  aj: Ajuste;
}

/** Los pasos comunes: datos, periodo y ajuste. `iSea` es la posición de seasonality. */
function modelo(
  args: Arg[],
  iVals: number,
  iSea: number,
  mult: boolean,
  conSea: boolean,
): Contexto | FormulaError {
  const sea = conSea ? opc(args, iSea, 1) : 1;
  if (isError(sea)) return sea;
  const op = opciones(args, conSea ? iSea + 1 : iSea);
  if (op instanceof FormulaError) return op;
  const serie = preparar(
    planos(args[iVals]!),
    planos(args[iVals + 1]!),
    op.completar,
    op.agregacion,
  );
  if (serie instanceof FormulaError) return serie;
  const m = periodo(serie.y, sea, mult);
  if (m instanceof FormulaError) return m;
  const aj = ajustar(serie.y, m, mult);
  if (aj instanceof FormulaError) return aj;
  return { serie, m, aj };
}

/** El pronóstico a j pasos (j ≥ 0 entero). */
function pronosticoEntero(c: Contexto, mult: boolean, j: number): number {
  const { aj, m } = c;
  const base = aj.nivel + j * aj.tend;
  if (m <= 1) return base;
  const e = aj.est[(((j - 1) % m) + m) % m]!;
  return mult ? base * e : base + e;
}

function pronostico(c: Contexto, mult: boolean, h: number): number {
  const j0 = Math.floor(h);
  const f0 = pronosticoEntero(c, mult, j0);
  const fr = h - j0;
  return fr === 0 ? f0 : f0 + fr * (pronosticoEntero(c, mult, j0 + 1) - f0);
}

function objetivoDe(args: Arg[], i: number, c: Contexto): number | FormulaError {
  const t = coerceNumber(args[i]!.value());
  if (isError(t)) return t;
  const h = pasosHasta(c.serie, t);
  // Microsoft: un objetivo anterior al final del histórico es #NUM!.
  return !(h > 0) ? ERR.NUM : h;
}

/** FORECAST.ETS.ADD / .MULT: (objetivo, valores, tiempo, [estacionalidad], [completar], [agregar]). */
function prever(args: Arg[], mult: boolean): CellValue {
  if (args.length < 3 || args.length > 6) return ERR.VALUE;
  const c = modelo(args, 1, 3, mult, true);
  if (c instanceof FormulaError) return c;
  const h = objetivoDe(args, 0, c);
  if (h instanceof FormulaError) return h;
  const r = pronostico(c, mult, h);
  return Number.isFinite(r) ? r : ERR.NUM;
}

/** FORECAST.ETS.CONFINT / .PI.ADD / .PI.MULT: (objetivo, valores, tiempo, [confianza], …). */
function intervalo(args: Arg[], mult: boolean): CellValue {
  if (args.length < 3 || args.length > 7) return ERR.VALUE;
  const nc = opc(args, 3, 0.95);
  if (isError(nc)) return nc;
  if (!(nc > 0 && nc < 1)) return ERR.NUM;
  const c = modelo(args, 1, 4, mult, true);
  if (c instanceof FormulaError) return c;
  const h = objetivoDe(args, 0, c);
  if (h instanceof FormulaError) return h;
  // Varianza del error a h pasos de Hyndman (ETS A,A,A): σ²·(1 + Σ_{j<h} c_j²), con
  // c_j = α(1 + jβ) + γ(1−α) si j es múltiplo del periodo. Con estación multiplicativa es una
  // aproximación (ponytail: el intervalo exacto pide simulación).
  const { aj, m } = c;
  const hh = Math.max(1, Math.round(h));
  let suma = 1;
  for (let j = 1; j < hh; j++) {
    let cj = aj.alfa * (1 + j * aj.beta);
    if (m > 1 && j % m === 0) cj += aj.gamma * (1 - aj.alfa);
    suma += cj * cj;
  }
  const sigma2 = aj.sse / aj.cuenta;
  const z = normInv((1 + nc) / 2);
  const r = z * Math.sqrt(sigma2 * suma);
  return Number.isFinite(r) ? r : ERR.NUM;
}

/** FORECAST.ETS.SEASONALITY(valores, tiempo, [completar], [agregar]). */
function estacionalidad(args: Arg[]): CellValue {
  if (args.length < 2 || args.length > 4) return ERR.VALUE;
  const op = opciones(args, 2);
  if (op instanceof FormulaError) return op;
  const serie = preparar(planos(args[0]!), planos(args[1]!), op.completar, op.agregacion);
  if (serie instanceof FormulaError) return serie;
  return detectarPeriodo(serie.y);
}

/** FORECAST.ETS.STAT(valores, tiempo, tipo, [estacionalidad], [completar], [agregar]). */
function estadistica(args: Arg[], mult: boolean): CellValue {
  if (args.length < 3 || args.length > 6) return ERR.VALUE;
  const tipo = coerceNumber(args[2]!.value());
  if (isError(tipo)) return tipo;
  const t = Math.trunc(tipo);
  if (t < 1 || t > 8) return ERR.NUM;
  const c = modelo(args, 0, 3, mult, true);
  if (c instanceof FormulaError) return c;
  const { aj, serie, m } = c;
  switch (t) {
    case 1:
      return aj.alfa;
    case 2:
      return aj.beta;
    case 3:
      return m > 1 ? aj.gamma : 0;
    case 8:
      return serie.paso;
  }
  const y = serie.y;
  switch (t) {
    case 4: {
      // MASE: el MAE entre el de la predicción ingenua (el valor anterior) de la serie.
      let d = 0;
      for (let i = 1; i < y.length; i++) d += Math.abs(y[i]! - y[i - 1]!);
      d /= y.length - 1;
      return d > 0 ? aj.mae / d : ERR.DIV0;
    }
    case 5:
      return aj.smape;
    case 6:
      return aj.mae;
    default:
      return Math.sqrt(aj.sse / aj.cuenta);
  }
}

export const funcs: FnTable = {
  'FORECAST.ETS.ADD': (_c, a) => prever(a, false),
  'FORECAST.ETS': (_c, a) => prever(a, false),
  'FORECAST.ETS.MULT': (_c, a) => prever(a, true),
  'FORECAST.ETS.PI.ADD': (_c, a) => intervalo(a, false),
  'FORECAST.ETS.CONFINT': (_c, a) => intervalo(a, false),
  'FORECAST.ETS.PI.MULT': (_c, a) => intervalo(a, true),
  'FORECAST.ETS.SEASONALITY': (_c, a) => estacionalidad(a),
  'FORECAST.ETS.STAT.ADD': (_c, a) => estadistica(a, false),
  'FORECAST.ETS.STAT': (_c, a) => estadistica(a, false),
  'FORECAST.ETS.STAT.MULT': (_c, a) => estadistica(a, true),
};
