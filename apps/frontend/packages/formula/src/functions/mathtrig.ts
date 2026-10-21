/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Trigonometría, exponenciales/logaritmos y combinatoria (PI, SIN…TANH,
 * DEGREES/RADIANS, EXP, LN, LOG, LOG10, FACT, FACTDOUBLE, COMBIN, PERMUT,
 * RAND, RANDBETWEEN). Semántica OpenFormula / ECMA-376; código original
 * (clean-room). NO edites functions/index.ts: se fusiona automáticamente.
 */
import type { Arg, FnTable } from '../registry.js';
import { coerceNumber, coerceText, ERR, isError, type FormulaError } from '../values.js';

/** Coacciona el único argumento a número; #VALUE! si la aridad no es 1. */
function arg1(args: Arg[]): number | FormulaError {
  if (args.length !== 1) return ERR.VALUE;
  return coerceNumber(args[0]!.value());
}

/**
 * Aplica una función real de un argumento y mapea el resultado no finito a
 * #NUM! (fuera de dominio o desbordamiento: ASIN(2), LN(0), EXP(1000)…).
 */
function map1(args: Arg[], f: (x: number) => number): number | FormulaError {
  const x = arg1(args);
  if (isError(x)) return x;
  const r = f(x);
  return Number.isFinite(r) ? r : ERR.NUM;
}

/**
 * Recíproca de una función trigonométrica: 1/f(x), con el polo (f(x) = 0) devolviendo
 * #DIV/0! en vez de infinito. Una hoja distingue «dividiste por cero» de «te saliste del
 * dominio», y quien lee la celda necesita saber cuál de las dos le pasó.
 */
/**
 * Recorre los números de los argumentos (escalares y rangos) en ORDEN, que es lo que
 * SERIESSUM necesita: la posición de cada coeficiente decide su exponente. Mismo criterio
 * que SUM para qué cuenta —texto y blancos fuera; un booleano solo si es escalar directo—.
 */
function eachNumber(args: Arg[], cb: (n: number) => void): FormulaError | undefined {
  for (const a of args) {
    const esRango = a.ref !== undefined;
    for (const fila of a.matrix()) {
      for (const v of fila) {
        if (isError(v)) return v;
        if (typeof v === 'number') cb(v);
        else if (typeof v === 'boolean' && !esRango) cb(v ? 1 : 0);
      }
    }
  }
  return undefined;
}

function recíproca(args: Arg[], f: (x: number) => number): number | FormulaError {
  const x = arg1(args);
  if (isError(x)) return x;
  const d = f(x);
  if (d === 0) return ERR.DIV0;
  const r = 1 / d;
  return Number.isFinite(r) ? r : ERR.NUM;
}

/**
 * Redondea al entero de la paridad pedida (`resto` 0 = par, 1 = impar) ALEJÁNDOSE de
 * cero, que es lo que hacen EVEN y ODD. El signo se saca antes y se reaplica: trabajar
 * sobre el valor absoluto evita el error clásico de que -1.5 y 1.5 redondeen en
 * direcciones distintas.
 */
function aParidad(args: Arg[], paso: 1 | 2): number | FormulaError {
  const x = arg1(args);
  if (isError(x)) return x;
  const signo = x < 0 ? -1 : 1;
  const a = Math.abs(x);
  // Par: el múltiplo de 2 igual o superior. Impar: el anterior a un par, desplazando.
  // Sale directo con un ceil, sin encadenar «si ya es par, si no lo es, si se pasó…»:
  // esa forma es donde se colaban los signos y el caso del valor ya exacto.
  const n = paso === 2 ? Math.ceil(a / 2) * 2 : Math.ceil((a + 1) / 2) * 2 - 1;
  return signo * n;
}

/** C(n, k) por producto acumulado: no pasa por el factorial, que desborda mucho antes. */
function combinaciones(n: number, k: number): number {
  if (k < 0 || n < 0 || k > n) return NaN;
  const m = Math.min(k, n - k);
  let r = 1;
  for (let i = 1; i <= m; i += 1) r = (r * (n - m + i)) / i;
  return Math.round(r);
}

/** Alfabeto de dígitos hasta la base 36. */
const DIGITOS = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ';

