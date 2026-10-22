/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Decisiones puras sobre qué vídeo ocupa la baldosa de un participante.
 * Sin DOM y sin LiveKit: lo que hay aquí se puede probar sin sala.
 */

/** Qué se pega en el hueco cuando un participante manda cámara y pantalla. */
export interface Hueco {
  /** La pista que se está considerando es una pantalla compartida. */
  esPantalla: boolean;
  /** El participante tiene ADEMÁS una pantalla compartida publicada. */
  hayPantalla: boolean;
}

/**
 * Decide si esta pista de vídeo debe ocupar el hueco del participante.
 *
 * UN SOLO VÍDEO POR BALDOSA, y cuando hay dos manda la PANTALLA COMPARTIDA:
 * quien comparte lo hace para que se mire eso, no su cara. Sin una regla fija,
 * las dos pistas se turnan el hueco en cada pasada de reconciliación y la
 * llamada parpadea entre la cara y la presentación.
 */
export function pegaEsteVideo({ esPantalla, hayPantalla }: Hueco): boolean {
  return esPantalla || !hayPantalla;
}
