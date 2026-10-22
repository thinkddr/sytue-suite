/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Elegir calidades, sin red: sumar las estimaciones de varios ficheros, cuánto
 * ocupa una elección, si cabe, qué se quita. Los números los pone el servicio
 * (`GET /videos:estimate`, una sola estimación para la web y el escritorio);
 * aquí solo se combinan.
 */
import type { Estimacion, OpcionDeCalidad } from './tipos.js';

/** Lo que se enseña de una altura: «1080p». */
export const etiqueta = (q: number): string => `${q}p`;

/** De mayor a menor, que es como las guarda el servicio. */
export const ordenar = (qs: readonly number[]): number[] => [...qs].sort((a, b) => b - a);

/**
 * La estimación de varios ficheros a la vez (subir varios con una sola elección):
 * cada calidad suma lo de los ficheros que la permiten, y se permite si la permite
 * alguno (a los demás el servicio les da la mayor que alcancen). Aproximada si
 * alguna lo es.
 */
export function sumar(es: readonly Estimacion[]): Estimacion | null {
  if (es.length === 0) return null;
  const porCalidad = new Map<number, OpcionDeCalidad>();
  for (const e of es)
    for (const o of e.options) {
      const a = porCalidad.get(o.quality);
      porCalidad.set(
        o.quality,
        a
          ? {
              quality: o.quality,
              allowed: a.allowed || o.allowed,
              recommended: a.recommended || o.recommended,
              bytes: a.bytes + o.bytes,
            }
          : { ...o },
      );
    }
  const options = ordenar([...porCalidad.keys()]).map((q) => porCalidad.get(q)!);
  return {
    options,
    recommended: options.filter((o) => o.recommended).map((o) => o.quality),
    original_bytes: es.reduce((s, e) => s + e.original_bytes, 0),
    duration_ms: es.reduce((s, e) => s + e.duration_ms, 0),
    approximate: es.some((e) => e.approximate),
  };
}

/** Lo que ocupan esas calidades (sin el original). */
export function bytesDe(e: Estimacion, elegidas: readonly number[]): number {
  return e.options
    .filter((o) => o.allowed && elegidas.includes(o.quality))
    .reduce((s, o) => s + o.bytes, 0);
}

/** Lo que ocupará en total al subirlo: el original más lo elegido. */
export const totalAlSubir = (e: Estimacion, elegidas: readonly number[]): number =>
  e.original_bytes + bytesDe(e, elegidas);

/**
 * Marcar o desmarcar una calidad. Nunca deja la elección vacía: desmarcar la
 * última no hace nada (el servicio exige al menos una).
 */
export function alternar(elegidas: readonly number[], q: number): number[] {
  if (elegidas.includes(q)) {
    const resto = elegidas.filter((x) => x !== q);
    return resto.length ? resto : [...elegidas];
  }
  return ordenar([...elegidas, q]);
}

/** La elección de partida: la recomendada (todas las que el original permite). */
export const recomendada = (e: Estimacion): number[] => ordenar(e.recommended);

/** Si es la recomendada, para marcarlo. */
export const esRecomendada = (e: Estimacion, elegidas: readonly number[]): boolean =>
  ordenar(elegidas).join() === recomendada(e).join();

/** Las calidades que se quitan al pasar de `antes` a `despues` (lo que se borra). */
export const quitadas = (antes: readonly number[], despues: readonly number[]): number[] =>
  ordenar(antes.filter((q) => !despues.includes(q)));

/** Lo que falta por liberar o por ocupar al cambiar de elección en un vídeo ya creado. */
export function diferenciaDeBytes(
  e: Estimacion,
  antes: readonly number[],
  despues: readonly number[],
): number {
  return bytesDe(e, despues) - bytesDe(e, antes);
}
