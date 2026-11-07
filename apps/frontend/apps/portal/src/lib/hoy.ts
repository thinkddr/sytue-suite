/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tarjeta «Hoy» del Hub: los límites del día en la zona horaria de la
 * persona y el orden de la agenda (todo el día primero, luego por hora).
 */
export interface EventoDeHoy {
  id: string;
  summary: string;
  start: string;
  end?: string;
  allDay: boolean;
  location?: string;
}

/** Desde la medianoche local de `ahora` hasta la siguiente, como instantes ISO. */
export function limitesDelDia(ahora: Date): { from: string; to: string } {
  const inicio = new Date(ahora.getFullYear(), ahora.getMonth(), ahora.getDate());
  const fin = new Date(ahora.getFullYear(), ahora.getMonth(), ahora.getDate() + 1);
  return { from: inicio.toISOString(), to: fin.toISOString() };
}

/** Todo el día delante; el resto por hora de inicio. No muta la lista. */
export function ordenarEventos(eventos: readonly EventoDeHoy[]): EventoDeHoy[] {
  return [...eventos].sort((a, b) => {
    if (a.allDay !== b.allDay) return a.allDay ? -1 : 1;
    return a.start.localeCompare(b.start);
  });
}

/** Un evento ya terminado a `ahora` se apaga en la lista, pero no desaparece. */
export function haTerminado(e: EventoDeHoy, ahora: Date): boolean {
  if (e.allDay || !e.end) return false;
  const fin = new Date(e.end).getTime();
  return !Number.isNaN(fin) && fin < ahora.getTime();
}

/**
 * La ruta, dentro de Calendar, que abre ESTE evento: la vista de día en su fecha
 * (`?date=<inicio>`, y `all_day=1` si es de todo el día), que es el enlace de evento que
 * Calendar atiende al montarse (`fechaDelEnlace`). Se compone con el inicio de la
 * INSTANCIA y no con `/events/{id}`: ese resuelve el evento maestro, y una repetición de
 * una serie llevaría al día de la primera.
 *
 * Un día completo puede llegar como `20260909` (iCalendar); Calendar lo quiere con
 * guiones, así que se normaliza aquí.
 */
export function rutaDelEvento(e: EventoDeHoy): string {
  const compacto = /^(\d{4})(\d{2})(\d{2})$/.exec(e.start);
  const date = compacto ? `${compacto[1]}-${compacto[2]}-${compacto[3]}` : e.start;
  const qs = new URLSearchParams({ date });
  if (e.allDay) qs.set('all_day', '1');
  return `/?${qs}`;
}