/** Los siete símbolos romanos y su valor, de mayor a menor. */
const SIMBOLOS = ['M', 'D', 'C', 'L', 'X', 'V', 'I'] as const;
const VALORES = [1000, 500, 100, 50, 10, 5, 1] as const;

/**
 * Entero (1–3999) → numeración romana en la forma 0 (clásica) a 4 (simplificada).
 *
 * Se recorren los cuatro lugares (M, C, X, I). Un dígito 4 o 9 se escribe con resta: el
 * símbolo del lugar delante del de 5 o 10 veces su valor (CD, CM). Las formas 1 a 4 dejan
 * que el que resta baje hasta `forma` símbolos más, mientras lo restado no pase de lo que
 * queda: 499 es CDXCIX en la forma 0, LDVLIV en la 1, XDIX en la 2, VDIV en la 3 y ID en la 4.
 */
function aRomano(n: number, forma: number): string {
  let resto = n;
  let out = '';
  for (let i = 0; i <= 6; i += 2) {
    const v = VALORES[i]!;
    const d = Math.floor(resto / v);
    if (i > 0 && d % 5 === 4) {
      const mayor = d === 4 ? i - 1 : i - 2; // el símbolo de 5·v o de 10·v
      let j = i;
      for (let pasos = 0; pasos < forma && j < 6; pasos++) {
        if (VALORES[mayor]! - VALORES[j + 1]! > resto) break;
        j++;
      }
      out += SIMBOLOS[j]! + SIMBOLOS[mayor]!;
      resto -= VALORES[mayor]! - VALORES[j]!;
    } else {
      let cuantos = d;
      if (i > 0 && d >= 5) {
        out += SIMBOLOS[i - 1]!;
        cuantos -= 5;
      }
      out += SIMBOLOS[i]!.repeat(cuantos);
      resto -= d * v;
    }
  }
  return out;
}

const VALOR_ROMANO: Record<string, number> = {
  I: 1,
  V: 5,
  X: 10,
  L: 50,
  C: 100,
  D: 500,
  M: 1000,
};

