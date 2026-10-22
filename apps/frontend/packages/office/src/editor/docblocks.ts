/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Formas y gráficos del EDITOR DE DOCUMENTOS (kind document). Igual que el
 * SmartArt del documento (smartart.ts), el HTML del folio ES el modelo (§6.5):
 * cada bloque vive como un `<figure contenteditable=false>` con su configuración
 * en atributos `data-*` (fuente de verdad) y el SVG HORNEADO dentro (persiste y
 * viaja al export, como notas/leyendas/columnas/diagramas).
 *
 *   · Forma:   <figure data-shape data-shape-type data-w data-h data-fill
 *              data-stroke data-text> con la forma SVG + texto opcional.
 *   · Gráfico: <figure data-chart data-chart-type data-chart-grid> con la mini
 *              tabla de datos serializada + el gráfico SVG (barras/líneas/circular).
 *
 * La geometría NO se reimplementa: las formas reutilizan slides/shapes.ts
 * (polígonos + cilindro) y el mismo trazado de núcleo (rect/elipse/línea/flecha)
 * que el render de diapositivas; los gráficos reutilizan extractChartData +
 * niceTicks de sheets/Chart.tsx (interpretación del rango + escala «bonita»).
 * Módulo puro (sin Preact ni DOM): el self-test lo cubre con bun
 * (test/docs-blocks.test.ts).
 */
import type { ChartData } from './sheets/Chart';
import { extractChartData, niceTicks } from './sheets/Chart';
import {
  cylinderBodyPath,
  cylinderDims,
  cylinderSeamPath,
  polygonAttr,
  polygonPoints,
} from './slides/shapes';
import { CONTENT_PALETTE, HIGHLIGHT_PALETTE } from './ribbon/primitives';
import type { SlideChartType, SlideShape } from './types';
import { editorT, type DocsKey } from '../i18n';

/* ---------- escape para inyección segura en HTML/SVG (anti-XSS) ---------- */

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};
/** Escapa texto/valor de usuario para contenido HTML/SVG o atributo con comillas. */
const esc = (s: string): string => s.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));
/** Redondeo a centésima de px: paths compactos (mismo criterio que slides/sheets). */
const cpx = (n: number): number => Math.round(n * 100) / 100;

/* ---------- Formas ---------- */

/** Formas ofrecidas por el ribbon (id = nombre del icono; mismo juego que slides). */
export const DOC_SHAPES: readonly { id: SlideShape; labelKey: DocsKey }[] = [
  { id: 'rect', labelKey: 'slides.shape.rect' },
  { id: 'ellipse', labelKey: 'slides.shape.ellipse' },
  { id: 'line', labelKey: 'slides.shape.line' },
  { id: 'arrow', labelKey: 'slides.shape.arrow' },
  { id: 'triangle', labelKey: 'slides.shape.triangle' },
  { id: 'diamond', labelKey: 'slides.shape.diamond' },
  { id: 'pentagon', labelKey: 'slides.shape.pentagon' },
  { id: 'hexagon', labelKey: 'slides.shape.hexagon' },
  { id: 'star', labelKey: 'slides.shape.star' },
  { id: 'cylinder', labelKey: 'slides.shape.cylinder' },
  { id: 'callout', labelKey: 'slides.shape.callout' },
];
const SHAPE_IDS = new Set<SlideShape>(DOC_SHAPES.map((s) => s.id));

/** Relleno/borde por defecto: contorno de tinta sin relleno (sobrio y con el
 *  texto siempre legible; el usuario elige un relleno cuando lo necesite). */
const DEFAULT_FILL = 'transparent';
const DEFAULT_STROKE = CONTENT_PALETTE[0]; // #0A0A0A tinta
/** Tinta del texto dentro de la forma.
 *  ponytail: siempre tinta; con un relleno oscuro el usuario elige un relleno
 *  claro para que se lea (v1). Auto-contraste = upgrade. */
const SHAPE_TEXT = CONTENT_PALETTE[0];

/** Tamaño por defecto y límites (px de maqueta) de una forma redimensionable. */
export const DEFAULT_SHAPE_W = 320;
export const DEFAULT_SHAPE_H = 180;
const SHAPE_MIN = 24;
const SHAPE_MAX = 1600;

