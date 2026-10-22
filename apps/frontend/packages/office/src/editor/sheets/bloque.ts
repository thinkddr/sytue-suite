/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Insertar un BLOQUE REUTILIZABLE en una hoja (comando `bloquehoja`): la cabecera de un
 * informe, un bloque de totales, una tabla con su gráfico… El servicio lo entrega como
 * `{cells, formats, styles, charts}` con sus referencias contadas desde su propia
 * esquina; aquí se desplaza a la celda activa: las referencias, las fórmulas (con el
 * mismo `shiftCellHtml` que el pegado) y el rango de cada gráfico.
 *
 * Lo que llega es de fuera (el bloque de la organización es la hoja de otra persona):
 * cada formato, estilo y gráfico pasa por su saneado, como al pegar. Módulo puro, sin
 * DOM (test/sheets-bloque.test.ts).
 *
 * ponytail: pisa lo que hubiera en las celdas que ocupa, como un pegado; avisar antes
 * si el destino no está vacío cuando alguien lo pida.
 */
import { numToCol } from '@sytue/formula';
import type { CellStyle, ChartSpec, NumFmt, SheetData } from '../types';
import {
  parseRangeRef,
  sanitizeCellStyle,
  sanitizeChartSpec,
  sanitizeNumFmt,
  shiftCellHtml,
} from './format';
import { MAX_COLS, MAX_ROWS } from './rejilla';

/** Tope de celdas de un bloque: es una pieza, no una hoja entera. */
export const MAX_CELDAS_BLOQUE = 5000;

const objeto = (v: unknown): Record<string, unknown> =>
  v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {};

const ref = (c: number, r: number): string => `${numToCol(c)}${r + 1}`;

/**
 * El parche de hoja que deja el bloque `raw` (JSON del servicio) con su esquina en
 * `ancla` (0-based). null si el bloque no se entiende, está vacío, es demasiado grande o
 * no cabe en la rejilla desde esa celda.
 */
export function colocarBloque(
  hoja: SheetData,
  raw: string,
  ancla: { c: number; r: number },
): Partial<SheetData> | null {
  let bloque: Record<string, unknown>;
  try {
    bloque = objeto(JSON.parse(raw));
  } catch {
    return null;
  }
  const celdas = objeto(bloque['cells']);
  const formatos = objeto(bloque['formats']);
  const estilos = objeto(bloque['styles']);
  const refs = [...Object.keys(celdas), ...Object.keys(formatos), ...Object.keys(estilos)];
  if (Object.keys(celdas).length === 0 || refs.length > MAX_CELDAS_BLOQUE) return null;

  // La esquina del bloque y su extensión, para desplazarlo entero.
  let c0 = Infinity;
  let r0 = Infinity;
  let c1 = -1;
  let r1 = -1;
  const pos = new Map<string, { c: number; r: number }>();
  for (const k of refs) {
    const p = parseRangeRef(k);
    if (!p || p.c1 !== p.c2 || p.r1 !== p.r2) return null;
    pos.set(k, { c: p.c1, r: p.r1 });
    c0 = Math.min(c0, p.c1);
    r0 = Math.min(r0, p.r1);
    c1 = Math.max(c1, p.c1);
    r1 = Math.max(r1, p.r1);
  }
  const dc = ancla.c - c0;
  const dr = ancla.r - r0;
  if (c1 + dc >= MAX_COLS || r1 + dr >= MAX_ROWS) return null;
  const a = (k: string): string => {
    const p = pos.get(k)!;
    return ref(p.c + dc, p.r + dr);
  };

  const cells = { ...hoja.cells };
  for (const [k, v] of Object.entries(celdas)) {
    if (typeof v === 'string' && v !== '') cells[a(k)] = shiftCellHtml(v, dc, dr);
  }
  const formats: Record<string, NumFmt> = { ...hoja.formats };
  for (const [k, v] of Object.entries(formatos)) {
    const f = sanitizeNumFmt(v);
    if (f) formats[a(k)] = f;
  }
  const styles: Record<string, CellStyle> = { ...hoja.styles };
  for (const [k, v] of Object.entries(estilos)) {
    const s = sanitizeCellStyle(v);
    if (!s) continue;
    // `tema` no es del editor sino del servicio: dice que esta celda la vistió la marca,
    // para que «Actualizar a la marca vigente» la encuentre. Se conserva tal cual.
    const tema = objeto(v)['tema'];
    styles[a(k)] = typeof tema === 'string' ? ({ ...s, tema } as CellStyle) : s;
  }
  const charts: ChartSpec[] = [...(hoja.charts ?? [])];
  for (const g of Array.isArray(bloque['charts']) ? bloque['charts'] : []) {
    const o = objeto(g);
    const rango = typeof o['range'] === 'string' ? parseRangeRef(o['range']) : null;
    if (!rango) continue;
    const n = charts.length;
    const spec = sanitizeChartSpec({
      ...o,
      id: crypto.randomUUID(),
      range: `${ref(rango.c1 + dc, rango.r1 + dr)}:${ref(rango.c2 + dc, rango.r2 + dr)}`,
      // En cascada, como «Insertar gráfico»: las tarjetas nuevas no se apilan exactas.
      x: 96 + (n % 8) * 28,
      y: 48 + (n % 8) * 28,
    });
    if (spec) charts.push(spec);
  }
  return {
    cells,
    formats,
    styles,
    ...(charts.length > 0 ? { charts } : {}),
  };
}
