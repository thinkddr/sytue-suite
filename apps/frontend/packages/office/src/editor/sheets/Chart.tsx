/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Gráficos incrustados de la hoja de cálculo: renderer SVG PROPIO (clean-room, sin
 * librerías) de columnas, barras, líneas, área (normales o apiladas), combinado,
 * circular, anillo, dispersión y radar sobre los VALORES calculados de un rango
 * (engine.getValue). La tarjeta flota sobre la rejilla, se arrastra por su cabecera,
 * cambia de tamaño por su esquina y persiste x/y/w/h en el ChartSpec al soltar.
 *
 * La geometría es una ESCENA pura (`chartScene`): rectángulos, trazos, círculos,
 * líneas y textos ya colocados. Este fichero la pinta como SVG; la app nativa
 * (`apps/fyne/calcus/graficos.go`) la porta y la pinta con las mismas coordenadas, y
 * `test/vectores/calcus.ts` comprueba que las dos escenas son la misma.
 *
 * Diseño: papel blanco SIEMPRE (como las celdas), retícula de hairlines 1 px,
 * ejes/rótulos recesivos en tinta apagada y series con el subconjunto AA de
 * CONTENT_PALETTE (≥3:1 sobre blanco, orden FIJO, validado también para daltonismo).
 * Se SALTAN #FFFFFF (blanco), #94A3B8 (casi blanco, 2,6:1), #F97316 (2,8:1) y
 * #0A0A0A (la tinta, reservada a texto); #475569 lee como gris, tampoco es serie.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import { numToCol } from '@sytue/formula';
import { Input, Select, Checkbox } from '@sytue/ui';
import type { CellValue, Engine } from '@sytue/formula';
import type { ChartLegend, ChartSpec } from '../types';
import { useT } from '../context';
import type { DocsKey } from '../../i18n';
import { EditorIcon } from '../icons';
import { CONTENT_PALETTE, HIGHLIGHT_PALETTE } from '../ribbon/primitives';
import {
  CHART_POS_MAX,
  CHART_SIZE_MAX,
  CHART_SIZE_MIN,
  formatCellValue,
  parseRangeRef,
} from './format';
import type { RangeRect } from './format';
import { MAX_COLS, MAX_ROWS } from './rejilla';

/* ---------- Paleta (solo hex ya permitidos: CONTENT/HIGHLIGHT_PALETTE) ---------- */

/** Orden FIJO de series (nunca ciclado): azul, rojo, verde, violeta, ámbar,
 *  rosa, verde azulado. Más series que colores no se pintan. */
export const SERIES_COLORS: readonly string[] = [
  CONTENT_PALETTE[8], // #2563EB azul
  CONTENT_PALETTE[5], // #DC2626 rojo
  CONTENT_PALETTE[7], // #16A34A verde
  CONTENT_PALETTE[9], // #7C3AED violeta
  CONTENT_PALETTE[6], // #D97706 ámbar
  CONTENT_PALETTE[10], // #DB2777 rosa
  CONTENT_PALETTE[11], // #0D9488 verde azulado
];

const INK = CONTENT_PALETTE[0]; // título sobre papel
const MUTED = CONTENT_PALETTE[1]; // rótulos de ejes y leyenda
const AXIS_COLOR = CONTENT_PALETTE[2]; // línea base del eje
const PAPER = CONTENT_PALETTE[3]; // separación entre marcas (porciones, puntos)
const OTHER = CONTENT_PALETTE[2]; // porción «Otros»: neutra a propósito
const GRID_LINE = HIGHLIGHT_PALETTE[6]; // #E2E8F0 hairlines de la retícula

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring).
const FOCUS_RING = 'sytue-focus-ring';

const NUM = new Intl.NumberFormat('es-ES', { maximumFractionDigits: 2 });
/** Número de un rótulo de eje o de dato: es-ES con hasta dos decimales. */
export const fmtNum = (n: number): string => NUM.format(n);

/* ---------- Medidas ---------- */

/** El lienzo de referencia: la tarjeta por defecto (w-80, 280 px con el borde) deja
 *  278 px de SVG, que es este viewBox de 320×180 visto a 278/320. Cambiar el tamaño
 *  del gráfico agranda el viewBox en la misma proporción: los textos no crecen. */
export const BASE_W = 320;
export const BASE_H = 180;
/** px de pantalla por unidad del viewBox. */
export const CHART_K = 278 / 320;
/** Tamaño por defecto del ÁREA del gráfico (el SVG) en px de maqueta. */
export const DEFAULT_PLOT_W = 278;
export const DEFAULT_PLOT_H = (278 * BASE_H) / BASE_W;
/** Ancho de la leyenda a la derecha (w-24) y su borde. */
export const LEGEND_SIDE_W = 84;

const PAD_T = 10;
const PAD_R = 8;
const PAD_B = 20;
const PAD_L = 42;
/** Holgura derecha AMPLIADA cuando hay eje secundario (deja sitio a sus rótulos). */
const PAD_R2 = 34;
/** Sitio de más que ocupa un título de eje. */
const AXIS_TITLE = 12;

/** Redondeo a centésima de px: escenas compactas y deterministas. */
const px = (n: number): number => Math.round(n * 100) / 100;

/* ---------- Datos: rango → tabla { rótulos, series } ---------- */

export interface ChartSeries {
  name: string;
  values: number[];
}
export interface ChartData {
  labels: string[];
  series: ChartSeries[];
}

/** Lector del rango en el sentido de las series: con `byRow`, se traspone (cada FILA
 *  es una serie), que es el «Cambiar filas/columnas» de Excel. */
function oriented(
  rect: RangeRect,
  get: (c: number, r: number) => CellValue,
  byRow: boolean,
): { rect: RangeRect; get: (c: number, r: number) => CellValue } {
  if (!byRow) return { rect, get };
  return {
    rect: { c1: rect.r1, r1: rect.c1, c2: rect.r2, r2: rect.c2 },
    get: (c, r) => get(r, c),
  };
}

/**
 * Interpreta el rango como tabla: con ≥2 columnas la PRIMERA es la de rótulos de
 * categoría; si además la primera fila de las columnas de datos es TODA texto, es
 * cabecera (nombra cada serie). Los valores no numéricos cuentan como 0. Sin columna
 * de rótulos se usa el número de fila; sin cabecera, la letra de columna. Con
 * `byRow` todo lo anterior vale cambiando filas por columnas.
 */
export function extractChartData(
  rect0: RangeRect,
  get0: (c: number, r: number) => CellValue,
  byRow = false,
): ChartData {
  const { rect, get } = oriented(rect0, get0, byRow);
  const name = (i: number): string => (byRow ? String(i + 1) : numToCol(i));
  const isText = (v: CellValue): boolean => typeof v === 'string' && v !== '';
  const hasLabelCol = rect.c2 > rect.c1;
  const dataC1 = hasLabelCol ? rect.c1 + 1 : rect.c1;
  let header = rect.r2 > rect.r1;
  for (let c = dataC1; header && c <= rect.c2; c++) {
    if (!isText(get(c, rect.r1))) header = false;
  }
  const dataR1 = header ? rect.r1 + 1 : rect.r1;
  const labels: string[] = [];
  for (let r = dataR1; r <= rect.r2; r++) {
    labels.push(
      hasLabelCol ? formatCellValue(get(rect.c1, r)) : byRow ? numToCol(r) : String(r + 1),
    );
  }
  const series: ChartSeries[] = [];
  for (let c = dataC1; c <= rect.c2; c++) {
    const values: number[] = [];
    for (let r = dataR1; r <= rect.r2; r++) {
      const v = get(c, r);
      values.push(typeof v === 'number' && Number.isFinite(v) ? v : 0);
    }
    series.push({ name: header ? formatCellValue(get(c, rect.r1)) : name(c), values });
  }
  return { labels, series };
}

