/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Helpers PUROS de las filas y columnas OCULTAS A MANO (`SheetData.hiddenRows` y
 * `hiddenCols`): sanear lo que llega del documento, ocultar y volver a mostrar la
 * selección, seguir a la estructura al insertar o borrar, y las unidades que se pintan.
 * No es el esquema (outline.ts): un grupo contraído se abre con su botón; esto es
 * «Ocultar filas» y «Mostrar filas» sin grupo de por medio, como en cualquier hoja.
 *
 * Se guarda en TRAMOS {start, end} y no fila a fila: ocultar desde una fila hasta el final
 * son un millón de filas, y en tramos es uno. Puerto en Go con los MISMOS vectores:
 * apps/nucleo/formula/analisis/ocultas.go y testdata/ocultas.json.
 */
import type { SheetSpan } from '../types';

/** Ordena y junta los tramos que se solapan o se tocan: una ocultación, una sola forma. */
function normalizar(spans: SheetSpan[]): SheetSpan[] {
  spans.sort((a, b) => a.start - b.start || a.end - b.end);
  const out: SheetSpan[] = [];
  for (const t of spans) {
    const last = out[out.length - 1];
    if (last && t.start <= last.end + 1) {
      last.end = Math.max(last.end, t.end);
      continue;
    }
    out.push({ start: t.start, end: t.end });
  }
  return out;
}

/** Tramos saneados contra `max` unidades (recortados a la rejilla), ordenados y juntados.
 *  Vacío ⇒ el campo no se persiste. */
export function sanitizeSpans(raw: unknown, max: number): SheetSpan[] {
  if (!Array.isArray(raw)) return [];
  const spans: SheetSpan[] = [];
  for (const x of raw) {
    if (!x || typeof x !== 'object') continue;
    const t = x as Partial<Record<'start' | 'end', unknown>>;
    if (!('start' in t) || !('end' in t)) continue;
    const s = Math.trunc(Number(t.start));
    const e = Math.trunc(Number(t.end));
    if (!Number.isInteger(s) || !Number.isInteger(e)) continue;
    if (s > e || e < 0 || s >= max) continue;
    spans.push({ start: Math.max(0, s), end: Math.min(max - 1, e) });
  }
  return normalizar(spans);
}

/** ¿La unidad `u` está oculta? */
export function isSpanHidden(spans: readonly SheetSpan[] | undefined, u: number): boolean {
  return (spans ?? []).some((t) => u >= t.start && u <= t.end);
}

/**
 * Oculta [a,b] (en cualquier orden, recortado a la rejilla). null si así no quedaría
 * ninguna unidad a la vista: una hoja sin una sola fila que seleccionar no deja ni volver
 * a mostrarlas, y Excel tampoco lo permite. Devuelve una lista NUEVA.
 */
export function hideSpan(
  spans: readonly SheetSpan[],
  a: number,
  b: number,
  max: number,
): SheetSpan[] | null {
  const s = Math.max(0, Math.min(a, b));
  const e = Math.min(max - 1, Math.max(a, b));
  if (s > e) return spans.map((t) => ({ ...t }));
  const out = normalizar([...spans.map((t) => ({ ...t })), { start: s, end: e }]);
  // El primer tramo y no «el único»: lo que haya más allá de `max` (los tramos de la web en
  // la rejilla corta de la app nativa) no deja nada a la vista.
  if (out[0]!.start === 0 && out[0]!.end >= max - 1) return null;
  return out;
}

/**
 * Vuelve a mostrar lo oculto dentro de [a,b]. Si dentro no hay nada oculto, muestra el
 * bloque oculto que TOCA la selección por un lado o por el otro: es como se recupera la
 * fila 1 (o la columna A) oculta, que no hay forma de abarcar seleccionando a ambos lados.
 */
export function showSpan(spans: readonly SheetSpan[], a: number, b: number): SheetSpan[] {
  let s = Math.min(a, b);
  let e = Math.max(a, b);
  if (!spans.some((t) => t.start <= e && t.end >= s)) {
    for (const t of spans) {
      if (t.end === s - 1) s = t.start;
      if (t.start === e + 1) e = t.end;
    }
  }
  const out: SheetSpan[] = [];
  for (const t of spans) {
    if (t.end < s || t.start > e) {
      out.push({ ...t });
      continue;
    }
    if (t.start < s) out.push({ start: t.start, end: s - 1 });
    if (t.end > e) out.push({ start: e + 1, end: t.end });
  }
  return out;
}

/**
 * Lo que les pasa a los tramos al insertar (`cantidad` > 0) o borrar (`cantidad` < 0)
 * unidades desde `desde`: lo de detrás se desplaza, lo borrado desaparece y lo que se sale
 * de la rejilla (`tope`) se pierde, como las celdas. Una inserción DENTRO de un tramo lo
 * parte y deja lo insertado a la vista.
 */
export function moveSpans(
  spans: readonly SheetSpan[],
  desde: number,
  cantidad: number,
  tope: number,
): SheetSpan[] {
  const out: SheetSpan[] = [];
  for (const t of spans) {
    if (t.start < desde) out.push({ start: t.start, end: Math.min(t.end, desde - 1) });
    if (cantidad > 0) {
      if (t.end >= desde) {
        const s = Math.max(t.start, desde) + cantidad;
        const e = Math.min(t.end + cantidad, tope - 1);
        if (s <= e) out.push({ start: s, end: e });
      }
      continue;
    }
    const finBorrado = desde - cantidad - 1;
    if (t.end > finBorrado) {
      out.push({ start: Math.max(t.start, finBorrado + 1) + cantidad, end: t.end + cantidad });
    }
  }
  return normalizar(out);
}

/** Unidades ocultas por debajo de `limit` (lo que se pinta o lo que tiene datos): un
 *  tramo hasta el final de la rejilla no se recorre entero. */
export function spanUnits(spans: readonly SheetSpan[] | undefined, limit: number): Set<number> {
  const out = new Set<number>();
  for (const t of spans ?? []) {
    for (let u = t.start; u <= t.end && u < limit; u++) out.add(u);
  }
  return out;
}

/** Unidades ocultas por debajo de `limit`, sin recorrerlas. */
function cuantasDebajo(spans: readonly SheetSpan[], limit: number): number {
  let n = 0;
  for (const t of spans) if (t.start < limit) n += Math.min(t.end, limit - 1) - t.start + 1;
  return n;
}

/**
 * La extensión `n` de la rejilla (cuántas filas o columnas se recorren) estirada por lo que
 * hay oculto dentro: la extensión se mide en lo que SE VE (la ventana, la holgura), y sin
 * esto un bloque oculto se comería esas filas y la hoja acabaría antes de llegar a lo que
 * hay detrás. Pocas pasadas y con tope: un tramo hasta el final de la hoja no la estira
 * hasta el millón de filas.
 */
export function extentPastHidden(
  n: number,
  spans: readonly SheetSpan[] | undefined,
  max: number,
): number {
  if (!spans || spans.length === 0) return n;
  let ext = n;
  for (let i = 0; i < 4; i++) {
    const next = Math.min(max, n + cuantasDebajo(spans, ext));
    if (next === ext) break;
    ext = next;
  }
  return ext;
}
