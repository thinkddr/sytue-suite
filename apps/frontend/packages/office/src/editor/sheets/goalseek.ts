/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Buscar objetivo (Goal Seek) del Análisis Y-si: solver de una variable PURO y
 * testeable. Dada una función objetivo f(x) (en la hoja: el valor de la celda de
 * fórmula al fijar la celda variable en x), un valor objetivo y un punto de
 * partida, halla x tal que f(x) ≈ objetivo.
 *
 * Método robusto: acorralar un cambio de signo por expansión geométrica «cuesta
 * abajo» (zbrac) y refinarlo con FALSA POSICIÓN + modificación de Illinois
 * (secante salvaguardada que jamás se sale del intervalo → convergencia
 * garantizada); si no hay bracket, secante abierta best-effort. Salvaguardas:
 * divergencia (pasos no finitos), «pared» (f no numérica: error/texto → null),
 * sin solución, máximo de iteraciones y tolerancia. NO depende del motor de
 * fórmulas: el llamador inyecta `f`; así el solver se prueba con funciones puras.
 * Self-test: packages/office/test/sheets-goalseek.test.ts (bun test).
 */

export interface GoalSeekResult {
  /** Se alcanzó el objetivo dentro de la tolerancia. */
  solved: boolean;
  /** Mejor valor hallado para la variable (el de menor error, aunque no resuelva). */
  value: number;
  /** Nº de evaluaciones de f consumidas (acotado por `maxIterations`). */
  iterations: number;
  /** |f(value) − objetivo| en el valor devuelto (0 si resolvió exacto). */
  finalError: number;
}

export interface GoalSeekOptions {
  /** Punto de partida (valor actual de la celda variable). Por defecto 0. */
  x0?: number;
  /** Error absoluto |f(x) − objetivo| aceptable. Por defecto 1e-7. */
  tolerance?: number;
  /** Tope de evaluaciones de f (no bloquear la UI). Por defecto 100. */
  maxIterations?: number;
}

const DEFAULT_TOLERANCE = 1e-7;
const DEFAULT_MAX_ITERATIONS = 100;
/** Factor de crecimiento de la expansión que acorrala el cambio de signo (zbrac). */
const EXPAND_FACTOR = 1.6;

/** Objetivo desplazado g(x) = f(x) − target; null cuando f no da número. */
type Objective = (x: number) => number | null;

/**
 * Halla x con f(x) ≈ target. `f` devuelve el valor de la celda objetivo al fijar
 * la variable en x, o null si esa celda no es numérica (error/texto): el solver
 * lo trata como una «pared» infranqueable (Buscar objetivo no cruza discontinuidades).
 */
export function goalSeek(
  f: (x: number) => number | null,
  target: number,
  opts: GoalSeekOptions = {},
): GoalSeekResult {
  const tol =
    typeof opts.tolerance === 'number' && Number.isFinite(opts.tolerance) && opts.tolerance >= 0
      ? opts.tolerance
      : DEFAULT_TOLERANCE;
  const maxIterations = Math.max(
    1,
    Math.trunc(
      typeof opts.maxIterations === 'number' && Number.isFinite(opts.maxIterations)
        ? opts.maxIterations
        : DEFAULT_MAX_ITERATIONS,
    ),
  );
  const x0 = typeof opts.x0 === 'number' && Number.isFinite(opts.x0) ? opts.x0 : 0;

  let iterations = 0;
  let bestX = x0;
  let bestErr = Infinity;

  /* g(x) con contabilidad de presupuesto y registro del mejor tanteo. Devuelve
   * null si x no es finito, si se agotó el presupuesto o si f no da un número. */
  const g: Objective = (x) => {
    if (!Number.isFinite(x) || iterations >= maxIterations) return null;
    iterations++;
    const y = f(x);
    if (y === null || !Number.isFinite(y)) return null;
    const err = y - target;
    const abs = Math.abs(err);
    if (abs < bestErr) {
      bestErr = abs;
      bestX = x;
    }
    return err;
  };

  const result = (): GoalSeekResult => ({
    solved: bestErr <= tol,
    value: bestX,
    iterations,
    finalError: bestErr,
  });

  // --- Punto de partida finito (si x0 cae en una «pared», tantea a su alrededor) ---
  let a = x0;
  let ga = g(a);
  if (ga === null) {
    for (const x of probesAround(x0)) {
      const gx = g(x);
      if (gx !== null) {
        a = x;
        ga = gx;
        break;
      }
    }
    if (ga === null) return result();
  }
  if (Math.abs(ga) <= tol) return result();

  // --- Segundo punto para la secante (paso relativo a la magnitud del punto) ---
  const step = initialStep(a);
  let b = a + step;
  let gb = g(b);
  if (gb === null) {
    b = a - step;
    gb = g(b);
  }
  if (gb === null) return result();
  if (Math.abs(gb) <= tol) return result();

  // --- Expansión zbrac: amplía [a,b] por el extremo «cuesta abajo» (menor |g|)
  //     hasta lograr un cambio de signo, chocar con una pared o agotar el presupuesto. ---
  while (ga !== null && gb !== null && ga * gb > 0) {
    if (Math.abs(ga) < Math.abs(gb)) {
      a += EXPAND_FACTOR * (a - b);
      ga = g(a);
    } else {
      b += EXPAND_FACTOR * (b - a);
      gb = g(b);
    }
  }

  // --- Refinamiento ---
  if (ga !== null && gb !== null) {
    if (ga * gb <= 0) refineBracket(a, ga, b, gb, g, tol);
    else refineSecant(a, ga, b, gb, g, tol);
  }
  return result();
}