export const funcs: FnTable = {
  PI(_ctx, args) {
    return args.length === 0 ? Math.PI : ERR.VALUE;
  },

  SIN(_ctx, args) {
    return map1(args, Math.sin);
  },
  COS(_ctx, args) {
    return map1(args, Math.cos);
  },
  TAN(_ctx, args) {
    return map1(args, Math.tan);
  },
  ASIN(_ctx, args) {
    return map1(args, Math.asin); // |x|>1 → NaN → #NUM!
  },
  ACOS(_ctx, args) {
    return map1(args, Math.acos);
  },
  ATAN(_ctx, args) {
    return map1(args, Math.atan);
  },
  ATAN2(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    const y = coerceNumber(args[1]!.value());
    if (isError(y)) return y;
    if (x === 0 && y === 0) return ERR.DIV0; // ángulo indefinido en el origen
    return Math.atan2(y, x); // ¡orden de Excel! ATAN2(x;y) = arctan(y/x)
  },

  SINH(_ctx, args) {
    return map1(args, Math.sinh);
  },
  COSH(_ctx, args) {
    return map1(args, Math.cosh); // desborda a ∞ → #NUM!
  },
  TANH(_ctx, args) {
    return map1(args, Math.tanh);
  },

  DEGREES(_ctx, args) {
    return map1(args, (x) => (x * 180) / Math.PI);
  },
  RADIANS(_ctx, args) {
    return map1(args, (x) => (x * Math.PI) / 180);
  },

  EXP(_ctx, args) {
    return map1(args, Math.exp); // EXP(710)→∞ → #NUM!
  },
  LN(_ctx, args) {
    return map1(args, Math.log); // x≤0 → -∞/NaN → #NUM!
  },
  LOG10(_ctx, args) {
    return map1(args, Math.log10);
  },
  LOG(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    let base = 10;
    if (args.length === 2) {
      const b = coerceNumber(args[1]!.value());
      if (isError(b)) return b;
      base = b;
    }
    if (x <= 0 || base <= 0) return ERR.NUM; // dominio del logaritmo
    if (base === 1) return ERR.DIV0; // ln(1)=0 → división por cero
    return Math.log(x) / Math.log(base);
  },

  FACT(_ctx, args) {
    const n = arg1(args);
    if (isError(n)) return n;
    if (n < 0) return ERR.NUM;
    let r = 1;
    for (let i = 2; i <= Math.trunc(n); i++) r *= i; // el no entero se trunca
    return Number.isFinite(r) ? r : ERR.NUM; // FACT(171)→∞ → #NUM!
  },
  FACTDOUBLE(_ctx, args) {
    const n = arg1(args);
    if (isError(n)) return n;
    if (n < -1) return ERR.NUM; // FACTDOUBLE(-1)=1 por convención; <-1 no
    let r = 1;
    for (let i = Math.trunc(n); i > 1; i -= 2) r *= i;
    return Number.isFinite(r) ? r : ERR.NUM;
  },
  COMBIN(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const nv = coerceNumber(args[0]!.value());
    if (isError(nv)) return nv;
    const kv = coerceNumber(args[1]!.value());
    if (isError(kv)) return kv;
    const n = Math.trunc(nv);
    const k = Math.trunc(kv);
    if (n < 0 || k < 0 || k > n) return ERR.NUM;
    // Forma multiplicativa con el factor menor: C(n,k)=∏ (n-i)/(i+1).
    const m = Math.min(k, n - k);
    let r = 1;
    for (let i = 0; i < m; i++) r = (r * (n - i)) / (i + 1);
    r = Math.round(r); // limpia el ruido de f64; el resultado es entero
    return Number.isFinite(r) ? r : ERR.NUM;
  },
  PERMUT(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const nv = coerceNumber(args[0]!.value());
    if (isError(nv)) return nv;
    const kv = coerceNumber(args[1]!.value());
    if (isError(kv)) return kv;
    const n = Math.trunc(nv);
    const k = Math.trunc(kv);
    if (n < 0 || k < 0 || k > n) return ERR.NUM;
    let r = 1;
    for (let i = 0; i < k; i++) r *= n - i; // P(n,k)=n·(n-1)···(n-k+1)
    return Number.isFinite(r) ? r : ERR.NUM;
  },

  // Volátiles: usan el RNG inyectable del motor (determinista en tests).
  // ponytail: el motor cachea el valor; refrescarlo en cada recálculo es cosa
  // del motor (volatilidad), no de la función.
  RAND(ctx, args) {
    return args.length === 0 ? ctx.random() : ERR.VALUE;
  },
  RANDBETWEEN(ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const b = coerceNumber(args[0]!.value());
    if (isError(b)) return b;
    const t = coerceNumber(args[1]!.value());
    if (isError(t)) return t;
    const lo = Math.ceil(b); // límites fraccionarios → entero dentro del rango
    const hi = Math.floor(t);
    if (lo > hi) return ERR.NUM;
    return lo + Math.floor(ctx.random() * (hi - lo + 1));
  },
  /* ---------- Recíprocas e inversas hiperbólicas ----------
   * Todas siguen OpenFormula. Los casos que deciden si están BIEN son los polos y el
   * dominio, no el caso feliz: 1/tan(0) es infinito, no un número enorme, y una hoja
   * responde #DIV/0! ahí. `map1` ya mapea lo no finito a #NUM!, que es lo correcto para
   * salirse del dominio (ACOSH(0)), pero un polo es división por cero y se distingue. */

  /** Secante: 1/cos(x). En los polos (cos = 0) el resultado no es finito → #NUM!. */
  SEC(_ctx, args) {
    return map1(args, (x) => 1 / Math.cos(x));
  },
  /** Cosecante: 1/sin(x). CSC(0) es división por cero, no un número grande. */
  CSC(_ctx, args) {
    return recíproca(args, Math.sin);
  },
  /** Cotangente: cos(x)/sin(x). COT(0) → #DIV/0!, igual que CSC. */
  COT(_ctx, args) {
    return recíproca(args, Math.tan);
  },
  /** Secante hiperbólica: 1/cosh(x). cosh nunca es 0, así que no hay polo. */
  SECH(_ctx, args) {
    return map1(args, (x) => 1 / Math.cosh(x));
  },
  /** Cosecante hiperbólica: 1/sinh(x). Único polo en 0. */
  CSCH(_ctx, args) {
    return recíproca(args, Math.sinh);
  },
  /** Cotangente hiperbólica: 1/tanh(x). Único polo en 0. */
  COTH(_ctx, args) {
    return recíproca(args, Math.tanh);
  },

  /** Arcoseno hiperbólico. Dominio: todos los reales. */
  ASINH(_ctx, args) {
    return map1(args, Math.asinh);
  },
  /** Arcocoseno hiperbólico. Dominio x ≥ 1; fuera de él, #NUM!. */
  ACOSH(_ctx, args) {
    return map1(args, Math.acosh);
  },
  /** Arcotangente hiperbólica. Dominio |x| < 1; en ±1 el límite es infinito → #NUM!. */
  ATANH(_ctx, args) {
    return map1(args, Math.atanh);
  },
  /**
   * Arcocotangente, rango (0, π).
   *
   * NO es `atan(1/x)`: para x negativo eso devuelve el ángulo del cuadrante equivocado
   * —ACOT(-1) daría -π/4 en vez de 3π/4— y en x=0 divide por cero. `π/2 - atan(x)` da el
   * rango correcto y es continua en 0, que es como se comporta una hoja de cálculo.
   */
  ACOT(_ctx, args) {
    return map1(args, (x) => Math.PI / 2 - Math.atan(x));
  },
  /** Arcocotangente hiperbólica: ½·ln((x+1)/(x−1)). Dominio |x| > 1. */
  ACOTH(_ctx, args) {
    return map1(args, (x) => Math.log((x + 1) / (x - 1)) / 2);
  },

  /* ---------- Redondeo a paridad ---------- */

  /**
   * Redondea al PAR más alejado de cero: EVEN(1.5)=2, EVEN(-1.5)=-2, EVEN(3)=4.
   * Un número ya par se deja quieto; el cero también. El signo importa —redondear
   * «hacia arriba» convertiría -1.5 en -2 por casualidad y -2.5 en -2, que es un valor
   * más cercano a cero y no lo que pide la función.
   */
  EVEN(_ctx, args) {
    return aParidad(args, 2);
  },
  /**
   * Redondea al IMPAR más alejado de cero: ODD(1.5)=3, ODD(-1.5)=-3, ODD(2)=3.
   * ODD(0) es 1, no 0: cero no es impar y hay que alejarse de él.
   */
  ODD(_ctx, args) {
    return aParidad(args, 1);
  },

  /* ---------- Combinatoria y series ---------- */

  /** Raíz de (x·π). Negativo → #NUM! (raíz de negativo no es real). */
  SQRTPI(_ctx, args) {
    return map1(args, (x) => (x < 0 ? NaN : Math.sqrt(x * Math.PI)));
  },
  /**
   * Combinaciones CON repetición: C(n+k−1, k). No confundir con COMBIN, que es sin
   * repetición — es la diferencia entre «cuántos ramos de 3 flores con 5 especies
   * pudiendo repetir» (35) y «sin repetir» (10).
   */
  COMBINA(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    if (isError(n)) return n;
    const k = coerceNumber(args[1]!.value());
    if (isError(k)) return k;
    const N = Math.trunc(n);
    const K = Math.trunc(k);
    if (N < 0 || K < 0) return ERR.NUM;
    if (N === 0 && K === 0) return 1;
    return combinaciones(N + K - 1, K);
  },
  /**
   * Coeficiente multinomial: (Σxᵢ)! / Πxᵢ!.
   *
   * Se calcula como producto de binomiales acumulados en vez de dividir factoriales:
   * MULTINOMIAL(20,20,20) desborda el factorial de 60 (>1e81) aunque el resultado quepa
   * de sobra en un double.
   */
  MULTINOMIAL(_ctx, args) {
    let total = 0;
    let acc = 1;
    const err = eachNumber(args, (x) => {
      const n = Math.trunc(x);
      if (n < 0) {
        acc = NaN;
        return;
      }
      total += n;
      acc *= combinaciones(total, n);
    });
    if (err) return err;
    return Number.isFinite(acc) ? acc : ERR.NUM;
  },
  /**
   * Suma de una serie de potencias: Σ coefᵢ · x^(n + i·m).
   *
   * Los coeficientes llegan como rango o lista, y su ORDEN define el exponente de cada
   * uno, así que se recorren en secuencia y no se agregan.
   */
  SERIESSUM(_ctx, args) {
    if (args.length !== 4) return ERR.VALUE;
    const x = coerceNumber(args[0]!.value());
    if (isError(x)) return x;
    const n = coerceNumber(args[1]!.value());
    if (isError(n)) return n;
    const m = coerceNumber(args[2]!.value());
    if (isError(m)) return m;
    let acc = 0;
    let i = 0;
    const err = eachNumber([args[3]!], (c) => {
      acc += c * Math.pow(x, n + i * m);
      i += 1;
    });
    if (err) return err;
    return Number.isFinite(acc) ? acc : ERR.NUM;
  },
  /* ---------- Bases arbitrarias y numeración romana ---------- */

  /**
   * Entero → texto en la base dada (2–36), opcionalmente rellenado con ceros.
   *
   * Solo enteros NO negativos: en una base arbitraria no hay convenio de signo (el
   * complemento a dos de DEC2BIN es propio de anchos fijos, aquí no aplica).
   */
  BASE(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    if (isError(n)) return n;
    const b = coerceNumber(args[1]!.value());
    if (isError(b)) return b;
    let min = 0;
    if (args.length === 3) {
      const m = coerceNumber(args[2]!.value());
      if (isError(m)) return m;
      min = Math.trunc(m);
      if (min < 0 || min > 255) return ERR.NUM;
    }
    const N = Math.trunc(n);
    const B = Math.trunc(b);
    if (N < 0 || B < 2 || B > 36) return ERR.NUM;
    return N.toString(B).toUpperCase().padStart(min, '0');
  },

  /**
   * Texto en la base dada → entero. Inversa de BASE.
   *
   * `parseInt` NO sirve aquí: se traga los dígitos válidos y descarta el resto en
   * silencio, así que «1Z» en base 2 devolvería 1 en vez de #NUM!. Se valida cada
   * dígito contra la base antes de convertir.
   */
  DECIMAL(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const t = coerceText(args[0]!.value());
    if (isError(t)) return t;
    const b = coerceNumber(args[1]!.value());
    if (isError(b)) return b;
    const B = Math.trunc(b);
    if (B < 2 || B > 36) return ERR.NUM;
    const s = t.trim().toUpperCase();
    if (s === '') return 0;
    let acc = 0;
    for (const ch of s) {
      const d = DIGITOS.indexOf(ch);
      if (d < 0 || d >= B) return ERR.NUM; // dígito ajeno a la base: no se ignora
      acc = acc * B + d;
    }
    return acc;
  },

  /**
   * Entero → numeración romana (0–3999), en la forma 0 (clásica, la que se da por omisión) a
   * 4 (simplificada); la forma es un número como cualquier otro (VERDADERO = 1, FALSO = 0).
   * Fuera de rango es #NUM!, como LibreOffice (la documentación de Excel habla de #VALUE!).
   * Ver `aRomano`.
   */
  ROMAN(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    if (isError(n)) return n;
    let forma = 0;
    if (args.length === 2) {
      const f = coerceNumber(args[1]!.value());
      if (isError(f)) return f;
      forma = Math.trunc(f);
      if (forma < 0 || forma > 4) return ERR.NUM;
    }
    const N = Math.trunc(n);
    if (N < 0 || N > 3999) return ERR.NUM;
    return N === 0 ? '' : aRomano(N, forma);
  },

  /**
   * Numeración romana → entero. Inversa de ROMAN.
   *
   * Un símbolo menor ANTES de uno mayor resta (IX = 9); si no, suma. Recorrer de
   * derecha a izquierda comparando con el máximo visto resuelve las dos reglas de una
   * vez, sin tabla de excepciones.
   */
  ARABIC(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = coerceText(args[0]!.value());
    if (isError(t)) return t;
    const s = t.trim().toUpperCase();
    if (s === '') return 0;
    const negativo = s.startsWith('-');
    const cuerpo = negativo ? s.slice(1) : s;
    let acc = 0;
    let mayor = 0;
    for (let i = cuerpo.length - 1; i >= 0; i -= 1) {
      const v = VALOR_ROMANO[cuerpo[i]!];
      if (v === undefined) return ERR.VALUE;
      if (v < mayor) acc -= v;
      else {
        acc += v;
        mayor = v;
      }
    }
    return negativo ? -acc : acc;
  },
};
