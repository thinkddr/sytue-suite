/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/**
 * AUTO-AVANCE: qué conversación se abre cuando la abierta sale de la vista.
 *
 * Archivar (o tirar, o mover) la conversación que se está leyendo devolvía a la
 * lista, y para seguir con la bandeja había que volver a elegir la siguiente: un
 * toque o un clic de más por cada correo. Como Gmail y Superhuman, ahora se abre
 * la SIGUIENTE —la más antigua, que es la que queda debajo en la lista—; cada
 * cuenta puede preferir la más reciente o volver a la lista (Ajustes → Lectura).
 *
 * `lista` es lo que se VE (ya filtrada), en el orden en que se pinta: de la más
 * reciente a la más antigua. Se calcula ANTES de quitar nada de ella.
 */
import type { AutoAdvance } from '../client/mailApi.js';

export function conversacionTrasQuitar(
  lista: readonly { threadId: string }[],
  abierta: string | null,
  quitadas: ReadonlySet<string>,
  modo: AutoAdvance,
): string | null {
  // La abierta no se va: se queda como estaba.
  if (!abierta || !quitadas.has(abierta)) return abierta;
  if (modo === 'list') return null;
  const i = lista.findIndex((c) => c.threadId === abierta);
  if (i < 0) return null;
  const paso = modo === 'older' ? 1 : -1;
  // Se salta lo que se va a la vez (archivar en lote con la abierta dentro).
  for (let j = i + paso; j >= 0 && j < lista.length; j += paso) {
    const c = lista[j];
    if (c && !quitadas.has(c.threadId)) return c.threadId;
  }
  // No queda ninguna en ese sentido: a la lista, no a la del otro lado (sería
  // saltar a un correo que no se pidió).
  return null;
}
