/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La vista de lista de Sytue Drive en la forma «Marco»: las carpetas salen ARRIBA, como
 * una franja de teselas de color pleno, y lo demás sigue en la lista. Son dos piezas en
 * pantalla y UNA sola lista para todo lo que cuenta posiciones: la selección por rango
 * (Mayús + clic), «seleccionar todo» y el orden, que es el que da el servicio.
 */

export interface Partes<T> {
  /** Lo que va a la franja de teselas (vacío si la franja no está activa). */
  carpetas: T[];
  /** Lo que sigue en la lista. */
  resto: T[];
  /** Todo, en el orden en que se ve: primero la franja y después la lista. */
  enOrden: readonly T[];
}

/**
 * Parte los elementos entre la franja y la lista. Cada parte conserva el orden en que
 * llegó —el del servicio: no se reordena en cliente, o la paginación perdería el hilo—.
 * Con la franja inactiva (forma «Carril», vista de cuadrícula, una búsqueda) no parte
 * nada: todo va a la lista, como siempre.
 */
export function partirEnTeselas<T extends { isDirectory: boolean }>(
  items: readonly T[] | null,
  activa: boolean,
): Partes<T> {
  const todo = items ?? [];
  const carpetas = activa ? todo.filter((it) => it.isDirectory) : [];
  if (carpetas.length === 0) return { carpetas: [], resto: [...todo], enOrden: todo };
  const resto = todo.filter((it) => !it.isDirectory);
  return { carpetas, resto, enOrden: [...carpetas, ...resto] };
}

/**
 * La posición de un elemento de la LISTA dentro de `enOrden`: la franja va delante, así
 * que sus teselas ocupan las primeras posiciones y la lista empieza después.
 */
export function posicionEnLista<T>(partes: Partes<T>, indiceEnLaLista: number): number {
  return partes.carpetas.length + indiceEnLaLista;
}

/** Los ids del tramo que va del ancla al índice pulsado, los dos incluidos. */
export function idsDelRango<T extends { id?: string | undefined }>(
  enOrden: readonly T[],
  ancla: number,
  indice: number,
): string[] {
  const [lo, hi] = ancla < indice ? [ancla, indice] : [indice, ancla];
  const ids: string[] = [];
  for (let i = lo; i <= hi; i++) {
    const id = enOrden[i]?.id;
    if (id) ids.push(id);
  }
  return ids;
}