/** Datos de dispersión (pares XY): 1.ª columna = X numérica CRUDA (no rótulos),
 *  resto = series Y; cabecera solo si la 1.ª fila es toda texto. Los valores no
 *  numéricos quedan como NaN y se descartan al pintar. */
export interface ChartXY {
  x: number[];
  series: ChartSeries[];
}
export function extractXY(
  rect0: RangeRect,
  get0: (c: number, r: number) => CellValue,
  byRow = false,
): ChartXY {
  const { rect, get } = oriented(rect0, get0, byRow);
  const isText = (v: CellValue): boolean => typeof v === 'string' && v !== '';
  let header = rect.r2 > rect.r1;
  for (let c = rect.c1; header && c <= rect.c2; c++) if (!isText(get(c, rect.r1))) header = false;
  const dataR1 = header ? rect.r1 + 1 : rect.r1;
  const num = (c: number, r: number): number => {
    const v = get(c, r);
    return typeof v === 'number' && Number.isFinite(v) ? v : NaN;
  };
  const x: number[] = [];
  for (let r = dataR1; r <= rect.r2; r++) x.push(num(rect.c1, r));
  const series: ChartSeries[] = [];
  for (let c = rect.c1 + 1; c <= rect.c2; c++) {
    const values: number[] = [];
    for (let r = dataR1; r <= rect.r2; r++) values.push(num(c, r));
    series.push({
      name: header ? formatCellValue(get(c, rect.r1)) : byRow ? String(c + 1) : numToCol(c),
      values,
    });
  }
  return { x, series };
}

/* ---------- Escala del eje de valores ---------- */

export interface Scale {
  min: number;
  max: number;
  ticks: number[];
}

/** Escala «bonita»: paso 1·2·5×10^k con ~4 divisiones que cubre [min, max]. */
export function niceTicks(min: number, max: number): Scale {
  if (!Number.isFinite(min)) min = 0;
  if (!Number.isFinite(max)) max = 0;
  if (min > max) [min, max] = [max, min];
  if (min === max) {
    if (max > 0) min = 0;
    else if (max < 0) max = 0;
    else max = 1;
  }
  const raw = (max - min) / 4;
  const mag = 10 ** Math.floor(Math.log10(raw));
  const norm = raw / mag;
  const step = (norm > 5 ? 10 : norm > 2 ? 5 : norm > 1 ? 2 : 1) * mag;
  const lo = Math.floor(min / step) * step;
  const count = Math.round((Math.ceil(max / step) * step - lo) / step);
  const ticks: number[] = [];
  for (let i = 0; i <= count; i++) ticks.push(Number((lo + i * step).toPrecision(12)));
  return { min: ticks[0] ?? 0, max: ticks[count] ?? 1, ticks };
}

/** Interpola `v∈[min,max]` a `[lo,hi]`; dominio degenerado → punto medio. */
export function project(v: number, min: number, max: number, lo: number, hi: number): number {
  return max === min ? (lo + hi) / 2 : lo + ((v - min) / (max - min)) * (hi - lo);
}

/** Vértice `i` de un polígono regular de `n` lados (i=0 arriba, sentido horario)
 *  a fracción `frac∈[0,1]` del radio `R` desde (cx,cy). Base del radar. */
export function polarPoint(
  cx: number,
  cy: number,
  R: number,
  i: number,
  n: number,
  frac: number,
): { x: number; y: number } {
  const a = -Math.PI / 2 + (i / Math.max(1, n)) * 2 * Math.PI;
  return { x: cx + R * frac * Math.cos(a), y: cy + R * frac * Math.sin(a) };
}

/** Escala «bonita» de un conjunto de valores; `zero` fuerza incluir el 0 (barras
 *  y áreas: un eje truncado miente). Ignora NaN y no divide por cero. */
function scaleOf(vals: number[], zero: boolean): Scale {
  const fin = vals.filter((v) => Number.isFinite(v));
  const lo = zero ? Math.min(0, ...fin) : Math.min(...fin);
  const hi = zero ? Math.max(0, ...fin) : Math.max(...fin);
  return niceTicks(lo, hi);
}

/* ---------- La escena ---------- */

/** Una marca de la escena, en unidades del viewBox. `title` es el rótulo emergente. */
export type ChartMark =
  | {
      k: 'rect';
      x: number;
      y: number;
      w: number;
      h: number;
      fill: string;
      title?: string;
    }
  | {
      k: 'path';
      d: string;
      fill: string;
      /** Opacidad del relleno (áreas, radar); ausente = 1. */
      op?: number;
      stroke?: string;
      sw?: number;
      /** Trazo redondeado (líneas y radar). */
      round?: boolean;
      title?: string;
    }
  | {
      k: 'circle';
      cx: number;
      cy: number;
      r: number;
      fill: string;
      op?: number;
      stroke?: string;
      sw?: number;
      title?: string;
    }
  | { k: 'line'; x1: number; y1: number; x2: number; y2: number; stroke: string }
  | {
      k: 'text';
      x: number;
      y: number;
      text: string;
      size: number;
      fill: string;
      anchor: 'start' | 'middle' | 'end';
    };

export interface ChartScene {
  /** viewBox (unidades del SVG); se pinta a `CHART_K` px por unidad. */
  w: number;
  h: number;
  marks: ChartMark[];
}

export interface LegendItem {
  name: string;
  color: string;
}

/** Color de la serie (o porción) `k`: el elegido en el editor o el de su puesto. */
export function seriesColor(spec: Pick<ChartSpec, 'colors'>, k: number): string {
  const c = spec.colors?.[k];
  return c ? c : (SERIES_COLORS[k] ?? OTHER);
}

/** Tipos con eje de categorías y valores en vertical. */
const CARTESIAN_TYPES = new Set<ChartSpec['type']>(['bar', 'line', 'area', 'combo']);
/** Tipos que se pueden apilar. */
export const STACKABLE_TYPES = new Set<ChartSpec['type']>(['bar', 'hbar', 'area']);
/** Tipos con ejes (títulos de eje y retícula). */
export const AXIS_TYPES = new Set<ChartSpec['type']>([
  'bar',
  'hbar',
  'line',
  'area',
  'combo',
  'scatter',
]);

/** Tamaño del viewBox de un gráfico: el área en px (o la de por defecto) entre CHART_K,
 *  menos la leyenda a la derecha si la hay. */
export function sceneSize(
  spec: Pick<ChartSpec, 'w' | 'h' | 'legend'>,
  legendShown: boolean,
): { w: number; h: number } {
  const plotW =
    (spec.w ?? DEFAULT_PLOT_W) - (legendShown && spec.legend === 'right' ? LEGEND_SIDE_W + 1 : 0);
  return { w: px(plotW / CHART_K), h: px((spec.h ?? DEFAULT_PLOT_H) / CHART_K) };
}

/** Recorta un rótulo largo con «…». */
const clip = (t: string, n: number): string => (t.length > n ? `${t.slice(0, n - 1)}…` : t);

interface Frame {
  W: number;
  H: number;
  l: number;
  r: number;
  t: number;
  b: number;
}
const plotW = (f: Frame): number => f.W - f.l - f.r;
const plotH = (f: Frame): number => f.H - f.t - f.b;