export interface DocShapeConfig {
  shape: SlideShape;
  w: number;
  h: number;
  /** Relleno #RRGGBB de CONTENT_PALETTE o 'transparent'. */
  fill: string;
  /** Borde/trazo #RRGGBB de CONTENT_PALETTE. */
  stroke: string;
  /** Texto centrado opcional dentro de la forma. */
  text: string;
}

/** Normaliza una forma cualquiera a una válida ('rect' por defecto). */
export const sanitizeShape = (raw: string | null | undefined): SlideShape =>
  SHAPE_IDS.has(raw as SlideShape) ? (raw as SlideShape) : 'rect';

/** Relleno válido: un hex de CONTENT_PALETTE o 'transparent'; si no, el defecto. */
export const sanitizeFill = (raw: string | null | undefined): string =>
  raw === 'transparent' || (CONTENT_PALETTE as readonly string[]).includes(raw ?? '')
    ? (raw as string)
    : DEFAULT_FILL;

/** Borde válido: un hex de CONTENT_PALETTE; si no, el defecto. */
export const sanitizeStroke = (raw: string | null | undefined): string =>
  (CONTENT_PALETTE as readonly string[]).includes(raw ?? '') ? (raw as string) : DEFAULT_STROKE;

/** Tamaño válido en px (entero acotado); NaN/fuera de rango → el defecto dado. */
export const sanitizeSize = (raw: string | number | null | undefined, fallback: number): number => {
  const n = Math.round(Number(raw));
  return Number.isFinite(n) ? clamp(n, SHAPE_MIN, SHAPE_MAX) : fallback;
};

/** Lee la configuración de una forma desde valores crudos (atributos data-*). */
export const sanitizeShapeConfig = (raw: {
  shape?: string | null;
  w?: string | number | null;
  h?: string | number | null;
  fill?: string | null;
  stroke?: string | null;
  text?: string | null;
}): DocShapeConfig => ({
  shape: sanitizeShape(raw.shape),
  w: sanitizeSize(raw.w, DEFAULT_SHAPE_W),
  h: sanitizeSize(raw.h, DEFAULT_SHAPE_H),
  fill: sanitizeFill(raw.fill),
  stroke: sanitizeStroke(raw.stroke),
  text: typeof raw.text === 'string' ? raw.text : '',
});

/** Capa de texto centrado (foreignObject) sobre la forma; '' = sin capa. */
const shapeTextSvg = (text: string, w: number, h: number): string => {
  if (text.trim() === '') return '';
  const fs = clamp(Math.min(h * 0.16, w * 0.1), 9, 28);
  const style =
    `box-sizing:border-box;width:100%;height:100%;display:flex;align-items:center;` +
    `justify-content:center;text-align:center;padding:6px 10px;` +
    `font-family:var(--font-sans,Inter,sans-serif);font-size:${cpx(fs)}px;font-weight:500;` +
    `line-height:1.2;color:${SHAPE_TEXT};overflow:hidden;word-break:break-word`;
  return (
    `<foreignObject x="0" y="0" width="${cpx(w)}" height="${cpx(h)}">` +
    `<div xmlns="http://www.w3.org/1999/xhtml" style="${style}">${esc(text)}</div>` +
    `</foreignObject>`
  );
};

/** Trazo SVG (cadena) de la forma inscrita en la caja w×h. Reutiliza la geometría
 *  poligonal/cilindro de slides/shapes.ts y el mismo trazado de núcleo que el
 *  render de diapositivas (rect/elipse/línea/flecha). Puro + test. */
