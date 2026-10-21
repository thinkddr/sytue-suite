/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Buscar» con una hoja. El panel del carril (SearchPanel) busca en el DOM del
 * lienzo, y con una hoja ese DOM era solo la CELDA enfocada (SheetCanvas pone de
 * raíz la celda en `focusin`): se buscaba en una celda y el reemplazo editaba su
 * DOM sin pasar por el modelo, sin deshacer ni validación.
 *
 * Ahora la hoja le da al panel un `SearchTarget`: busca en las celdas de la hoja
 * ACTIVA, fila a fila y de izquierda a derecha, en el texto crudo (el literal o la
 * fórmula, lo que enseña la barra); llevar una coincidencia a la vista es
 * seleccionar su celda; y reemplazar escribe por el camino de la barra de
 * fórmulas (protección, validación estricta, un paso de deshacer). Es lo mismo
 * que hace Calcus en nativo (`apps/fyne/calcus/buscar.go`).
 *
 * Lo que no se encuentra, en los dos: lo que la protección no deja seleccionar, lo
 * que el autofiltro o el esquema esconden, y las fórmulas OCULTAS de una hoja
 * protegida (buscar en su texto la delataría).
 */
import { matchIndices } from '../canvas/exec';
import type { MatchOptions } from '../canvas/exec';
import { escapeHtml, parseCell, parseRangeRef, renderCellHtml } from './format';

/** Amarillo #FEF08A (todas las coincidencias) y melocotón #FED7AA (la actual) de
 *  HIGHLIGHT_PALETTE: los mismos del folio, que pinta el panel. */
export const HIT_BG = '#fef08a';
export const HIT_ACTIVE_BG = '#fed7aa';
/** Tinta sobre esos dos fondos: el negro de CONTENT_PALETTE. */
export const HIT_INK = '#0a0a0a';

/** Las celdas con alguna coincidencia, como `c,r`, para pintarlas. */
export function hitCells(matches: readonly SheetMatch[]): Set<string> {
  return new Set(matches.map((m) => `${m.c},${m.r}`));
}

/** Un trozo del texto crudo de una celda que casa con la búsqueda (UTF-16). */
export interface SheetMatch {
  c: number;
  r: number;
  start: number;
  end: number;
}

/** Lo que el panel «Buscar» pide a un lienzo que no es el folio. */
export interface SearchTarget {
  /** Busca de nuevo y devuelve cuántas coincidencias hay. */
  find: (query: string, opts: MatchOptions) => number;
  /** Cuántas hay ahora (reemplazar cambia la cuenta). */
  count: () => number;
  /** Lleva a la vista la i-ésima: selecciona su celda. */
  goTo: (index: number) => void;
  /** Reemplaza la i-ésima y devuelve cuál es la siguiente tras volver a buscar. */
  replace: (index: number, by: string) => number;
  /** Reemplaza todas en UN paso de deshacer. */
  replaceAll: (by: string) => void;
  /** ¿Se puede reemplazar? (solo lectura = no). */
  editable: () => boolean;
}

let target: SearchTarget | null = null;

/** Lo pone la hoja al montarse y lo quita al desmontarse. */
export function setSearchTarget(t: SearchTarget | null): void {
  target = t;
}

/** El lienzo que busca por su cuenta, o null (el folio: se busca en el DOM). */
export function getSearchTarget(): SearchTarget | null {
  return target;
}

/**
 * Las coincidencias de `query` en las celdas con algo, fila a fila. `textOf` da el
 * texto crudo de una celda y `skip` dice cuáles no se pueden llevar a la vista.
 */
export function sheetMatches(
  cells: Readonly<Record<string, string>>,
  query: string,
  opts: MatchOptions,
  textOf: (ref: string) => string,
  skip: (c: number, r: number, ref: string) => boolean,
  limit = 500,
): SheetMatch[] {
  const out: SheetMatch[] = [];
  if (query === '') return out;
  const found: { c: number; r: number; ref: string }[] = [];
  for (const ref of Object.keys(cells)) {
    const p = parseRangeRef(ref);
    if (!p || p.c1 !== p.c2 || p.r1 !== p.r2 || skip(p.c1, p.r1, ref)) continue;
    found.push({ c: p.c1, r: p.r1, ref });
  }
  found.sort((a, b) => a.r - b.r || a.c - b.c);
  for (const f of found) {
    for (const [start, end] of matchIndices(textOf(f.ref), query, opts)) {
      if (out.length >= limit) return out;
      out.push({ c: f.c, r: f.r, start, end });
    }
  }
  return out;
}

/** El HTML de la celda con OTRO texto crudo y el mismo envoltorio (negrita,
 *  cursiva, alineación): lo que escribe la barra de fórmulas al confirmar. */
export function withText(html: string, text: string): string {
  const p = parseCell(html);
  p.inner = escapeHtml(text);
  return renderCellHtml(p);
}

/** `text` con [start, end) cambiado por `by`. */
export function splice(text: string, start: number, end: number, by: string): string {
  return text.slice(0, start) + by + text.slice(end);
}

/** Los textos nuevos de cada celda tras reemplazar todas sus coincidencias —de
 *  atrás adelante, para que las posiciones de delante sigan valiendo—, en orden
 *  de lectura. */
export function replaceAllPlan(
  matches: readonly SheetMatch[],
  textOf: (m: SheetMatch) => string,
  by: string,
): { c: number; r: number; text: string }[] {
  const byCell = new Map<string, { c: number; r: number; text: string }>();
  for (let i = matches.length - 1; i >= 0; i--) {
    const m = matches[i]!;
    const key = `${m.c},${m.r}`;
    const cur = byCell.get(key) ?? { c: m.c, r: m.r, text: textOf(m) };
    cur.text = splice(cur.text, m.start, m.end, by);
    byCell.set(key, cur);
  }
  return [...byCell.values()].reverse();
}

/** La primera coincidencia que empieza en (c, r, pos) o detrás; 0 si no hay. */
export function nextFrom(
  matches: readonly SheetMatch[],
  c: number,
  r: number,
  pos: number,
): number {
  const i = matches.findIndex(
    (m) => m.r > r || (m.r === r && (m.c > c || (m.c === c && m.start >= pos))),
  );
  return i === -1 ? 0 : i;
}

/** ¿Es la misma coincidencia? (para no escribir con una búsqueda ya vieja). */
export function sameMatch(a: SheetMatch | undefined, b: SheetMatch | undefined): boolean {
  return !!a && !!b && a.c === b.c && a.r === b.r && a.start === b.start && a.end === b.end;
}
