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