const shapeGeometrySvg = (cfg: DocShapeConfig): string => {
  const { shape, w, h, fill, stroke } = cfg;
  const poly = polygonPoints(shape, w, h);
  if (poly.length > 0) {
    return (
      `<polygon points="${polygonAttr(poly)}" fill="${fill}" stroke="${stroke}" ` +
      `stroke-width="2" stroke-linejoin="round"/>`
    );
  }
  if (shape === 'cylinder') {
    const cyl = cylinderDims(w, h);
    return (
      `<path d="${cylinderBodyPath(cyl)}" fill="${fill}" stroke="${stroke}" stroke-width="2" ` +
      `stroke-linejoin="round"/>` +
      `<path d="${cylinderSeamPath(cyl)}" fill="none" stroke="${stroke}" stroke-width="2" ` +
      `stroke-linecap="round"/>`
    );
  }
  if (shape === 'ellipse') {
    return (
      `<ellipse cx="${cpx(w / 2)}" cy="${cpx(h / 2)}" rx="${cpx(Math.max(1, w / 2 - 1))}" ` +
      `ry="${cpx(Math.max(1, h / 2 - 1))}" fill="${fill}" stroke="${stroke}" stroke-width="2"/>`
    );
  }
  if (shape === 'line' || shape === 'arrow') {
    const y = cpx(h / 2);
    const line = `<line x1="2" y1="${y}" x2="${cpx(w - 2)}" y2="${y}" stroke="${stroke}" stroke-width="3" stroke-linecap="round"/>`;
    const head =
      shape === 'arrow'
        ? `<path d="M${cpx(w - 14)} ${cpx(h / 2 - 7)} L${cpx(w - 2)} ${y} L${cpx(w - 14)} ${cpx(h / 2 + 7)}" ` +
          `fill="none" stroke="${stroke}" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>`
        : '';
    return line + head;
  }
  return (
    `<rect x="1" y="1" width="${cpx(Math.max(1, w - 2))}" height="${cpx(Math.max(1, h - 2))}" ` +
    `rx="${cpx(clamp(Math.min(w, h) * 0.04, 0, 8))}" fill="${fill}" stroke="${stroke}" stroke-width="2"/>`
  );
};

/** SVG (cadena) completo de la forma (geometría + texto). Puro y exportado para
 *  el self-test. `line`/`arrow` no llevan capa de texto (no tiene sentido centrarla). */