function frameFor(spec: ChartSpec, w: number, h: number, padR: number): Frame {
  return {
    W: w,
    H: h,
    l: PAD_L,
    r: padR,
    t: PAD_T + (spec.yTitle ? AXIS_TITLE : 0),
    b: PAD_B + (spec.xTitle ? AXIS_TITLE : 0),
  };
}

const yOf = (f: Frame, sc: Scale, v: number): number =>
  f.t + ((sc.max - v) / (sc.max - sc.min)) * plotH(f);

/** Títulos de los ejes: el de valores arriba a la izquierda (en horizontal: la app
 *  nativa no gira texto) y el de categorías centrado debajo. */
function axisTitles(spec: ChartSpec, f: Frame, out: ChartMark[]): void {
  if (spec.yTitle) {
    out.push({
      k: 'text',
      x: 4,
      y: 12,
      text: clip(spec.yTitle, 40),
      size: 9,
      fill: MUTED,
      anchor: 'start',
    });
  }
  if (spec.xTitle) {
    out.push({
      k: 'text',
      x: px(f.l + plotW(f) / 2),
      y: px(f.H - 4),
      text: clip(spec.xTitle, 40),
      size: 9,
      fill: MUTED,
      anchor: 'middle',
    });
  }
}

/** Retícula horizontal + rótulos de valor a la izquierda + línea base. */
function valueAxis(spec: ChartSpec, f: Frame, sc: Scale, axisY: number, out: ChartMark[]): void {
  for (const t of sc.ticks) {
    const y = px(yOf(f, sc, t));
    if (spec.grid !== false)
      out.push({ k: 'line', x1: f.l, x2: f.W - f.r, y1: y, y2: y, stroke: GRID_LINE });
    out.push({
      k: 'text',
      x: f.l - 5,
      y: y + 3,
      text: fmtNum(t),
      size: 9,
      fill: MUTED,
      anchor: 'end',
    });
  }
  const y = px(axisY);
  out.push({ k: 'line', x1: f.l, x2: f.W - f.r, y1: y, y2: y, stroke: AXIS_COLOR });
}

/** Rótulos de categoría bajo el eje: como mucho ~6 legibles. */
function categoryLabels(labels: string[], f: Frame, out: ChartMark[]): void {
  const n = Math.max(1, labels.length);
  const slot = plotW(f) / n;
  const step = Math.ceil(n / 6);
  labels.forEach((t, i) => {
    if (i % step !== 0 || t === '') return;
    out.push({
      k: 'text',
      x: px(f.l + slot * (i + 0.5)),
      y: px(f.t + plotH(f) + 14),
      text: clip(t, 9),
      size: 9,
      fill: MUTED,
      anchor: 'middle',
    });
  });
}

type CartMode = 'bar' | 'line' | 'area' | 'combo';

/** Columnas / líneas / área / combinado, con eje secundario y apilado opcionales. */
function cartesian(spec: ChartSpec, data: ChartData, w: number, h: number): ChartMark[] {
  const mode = spec.type as CartMode;
  const out: ChartMark[] = [];
  const series = data.series.slice(0, SERIES_COLORS.length);
  const n = Math.max(1, data.labels.length);
  const stacked = spec.stacked === true && (mode === 'bar' || mode === 'area');
  const secSet = new Set(
    stacked ? [] : (spec.secondaryAxis ?? []).filter((k) => k < series.length),
  );
  const hasSec = secSet.size > 0 && secSet.size < series.length; // 2º eje solo con series en ambos lados
  const onSec = (k: number): boolean => hasSec && secSet.has(k);
  const f = frameFor(spec, w, h, hasSec ? PAD_R2 : PAD_R);
  const lineReq = (spec.lineSeries ?? []).filter((k) => k < series.length);
  const lineSet =
    mode === 'combo'
      ? new Set(lineReq.length > 0 ? lineReq : [series.length - 1])
      : new Set<number>();
  const isLine = (k: number): boolean =>
    mode === 'line' || mode === 'area' || (mode === 'combo' && lineSet.has(k));

  // Apilado: acumulados por categoría (positivos y negativos por separado, como Excel).
  const base: number[][] = series.map(() => new Array<number>(data.labels.length).fill(0));
  const top: number[][] = series.map((s) => s.values.slice());
  if (stacked) {
    const pos = new Array<number>(data.labels.length).fill(0);
    const neg = new Array<number>(data.labels.length).fill(0);
    series.forEach((s, k) =>
      s.values.forEach((v, i) => {
        const acc = mode === 'area' || v >= 0 ? pos : neg;
        base[k]![i] = acc[i]!;
        acc[i] = acc[i]! + v;
        top[k]![i] = acc[i]!;
      }),
    );
  }

  const baseZero = mode !== 'line';
  const scPri = scaleOf(
    stacked
      ? [...top.flat(), ...base.flat()]
      : series.flatMap((s, k) => (onSec(k) ? [] : s.values)),
    baseZero,
  );
  // ponytail: el eje 2º no fuerza cero (suele llevar la línea de otra magnitud).
  const scSec = hasSec
    ? scaleOf(
        series.flatMap((s, k) => (onSec(k) ? s.values : [])),
        false,
      )
    : null;
  const scaleK = (k: number): Scale => (onSec(k) && scSec ? scSec : scPri);
  const axisY = baseZero ? yOf(f, scPri, 0) : yOf(f, scPri, scPri.min);
  axisTitles(spec, f, out);
  valueAxis(spec, f, scPri, axisY, out);

  const slot = plotW(f) / n;
  const xc = (i: number): number => f.l + slot * (i + 0.5);
  const barKs = series.map((_, k) => k).filter((k) => !isLine(k));
  const group = slot * 0.72;
  const bw = stacked ? group : group / Math.max(1, barKs.length);
  const dots = data.labels.length <= 24; // con más puntos, la línea sola
  const tip = (i: number, s: ChartSeries, v: number): string =>
    `${data.labels[i] ?? ''} — ${s.name}: ${fmtNum(v)}`;
  const labels: ChartMark[] = [];

  // Áreas (bajo las líneas): polígono hasta la base del eje (o la serie de debajo).
  if (mode === 'area') {
    series.forEach((s, k) => {
      const sc = scaleK(k);
      const floor = yOf(f, sc, baseZero ? 0 : sc.min);
      const up = top[k]!.map(
        (v, i) => `${i === 0 ? 'M' : 'L'}${px(xc(i))} ${px(yOf(f, sc, v))}`,
      ).join('');
      const down = stacked
        ? base[k]!.map((v, i) => `L${px(xc(i))} ${px(yOf(f, sc, v))}`)
            .reverse()
            .join('')
        : `L${px(xc(s.values.length - 1))} ${px(floor)}L${px(xc(0))} ${px(floor)}`;
      out.push({ k: 'path', d: `${up}${down}Z`, fill: seriesColor(spec, k), op: 0.18 });
    });
  }
  // Columnas (series no-línea): agrupadas entre sí, o una encima de otra.
  barKs.forEach((k, j) => {
    const sc = scaleK(k);
    const s = series[k]!;
    s.values.forEach((v, i) => {
      const y0 = yOf(f, sc, stacked ? base[k]![i]! : 0);
      const yv = yOf(f, sc, stacked ? top[k]![i]! : v);
      const x = f.l + i * slot + (slot - group) / 2 + (stacked ? 0 : j * bw) + 1;
      const hgt = Math.abs(yv - y0);
      out.push({
        k: 'rect',
        x: px(x),
        y: px(Math.min(yv, y0)),
        w: px(Math.max(1, bw - 2)),
        h: px(hgt),
        fill: seriesColor(spec, k),
        title: tip(i, s, v),
      });
      if (spec.labels && v !== 0) {
        const cx = px(x + Math.max(1, bw - 2) / 2);
        if (stacked) {
          if (hgt >= 11)
            labels.push({
              k: 'text',
              x: cx,
              y: px((yv + y0) / 2 + 3),
              text: fmtNum(v),
              size: 8,
              fill: PAPER,
              anchor: 'middle',
            });
        } else {
          labels.push({
            k: 'text',
            x: cx,
            y: px(v >= 0 ? Math.min(yv, y0) - 2 : Math.max(yv, y0) + 8),
            text: fmtNum(v),
            size: 8,
            fill: MUTED,
            anchor: 'middle',
          });
        }
      }
    });
  });
  // Líneas (+ puntos).
  series.forEach((s, k) => {
    if (!isLine(k)) return;
    const sc = scaleK(k);
    const color = seriesColor(spec, k);
    const vals = stacked ? top[k]! : s.values;
    out.push({
      k: 'path',
      d: vals.map((v, i) => `${i === 0 ? 'M' : 'L'}${px(xc(i))} ${px(yOf(f, sc, v))}`).join(''),
      fill: 'none',
      stroke: color,
      sw: 2,
      round: true,
    });
    vals.forEach((v, i) => {
      const cy = px(yOf(f, sc, v));
      if (dots)
        out.push({
          k: 'circle',
          cx: px(xc(i)),
          cy,
          r: 2.5,
          fill: color,
          stroke: PAPER,
          sw: 1,
          title: tip(i, s, s.values[i] ?? 0),
        });
      if (spec.labels)
        labels.push({
          k: 'text',
          x: px(xc(i)),
          y: px(cy - 5),
          text: fmtNum(s.values[i] ?? 0),
          size: 8,
          fill: MUTED,
          anchor: 'middle',
        });
    });
  });
  // Eje secundario (derecha): línea + marcas de su propia escala.
  if (scSec) {
    const x = px(f.W - PAD_R2);
    out.push({ k: 'line', x1: x, x2: x, y1: f.t, y2: px(f.t + plotH(f)), stroke: AXIS_COLOR });
    for (const t of scSec.ticks)
      out.push({
        k: 'text',
        x: px(x + 4),
        y: px(yOf(f, scSec, t) + 3),
        text: fmtNum(t),
        size: 9,
        fill: MUTED,
        anchor: 'start',
      });
  }
  categoryLabels(data.labels, f, out);
  return [...out, ...labels];
}

