/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Insertar un BLOQUE REUTILIZABLE en una presentación (comando `bloque`): una
 * diapositiva hecha (agenda, comparativa, cierre, o la primera de un bloque de la
 * organización). Llega como JSON `{id, boxes, …}`; aquí se queda con las cajas que se
 * entienden y les pone ids NUEVOS, para que insertar el mismo bloque dos veces no deje
 * dos cajas con el mismo id. Módulo puro (test/slides-bloque.test.ts).
 *
 * ponytail: los conectores del bloque no viajan (habría que remapearlos a los ids
 * nuevos, como hace «Duplicar diapositiva»); añadirlo si un bloque los necesita.
 */
import type { Slide, SlideBox } from '../types';

/** Tope de cajas de una diapositiva de bloque. */
export const MAX_CAJAS_BLOQUE = 200;

const finito = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);

/** La diapositiva que deja el bloque `raw`, con ids nuevos; null si no se entiende. */
export function diapositivaDeBloque(raw: string): Slide | null {
  let o: unknown;
  try {
    o = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!o || typeof o !== 'object' || Array.isArray(o)) return null;
  const s = o as Record<string, unknown>;
  if (!Array.isArray(s['boxes']) || s['boxes'].length > MAX_CAJAS_BLOQUE) return null;
  const boxes: SlideBox[] = [];
  for (const b of s['boxes'] as unknown[]) {
    if (!b || typeof b !== 'object' || Array.isArray(b)) continue;
    const c = b as Record<string, unknown>;
    if (!finito(c['x']) || !finito(c['y']) || !finito(c['w']) || !finito(c['h'])) continue;
    boxes.push({
      ...(c as unknown as SlideBox),
      id: crypto.randomUUID(),
      html: typeof c['html'] === 'string' ? c['html'] : '',
    });
  }
  if (boxes.length === 0) return null;
  const notes = typeof s['notes'] === 'string' ? s['notes'] : undefined;
  return { id: crypto.randomUUID(), boxes, ...(notes ? { notes } : {}) };
}