export const shapeSvg = (cfg: DocShapeConfig): string => {
  const w = cpx(cfg.w);
  const h = cpx(cfg.h);
  const t = editorT();
  const clave = DOC_SHAPES.find((s) => s.id === cfg.shape)?.labelKey;
  const label = clave ? t(clave) : t('shape.generic');
  const text =
    cfg.shape === 'line' || cfg.shape === 'arrow' ? '' : shapeTextSvg(cfg.text, cfg.w, cfg.h);
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="100%" ` +
    `preserveAspectRatio="none" style="display:block;width:100%;height:auto" role="img" ` +
    `aria-label="${esc(t('shape.aria', { kind: label.toLowerCase() }))}${cfg.text.trim() ? ` — ${esc(cfg.text.trim())}` : ''}">` +
    `${shapeGeometrySvg(cfg)}${text}</svg>`
  );
};

/**
 * HTML del bloque de forma para INSERTAR en el folio: figura contenteditable=false
 * con la configuración en `data-*` (fuente de verdad) y el SVG horneado. Escapa el
 * texto del usuario (anti-XSS). `tmpId` marca la figura recién insertada para
 * re-localizarla (se retira tras engancharla). Puro y exportado para el self-test.
 */
export const shapeFigureHtml = (cfg: DocShapeConfig, tmpId?: string): string => {
  const tmp = tmpId ? ` data-blk-tmp="${esc(tmpId)}"` : '';
  const style = `display:block;margin:1em auto;width:${cfg.w}px;max-width:100%`;
  return (
    `<figure data-shape data-shape-type="${esc(cfg.shape)}" data-w="${cfg.w}" data-h="${cfg.h}" ` +
    `data-fill="${esc(cfg.fill)}" data-stroke="${esc(cfg.stroke)}" data-text="${esc(cfg.text)}"${tmp} ` +
    `contenteditable="false" style="${style}">${shapeSvg(cfg)}</figure>`
  );
};

/* ---------- Gráficos ---------- */

/** Tipos ofrecidos (MVP: los tres de sheets/Chart.tsx). */
export const DOC_CHART_TYPES: readonly { id: SlideChartType; label: string }[] = [
  { id: 'bar', label: 'Barras' },
  { id: 'line', label: 'Líneas' },
  { id: 'pie', label: 'Circular' },
];
const CHART_TYPE_IDS = new Set<SlideChartType>(['bar', 'line', 'pie']);

/** Normaliza un tipo cualquiera a uno válido ('bar' por defecto). */
export const sanitizeChartType = (raw: string | null | undefined): SlideChartType =>
  CHART_TYPE_IDS.has(raw as SlideChartType) ? (raw as SlideChartType) : 'bar';

/** Datos de arranque: 1.ª fila cabecera, 1.ª columna rótulos (como slides). */
export const DEFAULT_CHART_GRID: readonly (readonly string[])[] = [
  ['', 'Serie 1'],
  ['Ene', '10'],
  ['Feb', '15'],
  ['Mar', '7'],
  ['Abr', '12'],
];

const CHART_MAX_ROWS = 60;
const CHART_MAX_COLS = 12;

/** Rejilla nueva editable (copia mutable del arranque). */
export const newChartGrid = (): string[][] => DEFAULT_CHART_GRID.map((r) => [...r]);

/** Serializa la rejilla para `data-chart-grid` (JSON). Round-trip con parseChartGrid. */
export const serializeChartGrid = (grid: string[][]): string => JSON.stringify(grid);

/**
 * Parsea `data-chart-grid` de forma TOLERANTE: JSON inválido/no-array → rejilla
 * de arranque; cada fila se castea a string[], se RECTANGULARIZA (todas al ancho
 * máximo, rellenando con '') y se acota a CHART_MAX_ROWS×CHART_MAX_COLS (cota de
 * coste). Frontera de datos: un modelo viejo/corrupto no revienta el render.
 */
export const parseChartGrid = (raw: string | null | undefined): string[][] => {
  if (!raw) return newChartGrid();
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return newChartGrid();
  }
  if (!Array.isArray(data) || data.length === 0) return newChartGrid();
  const rows = data
    .slice(0, CHART_MAX_ROWS)
    .map((r) => (Array.isArray(r) ? r.slice(0, CHART_MAX_COLS).map((c) => String(c ?? '')) : ['']));
  const cols = rows.reduce((m, r) => Math.max(m, r.length), 1);
  return rows.map((r) => Array.from({ length: cols }, (_, c) => r[c] ?? ''));
};

/* Colores del gráfico: mismo subconjunto AA de CONTENT_PALETTE que sheets/Chart.tsx
 * (≥3:1 sobre blanco, orden fijo), replicado aquí porque sheets no lo exporta. */
const CHART_SERIES: readonly string[] = [
  CONTENT_PALETTE[8], // azul
  CONTENT_PALETTE[5], // rojo
  CONTENT_PALETTE[7], // verde
  CONTENT_PALETTE[9], // violeta
  CONTENT_PALETTE[6], // ámbar
  CONTENT_PALETTE[10], // rosa
  CONTENT_PALETTE[11], // verde azulado
];
const CHART_MUTED = CONTENT_PALETTE[1];
const CHART_AXIS = CONTENT_PALETTE[2];
const CHART_GRID = HIGHLIGHT_PALETTE[6]; // #E2E8F0 hairlines
const CHART_NUM = new Intl.NumberFormat('es-ES', { maximumFractionDigits: 2 });

/** Ancho/alto del lienzo del gráfico (viewBox); la figura se pinta a width:100%. */
const CHART_W = 480;
const CHART_H = 300;

/** Texto crudo de celda → número (es-ES: coma decimal) o texto. Puro.
 *  (Replica parseChartCell de slides: 5 líneas frente a importar SlidesCanvas.) */
const parseChartCell = (raw: string): number | string => {
  const t = raw.trim();
  if (t === '' || /[a-zA-Z]/.test(t)) return t;
  const s = (t.includes(',') ? t.replace(/\./g, '').replace(',', '.') : t).replace(/%/g, '');
  const n = Number(s);
  return Number.isFinite(n) ? n : t;
};

/** Interpreta la rejilla igual que un rango de sheets (reusa extractChartData). Puro. */
export const docChartData = (grid: string[][]): ChartData => {
  const rows = grid.length;
  const cols = grid.reduce((m, r) => Math.max(m, r.length), 0);
  if (rows === 0 || cols === 0) return { labels: [], series: [] };
  return extractChartData({ c1: 0, r1: 0, c2: cols - 1, r2: rows - 1 }, (c, r) =>
    parseChartCell(grid[r]?.[c] ?? ''),
  );
};

/** Recorta un rótulo a `max` caracteres con elipsis. */
const cut = (t: string, max: number): string => (t.length > max ? `${t.slice(0, max - 1)}…` : t);

/** Cuerpo SVG (cadena) de barras/líneas: comparten eje Y «bonito» (niceTicks). */
const cartesianBody = (type: 'bar' | 'line', data: ChartData): string => {
  const series = data.series.slice(0, CHART_SERIES.length);
  const pad = { l: 44, r: 10, t: 12, b: 26 };
  const plotL = pad.l;
  const plotT = pad.t;
  const plotW = Math.max(1, CHART_W - pad.l - pad.r);
  const plotH = Math.max(1, CHART_H - pad.t - pad.b);
  const font = 12;
  const all = series.flatMap((s) => s.values);
  const sc =
    type === 'bar'
      ? niceTicks(Math.min(0, ...all), Math.max(0, ...all))
      : niceTicks(all.length ? Math.min(...all) : 0, all.length ? Math.max(...all) : 1);
  const yOf = (v: number): number => plotT + ((sc.max - v) / (sc.max - sc.min || 1)) * plotH;
  const n = Math.max(1, data.labels.length);
  const slot = plotW / n;
  const xStep = Math.ceil(n / Math.max(1, Math.floor(plotW / (font * 4))));
  const out: string[] = [];

  // Retícula + rótulos del eje Y.
  for (const t of sc.ticks) {
    out.push(
      `<line x1="${plotL}" x2="${CHART_W - pad.r}" y1="${cpx(yOf(t))}" y2="${cpx(yOf(t))}" ` +
        `stroke="${CHART_GRID}" stroke-width="1" shape-rendering="crispEdges"/>`,
      `<text x="${plotL - 4}" y="${cpx(yOf(t))}" dy="0.32em" text-anchor="end" ` +
        `font-size="${font}" fill="${CHART_MUTED}">${esc(CHART_NUM.format(t))}</text>`,
    );
  }

  if (type === 'bar') {
    const group = slot * 0.72;
    const bw = group / Math.max(1, series.length);
    const y0 = yOf(0);
    series.forEach((s, k) => {
      s.values.forEach((v, i) => {
        const yv = yOf(v);
        out.push(
          `<rect x="${cpx(plotL + i * slot + (slot - group) / 2 + k * bw + 1)}" ` +
            `y="${cpx(Math.min(yv, y0))}" width="${cpx(Math.max(1, bw - 2))}" ` +
            `height="${cpx(Math.abs(yv - y0))}" fill="${CHART_SERIES[k] ?? CHART_AXIS}" ` +
            `shape-rendering="crispEdges"><title>${esc(`${data.labels[i] ?? ''} — ${s.name}: ${CHART_NUM.format(v)}`)}</title></rect>`,
        );
      });
    });
  } else {
    series.forEach((s, k) => {
      const color = CHART_SERIES[k] ?? CHART_AXIS;
      const d = s.values
        .map((v, i) => `${i === 0 ? 'M' : 'L'}${cpx(plotL + slot * (i + 0.5))} ${cpx(yOf(v))}`)
        .join('');
      out.push(
        `<path d="${d}" fill="none" stroke="${color}" stroke-width="2" ` +
          `stroke-linecap="round" stroke-linejoin="round"/>`,
      );
    });
  }

  // Línea base del eje X + rótulos de categoría.
  const baseY = cpx(type === 'bar' ? yOf(0) : plotT + plotH);
  out.push(
    `<line x1="${plotL}" x2="${CHART_W - pad.r}" y1="${baseY}" y2="${baseY}" ` +
      `stroke="${CHART_AXIS}" stroke-width="1" shape-rendering="crispEdges"/>`,
  );
  data.labels.forEach((t, i) => {
    if (i % xStep === 0 && t !== '') {
      out.push(
        `<text x="${cpx(plotL + slot * (i + 0.5))}" y="${cpx(CHART_H - pad.b / 2 + font * 0.3)}" ` +
          `text-anchor="middle" font-size="${font}" fill="${CHART_MUTED}">${esc(cut(t, 9))}</text>`,
      );
    }
  });
  return out.join('');
};

/** Cuerpo SVG (cadena) del circular: sectores de la 1.ª serie (valores > 0). */
const pieBody = (data: ChartData): string => {
  const vals = data.series[0]?.values ?? [];
  const parts = data.labels
    .map((name, i) => ({
      name,
      value: Math.max(0, vals[i] ?? 0),
      color: CHART_SERIES[i % CHART_SERIES.length]!,
    }))
    .filter((p) => p.value > 0);
  const total = parts.reduce((sum, p) => sum + p.value, 0) || 1;
  const cx = CHART_W / 2;
  const cy = CHART_H / 2;
  const r = Math.max(1, Math.min(CHART_W, CHART_H) / 2 - 12);
  let a = -Math.PI / 2;
  const out: string[] = [];
  for (const p of parts) {
    const frac = p.value / total;
    const a1 = a + frac * 2 * Math.PI;
    const tip = esc(`${p.name}: ${CHART_NUM.format(p.value)} (${CHART_NUM.format(frac * 100)} %)`);
    if (frac > 0.9995) {
      out.push(
        `<circle cx="${cpx(cx)}" cy="${cpx(cy)}" r="${cpx(r)}" fill="${p.color}" ` +
          `stroke="#FFFFFF" stroke-width="1.5"><title>${tip}</title></circle>`,
      );
    } else {
      const x0 = cpx(cx + r * Math.cos(a));
      const y0 = cpx(cy + r * Math.sin(a));
      const x1 = cpx(cx + r * Math.cos(a1));
      const y1 = cpx(cy + r * Math.sin(a1));
      out.push(
        `<path d="M${cpx(cx)} ${cpx(cy)}L${x0} ${y0}A${cpx(r)} ${cpx(r)} 0 ${frac > 0.5 ? 1 : 0} 1 ${x1} ${y1}Z" ` +
          `fill="${p.color}" stroke="#FFFFFF" stroke-width="1.5" stroke-linejoin="round"><title>${tip}</title></path>`,
      );
    }
    a = a1;
  }
  return out.join('');
};

/**
 * SVG (cadena) del gráfico para el tipo y la rejilla dados. Reutiliza
 * extractChartData/niceTicks (no los duplica). Sin datos numéricos devuelve un
 * SVG con un aviso sobrio. Puro y exportado para el self-test.
 */
export const chartSvg = (type: SlideChartType, grid: string[][]): string => {
  const data = docChartData(grid);
  const pieVals = type === 'pie' ? (data.series[0]?.values ?? []) : [];
  const empty =
    data.labels.length === 0 ||
    data.series.length === 0 ||
    (type === 'pie' && !pieVals.some((v) => v > 0));
  const label = `Gráfico de ${type === 'bar' ? 'barras' : type === 'line' ? 'líneas' : 'sectores'}`;
  const header =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${CHART_W} ${CHART_H}" width="100%" ` +
    `style="display:block;width:100%;height:auto" role="img" aria-label="${esc(label)}">`;
  if (empty) {
    return (
      `${header}<text x="${CHART_W / 2}" y="${CHART_H / 2}" text-anchor="middle" ` +
      `dominant-baseline="middle" fill="${CHART_MUTED}" ` +
      `font-family="var(--font-sans,Inter,sans-serif)" font-size="14">` +
      `Sin datos numéricos: edita los datos del gráfico</text></svg>`
    );
  }
  return `${header}${type === 'pie' ? pieBody(data) : cartesianBody(type, data)}</svg>`;
};

/**
 * HTML del bloque de gráfico para INSERTAR en el folio: figura contenteditable=false
 * con el tipo + la rejilla serializada en `data-*` (fuente de verdad) y el SVG
 * horneado. Escapa los datos del usuario (anti-XSS). `tmpId` marca la figura recién
 * insertada para re-localizarla. Puro y exportado para el self-test.
 */
export const chartFigureHtml = (type: SlideChartType, grid: string[][], tmpId?: string): string => {
  const tmp = tmpId ? ` data-blk-tmp="${esc(tmpId)}"` : '';
  const style = 'display:block;margin:1em auto;width:480px;max-width:100%';
  return (
    `<figure data-chart data-chart-type="${esc(type)}" ` +
    `data-chart-grid="${esc(serializeChartGrid(grid))}"${tmp} contenteditable="false" ` +
    `style="${style}">${chartSvg(type, grid)}</figure>`
  );
};