/** Barras horizontales: categorías en vertical (la primera abajo, como Excel y Calc)
 *  y valores en horizontal; agrupadas o apiladas. */
function horizontalBars(spec: ChartSpec, data: ChartData, w: number, h: number): ChartMark[] {
  const out: ChartMark[] = [];
  const series = data.series.slice(0, SERIES_COLORS.length);
  const n = Math.max(1, data.labels.length);
  const stacked = spec.stacked === true;
  const f = frameFor(spec, w, h, PAD_R);
  const base: number[][] = series.map(() => new Array<number>(data.labels.length).fill(0));
  const top: number[][] = series.map((s) => s.values.slice());
  if (stacked) {
    const pos = new Array<number>(data.labels.length).fill(0);
    const neg = new Array<number>(data.labels.length).fill(0);
    series.forEach((s, k) =>
      s.values.forEach((v, i) => {
        const acc = v >= 0 ? pos : neg;
        base[k]![i] = acc[i]!;
        acc[i] = acc[i]! + v;
        top[k]![i] = acc[i]!;
      }),
    );
  }
  const sc = scaleOf(
    stacked ? [...top.flat(), ...base.flat()] : series.flatMap((s) => s.values),
    true,
  );
  const xOf = (v: number): number => project(v, sc.min, sc.max, f.l, f.W - f.r);
  axisTitles(spec, f, out);
  for (const t of sc.ticks) {
    const x = px(xOf(t));
    if (spec.grid !== false)
      out.push({ k: 'line', x1: x, x2: x, y1: f.t, y2: px(f.t + plotH(f)), stroke: GRID_LINE });
    out.push({
      k: 'text',
      x,
      y: px(f.t + plotH(f) + 14),
      text: fmtNum(t),
      size: 9,
      fill: MUTED,
      anchor: 'middle',
    });
  }
  const x0 = px(xOf(0));
  out.push({ k: 'line', x1: x0, x2: x0, y1: f.t, y2: px(f.t + plotH(f)), stroke: AXIS_COLOR });
  const slot = plotH(f) / n;
  const group = slot * 0.72;
  const bh = stacked ? group : group / Math.max(1, series.length);
  const labels: ChartMark[] = [];
  const step = Math.ceil(n / 8);
  data.labels.forEach((t, i) => {
    if (i % step !== 0 || t === '') return;
    out.push({
      k: 'text',
      x: f.l - 5,
      y: px(f.t + (n - 1 - i) * slot + slot / 2 + 3),
      text: clip(t, 9),
      size: 9,
      fill: MUTED,
      anchor: 'end',
    });
  });
  series.forEach((s, k) => {
    s.values.forEach((v, i) => {
      const a = xOf(stacked ? base[k]![i]! : 0);
      const b = xOf(stacked ? top[k]![i]! : v);
      const y = f.t + (n - 1 - i) * slot + (slot - group) / 2 + (stacked ? 0 : k * bh) + 1;
      const hh = Math.max(1, bh - 2);
      out.push({
        k: 'rect',
        x: px(Math.min(a, b)),
        y: px(y),
        w: px(Math.abs(b - a)),
        h: px(hh),
        fill: seriesColor(spec, k),
        title: `${data.labels[i] ?? ''} — ${s.name}: ${fmtNum(v)}`,
      });
      if (spec.labels && v !== 0) {
        if (stacked) {
          if (Math.abs(b - a) >= 18)
            labels.push({
              k: 'text',
              x: px((a + b) / 2),
              y: px(y + hh / 2 + 3),
              text: fmtNum(v),
              size: 8,
              fill: PAPER,
              anchor: 'middle',
            });
        } else {
          labels.push({
            k: 'text',
            x: px(v >= 0 ? Math.max(a, b) + 3 : Math.min(a, b) - 3),
            y: px(y + hh / 2 + 3),
            text: fmtNum(v),
            size: 8,
            fill: MUTED,
            anchor: v >= 0 ? 'start' : 'end',
          });
        }
      }
    });
  });
  return [...out, ...labels];
}

