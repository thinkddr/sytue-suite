// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Las fechas del historial y de los paneles de Sytue Chat.
//
// Un canal vive años: «viernes, 3 de noviembre» sin más es ambiguo en cuanto el
// mensaje es del año pasado. La regla, la misma en el separador de día y en las filas
// de los paneles (hilos, guardados, búsqueda, programados), es que el año aparece SOLO
// cuando no es el actual; repetirlo en cada fecha de este año gasta ancho sin decir
// nada. El reloj se inyecta para que los tests no dependan del día en que corren.
import { formatDate, formatDateTime, type DateLike, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';

function aFecha(v: DateLike): Date {
  return v instanceof Date ? v : new Date(v);
}

function mismoDia(a: Date, b: Date): boolean {
  return a.toDateString() === b.toDateString();
}

/** `numeric` si `d` cae en otro año (local) que `now`; si no, sin año. */
function anoSiNoEsElActual(d: Date, now: Date): { year?: 'numeric' } {
  return d.getFullYear() === now.getFullYear() ? {} : { year: 'numeric' };
}

/**
 * Etiqueta del separador de día. «Hoy»/«Ayer» salen del catálogo; el resto es una
 * fecha larga formateada por `Intl` en el idioma del usuario, con el año solo si no
 * es el actual. La mayúscula inicial NO se fuerza: en alemán e inglés el día ya va en
 * mayúscula y en español/francés/italiano va en minúscula a propósito (capitalizarla a
 * mano sería un error de ortografía en tres de las cinco lenguas).
 */
export function dayLabel(locale: Locale, t: T, iso: DateLike, now: DateLike = new Date()): string {
  const d = aFecha(iso);
  const hoy = aFecha(now);
  const ayer = new Date(hoy);
  ayer.setDate(hoy.getDate() - 1);
  if (mismoDia(d, hoy)) return t('day.today');
  if (mismoDia(d, ayer)) return t('day.yesterday');
  return formatDate(locale, d, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    ...anoSiNoEsElActual(d, hoy),
  });
}

/** Fecha corta + hora de una fila de panel (el orden y el reloj los pone el locale),
 *  con el año solo si no es el actual. */
export function whenLabel(locale: Locale, iso: DateLike, now: DateLike = new Date()): string {
  const d = aFecha(iso);
  return formatDateTime(locale, d, {
    day: 'numeric',
    month: 'short',
    ...anoSiNoEsElActual(d, aFecha(now)),
    hour: '2-digit',
    minute: '2-digit',
  });
}
