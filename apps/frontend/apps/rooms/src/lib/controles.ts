/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cuándo puede esconderse la barra de controles de una videollamada.
 *
 * ESTILO TEAMS Y NO MEET: la barra se quita de en medio sola y vuelve al menor
 * movimiento. En un teléfono la barra fija se come un tercio de la pantalla —que
 * en una llamada ES el producto— y en el escritorio tapa la fila de abajo del
 * mosaico. Sin DOM aquí: lo que decide se prueba sin sala.
 */

/** Todo lo que impide esconder la barra en un momento dado. */
export interface Contexto {
  /** Hay un menú desplegado dentro de la barra (reacciones, dispositivos…). */
  menuAbierto: boolean;
  /** El foco del teclado está dentro de la barra. */
  focoDentro: boolean;
  /** El puntero está encima de la barra. */
  punteroEncima: boolean;
  /** La persona pidió verla expresamente (pulsó sobre el vídeo). */
  fijada: boolean;
}

/**
 * Decide si la barra puede esconderse ahora.
 *
 * LAS CUATRO CONDICIONES SON DE SEGURIDAD, no de gusto:
 *
 *  · `menuAbierto` — esconder la barra con un menú desplegado deja el menú
 *    flotando sobre el vídeo, anclado a algo que ya no está.
 *  · `focoDentro` — quien navega con teclado perdería el elemento enfocado, y un
 *    `display:none` sobre el foco lo manda al principio del documento. Es la
 *    diferencia entre una animación y quedarse sin poder colgar.
 *  · `punteroEncima` — la barra desapareciendo bajo el ratón justo antes del
 *    clic es la forma más rápida de que alguien pulse «salir» sin querer.
 *  · `fijada` — si la pidieron expresamente, mandan ellos y no el temporizador.
 */
export function puedeEsconderse(c: Contexto): boolean {
  return !c.menuAbierto && !c.focoDentro && !c.punteroEncima && !c.fijada;
}

/**
 * Milisegundos de quietud antes de esconder.
 *
 * Cuatro segundos: por debajo, la barra parpadea mientras alguien decide; por
 * encima, en un teléfono no llega a desaparecer nunca entre gesto y gesto.
 */
export const ESPERA_MS = 4000;