function scatter(
  spec: ChartSpec,
  xy: ChartXY,
  labels0: string[],
  w: number,
  h: number,
): ChartMark[] {
  const out: ChartMark[] = [];
  const f = frameFor(spec, w, h, PAD_R);
  // Sin columnas Y (rango de una sola columna): valor frente al índice de fila.
  const ySeries = xy.series.length > 0 ? xy.series : [{ name: labels0[0] ?? '', values: xy.x }];
  const xs = xy.series.length > 0 ? xy.x : xy.x.map((_, i) => i);
  const xsFin = xs.filter((v) => Number.isFinite(v));
  const yFin = ySeries.flatMap((s) => s.values).filter((v) => Number.isFinite(v));
  const xsc = niceTicks(
    xsFin.length ? Math.min(...xsFin) : 0,
    xsFin.length ? Math.max(...xsFin) : 1,
  );
  const ysc = niceTicks(
    yFin.length ? Math.min(0, ...yFin) : 0,
    yFin.length ? Math.max(0, ...yFin) : 1,
  );
  const xOf = (v: number): number => project(v, xsc.min, xsc.max, f.l, f.W - f.r);
  axisTitles(spec, f, out);
  valueAxis(spec, f, ysc, yOf(f, ysc, ysc.min), out);
  for (const t of xsc.ticks)
    out.push({
      k: 'text',
      x: px(xOf(t)),
      y: px(f.t + plotH(f) + 14),
      text: fmtNum(t),
      size: 9,
      fill: MUTED,
      anchor: 'middle',
    });
  const labels: ChartMark[] = [];
  ySeries.slice(0, SERIES_COLORS.length).forEach((s, k) => {
    const color = seriesColor(spec, k);
    s.values.forEach((v, i) => {
      const xv = xs[i];
      if (xv === undefined || !Number.isFinite(xv) || !Number.isFinite(v)) return;
      const cx = px(xOf(xv));
      const cy = px(yOf(f, ysc, v));
      out.push({
        k: 'circle',
        cx,
        cy,
        r: 2.6,
        fill: color,
        op: 0.85,
        title: `${s.name}: (${fmtNum(xv)}, ${fmtNum(v)})`,
      });
      if (spec.labels)
        labels.push({
          k: 'text',
          x: cx,
          y: px(cy - 5),
          text: fmtNum(v),
          size: 8,
          fill: MUTED,
          anchor: 'middle',
        });
    });
  });
  return [...out, ...labels];
}

function radar(spec: ChartSpec, data: ChartData, w: number, h: number): ChartMark[] {
  const out: ChartMark[] = [];
  const series = data.series.slice(0, SERIES_COLORS.length);
  const n = data.labels.length;
  const max = Math.max(1, 0, ...series.flatMap((s) => s.values).filter((v) => Number.isFinite(v)));
  const cx = w / 2;
  const cy = h / 2;
  const R = Math.min(h - PAD_T - PAD_B, h) / 2 - 8;
  const poly = (frac: (i: number) => number): string =>
    data.labels
      .map((_, i) => {
        const p = polarPoint(cx, cy, R, i, n, frac(i));
        return `${i === 0 ? 'M' : 'L'}${px(p.x)} ${px(p.y)}`;
      })
      .join('') + 'Z';
  for (const fr of [0.5, 1])
    out.push({ k: 'path', d: poly(() => fr), fill: 'none', stroke: GRID_LINE, sw: 1 });
  data.labels.forEach((_, i) => {
    const p = polarPoint(cx, cy, R, i, n, 1);
    out.push({ k: 'line', x1: px(cx), y1: px(cy), x2: px(p.x), y2: px(p.y), stroke: GRID_LINE });
  });
  series.forEach((s, k) => {
    const color = seriesColor(spec, k);
    out.push({
      k: 'path',
      d: poly((i) => Math.max(0, s.values[i] ?? 0) / max),
      fill: color,
      op: 0.14,
      stroke: color,
      sw: 1.5,
      round: true,
      title: s.name,
    });
  });
  data.labels.forEach((t, i) => {
    const p = polarPoint(cx, cy, R, i, n, 1.12);
    out.push({
      k: 'text',
      x: px(p.x),
      y: px(p.y + 3),
      text: clip(t, 7),
      size: 8,
      fill: MUTED,
      anchor: 'middle',
    });
  });
  return out;
}

interface PiePart {
  name: string;
  value: number;
  color: string;
}

/** Porciones del circular: solo valores > 0 de la PRIMERA serie; a partir de
 *  la séptima categoría se agrupan en «Otros» (neutro) para no ciclar tonos. */
export function pieParts(spec: ChartSpec, data: ChartData, otherLabel: string): PiePart[] {
  const first = data.series[0];
  if (!first) return [];
  const pos = data.labels
    .map((name, i) => ({ name, value: Math.max(0, first.values[i] ?? 0) }))
    .filter((p) => p.value > 0);
  if (pos.length <= SERIES_COLORS.length) {
    return pos.map((p, i) => ({ ...p, color: seriesColor(spec, i) }));
  }
  const head = pos.slice(0, SERIES_COLORS.length - 1);
  const rest = pos.slice(SERIES_COLORS.length - 1).reduce((sum, p) => sum + p.value, 0);
  return [
    ...head.map((p, i) => ({ ...p, color: seriesColor(spec, i) })),
    { name: otherLabel, value: rest, color: OTHER },
  ];
}

function pie(spec: ChartSpec, parts: readonly PiePart[], w: number, h: number): ChartMark[] {
  const out: ChartMark[] = [];
  const total = parts.reduce((sum, p) => sum + p.value, 0);
  const cx = w / 2;
  const cy = h / 2;
  const r = h / 2 - 12;
  const donut = spec.type === 'donut';
  const labels: ChartMark[] = [];
  let a = -Math.PI / 2; // arranca a las 12 y gira en el sentido del reloj
  parts.forEach((p) => {
    const frac = p.value / total;
    const a1 = a + frac * 2 * Math.PI;
    const title = `${p.name}: ${fmtNum(p.value)} (${fmtNum(frac * 100)} %)`;
    if (frac > 0.9995) {
      out.push({
        k: 'circle',
        cx: px(cx),
        cy: px(cy),
        r: px(r),
        fill: p.color,
        stroke: PAPER,
        sw: 2,
        title,
      });
    } else {
      const x0 = cx + r * Math.cos(a);
      const y0 = cy + r * Math.sin(a);
      const x1 = cx + r * Math.cos(a1);
      const y1 = cy + r * Math.sin(a1);
      out.push({
        k: 'path',
        d: `M${px(cx)} ${px(cy)}L${px(x0)} ${px(y0)}A${px(r)} ${px(r)} 0 ${frac > 0.5 ? 1 : 0} 1 ${px(x1)} ${px(y1)}Z`,
        fill: p.color,
        stroke: PAPER,
        sw: 2,
        title,
      });
    }
    if (spec.labels && frac >= 0.05) {
      const mid = (a + a1) / 2;
      const rr = r * (donut ? 0.79 : 0.62);
      labels.push({
        k: 'text',
        x: px(cx + rr * Math.cos(mid)),
        y: px(cy + rr * Math.sin(mid) + 3),
        text: `${fmtNum(Math.round(frac * 100))} %`,
        size: 8,
        fill: PAPER,
        anchor: 'middle',
      });
    }
    a = a1;
  });
  // Anillo: el circular con un hueco central en color papel (donut de Excel).
  if (donut) out.push({ k: 'circle', cx: px(cx), cy: px(cy), r: px(r * 0.58), fill: PAPER });
  return [...out, ...labels];
}

/** Lo que hay que pintar de un gráfico: la escena (vacía si no hay datos), la leyenda
 *  y los nombres de sus series. `get` lee los VALORES calculados de la hoja. */
export interface ChartView {
  scene: ChartScene;
  legend: LegendItem[];
  seriesNames: string[];
  empty: boolean;
}

/** ¿Se enseña la leyenda? Con ≥2 entradas (una sola serie ya la nombra el título). */
export const legendVisible = (spec: Pick<ChartSpec, 'legend'>, items: number): boolean =>
  spec.legend !== 'none' && items >= 2;

