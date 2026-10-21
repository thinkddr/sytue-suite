/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los comentarios de un registro llegan planos y en orden; aquí se agrupan en hilos de
 * un nivel (raíz + respuestas), que es como los guarda el servicio.
 */
import type { BasesComment } from '@sytue/api-clients/bases';

export interface Hilo {
  raiz: BasesComment;
  respuestas: BasesComment[];
}

/** Máximo de caracteres de un comentario (el servicio manda: `MaxLongComentario`). */
export const MAX_COMENTARIO = 2000;

/** Agrupa en hilos conservando el orden. Una respuesta sin su raíz se enseña como raíz. */
export function hilosDe(comentarios: readonly BasesComment[]): Hilo[] {
  const hilos: Hilo[] = [];
  const porId = new Map<string, Hilo>();
  for (const c of comentarios) {
    const padre = c.parent_id ? porId.get(c.parent_id) : undefined;
    if (padre) {
      padre.respuestas.push(c);
      continue;
    }
    const h = { raiz: c, respuestas: [] };
    hilos.push(h);
    porId.set(c.id, h);
  }
  return hilos;
}

/** El registro que pide abrir un enlace de aviso (`?table=…&record=…`), o null. */
export function registroDelEnlace(busqueda: string): { tabla: string; registro: string } | null {
  const q = new URLSearchParams(busqueda);
  const tabla = q.get('table') ?? '';
  const registro = q.get('record') ?? '';
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  return uuid.test(tabla) && uuid.test(registro) ? { tabla, registro } : null;
}
