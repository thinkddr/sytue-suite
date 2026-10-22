/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Solver del Análisis Y-si: optimización MULTIVARIABLE con restricciones —
 * generalización de Buscar objetivo (goalseek.ts) a varias celdas variable.
 * Maximiza/minimiza (o lleva a un valor) una celda objetivo cambiando VARIAS
 * celdas variable, sujeto a restricciones «celda ≤/≥/= (número o celda)».
 *
 * Método robusto y SIN derivadas: símplex de Nelder-Mead (reflexión/expansión/
 * contracción/encogimiento) sobre un objetivo PENALIZADO — penalización L1 exacta
 * de las violaciones (base ± P·Σ|violación|), con ESCALADO de P por reinicios
 * (penalty method) que además rearranca el símplex desde el mejor punto para
 * escapar de un colapso prematuro. Salvaguardas: tope de evaluaciones (no bloquea
 * la UI), tolerancia de factibilidad/convergencia y «pared» (objetivo/restricción
 * no numérica → +∞, el símplex la evita). NO garantiza óptimo GLOBAL (Nelder-Mead
 * converge a un óptimo LOCAL): devuelve la MEJOR solución factible hallada.
 * ponytail: NM + penalización L1 con reinicios; si hiciera falta óptimo global,
 * multi-arranque aleatorio o GRG es el camino de mejora.
 *
 * NO depende del motor de fórmulas: el llamador inyecta `evaluate` (fija las
 * variables y lee objetivo + restricciones en una recalculación), igual que
 * goalseek/scenario/datatable inyectan su evaluador → helper PURO y testeable.
 * Self-test: packages/office/test/sheets-solver.test.ts (bun test).
 */

/** Relación de una restricción: LHS ≤ / ≥ / = RHS. */
export type SolverRelation = 'lte' | 'gte' | 'eq';

/** Sentido de la optimización de la celda objetivo. */
export interface SolverGoal {
  kind: 'max' | 'min' | 'value';
  /** Valor buscado cuando `kind === 'value'` (se minimiza |objetivo − target|). */
  target?: number;
}

/** Restricción: solo la relación; el LHS/RHS los evalúa la sonda (celdas). */
export interface SolverConstraint {
  relation: SolverRelation;
}

/** Resultado de una evaluación: valor de la celda objetivo y, por cada
 *  restricción (mismo orden), el valor de su celda LHS y de su RHS (número o
 *  celda). null = no numérico (error/texto) → el solver lo trata como pared. */
export interface SolverEvalResult {
  objective: number | null;
  constraints: readonly { lhs: number | null; rhs: number | null }[];
}

/** Fija las variables en `x` y devuelve objetivo + LHS/RHS de las restricciones. */
export type SolverEvaluate = (x: readonly number[]) => SolverEvalResult;

export interface SolverResult {
  /** Se halló una solución FACTIBLE (todas las restricciones dentro de tolerancia). */
  feasible: boolean;
  /** Mejor punto hallado (valores de las celdas variable); factible si `feasible`. */
  values: number[];
  /** Valor de la celda objetivo en `values` (null si no fue numérica). */
  objective: number | null;
  /** Nº de evaluaciones consumidas (acotado por `maxEvaluations`). */
  iterations: number;
  /** Máxima violación de restricción en `values` (0 = factible exacto; +∞ = pared). */
  maxViolation: number;
}

export interface SolverOptions {
  /** Tolerancia de factibilidad y de convergencia del símplex. Por defecto 1e-6. */
  tolerance?: number;
  /** Tope de evaluaciones de `evaluate` (no bloquear la UI). Por defecto 2000. */
  maxEvaluations?: number;
  /** Reinicios con penalización escalada (anti-colapso + factibilidad). Por defecto 6. */
  restarts?: number;
  /** Escala del símplex inicial en variables cercanas a 0. Por defecto 1. */
  step?: number;
}

const DEFAULT_TOLERANCE = 1e-6;
const DEFAULT_MAX_EVALUATIONS = 2000;
const DEFAULT_RESTARTS = 6;
const DEFAULT_STEP = 1;
/** Penalización L1 inicial y su factor de crecimiento por reinicio (penalty method). */
const PENALTY_0 = 1e3;
const PENALTY_GROWTH = 10;

/** Número finito de `v`, o `d` si no lo es. */
const numOr = (v: unknown, d: number): number =>
  typeof v === 'number' && Number.isFinite(v) ? v : d;

