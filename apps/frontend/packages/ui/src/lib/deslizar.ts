/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DESLIZAR CON EL DEDO: cuándo un arrastre horizontal es «pasa a lo siguiente».
 *
 * Nació en el visor de Sytue Photos (pasar de foto) y Calendar la necesitaba igual para
 * cambiar de mes en el teléfono: sería la tercera copia (la web de Photos, su pareja en
 * Fyne y Calendar), así que la regla vive aquí y las dos apps la importan.
 *
 * Es la MISMA en Fyne (`ui.DecidirDeslizamiento`, apps/fyne/ui/deslizar.go): mismos
 * umbrales, mismo resultado, y los mismos casos de test a los dos lados. Si se cambia
 * un umbral aquí, se cambia allí.
 */

/** Lo menos que tiene que recorrer el dedo, en px, para pasar. */
export const RECORRIDO_MIN = 56;
/** O esta fracción del ancho, si es mayor: en una tableta 56 px es un roce. */
export const FRACCION_ANCHO = 0.18;
/** Un golpe rápido pasa aunque recorra menos: px por milisegundo. */
export const VELOCIDAD_RAPIDA = 0.5;
/** Y aun rápido, al menos esto: un temblor no es un gesto. */
export const RECORRIDO_RAPIDO = 24;
/** Horizontal de verdad: |dx| tiene que pasar de |dy| por este factor. */
export const DOMINIO_HORIZONTAL = 1.4;

/**
 * Qué hace un deslizamiento al soltar: −1 lo anterior, 1 lo siguiente, 0 nada. Hacia la
 * IZQUIERDA (dx < 0) se va a lo siguiente, como pasar una página. `ancho` es el del
 * área que se desliza y `ms` lo que duró el gesto.
 */
export function decidirDeslizamiento(
  dx: number,
  dy: number,
  ancho: number,
  ms: number,
): -1 | 0 | 1 {
  const ax = Math.abs(dx);
  if (ax < DOMINIO_HORIZONTAL * Math.abs(dy)) return 0;
  const lejos = ax >= Math.max(RECORRIDO_MIN, FRACCION_ANCHO * ancho);
  const rapido = ax >= RECORRIDO_RAPIDO && ax / Math.max(ms, 1) >= VELOCIDAD_RAPIDA;
  if (!lejos && !rapido) return 0;
  return dx < 0 ? 1 : -1;
}
