/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cursor de la lista: adónde lleva «j»/«k» y adónde los botones «Más reciente» y
 * «Más antigua» del lector. Es el MISMO cálculo para los dos, y por eso vive aquí y
 * no dentro del manejador de teclado: si cada camino decidiera por su cuenta, el
 * botón y la tecla acabarían discrepando en los bordes. Fyne: `Buzon.moverCursor`.
 */

/** Lo que hay que hacer al mover el cursor. */
export type PasoDelCursor =
  /** Ir a la fila `indice` (puede ser la misma: tocar el borde no la cambia). */
  | { tipo: 'ir'; indice: number }
  /** Se está en la última cargada y quedan páginas: pedir la siguiente y seguir. */
  | { tipo: 'cargar' }
  | null;

export interface EstadoDelCursor {
  /** Conversaciones visibles (de la más reciente a la más antigua). */
  total: number;
  /** Índice de la abierta en la lista; -1 si no hay ninguna (o no está a la vista). */
  actual: number;
  /** Donde se quedó el cursor sin nada abierto (tras archivar, por ejemplo). */
  cursor: number;
  /** `true` hacia abajo (la más antigua, «j»), `false` hacia arriba («k»). */
  siguiente: boolean;
  /** Quedan páginas por pedir más allá de la última cargada. */
  hayMas: boolean;
}

export function pasoDelCursor({
  total,
  actual,
  cursor,
  siguiente,
  hayMas,
}: EstadoDelCursor): PasoDelCursor {
  if (total === 0) return null;
  // Sin nada abierto se retoma donde estaba el cursor: tras archivar, esa posición
  // ya la ocupa la conversación siguiente (como Gmail).
  if (actual < 0) return { tipo: 'ir', indice: Math.min(cursor, total - 1) };
  if (!siguiente) return { tipo: 'ir', indice: Math.max(actual - 1, 0) };
  if (actual < total - 1) return { tipo: 'ir', indice: actual + 1 };
  // En la última cargada la lista no ha terminado si el servidor tiene más: bajar
  // es pedir la página siguiente, como haría el scroll al llegar al final.
  return hayMas ? { tipo: 'cargar' } : { tipo: 'ir', indice: actual };
}

/**
 * Si los botones del lector llevan a algún sitio. La más reciente no tiene nada por
 * encima, y la más antigua solo tiene algo debajo si quedan páginas. Una conversación
 * que no está en la lista (se llegó por enlace) no tiene vecinas.
 */
export function vecinasDeLaAbierta(
  total: number,
  actual: number,
  hayMas: boolean,
): { anterior: boolean; siguiente: boolean } {
  if (actual < 0 || actual >= total) return { anterior: false, siguiente: false };
  return { anterior: actual > 0, siguiente: actual < total - 1 || hayMas };
}
