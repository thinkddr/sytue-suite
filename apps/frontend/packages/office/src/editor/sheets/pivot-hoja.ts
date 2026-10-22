/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Escribir una tabla dinámica en la hoja: leer su origen del motor, calcularla con
 * `computePivotTable` y volcar la rejilla como celdas normales, con su negrita y su
 * formato de número. Lo que ocupó la vez anterior se vacía antes, para que una tabla
 * que encoge no deje filas viejas debajo.
 *
 * No toca el estado del lienzo: devuelve la hoja nueva y la definición con su rango.
 */
import { formatNumber, numToCol, type CellValue, type Engine } from '@sytue/formula';
import type { NumFmt, SheetData, SheetModel } from '../types';
import type { DocsKey } from '../../i18n';
import { applyNumFmt, escapeHtml, formatCellValue, parseRangeRef } from './format';
import { formatWithMask } from './numfmt-mask';
import { computePivotTable } from './pivot';
import { MAX_COLS, MAX_ROWS } from './rejilla';
import type { PivotFn, PivotLabels, SheetPivot } from './pivot';

/** Hasta dónde puede llegar la tabla: los límites de la hoja, no lo que se pinta. */
export const PIVOT_GRID = { cols: MAX_COLS, rows: MAX_ROWS } as const;

const ref = (c: number, r: number): string => `${numToCol(c)}${r + 1}`;

/** Etiqueta de catálogo de cada función de resumen. */
export const PIVOT_FN_LABEL: Record<PivotFn, DocsKey> = {
  sum: 'query.agg.sum',
  count: 'pivot.agg.count',
  avg: 'query.agg.avg',
  max: 'query.agg.max',
  min: 'query.agg.min',
  product: 'pivot.agg.product',
  countNums: 'pivot.agg.countNums',
  stdev: 'pivot.agg.stdev',
  stdevp: 'pivot.agg.stdevp',
  var: 'pivot.agg.var',
  varp: 'pivot.agg.varp',
};

/** Número de serie del día 1 del mes m de 2001 (para sacar el nombre del mes). */
const monthSerial = (m: number): number =>
  (Date.UTC(2001, m - 1, 1) - Date.UTC(1899, 11, 30)) / 86400000;

/** Los rótulos de la tabla con el catálogo. `show` lo completa writePivot con el formato
 *  de cada columna del origen. */
export function pivotLabels(
  t: (k: DocsKey, v?: Record<string, string | number>) => string,
): PivotLabels {
  return {
    blank: t('pivot.blank'),
    grand: t('sheet.total.grand'),
    totalOf: (key) => t('sheet.total.of', { key }),
    valueName: (fn, field) => t('pivot.valueOf', { fn: t(PIVOT_FN_LABEL[fn]), field }),
    all: t('pivot.all'),
    multiple: t('pivot.multiple'),
    quarter: (q) => t('pivot.quarter', { n: q }),
    // Los nombres de mes y la fecha salen de las máscaras de la hoja, las mismas que
    // pinta una celda con formato de fecha: así coinciden con lo que se ve en el origen.
    month: (m) => formatWithMask('mmm', monthSerial(m)).text,
    day: (d) => applyNumFmt({ id: 'date' }, d),
    show: (v) => formatCellValue(v),
  };
}

/** El `pivot` de antes ({source, target, rowField, colField?, valueField, agg}), que
 *  pueden traer las macros grabadas, como definición nueva. null si no es de antes. */
export function legacyPivot(raw: unknown): Record<string, unknown> | null {
  if (!raw || typeof raw !== 'object' || !('rowField' in raw)) return null;
  const r = raw as Record<string, unknown>;
  const agg = r['agg'] === 'count' || r['agg'] === 'avg' ? r['agg'] : 'sum';
  return {
    source: r['source'],
    anchor: r['target'],
    rows: [{ field: r['rowField'] }],
    cols: r['colField'] === undefined || r['colField'] === '' ? [] : [{ field: r['colField'] }],
    values: [{ field: r['valueField'], fn: agg }],
  };
}

export type WritePivotResult =
  { sheet: SheetData; pivot: SheetPivot } | { error: 'source' | 'overlap' | 'room' };

/** Porcentaje de «mostrar valores como»: el de Excel, con dos decimales. */
const PERCENT: NumFmt = { id: 'percent', decimals: 2 };

/** El origen de la tabla `pv`, leído del motor: su rejilla (la fila 0, los
 *  encabezados), el formato de cada campo (el de su primera celda de datos con uno) y
 *  los rótulos con ese formato. null si el origen no se lee. Lo comparten writePivot y
 *  la segmentación de la tabla, para que los elementos se llamen igual en las dos. */
