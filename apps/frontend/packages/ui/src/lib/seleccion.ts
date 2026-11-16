/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * La selección múltiple de una lista o una galería: clic marca y desmarca, y Mayús + clic
 * marca el TRAMO desde la última marcada sin Mayús (el ancla). Es lo que hacen Drive y
 * Photos, y cualquier explorador de escritorio; vive aquí para que las dos cuenten igual.
 */

/** Los ids del tramo que va del ancla al índice pulsado, los dos incluidos. */
export function idsDelRango<T extends { id?: string | undefined }>(
  enOrden: readonly T[],
  ancla: number,
  indice: number,
): string[] {
  const [lo, hi] = ancla < indice ? [ancla, indice] : [indice, ancla];
  const ids: string[] = [];
  for (let i = Math.max(lo, 0); i <= hi && i < enOrden.length; i++) {
    const id = enOrden[i]?.id;
    if (id) ids.push(id);
  }
  return ids;
}

/**
 * Un clic sobre el elemento `indice` de `enOrden`. Sin Mayús (o sin ancla) conmuta ese y
 * lo deja de ancla; con Mayús AÑADE el tramo del ancla a él sin desmarcar nada ni mover
 * el ancla, para que un segundo Mayús + clic corrija el tramo desde el mismo sitio.
 */
export function marcarConTramo<T extends { id?: string | undefined }>(
  marcadas: ReadonlySet<string>,
  enOrden: readonly T[],
  indice: number,
  ancla: number,
  mayus: boolean,
): { marcadas: Set<string>; ancla: number } {
  const next = new Set(marcadas);
  if (mayus && ancla >= 0 && indice >= 0) {
    for (const id of idsDelRango(enOrden, ancla, indice)) next.add(id);
    return { marcadas: next, ancla };
  }
  const id = enOrden[indice]?.id;
  if (!id) return { marcadas: next, ancla };
  if (next.has(id)) next.delete(id);
  else next.add(id);
  return { marcadas: next, ancla: indice };
}
