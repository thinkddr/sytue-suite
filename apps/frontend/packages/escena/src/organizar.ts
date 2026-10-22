/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Organizar objetos (movido desde `packages/office/src/editor/slides/organize.ts`, que
 * ahora lo importa de aquí): helpers PUROS y genéricos para z-order, alineación
 * y distribución de cualquier lista de `{ id, x, y, w, h }`. El z-order ES el
 * orden en el array (más al final = más arriba). Las unidades las pone el
 * llamante (% del lienzo en Diapos, px en la escena). Sin estado ni efectos.
 */

/** Caja (x/y esquina superior-izquierda, w/h) en las unidades del llamante. */
export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}
/** Elemento ordenable/alineable: un id y su caja. */
type Item = Rect & { id: string };

/** Redondeo a milésima (JSON compacto + idempotencia: el ruido de coma
 *  flotante no se acumula al alinear/distribuir varias veces seguidas). */
const r3 = (n: number): number => Math.round(n * 1000) / 1000;

/** Tope de una coordenada a [lo, lo+span−size]: la caja no se sale por ningún borde. */
const clampPos = (v: number, size: number, lo: number, span: number): number =>
  Math.min(lo + span - size, Math.max(lo, v));

/** Rectángulo envolvente (mínimo que contiene todos los rects). Vacío = origen. Pura. */
export function boundingBox(rects: readonly Rect[]): Rect {
  if (rects.length === 0) return { x: 0, y: 0, w: 0, h: 0 };
  let x1 = Infinity;
  let y1 = Infinity;
  let x2 = -Infinity;
  let y2 = -Infinity;
  for (const r of rects) {
    if (r.x < x1) x1 = r.x;
    if (r.y < y1) y1 = r.y;
    if (r.x + r.w > x2) x2 = r.x + r.w;
    if (r.y + r.h > y2) y2 = r.y + r.h;
  }
  return { x: x1, y: y1, w: x2 - x1, h: y2 - y1 };
}

/* ---------- Z-order (orden en el array) ---------- */

/** Operación de reordenación en Z. */
export type ZOrderOp = 'front' | 'forward' | 'backward' | 'back';

/** Reordena en Z el conjunto `ids` dentro de `boxes` (orden del array = Z; más al
 *  final = más arriba). 'front'/'back' llevan el bloque al extremo; 'forward'/
 *  'backward' lo desplazan un paso saltando al vecino inmediato. El bloque conserva
 *  su orden interno; una selección dispersa se compacta. Sin cambios si no hay nada
 *  que mover o ya está en el extremo. Pura. */
export function reorderZ<T extends { id: string }>(
  boxes: T[],
  ids: readonly string[],
  op: ZOrderOp,
): T[] {
  const sel = new Set(ids);
  const selected = boxes.filter((b) => sel.has(b.id));
  const others = boxes.filter((b) => !sel.has(b.id));
  if (selected.length === 0 || others.length === 0) return boxes; // nada que reordenar
  if (op === 'front') return [...others, ...selected];
  if (op === 'back') return [...selected, ...others];
  // forward/backward: índices que ocupa el bloque en el array original.
  let lo = boxes.length;
  let hi = -1;
  boxes.forEach((b, i) => {
    if (sel.has(b.id)) {
      if (i < lo) lo = i;
      if (i > hi) hi = i;
    }
  });
  if (op === 'forward') {
    if (hi >= boxes.length - 1) return boxes; // ya al frente
    const neighborId = boxes[hi + 1]!.id; // vecino inmediato hacia el frente
    const out: T[] = [];
    for (const b of others) {
      out.push(b);
      if (b.id === neighborId) out.push(...selected); // el bloque cae tras el vecino
    }
    return out;
  }
  // backward
  if (lo <= 0) return boxes; // ya al fondo
  const neighborId = boxes[lo - 1]!.id; // vecino inmediato hacia el fondo
  const out: T[] = [];
  for (const b of others) {
    if (b.id === neighborId) out.push(...selected); // el bloque cae antes del vecino
    out.push(b);
  }
  return out;
}

