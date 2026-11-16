/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Geometría y aritmética del arrastre en la vista de MES: qué celda queda bajo el
 * puntero y dónde cae un evento movido N días. Es lógica pura —sin DOM ni estado—
 * para que la rejilla solo tenga que pintar y para poder probarla sin montar nada.
 */
import { eventInstant, parseTzid, toIcalUtc } from './dates.js';

/** Lo que hace falta de un evento para desplazarlo de día. */
export interface EventoDesplazable {
  start: string;
  end?: string | undefined;
  ical?: string | undefined;
}

/** Rectángulo de la rejilla del mes en coordenadas del viewport. */
export interface RectRejilla {
  left: number;
  top: number;
  width: number;
  height: number;
}

/**
 * Índice de la celda (0..semanas·7-1) bajo el punto, o null si la rejilla no tiene
 * tamaño. Fuera de la rejilla se pega al borde: soltar un poco por debajo de la última
 * semana cuenta como la última semana, que es lo que espera quien arrastra deprisa.
 */
export function celdaEnPunto(
  rect: RectRejilla,
  semanas: number,
  x: number,
  y: number,
): number | null {
  if (rect.width <= 0 || rect.height <= 0 || semanas <= 0) return null;
  const acotar = (v: number, max: number): number => Math.max(0, Math.min(max, Math.floor(v)));
  const col = acotar(((x - rect.left) / rect.width) * 7, 6);
  const fila = acotar(((y - rect.top) / rect.height) * semanas, semanas - 1);
  return fila * 7 + col;
}

/** ¿Es un evento de día completo (DTSTART;VALUE=DATE)? */
export function esDiaCompleto(event: EventoDesplazable): boolean {
  return /^\d{8}$/.test(event.start);
}

/** Suma días de CALENDARIO: conserva la hora de pared aunque haya un cambio de hora. */
function sumarDias(d: Date, dias: number): Date {
  const r = new Date(d);
  r.setDate(r.getDate() + dias);
  return r;
}

/**
 * El inicio y el fin del evento movido `dias` días, con la misma hora de pared y la
 * misma duración en días. Null si su inicio no se puede leer. Sin DTEND, un día
 * completo dura un día y uno con hora no dura nada (RFC 5545 §3.6.1), igual que lo
 * pinta la rejilla. El fin de un día completo sigue siendo EXCLUSIVO, como lo espera
 * `saveEvent`.
 */
export function desplazarDias(
  event: EventoDesplazable,
  dias: number,
): { start: Date; end: Date } | null {
  const tz = event.ical ? parseTzid(event.ical) : null;
  const start = eventInstant(event.start, tz);
  if (!start) return null;
  const finLeido = event.end ? eventInstant(event.end, tz) : null;
  const end = finLeido ?? (esDiaCompleto(event) ? sumarDias(start, 1) : start);
  return { start: sumarDias(start, dias), end: sumarDias(end, dias) };
}

function fechaIcal(d: Date): string {
  const pad = (n: number): string => String(n).padStart(2, '0');
  return `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}`;
}

/**
 * El evento ya en su nuevo sitio, para pintarlo ahí mientras se guarda (y no verlo
 * volver un instante a su día de antes). Un día completo sigue siendo de fecha; uno
 * con hora pasa a UTC, que `eventInstant` lee sin depender de su TZID.
 */
export function eventoDesplazado<T extends EventoDesplazable>(event: T, dias: number): T {
  const nuevo = desplazarDias(event, dias);
  if (!nuevo) return event;
  const formato = esDiaCompleto(event) ? fechaIcal : toIcalUtc;
  return {
    ...event,
    start: formato(nuevo.start),
    ...(event.end ? { end: formato(nuevo.end) } : {}),
  };
}