export function chartView(
  spec: ChartSpec,
  get: (c: number, r: number) => CellValue,
  otherLabel: string,
): ChartView {
  const rect = parseRangeRef(spec.range);
  // Acotado a los límites de la hoja. Antes era A..Z × 1..100, y un gráfico sobre
  // datos importados de 500 filas pintaba solo las cien primeras sin avisar.
  // ponytail: un rango enorme escrito a mano (A1:B1048576) se lee entero; acotarlo a
  // lo usado pediría el motor aquí, que hoy solo recibe `get`.
  const clamped = rect
    ? {
        c1: Math.min(rect.c1, MAX_COLS - 1),
        c2: Math.min(rect.c2, MAX_COLS - 1),
        r1: Math.min(rect.r1, MAX_ROWS - 1),
        r2: Math.min(rect.r2, MAX_ROWS - 1),
      }
    : null;
  const byRow = spec.byRow === true;
  const data = clamped ? extractChartData(clamped, get, byRow) : null;
  const isPie = spec.type === 'pie' || spec.type === 'donut';
  const parts = isPie && data ? pieParts(spec, data, otherLabel) : null;
  const legend: LegendItem[] =
    parts?.map((p) => ({ name: p.name, color: p.color })) ??
    (data?.series ?? [])
      .slice(0, SERIES_COLORS.length)
      .map((s, i) => ({ name: s.name, color: seriesColor(spec, i) }));
  const empty = !data || data.labels.length === 0 || (parts !== null && parts.length === 0);
  const { w, h } = sceneSize(spec, legendVisible(spec, legend.length));
  let marks: ChartMark[] = [];
  if (!empty && data) {
    if (CARTESIAN_TYPES.has(spec.type)) marks = cartesian(spec, data, w, h);
    else if (spec.type === 'hbar') marks = horizontalBars(spec, data, w, h);
    else if (spec.type === 'scatter' && clamped)
      marks = scatter(spec, extractXY(clamped, get, byRow), data.labels, w, h);
    else if (spec.type === 'radar') marks = radar(spec, data, w, h);
    else if (parts) marks = pie(spec, parts, w, h);
  }
  return {
    scene: { w, h, marks },
    legend,
    seriesNames: (data?.series ?? []).slice(0, SERIES_COLORS.length).map((s) => s.name),
    empty,
  };
}

/* ---------- Pintar la escena como SVG ---------- */

function SceneSvg({ scene, label }: { scene: ChartScene; label: string }): JSX.Element {
  return (
    <svg
      viewBox={`0 0 ${scene.w} ${scene.h}`}
      class="block w-full"
      role="img"
      aria-label={label}
      style={{ height: `${px(scene.h * CHART_K)}px` }}
    >
      {scene.marks.map((m, i) => {
        switch (m.k) {
          case 'rect':
            return (
              <rect
                key={i}
                x={m.x}
                y={m.y}
                width={m.w}
                height={m.h}
                fill={m.fill}
                shape-rendering="crispEdges"
              >
                {m.title ? <title>{m.title}</title> : null}
              </rect>
            );
          case 'path':
            return (
              <path
                key={i}
                d={m.d}
                fill={m.fill}
                fill-opacity={m.op}
                stroke={m.stroke ?? 'none'}
                stroke-width={m.sw}
                stroke-linecap={m.round ? 'round' : undefined}
                stroke-linejoin="round"
              >
                {m.title ? <title>{m.title}</title> : null}
              </path>
            );
          case 'circle':
            return (
              <circle
                key={i}
                cx={m.cx}
                cy={m.cy}
                r={m.r}
                fill={m.fill}
                fill-opacity={m.op}
                stroke={m.stroke}
                stroke-width={m.sw}
              >
                {m.title ? <title>{m.title}</title> : null}
              </circle>
            );
          case 'line':
            return (
              <line
                key={i}
                x1={m.x1}
                y1={m.y1}
                x2={m.x2}
                y2={m.y2}
                stroke={m.stroke}
                stroke-width={1}
                shape-rendering="crispEdges"
              />
            );
          default:
            return (
              <text key={i} x={m.x} y={m.y} text-anchor={m.anchor} font-size={m.size} fill={m.fill}>
                {m.text}
              </text>
            );
        }
      })}
    </svg>
  );
}

/* ---------- Tarjeta flotante ---------- */

/** Tipo → CLAVE del catálogo (no rótulo): un mapa de módulo con texto se
 *  congelaría con el idioma de la primera carga. La `t` lo resuelve al pintar. */
const TYPE_KEY: Record<ChartSpec['type'], DocsKey> = {
  bar: 'chart.type.bar',
  hbar: 'chart.type.hbar',
  line: 'chart.type.line',
  area: 'chart.type.area',
  combo: 'chart.type.combo',
  pie: 'chart.type.pie',
  donut: 'chart.type.donut',
  scatter: 'chart.type.scatter',
  radar: 'chart.type.radar',
};

/** Posiciones de la leyenda con su clave. */
export const LEGEND_OPTIONS: { value: ChartLegend; labelKey: DocsKey }[] = [
  { value: 'bottom', labelKey: 'chart.legend.bottom' },
  { value: 'top', labelKey: 'chart.legend.top' },
  { value: 'right', labelKey: 'chart.legend.right' },
  { value: 'none', labelKey: 'chart.legend.none' },
];

/** Editor del gráfico: datos (tipo, rango, filas/columnas), título y ejes, leyenda,
 *  etiquetas, retícula, apilado y, por serie, color, línea (combinado) y eje 2º.
 *  Ocupa el cuerpo de la tarjeta al pulsar el ajuste. */
