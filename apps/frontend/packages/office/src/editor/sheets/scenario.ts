/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Administrador de escenarios del Análisis Y-si (paridad Excel): helpers PUROS y
 * testeables. Un escenario = nombre + un conjunto de celdas de ENTRADA con el
 * valor (texto crudo) que toma cada una. Aquí viven tres piezas sin estado ni
 * motor de fórmulas: aplicar un escenario a un mapa de celdas (conservando el
 * formato de cada celda, como Buscar objetivo), construir el INFORME RESUMEN que
 * compara escenarios contra unas celdas de resultado (el motor se INYECTA vía
 * `evaluate`, igual que la sonda de Buscar objetivo inyecta `f`), y sanear un
 * escenario persistido. El lienzo (SheetCanvas) los cablea a comandos.
 * Self-test: packages/office/test/sheets-scenario.test.ts (bun test).
 */
import { escapeHtml, parseCell, parseRangeRef, renderCellHtml } from './format';
import type { Scenario } from '../types';

/** Tope de celdas de entrada por escenario (criterio Excel: 32 celdas cambiantes). */
export const SCENARIO_MAX_CELLS = 32;

/** Título de la primera celda del informe resumen. */
export const SCENARIO_REPORT_TITLE = 'Resumen de escenarios';
/** Rótulos de sección del informe (filas de encabezado). */
export const SCENARIO_CHANGING_LABEL = 'Celdas cambiantes:';
export const SCENARIO_RESULT_LABEL = 'Celdas de resultado:';

/**
 * Aplica los valores de un escenario a un mapa de celdas (ref A1 → HTML de la
 * celda), devolviendo un mapa NUEVO (no muta la entrada). Conserva negrita,
 * cursiva y alineación de cada celda escrita (mismo criterio que Buscar
 * objetivo); un valor vacío BORRA la celda. Las celdas ausentes en `values` se
 * quedan como están. PURA.
 */
export const applyScenarioValues = (
  cells: Record<string, string>,
  values: Record<string, string>,
): Record<string, string> => {
  const out = { ...cells };
  for (const ref in values) {
    const p = parseCell(out[ref] ?? ''); // conserva formato de la celda
    p.inner = escapeHtml(values[ref] ?? '');
    const html = renderCellHtml(p);
    if (html === '') delete out[ref];
    else out[ref] = html;
  }
  return out;
};

/** Evalúa unas celdas de resultado tras FIJAR unos valores de entrada; devuelve
 *  el texto MOSTRADO de cada ref pedida. Se inyecta (el motor lo pone el
 *  llamador → helper puro, como `f` en Buscar objetivo). */
export type ScenarioEvaluate = (
  overrides: Record<string, string>,
  resultCells: readonly string[],
) => Record<string, string>;

export interface ScenarioReportOptions {
  /** Rótulos del título y de las dos secciones, ya traducidos; ausentes = los de arriba. */
  title?: string;
  changingLabel?: string;
  resultLabel?: string;
  /** Rótulo de la columna de valores actuales; ausente = no se incluye. */
  currentLabel?: string;
  /** Valores crudos actuales de las celdas de entrada (para la columna de
   *  valores actuales y como respaldo de las entradas que un escenario no fija). */
  current?: Record<string, string>;
}

/**
 * Construye el INFORME RESUMEN (rejilla de texto) que compara escenarios.
 * Filas: título + sección «Celdas cambiantes» (una fila por celda de entrada) +
 * sección «Celdas de resultado» (una por celda de resultado). Columnas: rótulo +
 * [valores actuales] + una por escenario. Todas las filas tienen el mismo ancho.
 *
 * Las celdas de entrada son la UNIÓN de las de todos los escenarios, ordenadas
 * en lectura (fila, luego columna). Cada columna evalúa las celdas de resultado
 * con SUS overrides mediante `evaluate` (una pasada de motor por columna; las
 * entradas que ese escenario no fija conservan su valor actual del modelo). PURA
 * por inyección de `evaluate`.
 */