export function pivotSource(
  model: SheetModel,
  engine: Engine,
  active: number,
  pv: SheetPivot,
  labels: PivotLabels,
  size: { cols: number; rows: number } = PIVOT_GRID,
): {
  grid: CellValue[][];
  fieldFmt: (f: number) => NumFmt | undefined;
  labels: PivotLabels;
  rect: { c1: number; r1: number; c2: number; r2: number };
  sheet: SheetData;
} | null {
  const s = model.sheets[active];
  const src = pv.sourceSheet ? model.sheets.find((x) => x.name === pv.sourceSheet) : s;
  const rr = parseRangeRef(pv.source);
  if (!s || !src || !rr) return null;
  const c2 = Math.min(rr.c2, size.cols - 1);
  const r2 = Math.min(rr.r2, size.rows - 1);
  const grid: CellValue[][] = [];
  for (let r = rr.r1; r <= r2; r++) {
    const row: CellValue[] = [];
    for (let c = rr.c1; c <= c2; c++) row.push(engine.getValue(src.name, ref(c, r)));
    grid.push(row);
  }
  const fieldFmt = (f: number): NumFmt | undefined => {
    for (let r = rr.r1 + 1; r <= r2; r++) {
      const x = src.formats?.[ref(rr.c1 + f, r)];
      if (x) return x;
    }
    return undefined;
  };
  return {
    grid,
    fieldFmt,
    labels: { ...labels, show: (v, f) => formatCellValue(v, f >= 0 ? fieldFmt(f) : undefined) },
    rect: { c1: rr.c1, r1: rr.r1, c2, r2 },
    sheet: src,
  };
}

/**
 * Calcula y escribe la tabla `pv` en la hoja `active` del modelo. El motor es el del
 * modelo (se leen de él los valores del origen, ya calculados).
 */
export function writePivot(
  model: SheetModel,
  engine: Engine,
  active: number,
  pv: SheetPivot,
  labels: PivotLabels,
  size: { cols: number; rows: number } = PIVOT_GRID,
): WritePivotResult {
  const s = model.sheets[active];
  const origen = pivotSource(model, engine, active, pv, labels, size);
  const at = parseRangeRef(pv.anchor);
  if (!s || !origen || !at) return { error: 'source' };
  const { grid, fieldFmt, rect: rr, sheet: src } = origen;
  const { c2, r2 } = rr;
  // Los índices de valor de la rejilla cuentan solo los campos que existen en el origen.
  const values = pv.values.filter((v) => v.field < (grid[0]?.length ?? 0));
  const out = computePivotTable(grid, pv, origen.labels);
  const h = out.length;
  const w = Math.max(1, ...out.map((f) => f.length));
  if (at.c1 + w > size.cols || at.r1 + h > size.rows) return { error: 'room' };
  const range = { c1: at.c1, r1: at.r1, c2: at.c1 + w - 1, r2: at.r1 + h - 1 };
  if (src === s && range.c1 <= c2 && range.c2 >= rr.c1 && range.r1 <= r2 && range.r2 >= rr.r1) {
    return { error: 'overlap' };
  }

  const cells = { ...s.cells };
  const formats: Record<string, NumFmt> = { ...s.formats };
  const old = parseRangeRef(pv.range ?? '');
  if (old) {
    for (let r = old.r1; r <= old.r2; r++) {
      for (let c = old.c1; c <= old.c2; c++) {
        delete cells[ref(c, r)];
        delete formats[ref(c, r)];
      }
    }
  }
  out.forEach((fila, i) =>
    fila.forEach((cell, j) => {
      const k = ref(at.c1 + j, at.r1 + i);
      delete cells[k];
      delete formats[k];
      if (!cell) return;
      const raw =
        typeof cell.v === 'number'
          ? formatNumber(cell.v)
          : typeof cell.v === 'boolean'
            ? cell.v
              ? 'TRUE'
              : 'FALSE'
            : cell.v;
      if (raw === '') return;
      cells[k] = cell.bold ? `<b>${escapeHtml(raw)}</b>` : escapeHtml(raw);
      const f = cell.fmt;
      const fmt = !f
        ? undefined
        : 'pct' in f
          ? PERCENT
          : 'field' in f
            ? fieldFmt(f.field)
            : values[f.value]?.fmt;
      if (fmt) formats[k] = fmt;
    }),
  );
  const pivot: SheetPivot = {
    ...pv,
    range: `${ref(range.c1, range.r1)}:${ref(range.c2, range.r2)}`,
  };
  const before = s.pivots ?? [];
  const pivots = before.some((x) => x.name === pv.name)
    ? before.map((x) => (x.name === pv.name ? pivot : x))
    : [...before, pivot];
  const sheet: SheetData = { ...s, cells, pivots };
  if (Object.keys(formats).length > 0) sheet.formats = formats;
  else delete sheet.formats;
  return { sheet, pivot };
}
