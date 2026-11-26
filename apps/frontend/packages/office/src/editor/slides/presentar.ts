/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que la tanda del 2026-11-25 añade al modo presentación de Diapos, puro y sin DOM:
 * el avance automático en bucle (la pantalla de una recepción o de un estand, que nadie
 * pasa a mano), la rueda del ratón para pasar de diapositiva, la pausa del cronómetro del
 * moderador y las notas del orador como texto para descargarlas.
 *
 * La app nativa tiene sus gemelos en `apps/fyne/diapos/herramientas.go`, y los dos recorren
 * los mismos vectores (`test/vectores/diapos-herramientas.json`): la misma presentación no
 * puede saltar a otra diapositiva según dónde se proyecte.
 */
import type { Slide } from '../types';
import { hiddenMask, nextShown, presentAdvance } from './anim';

/** Los segundos que se ofrecen para el avance automático; 0 es «a mano». */
export const SEGUNDOS_AVANCE = [0, 5, 10, 20, 30, 60] as const;

/**
 * La posición siguiente en una presentación que avanza sola: la de la flecha (paso a
 * paso, saltando las ocultas) y, al acabar, vuelta a la primera que se enseña, que es
 * lo que hace PowerPoint «examinada en un quiosco».
 */
export function avanceAutomatico(
  slides: Slide[],
  slide: number,
  step: number,
): { slide: number; step: number } {
  const sig = presentAdvance(slides, slide, step, 1);
  if (sig.slide !== slide || sig.step !== step) return sig;
  return { slide: nextShown(hiddenMask(slides), -1, 1) ?? 0, step: 0 };
}

/**
 * La rueda del ratón: se acumula lo girado HACIA ABAJO (negativo, hacia arriba) y, al
 * pasar del umbral, se pasa una diapositiva y se empieza de cero. Así una muesca de una
 * rueda pasa una, y el deslizamiento continuo de un panel táctil no se come diez.
 */
export function pasoDeRueda(
  acumulado: number,
  abajo: number,
  umbral: number,
): { dir: -1 | 0 | 1; resto: number } {
  const total = acumulado + abajo;
  if (total >= umbral) return { dir: 1, resto: 0 };
  if (total <= -umbral) return { dir: -1, resto: 0 };
  return { dir: 0, resto: total };
}

/** El cronómetro del moderador: cuándo empezó y, si está en pausa, desde cuándo. */
export interface Cronometro {
  inicio: number;
  pausa: number | null;
}

/** Lo transcurrido a `ahora`, sin contar lo que lleva en pausa. */
export function transcurrido(c: Cronometro, ahora: number): number {
  return Math.max(0, (c.pausa ?? ahora) - c.inicio);
}

/** Pausa o reanuda: al reanudar, el inicio se corre lo que duró la pausa. */
export function alternarPausa(c: Cronometro, ahora: number): Cronometro {
  return c.pausa === null
    ? { inicio: c.inicio, pausa: ahora }
    : { inicio: c.inicio + (ahora - c.pausa), pausa: null };
}

/**
 * «Descargar las notas del orador»: cada diapositiva con su título («Diapositiva 3») y
 * sus notas debajo, separadas por una línea en blanco; las líneas sin el blanco del final
 * y sin las vacías de los extremos. Las ocultas también: quien presenta las lleva.
 */
export function notasComoTexto(slides: readonly Slide[], titulo: (n: number) => string): string {
  if (slides.length === 0) return '';
  const bloques = slides.map((s, i) => {
    const lineas = (s.notes ?? '').split(/\r\n|\r|\n/).map((l) => l.replace(/\s+$/u, ''));
    while (lineas.length > 0 && lineas[0] === '') lineas.shift();
    while (lineas.length > 0 && lineas[lineas.length - 1] === '') lineas.pop();
    return [titulo(i + 1), ...lineas].join('\n');
  });
  return `${bloques.join('\n\n')}\n`;
}

const CLAVE_AVANCE = 'sytue.diapos.avance';

/** Los segundos del avance automático elegidos en este dispositivo (0, a mano). Es de
 *  quien presenta, no de la presentación: no viaja con el documento. */
export function leerAvance(): number {
  try {
    const n = Number(localStorage.getItem(CLAVE_AVANCE) ?? '0');
    return (SEGUNDOS_AVANCE as readonly number[]).includes(n) ? n : 0;
  } catch {
    return 0; // ventana privada o almacenamiento bloqueado: a mano
  }
}

export function fijarAvance(segundos: number): void {
  try {
    if (segundos > 0) localStorage.setItem(CLAVE_AVANCE, String(segundos));
    else localStorage.removeItem(CLAVE_AVANCE);
  } catch {
    // sin almacenamiento, la elección dura lo que la página
  }
}