export const buildScenarioReport = (
  scenarios: readonly Scenario[],
  resultCells: readonly string[],
  evaluate: ScenarioEvaluate,
  opts: ScenarioReportOptions = {},
): string[][] => {
  const inputCells = sortedRefs(unionRefs(scenarios));
  const results = uniqueValidRefs(resultCells);
  const current = opts.current ?? {};

  // Columnas: cada una con su rótulo y sus overrides (null = valores actuales).
  const cols: { header: string; overrides: Record<string, string> | null }[] = [];
  if (opts.currentLabel !== undefined) cols.push({ header: opts.currentLabel, overrides: null });
  for (const sc of scenarios) cols.push({ header: sc.name, overrides: sc.values });

  // Resultados por columna (una llamada a `evaluate` por columna).
  const resultsByCol = cols.map((c) =>
    results.length > 0 ? evaluate(c.overrides ?? {}, results) : {},
  );

  const pad = cols.map(() => '');
  const grid: string[][] = [[opts.title ?? SCENARIO_REPORT_TITLE, ...cols.map((c) => c.header)]];

  if (inputCells.length > 0) {
    grid.push([opts.changingLabel ?? SCENARIO_CHANGING_LABEL, ...pad]);
    for (const ref of inputCells) {
      grid.push([
        ref,
        ...cols.map((c) =>
          c.overrides ? (c.overrides[ref] ?? current[ref] ?? '') : (current[ref] ?? ''),
        ),
      ]);
    }
  }
  if (results.length > 0) {
    grid.push([opts.resultLabel ?? SCENARIO_RESULT_LABEL, ...pad]);
    for (const ref of results) {
      grid.push([ref, ...resultsByCol.map((m) => m[ref] ?? '')]);
    }
  }
  return grid;
};

/** Escenario persistido → saneado; null si no vale. El nombre no puede ir vacío
 *  y las claves de `values` deben ser UNA celda (no un rango); refs a MAYÚSCULAS. */
export const sanitizeScenario = (raw: unknown): Scenario | null => {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as { name?: unknown; values?: unknown };
  if (typeof r.name !== 'string' || r.name.trim() === '') return null;
  if (!r.values || typeof r.values !== 'object') return null;
  const src = r.values as Record<string, unknown>;
  const values: Record<string, string> = {};
  for (const ref in src) {
    const rr = parseRangeRef(ref);
    const v = src[ref];
    if (rr && rr.c1 === rr.c2 && rr.r1 === rr.r2 && typeof v === 'string') {
      values[ref.toUpperCase()] = v;
    }
  }
  if (Object.keys(values).length === 0) return null;
  return { name: r.name.trim(), values };
};

/** Unión de las refs de entrada de todos los escenarios (sin repetir). */
const unionRefs = (scenarios: readonly Scenario[]): string[] => {
  const seen = new Set<string>();
  for (const sc of scenarios) for (const ref in sc.values) seen.add(ref);
  return [...seen];
};

/** Refs válidas y únicas (una celda), en MAYÚSCULAS, conservando el orden dado. */
const uniqueValidRefs = (refs: readonly string[]): string[] => {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const ref of refs) {
    const rr = parseRangeRef(ref);
    if (!rr || rr.c1 !== rr.c2 || rr.r1 !== rr.r2) continue;
    const up = ref.trim().toUpperCase();
    if (!seen.has(up)) {
      seen.add(up);
      out.push(up);
    }
  }
  return out;
};

/** Ordena refs A1 en lectura (fila, luego columna); descarta las que no parsean. */
const sortedRefs = (refs: readonly string[]): string[] =>
  refs
    .map((ref) => ({ ref, rr: parseRangeRef(ref) }))
    .filter((x): x is { ref: string; rr: NonNullable<typeof x.rr> } => x.rr !== null)
    .sort((a, b) => a.rr.r1 - b.rr.r1 || a.rr.c1 - b.rr.c1)
    .map((x) => x.ref);
