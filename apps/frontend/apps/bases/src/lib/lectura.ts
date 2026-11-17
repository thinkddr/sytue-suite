/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Buscar y ordenar «de un momento»: lo que alguien teclea o elige encima de la vista
 * activa sin guardarla. Lo resuelve el servicio (`?q=` y `?sort=`) para que abarque la
 * tabla entera y no solo la ventana ya cargada.
 */
import type { BasesField } from '@sytue/api-clients/bases';

export interface Lectura {
  q: string;
  /** Campo por el que ordenar; vacío = el orden de la vista. */
  campo: string;
  dir: 'asc' | 'desc';
}

export const SIN_LECTURA: Lectura = { q: '', campo: '', dir: 'asc' };

/** Los parámetros de la consulta; lo que no se ha pedido no viaja. */
export function opcionesDeLectura(l: Lectura): { q?: string; sort?: string } {
  const q = l.q.trim();
  return { ...(q ? { q } : {}), ...(l.campo ? { sort: `${l.campo}:${l.dir}` } : {}) };
}

/** Se puede ordenar por lo que tiene un valor propio; enlaces y adjuntos guardan ids. */
export function camposOrdenables(campos: BasesField[]): BasesField[] {
  return campos.filter((c) => c.kind !== 'link' && c.kind !== 'attachment');
}

export function hayLectura(l: Lectura): boolean {
  return l.q.trim() !== '' || l.campo !== '';
}

/**
 * Turnos de lectura: cada lectura nueva (otra tabla, vista, búsqueda u orden) deja atrás
 * las que aún no han vuelto, para que la respuesta de «an» no pise a la de «ana» si
 * llega después.
 */
export interface Turnos {
  /** Abre un turno nuevo y lo devuelve. */
  nuevo: () => number;
  /** El turno en curso (el de una página más de la misma lectura). */
  actual: () => number;
  vigente: (turno: number) => boolean;
}

export function crearTurnos(): Turnos {
  let n = 0;
  return { nuevo: () => ++n, actual: () => n, vigente: (t) => t === n };
}
