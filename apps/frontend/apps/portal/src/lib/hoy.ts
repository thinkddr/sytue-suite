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
 * Dónde está un evento respecto a `ahora`: ya terminado (`pasado`), en curso
 * (`ahora`) o por empezar (`siguiente`). Uno de todo el día no está «en curso» ni
 * «por venir»: dura el día entero y marcarlo no dice nada, así que es `null`. Sin
 * fin, un evento empezado tampoco es nada (no se sabe si sigue; `haTerminado`
 * tampoco lo da por pasado).
 */
export type EstadoDelEvento = 'pasado' | 'ahora' | 'siguiente';

export function estadoDelEvento(e: EventoDeHoy, ahora: Date): EstadoDelEvento | null {
  if (e.allDay) return null;
  const inicio = new Date(e.start).getTime();
  if (Number.isNaN(inicio)) return null;
  const t = ahora.getTime();
  if (inicio > t) return 'siguiente';
  if (haTerminado(e, ahora)) return 'pasado';
  return e.end ? 'ahora' : null;
}

/**
 * Lo que la tarjeta marca de cada evento de la lista (en su orden): «Ahora» en los
 * que están en curso y «Siguiente» SOLO en el primero que falta por empezar. Los
 * demás por venir, igual que los pasados y los de todo el día, sin marca.
 */
export function marcasDeLaAgenda(
  eventos: readonly EventoDeHoy[],
  ahora: Date,
): ('ahora' | 'siguiente' | null)[] {
  let haySiguiente = false;
  return eventos.map((e) => {
    const estado = estadoDelEvento(e, ahora);
    if (estado === 'ahora') return 'ahora';
    if (estado !== 'siguiente' || haySiguiente) return null;
    haySiguiente = true;
    return 'siguiente';
  });
}

/** Cuántos eventos del día quedan fuera de la tarjeta (el «+N más en Calendar»). */
export function eventosDeMas(total: number | undefined, mostrados: number): number {
  return Math.max((total ?? mostrados) - mostrados, 0);
}

/** Los eventos que caben en la tarjeta «Hoy». Es el `api.MaxEventosHoy` de las nativas. */
export const MAX_EVENTOS = 6;

/**
 * Lo que el BFF devuelve de la agenda: los primeros `MAX_EVENTOS` por hora de inicio,
 * cuántos había en total (para el «+N más») y si falta algún calendario (`parcial`).
 * Si no contestó NINGUNO de los calendarios no hay agenda que enseñar: `null`, que
 * la tarjeta pinta como «no disponible» y no como un día vacío.
 */
export function recortarAgenda(
  eventos: readonly EventoDeHoy[],
  fallidos: number,
  calendarios: number,
): { eventos: EventoDeHoy[]; total: number; parcial: boolean } | null {
  if (calendarios > 0 && fallidos >= calendarios) return null;
  const ordenados = [...eventos].sort((a, b) => a.start.localeCompare(b.start));
  return { eventos: ordenados.slice(0, MAX_EVENTOS), total: eventos.length, parcial: fallidos > 0 };
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