/** Violación (≥ 0) de una restricción; +∞ si algún lado no es numérico (pared). */
const violationOf = (rel: SolverRelation, lhs: number | null, rhs: number | null): number => {
  if (lhs === null || rhs === null || !Number.isFinite(lhs) || !Number.isFinite(rhs))
    return Infinity;
  if (rel === 'lte') return Math.max(0, lhs - rhs);
  if (rel === 'gte') return Math.max(0, rhs - lhs);
  return Math.abs(lhs - rhs); // eq
};

/**
 * Optimiza la celda objetivo cambiando las variables `x0` sujeto a `constraints`.
 * `evaluate` fija las variables y lee objetivo + restricciones (lo inyecta el
 * lienzo con un motor desechable). Devuelve la mejor solución factible hallada, o
 * la menos infactible si ninguna lo es. PURA por inyección de `evaluate`.
 */
export const solve = (
  evaluate: SolverEvaluate,
  x0: readonly number[],
  goal: SolverGoal,
  constraints: readonly SolverConstraint[],
  opts: SolverOptions = {},
): SolverResult => {
  const n = x0.length;
  const tol = Math.max(0, numOr(opts.tolerance, DEFAULT_TOLERANCE));
  const step = Math.max(1e-9, numOr(opts.step, DEFAULT_STEP));
  const restarts = Math.max(0, Math.trunc(numOr(opts.restarts, DEFAULT_RESTARTS)));
  const budget = {
    used: 0,
    max: Math.max(n + 2, Math.trunc(numOr(opts.maxEvaluations, DEFAULT_MAX_EVALUATIONS))),
  };

  if (n === 0)
    return { feasible: false, values: [], objective: null, iterations: 0, maxViolation: Infinity };

  // Base a MINIMIZAR según el sentido (max → −obj; value → |obj − target|; min → obj);
  // null si el objetivo no es numérico (pared).
  const base = (o: number | null): number | null => {
    if (o === null || !Number.isFinite(o)) return null;
    if (goal.kind === 'max') return -o;
    if (goal.kind === 'value') return Math.abs(o - numOr(goal.target, 0));
    return o;
  };
  const sumViolation = (cs: SolverEvalResult['constraints']): number => {
    let s = 0;
    for (let i = 0; i < constraints.length; i++)
      s += violationOf(constraints[i]!.relation, cs[i]?.lhs ?? null, cs[i]?.rhs ?? null);
    return s;
  };
  const maxViolation = (cs: SolverEvalResult['constraints']): number => {
    let m = 0;
    for (let i = 0; i < constraints.length; i++)
      m = Math.max(
        m,
        violationOf(constraints[i]!.relation, cs[i]?.lhs ?? null, cs[i]?.rhs ?? null),
      );
    return m;
  };

  // Mejor punto FACTIBLE (por menor base) y mejor CUALQUIERA (por menor violación,
  // desempate por base): así siempre hay algo que devolver aunque no haya factible.
  // `as … | null` en el inicializador evita que el flujo lo estreche a `null` (se
  // reasigna dentro de `consider`, que TS no rastrea al invocarse vía nelderMead).
  let bestFeas = null as { x: number[]; base: number; obj: number | null; viol: number } | null;
  let bestAny: { x: number[]; viol: number; base: number; obj: number | null } = {
    x: x0.slice(),
    viol: Infinity,
    base: Infinity,
    obj: null,
  };

  const consider = (x: readonly number[], ev: SolverEvalResult): void => {
    const viol = maxViolation(ev.constraints);
    const b = base(ev.objective);
    const obj =
      typeof ev.objective === 'number' && Number.isFinite(ev.objective) ? ev.objective : null;
    if (viol <= tol && b !== null && (!bestFeas || b < bestFeas.base)) {
      bestFeas = { x: x.slice(), base: b, obj, viol };
    }
    const bv = b === null ? Infinity : b;
    if (viol < bestAny.viol || (viol === bestAny.viol && bv < bestAny.base)) {
      bestAny = { x: x.slice(), viol, base: bv, obj };
    }
  };

  // Evaluación con presupuesto + registro del mejor tanteo; null si se agotó.
  const raw = (x: readonly number[]): SolverEvalResult | null => {
    if (budget.used >= budget.max) return null;
    budget.used++;
    const ev = evaluate(x);
    consider(x, ev);
    return ev;
  };

  // Objetivo penalizado L1 (a minimizar); +∞ en pared o presupuesto agotado.
  const penalized =
    (P: number) =>
    (x: readonly number[]): number => {
      const ev = raw(x);
      if (!ev) return Infinity;
      const b = base(ev.objective);
      if (b === null) return Infinity;
      return b + P * sumViolation(ev.constraints);
    };

  let start = x0.slice();
  const local = Math.max((n + 1) * 8, Math.floor(budget.max / (restarts + 1)));
  for (let k = 0; k <= restarts && budget.used < budget.max; k++) {
    const P = PENALTY_0 * Math.pow(PENALTY_GROWTH, k);
    start = nelderMead(penalized(P), start, step, tol, budget, local);
  }

  if (bestFeas) {
    return {
      feasible: true,
      values: bestFeas.x,
      objective: bestFeas.obj,
      iterations: budget.used,
      maxViolation: bestFeas.viol,
    };
  }
  return {
    feasible: bestAny.viol <= tol,
    values: bestAny.x,
    objective: bestAny.obj,
    iterations: budget.used,
    maxViolation: bestAny.viol,
  };
};

