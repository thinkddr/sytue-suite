/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Velocidad de reproducción del vídeo bajo demanda, sin DOM: la escalera que ofrece
 * el menú y el paso de las teclas < y >. De 0,5× a 2×, como cualquier reproductor:
 * por debajo el audio ya no se entiende y por encima los navegadores lo silencian
 * o lo distorsionan, y una formación o una reunión grabada no necesita más.
 */

/** Las velocidades del menú, de menor a mayor. La 1 es la normal. */
export const VELOCIDADES: readonly number[] = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2];

/**
 * La siguiente velocidad hacia arriba (`+1`) o hacia abajo (`-1`), y en los extremos
 * la misma: no da la vuelta, que pasar de 2× a 0,5× por pulsar una vez de más
 * desconcierta. Una velocidad fuera de la escalera (la pudo poner el navegador)
 * salta a la vecina de la escalera en ese sentido.
 */
export function siguiente(actual: number, sentido: 1 | -1): number {
  const primera = VELOCIDADES[0]!;
  const ultima = VELOCIDADES[VELOCIDADES.length - 1]!;
  if (sentido > 0) return VELOCIDADES.find((v) => v > actual + 1e-9) ?? ultima;
  return [...VELOCIDADES].reverse().find((v) => v < actual - 1e-9) ?? primera;
}