function ChartEditor({
  spec,
  seriesNames,
  onApply,
  onCancel,
}: {
  spec: ChartSpec;
  seriesNames: string[];
  onApply: (patch: Partial<ChartSpec>) => void;
  onCancel: () => void;
}): JSX.Element {
  const t = useT();
  const [type, setType] = useState<ChartSpec['type']>(spec.type);
  const [title, setTitle] = useState(spec.title ?? '');
  const [range, setRange] = useState(spec.range);
  const [byRow, setByRow] = useState(spec.byRow === true);
  const [stacked, setStacked] = useState(spec.stacked === true);
  const [legend, setLegend] = useState<ChartLegend>(spec.legend ?? 'bottom');
  const [labels, setLabels] = useState(spec.labels === true);
  const [grid, setGrid] = useState(spec.grid !== false);
  const [xTitle, setXTitle] = useState(spec.xTitle ?? '');
  const [yTitle, setYTitle] = useState(spec.yTitle ?? '');
  const [colors, setColors] = useState<string[]>(spec.colors ?? []);
  const [lineSeries, setLineSeries] = useState<number[]>(spec.lineSeries ?? []);
  const [secondaryAxis, setSecondaryAxis] = useState<number[]>(spec.secondaryAxis ?? []);
  const validRange = parseRangeRef(range) !== null;

  const toggle = (arr: number[], set: (v: number[]) => void, k: number): void =>
    set(arr.includes(k) ? arr.filter((x) => x !== k) : [...arr, k].sort((a, b) => a - b));

  const apply = (): void => {
    if (!validRange) return;
    onApply({
      type,
      title: title.trim(),
      range: range.trim(),
      byRow,
      stacked: STACKABLE_TYPES.has(type) && stacked,
      legend,
      labels,
      grid,
      xTitle: AXIS_TYPES.has(type) ? xTitle.trim() : '',
      yTitle: AXIS_TYPES.has(type) ? yTitle.trim() : '',
      colors,
      lineSeries: type === 'combo' ? lineSeries : [], // se limpia al cambiar de tipo
      secondaryAxis: CARTESIAN_TYPES.has(type) && !stacked ? secondaryAxis : [],
    });
  };

  const showAxisSeries = CARTESIAN_TYPES.has(type) && seriesNames.length > 0;
  const field = (key: DocsKey, body: JSX.Element): JSX.Element => (
    <label class="block">
      <span class="text-2xs mb-0.5 block" style={{ color: MUTED }}>
        {t(key)}
      </span>
      {body}
    </label>
  );
  const check = (key: DocsKey, value: boolean, set: (v: boolean) => void): JSX.Element => (
    <label class="text-text flex cursor-pointer items-center gap-1.5 text-xs">
      <Checkbox class="accent-primary" checked={value} onChange={() => set(!value)} />
      {t(key)}
    </label>
  );

  return (
    <div class="flex flex-col gap-2 p-2" role="group" aria-label={t('chart.edit')}>
      {field(
        'chart.field.type',
        <Select
          size="sm"
          value={type}
          onChange={(e) => setType(e.currentTarget.value as ChartSpec['type'])}
        >
          {/* OJO: la variable del map NO se llama `t` — ensombrecería la `t` de traducción. */}
          {(Object.keys(TYPE_KEY) as ChartSpec['type'][]).map((option) => (
            <option key={option} value={option}>
              {t(TYPE_KEY[option])}
            </option>
          ))}
        </Select>,
      )}
      {field(
        'chart.field.title',
        <Input
          size="sm"
          value={title}
          placeholder={t('chart.defaultTitle', { type: t(TYPE_KEY[type]), range })}
          onInput={(e) => setTitle(e.currentTarget.value)}
        />,
      )}
      {field(
        'chart.field.range',
        <Input
          size="sm"
          value={range}
          invalid={!validRange}
          onInput={(e) => setRange(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              apply();
            }
          }}
        />,
      )}
      {check('chart.byRow', byRow, setByRow)}
      {AXIS_TYPES.has(type) && (
        <div class="grid grid-cols-2 gap-1.5">
          {field(
            'chart.xTitle',
            <Input size="sm" value={xTitle} onInput={(e) => setXTitle(e.currentTarget.value)} />,
          )}
          {field(
            'chart.yTitle',
            <Input size="sm" value={yTitle} onInput={(e) => setYTitle(e.currentTarget.value)} />,
          )}
        </div>
      )}
      {field(
        'chart.legend',
        <Select
          size="sm"
          value={legend}
          onChange={(e) => setLegend(e.currentTarget.value as ChartLegend)}
        >
          {LEGEND_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {t(o.labelKey)}
            </option>
          ))}
        </Select>,
      )}
      <div class="flex flex-col gap-1">
        {check('chart.labels', labels, setLabels)}
        {AXIS_TYPES.has(type) && check('chart.grid', grid, setGrid)}
        {STACKABLE_TYPES.has(type) && check('chart.stacked', stacked, setStacked)}
      </div>
      {seriesNames.length > 0 && (
        <div>
          <div class="text-2xs mb-0.5 flex items-center justify-between" style={{ color: MUTED }}>
            <span>{t('chart.series')}</span>
            {showAxisSeries && (
              <span class="flex gap-2">
                {type === 'combo' && <span>{t('chart.series.line')}</span>}
                {!stacked && <span>{t('chart.series.secondaryAxis')}</span>}
              </span>
            )}
          </div>
          <div class="flex flex-col gap-0.5">
            {seriesNames.map((name, k) => (
              <div key={k} class="text-text flex items-center justify-between gap-2 text-xs">
                <span class="flex min-w-0 flex-1 items-center gap-1.5">
                  <select
                    class={`h-5 w-9 shrink-0 rounded-sm border border-[var(--sytue-border)] bg-transparent text-xs ${FOCUS_RING}`}
                    style={{ color: seriesColor({ colors }, k) }}
                    aria-label={t('chart.series.color', { name })}
                    value={colors[k] ?? ''}
                    onChange={(e) => {
                      const next = colors.slice();
                      while (next.length <= k) next.push('');
                      next[k] = e.currentTarget.value;
                      setColors(next);
                    }}
                  >
                    <option value="" title={t('chart.series.auto')}>
                      ■
                    </option>
                    {SERIES_COLORS.map((c) => (
                      <option key={c} value={c} style={{ color: c }}>
                        ■
                      </option>
                    ))}
                  </select>
                  <span class="min-w-0 flex-1 truncate">{name}</span>
                </span>
                {showAxisSeries && (
                  <span class="flex items-center gap-2">
                    {type === 'combo' && (
                      <Checkbox
                        class="accent-primary"
                        aria-label={t('chart.series.line.label', { name })}
                        checked={lineSeries.includes(k)}
                        onChange={() => toggle(lineSeries, setLineSeries, k)}
                      />
                    )}
                    {!stacked && (
                      <Checkbox
                        class="accent-primary"
                        aria-label={t('chart.series.secondaryAxis.label', { name })}
                        checked={secondaryAxis.includes(k)}
                        onChange={() => toggle(secondaryAxis, setSecondaryAxis, k)}
                      />
                    )}
                  </span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
      <div class="flex gap-1.5">
        <button
          type="button"
          class={`border-border bg-surface text-text hover:bg-bg-subtle h-6 flex-1 rounded-sm border px-2 text-xs disabled:pointer-events-none disabled:opacity-40 ${FOCUS_RING}`}
          disabled={!validRange}
          onClick={apply}
        >
          {t('action.apply')}
        </button>
        <button
          type="button"
          class={`border-border bg-surface text-text hover:bg-bg-subtle h-6 flex-1 rounded-sm border px-2 text-xs ${FOCUS_RING}`}
          onClick={onCancel}
        >
          {t('action.cancel')}
        </button>
      </div>
    </div>
  );
}

export interface SheetChartProps {
  spec: ChartSpec;
  engine: Engine;
  sheetName: string;
  /** Factor del zoom CSS de la rejilla (ctx.ui.zoom/100): corrige los deltas del puntero. */
  zoom: number;
  onMove: (x: number, y: number) => void;
  onRemove: () => void;
  /** Aplica un parche al ChartSpec (tipo/título/rango/series/tamaño); el lienzo lo sanea. */
  onEdit: (patch: Partial<ChartSpec>) => void;
}

/** Paso del teclado al cambiar el tamaño con el asa enfocada. */
const RESIZE_STEP = 10;

const clampSize = (v: number): number =>
  Math.min(CHART_SIZE_MAX, Math.max(CHART_SIZE_MIN, Math.round(v)));

export function SheetChart({
  spec,
  engine,
  sheetName,
  zoom,
  onMove,
  onRemove,
  onEdit,
}: SheetChartProps): JSX.Element {
  const t = useT();
  /** Desplazamiento del arrastre en curso (transform de previsualización). */
  const [drag, setDrag] = useState<{ dx: number; dy: number } | null>(null);
  const dragRef = useRef<{ x0: number; y0: number; dx: number; dy: number } | null>(null);
  /** Tamaño provisional mientras se arrastra la esquina. */
  const [size, setSize] = useState<{ w: number; h: number } | null>(null);
  const sizeRef = useRef<{ x0: number; y0: number; w: number; h: number } | null>(null);
  const [editing, setEditing] = useState(false);

  const x = spec.x ?? 96;
  const y = spec.y ?? 48;

  const onPointerDown = (e: PointerEvent): void => {
    e.preventDefault(); // ni selección de texto ni foco robado a la rejilla
    dragRef.current = { x0: e.clientX, y0: e.clientY, dx: 0, dy: 0 };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  };
  const onPointerMove = (e: PointerEvent): void => {
    const d = dragRef.current;
    if (!d) return;
    d.dx = (e.clientX - d.x0) / zoom;
    d.dy = (e.clientY - d.y0) / zoom;
    setDrag({ dx: d.dx, dy: d.dy });
  };
  const onPointerEnd = (commit: boolean): void => {
    const d = dragRef.current;
    dragRef.current = null;
    setDrag(null);
    if (commit && d && (d.dx !== 0 || d.dy !== 0)) {
      onMove(
        Math.min(CHART_POS_MAX, Math.max(0, Math.round(x + d.dx))),
        Math.min(CHART_POS_MAX, Math.max(0, Math.round(y + d.dy))),
      );
    }
  };

  const baseW = spec.w ?? DEFAULT_PLOT_W;
  const baseH = spec.h ?? DEFAULT_PLOT_H;
  const onResizeDown = (e: PointerEvent): void => {
    e.preventDefault();
    e.stopPropagation();
    sizeRef.current = { x0: e.clientX, y0: e.clientY, w: baseW, h: baseH };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  };
  const onResizeMove = (e: PointerEvent): void => {
    const s = sizeRef.current;
    if (!s) return;
    setSize({
      w: clampSize(s.w + (e.clientX - s.x0) / zoom),
      h: clampSize(s.h + (e.clientY - s.y0) / zoom),
    });
  };
  const onResizeEnd = (commit: boolean): void => {
    sizeRef.current = null;
    const s = size;
    setSize(null);
    if (commit && s && (s.w !== baseW || s.h !== baseH)) onEdit({ w: s.w, h: s.h });
  };
  const onResizeKey = (e: KeyboardEvent): void => {
    const d: Record<string, [number, number]> = {
      ArrowRight: [RESIZE_STEP, 0],
      ArrowLeft: [-RESIZE_STEP, 0],
      ArrowDown: [0, RESIZE_STEP],
      ArrowUp: [0, -RESIZE_STEP],
    };
    const step = d[e.key];
    if (!step) return;
    e.preventDefault();
    onEdit({ w: clampSize(baseW + step[0]), h: clampSize(baseH + step[1]) });
  };

  const shown: ChartSpec = size ? { ...spec, w: size.w, h: size.h } : spec;
  const get = (c: number, r: number): CellValue =>
    engine.getValue(sheetName, `${numToCol(c)}${r + 1}`);
  const view = chartView(shown, get, t('chart.other'));
  const typeLabel = t(TYPE_KEY[spec.type]);
  const title = spec.title ?? t('chart.defaultTitle', { type: typeLabel, range: spec.range });
  const withLegend = legendVisible(shown, view.legend.length);
  const legendEl = withLegend ? (
    <div
      class={
        shown.legend === 'right'
          ? 'border-border flex w-24 shrink-0 flex-col gap-0.5 border-l px-2 py-1'
          : `border-border flex flex-wrap gap-x-3 gap-y-0.5 px-2 py-1 ${shown.legend === 'top' ? 'border-b' : 'border-t'}`
      }
      style={{ color: MUTED }}
    >
      {view.legend.map((it, i) => (
        <span key={i} class="text-2xs inline-flex min-w-0 items-center gap-1">
          <span
            aria-hidden="true"
            class="h-2 w-2 shrink-0 rounded-[2px]"
            style={{ background: it.color }}
          />
          <span class="max-w-24 truncate">{it.name}</span>
        </span>
      ))}
    </div>
  ) : null;

  return (
    <section
      class="sytue-papel border-border bg-surface absolute z-10 max-w-[calc(100vw-16px)] overflow-hidden rounded-lg border shadow-[var(--shadow-md)] select-none"
      style={{
        left: `${x}px`,
        top: `${y}px`,
        width: `${px((shown.w ?? DEFAULT_PLOT_W) + 2)}px`,
        transform: drag ? `translate(${drag.dx}px, ${drag.dy}px)` : undefined,
      }}
      aria-label={t('chart.card.label', { title })}
    >
      {/* Cabecera: asa de arrastre + editar + borrar. */}
      <div
        class="border-border flex h-7 cursor-move items-center gap-1.5 border-b px-2"
        style={{ touchAction: 'none', color: MUTED }}
        title={t('chart.drag')}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={() => onPointerEnd(true)}
        onPointerCancel={() => onPointerEnd(false)}
      >
        <EditorIcon name="chart" size="sm" />
        <span class="min-w-0 flex-1 truncate text-xs font-medium" style={{ color: INK }}>
          {title}
        </span>
        <button
          type="button"
          class={`hover:bg-bg-subtle inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ${FOCUS_RING}`}
          aria-label={t('chart.edit')}
          title={t('chart.edit')}
          aria-pressed={editing}
          onPointerDown={(e) => e.stopPropagation()}
          onClick={() => setEditing((v) => !v)}
        >
          <EditorIcon name="settings" size="sm" />
        </button>
        <button
          type="button"
          class={`hover:bg-bg-subtle inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ${FOCUS_RING}`}
          aria-label={t('chart.remove')}
          title={t('chart.remove')}
          onPointerDown={(e) => e.stopPropagation()}
          onClick={onRemove}
        >
          <EditorIcon name="close" size="sm" />
        </button>
      </div>

      {editing ? (
        <ChartEditor
          spec={spec}
          seriesNames={view.seriesNames}
          onApply={(patch) => {
            onEdit(patch);
            setEditing(false);
          }}
          onCancel={() => setEditing(false)}
        />
      ) : view.empty ? (
        <div class="grid h-28 place-items-center text-xs" style={{ color: MUTED }}>
          {t('chart.empty')}
        </div>
      ) : (
        <>
          {shown.legend === 'top' && legendEl}
          <div class={shown.legend === 'right' && withLegend ? 'flex' : undefined}>
            <div class="min-w-0 flex-1">
              <SceneSvg
                scene={view.scene}
                // Sin `.toLowerCase()`: en alemán los sustantivos van en mayúscula.
                label={t('chart.aria', { type: typeLabel, range: spec.range })}
              />
            </div>
            {shown.legend === 'right' && legendEl}
          </div>
          {(shown.legend ?? 'bottom') === 'bottom' && legendEl}
        </>
      )}
      {/* Asa de la esquina: arrastrar (o flechas con el foco) cambia el tamaño. */}
      {!editing && (
        <div
          role="slider"
          tabIndex={0}
          aria-label={t('chart.resize')}
          aria-valuenow={Math.round(shown.w ?? DEFAULT_PLOT_W)}
          aria-valuemin={CHART_SIZE_MIN}
          aria-valuemax={CHART_SIZE_MAX}
          aria-valuetext={`${Math.round(shown.w ?? DEFAULT_PLOT_W)} × ${Math.round(shown.h ?? DEFAULT_PLOT_H)}`}
          class={`absolute right-0.5 bottom-0.5 h-2.5 w-2.5 cursor-nwse-resize border-r-2 border-b-2 ${FOCUS_RING}`}
          style={{ touchAction: 'none', borderColor: AXIS_COLOR }}
          onPointerDown={onResizeDown}
          onPointerMove={onResizeMove}
          onPointerUp={() => onResizeEnd(true)}
          onPointerCancel={() => onResizeEnd(false)}
          onKeyDown={onResizeKey}
        />
      )}
    </section>
  );
}
