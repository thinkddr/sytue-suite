/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * El «12 / 240» de la cabecera del visor y los saltos de Inicio y Fin. La misma
 * regla vive en Fyne (`posicionVisor` y `saltoVisor` en
 * apps/fyne/apps/photos/app_visor.go).
 */

export interface PosicionVisor {
  /** Desde 1, como se cuenta en voz alta. */
  indice: number;
  total: number;
  /** Quedan páginas por pedir: el total es lo cargado, no todas («240+»). */
  hayMas: boolean;
}

/**
 * Dónde va la foto abierta, o `null` si no hay nada que contar: una sola foto
 * (y nada más por cargar) no necesita «1 / 1».
 */
export function posicionVisor(
  abierta: number,
  total: number,
  hayMas = false,
): PosicionVisor | null {
  if (abierta < 0 || abierta >= total || (total === 1 && !hayMas)) return null;
  return { indice: abierta + 1, total, hayMas };
}

/**
 * A qué foto lleva una tecla: Inicio a la primera y Fin a la última CARGADA —en
 * la galería, pedir todas las páginas para llegar al final de miles sería
 * descargar la biblioteca entera—. `null`: esa tecla no salta, o ya está ahí.
 */
export function saltoVisor(tecla: string, abierta: number, total: number): number | null {
  if (tecla === 'Home') return abierta > 0 ? 0 : null;
  if (tecla === 'End') return abierta < total - 1 ? total - 1 : null;
  return null;
}
