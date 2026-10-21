/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La proporción con la que una foto entra en las FILAS JUSTIFICADAS de la galería
 * (`.filas`, en styles/app.css): cada tesela conserva la suya, las filas llenan el
 * ancho y no se recorta nada.
 *
 * SALVO LAS EXTREMAS. Una panorámica de 4:1 sola ocupa una fila entera y deja a las
 * de al lado en una rendija; una captura de pantalla larguísima es una tira de
 * veinte píxeles que no se acierta con el dedo. Esas dos se acotan y son las únicas
 * que `object-cover` recorta, al centro:
 *
 *   - 9:20 por abajo: es la pantalla de un móvil de hoy, así que sus capturas
 *     —que son media galería de cualquiera— entran enteras.
 *   - 5:2 por arriba: cabe el 21:9 de cine; una panorámica de verdad (3:1 o más)
 *     se recorta por los lados.
 *
 * SIN DIMENSIONES, UN CUADRADO. Un fichero que el índice no supo medir llega con
 * ceros: 0/0 es NaN y un `aspect-ratio: NaN` deja la tesela sin alto.
 */
import type { CSSProperties } from 'preact';

export const PROPORCION_MIN = 9 / 20;
export const PROPORCION_MAX = 5 / 2;

export function proporcion(foto: { width: number; height: number }): number {
  const r = foto.width / foto.height;
  if (!Number.isFinite(r) || r <= 0) return 1;
  return Math.min(Math.max(r, PROPORCION_MIN), PROPORCION_MAX);
}

/** Lo que el `<li>` de una tesela le dice a `.filas`: su proporción, en `--r`. */
export function estiloDeTesela(foto: { width: number; height: number }): CSSProperties {
  return { '--r': Math.round(proporcion(foto) * 1e4) / 1e4 };
}