/**
 * Refina un bracket [a,b] con f(a),f(b) de signos opuestos: falsa posición
 * (secante entre los extremos, siempre dentro del intervalo) con la modificación
 * de Illinois para evitar el estancamiento de un extremo. Converge de forma
 * garantizada; para en tolerancia, al colapsar el intervalo o al agotar presupuesto.
 */
function refineBracket(
  a: number,
  ga: number,
  b: number,
  gb: number,
  g: Objective,
  tol: number,
): void {
  let side = 0; // +1: se movió `a` la última vez; −1: se movió `b`
  while (Math.abs(b - a) > intervalEps(a, b)) {
    if (Math.abs(ga) <= tol || Math.abs(gb) <= tol) return;
    const denom = gb - ga;
    // Falsa posición; si degenera, punto medio (bisección).
    let x = denom !== 0 ? (a * gb - b * ga) / denom : (a + b) / 2;
    if (!(x > Math.min(a, b) && x < Math.max(a, b))) x = (a + b) / 2;
    const gx = g(x);
    if (gx === null) {
      // Pared o presupuesto agotado dentro del bracket: intenta el punto medio.
      const mid = (a + b) / 2;
      const gm = g(mid);
      if (gm === null) return;
      if (ga * gm > 0) {
        a = mid;
        ga = gm;
      } else {
        b = mid;
        gb = gm;
      }
      side = 0;
      continue;
    }
    if (ga * gx > 0) {
      a = x;
      ga = gx;
      if (side === 1) gb /= 2; // Illinois: rebaja el peso del extremo estancado
      side = 1;
    } else {
      b = x;
      gb = gx;
      if (side === -1) ga /= 2;
      side = -1;
    }
  }
}

/**
 * Secante abierta best-effort cuando no hay cambio de signo (función monótona sin
 * raíz alcanzable o sin solución): itera hasta la tolerancia, hasta que un paso
 * deje de ser finito, hasta detectar un bracket (y entonces lo refina) o hasta
 * agotar el presupuesto. No garantiza solución; devuelve el mejor tanteo.
 */
function refineSecant(
  a: number,
  ga: number,
  b: number,
  gb: number,
  g: Objective,
  tol: number,
): void {
  for (;;) {
    if (Math.abs(gb) <= tol) return;
    if (gb === ga) return; // pendiente nula: sin información para avanzar
    const x = b - (gb * (b - a)) / (gb - ga);
    if (!Number.isFinite(x)) return;
    const gx = g(x);
    if (gx === null) return;
    if (ga * gx < 0) return refineBracket(a, ga, x, gx, g, tol);
    if (gb * gx < 0) return refineBracket(b, gb, x, gx, g, tol);
    a = b;
    ga = gb;
    b = x;
    gb = gx;
  }
}

/** Paso inicial de la secante, relativo a la magnitud del punto de partida. */
function initialStep(a: number): number {
  const m = Math.abs(a);
  return m > 1 ? m * 1e-3 : 1e-3;
}

/** Anchura mínima del intervalo (relativa) por debajo de la cual se considera colapsado. */
function intervalEps(a: number, b: number): number {
  return 1e-14 * (1 + Math.abs(a) + Math.abs(b));
}

/** Puntos alrededor de x0 para hallar una evaluación numérica si x0 cae en una pared. */
function probesAround(x0: number): number[] {
  const base = Number.isFinite(x0) ? x0 : 0;
  const scale = Math.abs(base) > 1 ? Math.abs(base) : 1;
  const out: number[] = [];
  for (const f of [1e-3, 1e-2, 0.1, 1, 10, 100]) {
    out.push(base + f * scale, base - f * scale);
  }
  return out;
}