/* ---------- Alineación ---------- */

/** Borde/eje de alineación (h* = horizontal, v* = vertical). */
export type AlignEdge = 'left' | 'hcenter' | 'right' | 'top' | 'vmiddle' | 'bottom';

/** Nueva coordenada (solo el eje que toca `edge`) de una caja alineada a `bnd`,
 *  con tope a `lim` (el lienzo). */
function alignedCoord(b: Rect, bnd: Rect, edge: AlignEdge, lim: Rect): Partial<Rect> {
  const cx = (v: number): number => r3(clampPos(v, b.w, lim.x, lim.w));
  const cy = (v: number): number => r3(clampPos(v, b.h, lim.y, lim.h));
  switch (edge) {
    case 'left':
      return { x: cx(bnd.x) };
    case 'hcenter':
      return { x: cx(bnd.x + (bnd.w - b.w) / 2) };
    case 'right':
      return { x: cx(bnd.x + bnd.w - b.w) };
    case 'top':
      return { y: cy(bnd.y) };
    case 'vmiddle':
      return { y: cy(bnd.y + (bnd.h - b.h) / 2) };
    case 'bottom':
      return { y: cy(bnd.y + bnd.h - b.h) };
  }
}

/** Alinea las cajas `ids` respecto a `ref` (la envolvente de la selección o un
 *  rectángulo dado, p. ej. el lienzo), con tope a `lim`. Devuelve un array NUEVO
 *  (las no seleccionadas intactas). Pura. */
export function alignBoxes<T extends Item>(
  boxes: T[],
  ids: readonly string[],
  edge: AlignEdge,
  ref: 'selection' | Rect,
  lim: Rect,
): T[] {
  const sel = new Set(ids);
  const targets = boxes.filter((b) => sel.has(b.id));
  if (targets.length === 0) return boxes;
  const bnd = ref === 'selection' ? boundingBox(targets) : ref;
  return boxes.map((b) => (sel.has(b.id) ? { ...b, ...alignedCoord(b, bnd, edge, lim) } : b));
}

/* ---------- Distribución (espaciado uniforme) ---------- */

/** Eje de distribución. */
export type DistAxis = 'h' | 'v';

/** Distribuye las cajas `ids` con huecos IGUALES entre bordes consecutivos a lo
 *  largo del eje: los extremos quedan fijos y las de en medio se recolocan. Hacen
 *  falta ≥3 cajas (con menos no hay nada entre medias). Devuelve un array nuevo. Pura. */
export function distributeBoxes<T extends Item>(
  boxes: T[],
  ids: readonly string[],
  axis: DistAxis,
): T[] {
  const sel = new Set(ids);
  const targets = boxes.filter((b) => sel.has(b.id));
  if (targets.length < 3) return boxes;
  const start = (b: Rect): number => (axis === 'h' ? b.x : b.y);
  const size = (b: Rect): number => (axis === 'h' ? b.w : b.h);
  // Orden por borde inicial (desempate por centro): fija extremos y recorrido.
  const sorted = [...targets].sort(
    (a, b) => start(a) - start(b) || start(a) + size(a) / 2 - (start(b) + size(b) / 2),
  );
  const first = sorted[0]!;
  const last = sorted[sorted.length - 1]!;
  const span = start(last) + size(last) - start(first); // del borde inicial del 1.º al final del último
  const sumSize = sorted.reduce((s, r) => s + size(r), 0);
  const gap = (span - sumSize) / (sorted.length - 1); // hueco uniforme (negativo si solapan)
  const at = new Map<string, number>();
  let cursor = start(first);
  for (const r of sorted) {
    at.set(r.id, r3(cursor));
    cursor += size(r) + gap;
  }
  return boxes.map((b) => {
    const p = at.get(b.id);
    if (p === undefined) return b;
    return axis === 'h' ? { ...b, x: p } : { ...b, y: p };
  });
}
