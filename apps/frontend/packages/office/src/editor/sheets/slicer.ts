/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Helpers PUROS de la segmentación de datos (slicers de Excel): valores ÚNICOS
 * de una columna con su estado de exclusión, y traducción «selección → conjunto
 * excluido». La segmentación NO tiene lógica de filtrado propia: reutiliza el
 * autofiltro de la hoja (SheetAutoFilter.excl + filteredRows del lienzo). Aquí
 * solo vive la aritmética de conjuntos, sin DOM ni estado de UI.
 * Self-test: packages/office/test/sheets-slicer.test.ts (bun test).
 */
import type { Slicer } from '../types';
import type { PivotDef, PivotFieldRef } from './pivot';
import { MAX_COLS } from './rejilla';

export interface SlicerEntry {
  value: string;
  /** Lo que se enseña, si no es el valor (los elementos de una tabla dinámica, que se
   *  guardan por su clave). Ausente: el valor, o «(Vacías)» si es ''. */
  label?: string;
  /** true = EXCLUIDO (oculto por el filtro); en la UI `selected` = !hidden. */
  hidden: boolean;
}

/**
 * Entradas de un slicer (y del desplegable de autofiltro): valores ÚNICOS de la
 * columna con su estado de exclusión, en el orden del desplegable —es-ES
 * numérico, «(Vacías)» al final—. `cellTexts` = texto ya calculado de cada fila
 * del cuerpo (una por fila); `excluded` = valores actualmente ocultos.
 */
export function slicerEntries(
  cellTexts: readonly string[],
  excluded: readonly string[],
): SlicerEntry[] {
  const ex = new Set(excluded);
  const seen = new Map<string, boolean>();
  for (const t of cellTexts) if (!seen.has(t)) seen.set(t, ex.has(t));
  return [...seen]
    .map(([value, hidden]) => ({ value, hidden }))
    .sort((a, b) =>
      a.value === ''
        ? 1
        : b.value === ''
          ? -1
          : a.value.localeCompare(b.value, 'es', { numeric: true }),
    );
}

/**
 * Traduce una SELECCIÓN a su conjunto EXCLUIDO: dados los valores únicos y los
 * seleccionados (visibles), devuelve los que hay que ocultar = únicos \
 * seleccionados. Es justo lo que se guarda en `SheetAutoFilter.excl[col]` y lo
 * que oculta las filas (filteredRows del lienzo). Conserva el orden de `unique`.
 */
export function excludedFromSelection(
  unique: readonly string[],
  selected: Iterable<string>,
): string[] {
  const sel = new Set(selected);
  return unique.filter((v) => !sel.has(v));
}

/** Tope de x/y de la tarjeta sobre la rejilla (igual criterio que CHART_POS_MAX). */
const POS_MAX = 2200;

/** Slicer saneado desde datos no confiables (persistencia o comando); null si no
 *  vale. `col` debe caer en la rejilla; x/y se acotan como las tarjetas de gráfico.
 *  Un id ausente se regenera (docs editados a mano). */
export function sanitizeSlicer(raw: unknown): Slicer | null {
  if (!raw || typeof raw !== 'object') return null;
  const s = raw as Partial<Slicer>;
  const col = Math.trunc(Number(s.col));
  if (!Number.isInteger(col) || col < 0 || col >= MAX_COLS) return null;
  const out: Slicer = {
    id: typeof s.id === 'string' && s.id !== '' ? s.id : crypto.randomUUID(),
    col,
  };
  if (typeof s.x === 'number' && Number.isFinite(s.x))
    out.x = Math.min(POS_MAX, Math.max(0, Math.round(s.x)));
  if (typeof s.y === 'number' && Number.isFinite(s.y))
    out.y = Math.min(POS_MAX, Math.max(0, Math.round(s.y)));
  if (typeof s.pivot === 'string' && s.pivot !== '') out.pivot = s.pivot;
  return out;
}

/** El campo `field` del origen en la tabla dinámica: su referencia en filtros, filas o
 *  columnas, por ese orden (con su agrupación, para que los elementos sean los de la
 *  tabla); si no está en ella, su filtro oculto, o uno nuevo sin nada oculto. Como
 *  Excel, la segmentación admite cualquier campo del origen. */
export function pivotSlicerRef(pv: PivotDef, field: number): PivotFieldRef {
  return (
    [...(pv.filters ?? []), ...pv.rows, ...pv.cols, ...(pv.hiddenFilters ?? [])].find(
      (r) => r.field === field,
    ) ?? { field }
  );
}

/** La tabla con la selección de una segmentación: el campo oculta `hidden` (vacío =
 *  todo a la vista) en TODOS los sitios en que está, que es como Excel lo comparte con
 *  la lista de elementos del campo. Si no está en la tabla, va en su filtro oculto. */
export function pivotWithHidden<T extends PivotDef>(pv: T, field: number, hidden: string[]): T {
  const conHidden = (r: PivotFieldRef): PivotFieldRef => {
    const out: PivotFieldRef = { ...r };
    if (hidden.length > 0) out.hidden = [...hidden];
    else delete out.hidden;
    return out;
  };
  const poner = (refs: PivotFieldRef[]): PivotFieldRef[] =>
    refs.map((r) => (r.field === field ? conHidden(r) : r));
  const out: T = { ...pv, rows: poner(pv.rows), cols: poner(pv.cols) };
  if (pv.filters) out.filters = poner(pv.filters);
  const enTabla = [...(pv.filters ?? []), ...pv.rows, ...pv.cols].some((r) => r.field === field);
  const resto = (pv.hiddenFilters ?? []).filter((r) => r.field !== field);
  if (!enTabla && hidden.length > 0) resto.push(conHidden({ field }));
  if (resto.length > 0) out.hiddenFilters = resto;
  else delete out.hiddenFilters;
  return out;
}

/** La tabla sin el filtro oculto del campo, para cuando se quita su segmentación: nada
 *  más lo enseña, así que se va con ella (lo que oculta un campo que está en la tabla se
 *  queda: se ve y se cambia en la tabla). null si no tenía. */
export function pivotWithoutHiddenFilter<T extends PivotDef>(pv: T, field: number): T | null {
  const resto = (pv.hiddenFilters ?? []).filter((r) => r.field !== field);
  if (resto.length === (pv.hiddenFilters ?? []).length) return null;
  const out: T = { ...pv };
  if (resto.length > 0) out.hiddenFilters = resto;
  else delete out.hiddenFilters;
  return out;
}

/** Qué hay que hacer con la tabla dinámica al quitar la segmentación `sl`: null si
 *  nada (no tiene filtro oculto que llevarse), la tabla sin él para reescribirla, o
 *  'protected' si la hoja protegida no deja reescribirla (y entonces no se quita). */
export function pivotOnSlicerRemoval<T extends PivotDef & { name: string }>(
  sl: Slicer | undefined,
  pivots: readonly T[] | undefined,
  isProtected: boolean,
): T | 'protected' | null {
  const pv = sl?.pivot ? pivots?.find((p) => p.name === sl.pivot) : undefined;
  const sin = pv && sl ? pivotWithoutHiddenFilter(pv, sl.col) : null;
  if (!sin) return null;
  return isProtected ? 'protected' : sin;
}
