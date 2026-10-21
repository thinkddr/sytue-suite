/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las medidas del folio, en un solo sitio.
 *
 * Estaban escritas a mano en cuatro ficheros —el lienzo, la regla, la marca de agua y el
 * catálogo de tamaños de papel—, todos con los mismos números y ninguno enterado de los
 * otros: cambiar uno dejaba la regla midiendo un folio distinto del que se pinta.
 *
 * En CSS viven como `--editor-page-w/h` y `--editor-ruler-h` (editor.css §8), y ahí siguen
 * porque una hoja de estilos no puede importar TypeScript. Son los mismos números y hay
 * que cambiarlos a la vez; el aviso está en los dos lados.
 */

/** Folio A4 en px @96 dpi y zoom 100 (210 × 297 mm). */
export const PAGE_W = 794;
export const PAGE_H = 1123;

/** Alto de la regla en px de maqueta. */
export const RULER_H = 20;

/**
 * Zoom mínimo, en %. Era 50 en la barra de estado, y con ese suelo un A4 (794px)
 * no llega a caber en un teléfono: 320px de pantalla piden ~37 %. Lo comparten el
 * deslizador y el ajuste inicial al ancho del lienzo, que si no se recortarían el
 * uno al otro.
 */
export const MIN_ZOOM = 25;
export const MAX_ZOOM = 200;