/**
 * Símplex de Nelder-Mead: minimiza `f` desde `start` con presupuesto compartido
 * (`budget`) y un tope local `localMax` por reinicio. Coeficientes clásicos
 * (reflexión 1, expansión 2, contracción 0.5, encogimiento 0.5). Para al converger
 * (rango de f y tamaño del símplex ≤ `tol`) o al agotar el presupuesto. Devuelve
 * el mejor vértice. `f` cuenta el presupuesto y devuelve +∞ al agotarlo.
 */
const nelderMead = (
  f: (x: readonly number[]) => number,
  start: readonly number[],
  step: number,
  tol: number,
  budget: { used: number; max: number },
  localMax: number,
): number[] => {
  const n = start.length;
  // Símplex de n+1 vértices: `start` y `start` desplazado en cada eje (paso
  // relativo a la magnitud de la coordenada, o `step` cerca de 0).
  const simplex: number[][] = [start.slice()];
  for (let i = 0; i < n; i++) {
    const v = start.slice();
    const mag = Math.abs(v[i]!);
    v[i] = v[i]! + (mag > 1 ? 0.05 * mag : step);
    simplex.push(v);
  }
  const fv = simplex.map((v) => f(v));
  const localStart = budget.used;

  const worstOf = (): number => fv.reduce((w, _v, i) => (fv[i]! > fv[w]! ? i : w), 0);
  const bestOf = (): number => fv.reduce((b, _v, i) => (fv[i]! < fv[b]! ? i : b), 0);

  while (budget.used < budget.max && budget.used - localStart < localMax) {
    const worst = worstOf();
    const best = bestOf();
    // Segundo peor (para decidir aceptar la reflexión).
    let second = best;
    for (let i = 0; i < fv.length; i++) if (i !== worst && fv[i]! > fv[second]!) second = i;

    // Convergencia: rango de f pequeño Y símplex encogido en torno al mejor.
    let size = 0;
    for (let i = 0; i < simplex.length; i++) {
      let d = 0;
      for (let k = 0; k < n; k++) d += Math.abs(simplex[i]![k]! - simplex[best]![k]!);
      size = Math.max(size, d);
    }
    if (Math.abs(fv[worst]! - fv[best]!) <= tol && size <= tol) break;

    // Centroide de todos los vértices menos el peor.
    const cent = new Array<number>(n).fill(0);
    for (let i = 0; i < simplex.length; i++) {
      if (i === worst) continue;
      for (let k = 0; k < n; k++) cent[k]! += simplex[i]![k]! / n;
    }
    const reflect = (coef: number): number[] =>
      cent.map((c, k) => c + coef * (c - simplex[worst]![k]!));

    const xr = reflect(1);
    const fr = f(xr);
    if (fr < fv[best]!) {
      // Mejor que el óptimo: intenta EXPANDIR.
      const xe = reflect(2);
      const fe = f(xe);
      if (fe < fr) {
        simplex[worst] = xe;
        fv[worst] = fe;
      } else {
        simplex[worst] = xr;
        fv[worst] = fr;
      }
    } else if (fr < fv[second]!) {
      simplex[worst] = xr;
      fv[worst] = fr;
    } else {
      // CONTRACCIÓN hacia el centroide (entre el peor y el centroide).
      const xc = cent.map((c, k) => c + 0.5 * (simplex[worst]![k]! - c));
      const fc = f(xc);
      if (fc < fv[worst]!) {
        simplex[worst] = xc;
        fv[worst] = fc;
      } else {
        // ENCOGIMIENTO de todo el símplex hacia el mejor vértice.
        for (let i = 0; i < simplex.length; i++) {
          if (i === best) continue;
          simplex[i] = simplex[i]!.map(
            (vi, k) => simplex[best]![k]! + 0.5 * (vi - simplex[best]![k]!),
          );
          fv[i] = f(simplex[i]!);
          if (budget.used >= budget.max) break;
        }
      }
    }
  }
  return simplex[bestOf()]!.slice();
};
