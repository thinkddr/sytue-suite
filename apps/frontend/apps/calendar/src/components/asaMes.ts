/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL ASA DEL MES DEL TELÉFONO, como datos: a qué posición va la hoja según lo que se
 * desplazó el dedo y a qué velocidad. Sin DOM: lo pinta MesCompacto.tsx.
 * La pareja en Fyne es `apps/fyne/calendario/asames.go`, con los mismos vectores de test
 * (A1…) y el mismo nombre a los dos lados. Si se cambia un umbral aquí, se cambia allí.
 *
 * Tres posiciones, de abajo arriba, como la hoja del calendario de iOS:
 *   - `mes`: la rejilla entera (24 de iniciales + 40 por semana);
 *   - `semana`: plegada a la semana del día elegido (24 + 40);
 *   - `agenda`: sin rejilla (0); debajo va la agenda.
 * Arrastrar hacia ARRIBA pliega (mes → semana → agenda); hacia abajo, al revés. Al soltar,
 * la más cercana a la altura alcanzada; con un lanzamiento (VELOCIDAD_RAPIDA, la misma
 * que el «golpe rápido» del deslizar), la siguiente en el sentido del lanzamiento.
 */
import { VELOCIDAD_RAPIDA } from '@sytue/ui';
import type { ViewMode } from './dates.js';

export type PosicionAsa = 'mes' | 'semana' | 'agenda';

/** De abajo arriba: el índice es el `aria-valuenow` del asa. */
export const POSICIONES: readonly PosicionAsa[] = ['mes', 'semana', 'agenda'];

/** La fila de iniciales (`h-6`) y cada semana (`h-10`), como en la rejilla. */
export const ALTO_DIAS = 24;
export const ALTO_SEMANA = 40;
/** Menos que esto no es un arrastre: es un toque, y lo decide el clic. */
export const RECORRIDO_TOQUE = 6;
/** La velocidad al soltar se mide sobre los últimos milisegundos del gesto. */
export const VENTANA_VELOCIDAD = 100;

/** Alto de la hoja (sin el asa) en una posición, con `filas` semanas en el mes. */
export function alturaHoja(pos: PosicionAsa, filas: number): number {
  if (pos === 'mes') return ALTO_DIAS + filas * ALTO_SEMANA;
  if (pos === 'semana') return ALTO_DIAS + ALTO_SEMANA;
  return 0;
}

/** El alto mientras se arrastra: el de partida más lo recorrido, sin salirse. */
export function alturaArrastre(desde: PosicionAsa, dy: number, filas: number): number {
  return Math.min(Math.max(alturaHoja(desde, filas) + dy, 0), alturaHoja('mes', filas));
}

/**
 * Cómo se coloca el contenido de la hoja a un alto `h`, con la semana del cursor en la
 * fila `filaCursor`. `plegado` es lo que suben las semanas bajo las iniciales (de 0 en el
 * mes a `filaCursor·40` en la semana, en proporción); `subida`, lo que sube todo —
 * iniciales incluidas— por debajo de la semana, hasta desaparecer en la agenda. Las dos
 * son continuas: la hoja sigue al dedo sin saltos.
 */
export function geometriaHoja(
  h: number,
  filas: number,
  filaCursor: number,
): { plegado: number; subida: number } {
  const mes = alturaHoja('mes', filas);
  const semana = alturaHoja('semana', filas);
  const f = mes > semana ? Math.min(Math.max((mes - h) / (mes - semana), 0), 1) : 1;
  return { plegado: filaCursor * ALTO_SEMANA * f, subida: Math.max(0, semana - h) };
}

/**
 * La velocidad al soltar, en px/ms (positiva hacia abajo): de la primera muestra de los
 * últimos VENTANA_VELOCIDAD ms a la última. Con menos de dos, o sin tiempo, 0.
 */
export function velocidadFinal(muestras: readonly { t: number; y: number }[]): number {
  const ultima = muestras[muestras.length - 1];
  if (!ultima) return 0;
  const primera = muestras.find((m) => m.t >= ultima.t - VENTANA_VELOCIDAD) ?? ultima;
  const dt = ultima.t - primera.t;
  return dt > 0 ? (ultima.y - primera.y) / dt : 0;
}

/**
 * Adónde va la hoja al soltar: `dy` es lo recorrido (positivo hacia abajo) y `velocidad`
 * la de `velocidadFinal`. Con menos de RECORRIDO_TOQUE se queda donde estaba.
 */
export function posicionAlSoltar(
  desde: PosicionAsa,
  dy: number,
  velocidad: number,
  filas: number,
): PosicionAsa {
  if (Math.abs(dy) < RECORRIDO_TOQUE) return desde;
  const h = alturaArrastre(desde, dy, filas);
  const alto = (p: PosicionAsa): number => alturaHoja(p, filas);
  if (Math.abs(velocidad) >= VELOCIDAD_RAPIDA) {
    // Hacia arriba, la primera por debajo de lo alcanzado; hacia abajo, la primera por
    // encima. Si no queda ninguna, la del extremo.
    // POSICIONES va de más alta a menos: hacia arriba se busca en ese orden, hacia
    // abajo en el contrario.
    if (velocidad < 0) return POSICIONES.find((p) => alto(p) < h) ?? 'agenda';
    return [...POSICIONES].reverse().find((p) => alto(p) > h) ?? 'mes';
  }
  // La más cercana; a la misma distancia, la más próxima a la de partida.
  const i0 = POSICIONES.indexOf(desde);
  let mejor = desde;
  for (const p of POSICIONES) {
    const d = Math.abs(alto(p) - h);
    const dm = Math.abs(alto(mejor) - h);
    const masCerca =
      Math.abs(POSICIONES.indexOf(p) - i0) < Math.abs(POSICIONES.indexOf(mejor) - i0);
    if (d < dm || (d === dm && masCerca)) mejor = p;
  }
  return mejor;
}

/** Un toque: mes ↔ semana; desde la agenda, a la semana. */
export function posicionAlTocar(pos: PosicionAsa): PosicionAsa {
  return pos === 'semana' ? 'mes' : 'semana';
}

/** Un paso con el teclado: +1 hacia la agenda (↑), −1 hacia el mes (↓), sin salirse. */
export function posicionTrasPaso(pos: PosicionAsa, paso: number): PosicionAsa {
  const i = Math.min(Math.max(POSICIONES.indexOf(pos) + paso, 0), POSICIONES.length - 1);
  return POSICIONES[i]!;
}

/** La posición que enseña una vista en el teléfono; el día no tiene asa. */
export function posicionDeVista(mode: ViewMode): PosicionAsa | null {
  if (mode === 'month') return 'mes';
  if (mode === 'week') return 'semana';
  if (mode === 'agenda') return 'agenda';
  return null;
}

/** La vista de cada posición: el selector del cajón y el asa dicen lo mismo. */
export function vistaDePosicion(pos: PosicionAsa): ViewMode {
  return pos === 'mes' ? 'month' : pos === 'semana' ? 'week' : 'agenda';
}
