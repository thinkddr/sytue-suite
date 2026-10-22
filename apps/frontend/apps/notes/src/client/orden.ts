/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * En qué orden se ven las notas de un cuaderno. El orden «propio» es el que da
 * el cuaderno —lo que se coloca a mano en el lienzo o en columnas— y es el de
 * siempre; los otros tres son de lectura y no tocan nada en el servidor.
 *
 * Las FIJADAS van delante en todos los órdenes: fijar una nota es decir «esta
 * primero», y un orden que la mandara al final anularía la única cosa que se
 * pidió expresamente.
 */
import type { Note } from '@sytue/api-clients';

export const ORDENES = ['manual', 'title', 'updated', 'created'] as const;
export type Orden = (typeof ORDENES)[number];

export function esOrden(v: string | null): v is Orden {
  return v !== null && (ORDENES as readonly string[]).includes(v);
}

/** Ordena una copia; el orden `manual` sigue las posiciones guardadas. */
export function ordenarNotas(notes: readonly Note[], orden: Orden, locale?: string): Note[] {
  const fijadas = (a: Note, b: Note) => Number(Boolean(b.pinned)) - Number(Boolean(a.pinned));
  if (orden === 'manual') {
    return [...notes].sort((a, b) => fijadas(a, b) || a.col - b.col || a.position - b.position);
  }
  const cmp: Record<Exclude<Orden, 'manual'>, (a: Note, b: Note) => number> = {
    // Sin título, al final: una nota vacía arriba del todo no dice nada.
    title: (a, b) => {
      const ta = (a.title ?? '').trim();
      const tb = (b.title ?? '').trim();
      if (!ta !== !tb) return ta ? -1 : 1;
      return ta.localeCompare(tb, locale, { sensitivity: 'base' });
    },
    updated: (a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? ''),
    created: (a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? ''),
  };
  return [...notes].sort((a, b) => fijadas(a, b) || cmp[orden](a, b));
}
