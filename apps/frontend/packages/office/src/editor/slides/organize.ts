/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Organizar objetos de una diapositiva (fase Slides-Organizar): z-order,
 * alineación/distribución y grupos de cajas. El z-order ES el orden en el array
 * `boxes` (más al final del array = más arriba). Todo en % del lienzo 16:9
 * (0..100), como SlideBox. Lo GENÉRICO (z-order, envolvente, alinear, distribuir)
 * vive ahora en `@sytue/escena`; aquí queda lo propio del modelo de Diapos (el
 * lienzo de 100 × 100 y los grupos por `groupId`). Pruebas: test/slides-organize.test.ts.
 */
import { alignBoxes as alignGeneric, boundingBox, distributeBoxes, reorderZ } from '@sytue/escena';
import type { AlignEdge } from '@sytue/escena';
import type { SlideBox } from '../types';

export { boundingBox, distributeBoxes, reorderZ };
export type { AlignEdge, DistAxis, ZOrderOp } from '@sytue/escena';

/** Referencia de la alineación: la caja envolvente de la selección o el lienzo. */
export type AlignRef = 'selection' | 'canvas';

const CANVAS = { x: 0, y: 0, w: 100, h: 100 };

/** Alinea las cajas `ids` respecto a la selección (su envolvente) o al lienzo,
 *  con tope al lienzo. Devuelve un array NUEVO. Pura. */
export const alignBoxes = (
  boxes: SlideBox[],
  ids: readonly string[],
  edge: AlignEdge,
  ref: AlignRef,
): SlideBox[] => alignGeneric(boxes, ids, edge, ref === 'canvas' ? CANVAS : 'selection', CANVAS);

/* ---------- Grupos ---------- */

/** Expande `ids` para incluir a TODOS los miembros de los grupos tocados: mover,
 *  alinear o animar cualquier caja de un grupo actúa sobre el grupo entero. El
 *  resultado va en el orden del array `boxes` (y descarta ids inexistentes). Pura. */
export function expandGroups(boxes: SlideBox[], ids: readonly string[]): string[] {
  const sel = new Set(ids);
  const groups = new Set<string>();
  for (const b of boxes) if (sel.has(b.id) && b.groupId) groups.add(b.groupId);
  if (groups.size === 0) return boxes.filter((b) => sel.has(b.id)).map((b) => b.id);
  const out = new Set(sel);
  for (const b of boxes) if (b.groupId && groups.has(b.groupId)) out.add(b.id);
  return boxes.filter((b) => out.has(b.id)).map((b) => b.id);
}

/** Reagrupa las cajas de `sel` en un bloque contiguo situado donde estaba su
 *  miembro más al frente (conserva el orden interno y el del resto). */
function compact(boxes: SlideBox[], sel: Set<string>): SlideBox[] {
  const selected = boxes.filter((b) => sel.has(b.id));
  const others = boxes.filter((b) => !sel.has(b.id));
  let hi = -1;
  boxes.forEach((b, i) => {
    if (sel.has(b.id) && i > hi) hi = i;
  });
  let below = 0; // nº de «otras» por debajo del tope del bloque
  boxes.forEach((b, i) => {
    if (!sel.has(b.id) && i < hi) below++;
  });
  return [...others.slice(0, below), ...selected, ...others.slice(below)];
}

/** Agrupa las cajas `ids` bajo un `groupId` común (fusiona cualquier grupo previo
 *  de la selección en uno solo) y las compacta a un rango CONTIGUO en Z —donde
 *  estaba su miembro más al frente— para que el grupo se comporte como un bloque.
 *  Necesita ≥2 cajas. Devuelve un array nuevo. Pura (el id lo genera el llamante). */
export function groupBoxes(boxes: SlideBox[], ids: readonly string[], groupId: string): SlideBox[] {
  const sel = new Set(ids);
  if (boxes.filter((b) => sel.has(b.id)).length < 2) return boxes; // un grupo necesita ≥2
  const tagged = boxes.map((b) => (sel.has(b.id) ? { ...b, groupId } : b));
  return compact(tagged, sel);
}

/** Quita el groupId de todas las cajas que compartan grupo con la selección
 *  (desagrupa por completo los grupos tocados). Devuelve un array nuevo. Pura. */
export function ungroupBoxes(boxes: SlideBox[], ids: readonly string[]): SlideBox[] {
  const sel = new Set(ids);
  const groups = new Set<string>();
  for (const b of boxes) if (sel.has(b.id) && b.groupId) groups.add(b.groupId);
  if (groups.size === 0) return boxes;
  return boxes.map((b) => {
    if (b.groupId !== undefined && groups.has(b.groupId)) {
      const { groupId: _drop, ...rest } = b;
      return rest;
    }
    return b;
  });
}
