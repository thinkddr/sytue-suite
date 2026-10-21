/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Hilos de UN nivel (CAST-ENCARGO §10): cada comentario raíz con sus respuestas
 * debajo, en orden de llegada. Una respuesta a una respuesta cuelga del raíz del
 * hilo —el servicio la acepta, la interfaz no anida más—, y una respuesta cuyo
 * padre no ha llegado todavía (otra página) se enseña como raíz antes que perderla.
 */
import type { Comentario } from './tipos.js';

export interface Hilo {
  raiz: Comentario;
  respuestas: Comentario[];
}

export function enHilos(comentarios: readonly Comentario[]): Hilo[] {
  const porId = new Map(comentarios.map((c) => [c.id, c]));
  const raizDe = (c: Comentario): string => {
    let actual = c;
    const vistos = new Set<string>();
    while (actual.reply_to && porId.has(actual.reply_to) && !vistos.has(actual.id)) {
      vistos.add(actual.id);
      actual = porId.get(actual.reply_to)!;
    }
    return actual.id;
  };
  const hilos = new Map<string, Hilo>();
  for (const c of comentarios) {
    const r = raizDe(c);
    if (r === c.id) hilos.set(c.id, { raiz: c, respuestas: hilos.get(c.id)?.respuestas ?? [] });
    else {
      const h = hilos.get(r);
      if (h) h.respuestas.push(c);
      else hilos.set(r, { raiz: porId.get(r)!, respuestas: [c] });
    }
  }
  // Un hueco borrado sin respuestas no dice nada: fuera.
  return [...hilos.values()].filter((h) => !h.raiz.deleted || h.respuestas.length > 0);
}
