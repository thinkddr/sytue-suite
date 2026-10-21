// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import type { Event, EventInput } from '@sytue/api-clients';
import { desplazarDia } from './diario.js';

/**
 * ¿Cae este evento en `date`? Los de día completo son flotantes (00:00 UTC) y se
 * comparan por fecha; los demás ya vienen de la ventana local del día.
 */
export function eventoDelDia(e: Pick<Event, 'start' | 'allDay'>, date: string): boolean {
  return !e.allDay || e.start.slice(0, 10) === date;
}

/** El evento de día completo que nace de una nota: `date` a `date + 1`, en UTC. */
export function eventoDeDiaCompleto(
  date: string,
  summary: string,
): Omit<EventInput, 'calendarId'> | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || !summary) return null;
  return {
    summary,
    start: `${date}T00:00:00Z`,
    end: `${desplazarDia(date, 1)}T00:00:00Z`,
    allDay: true,
  };
}

/** La ventana local `[00:00, 24:00)` de un día civil, en RFC 3339 con zona. */
export function ventanaLocal(date: string): { from: string; to: string } {
  return {
    from: new Date(`${date}T00:00:00`).toISOString(),
    to: new Date(`${desplazarDia(date, 1)}T00:00:00`).toISOString(),
  };
}
