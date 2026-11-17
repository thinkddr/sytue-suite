/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Deshacer» tras mover, redimensionar o borrar un evento.
 *
 * Un arrastre accidental en la rejilla horaria o un «Eliminar» de más no tenían
 * vuelta atrás: había que recordar dónde estaba el bloque o volver a escribir el
 * evento entero. Google Calendar y Notepads ofrecen deshacer en el mismo aviso
 * de éxito; aquí se decide QUÉ se puede deshacer y CÓMO se vuelve al estado
 * anterior, para que la vista solo tenga que pintar el botón.
 */
import { parseIcalDate, toDatetimeLocal } from '../components/dates.js';
import {
  guestPartstats,
  parseAlarms,
  parseAttendees,
  parseColor,
  tieneRepeticion,
} from './ical.js';
import type { EventInput } from '../client/calendarApi.js';
import { icsCompleto } from './icsDescarga.js';

/** Lo que hace falta de un evento de la rejilla para moverlo o restaurarlo. */
export interface EventoDeshacible {
  uid?: string | undefined;
  summary: string;
  start: string;
  end?: string | undefined;
  location?: string | undefined;
  description?: string | undefined;
  conferenceUrl?: string | undefined;
  ical?: string | undefined;
  href?: string | undefined;
  calendarUrl?: string | undefined;
  readOnly?: boolean | undefined;
}

/**
 * El evento con otro inicio/fin, listo para el mismo `saveEvent` del modal. Lo usan
 * el arrastre y su «Deshacer»: si cada uno armara el suyo, deshacer podría perder
 * lo que el arrastre conservó (invitados con sus respuestas, VALARM, color…). El
 * BFF funde además sobre lo guardado, y `recurrenceUnchanged` impide que un
 * movimiento toque la repetición.
 */
export function entradaConHorario(event: EventoDeshacible, start: Date, end: Date): EventInput {
  const attendees = event.ical ? parseAttendees(event.ical) : [];
  const alarms = event.ical ? parseAlarms(event.ical) : [];
  const color = event.ical ? parseColor(event.ical) : undefined;
  return {
    summary: event.summary,
    start: toDatetimeLocal(start),
    end: toDatetimeLocal(end),
    // Un día completo arrastrado en el mes sigue siendo de día completo: sin esto se
    // guardaba como «00:00–00:00» con hora. El fin sigue siendo exclusivo.
    ...(/^\d{8}$/.test(event.start) ? { allDay: true } : {}),
    ...(event.uid ? { uid: event.uid } : {}),
    ...(event.location ? { location: event.location } : {}),
    ...(event.description ? { description: event.description } : {}),
    ...(color ? { color } : {}),
    ...(event.conferenceUrl ? { conferenceUrl: event.conferenceUrl } : {}),
    ...(attendees.length ? { attendees, partstats: guestPartstats(event.ical) } : {}),
    ...(alarms.length ? { alarms } : {}),
    recurrenceUnchanged: true,
  };
}

/**
 * Dónde estaba el bloque antes de arrastrarlo, o null si no se puede devolver con
 * fidelidad. Un día completo (arrastrado en el mes) vuelve como día completo porque
 * `entradaConHorario` conserva su `allDay`. Sin DTEND, la duración es la que tiene
 * ahora el bloque (lo único que se sabe).
 */
export function horarioAnterior(
  event: EventoDeshacible,
  nuevoInicio: Date,
  nuevoFin: Date,
): { start: Date; end: Date } | null {
  const start = parseIcalDate(event.start);
  if (!start) return null;
  const end =
    (event.end ? parseIcalDate(event.end) : null) ??
    new Date(start.getTime() + (nuevoFin.getTime() - nuevoInicio.getTime()));
  return { start, end };
}

/**
 * ¿Se ofrece deshacer el borrado? Solo de un evento propio, suelto y SIN invitados:
 * recrearlo es volver a escribir su .ics tal cual. Con invitados, el borrado ya
 * mandó un CANCEL a cada uno y recrearlo les mandaría otra invitación —dos correos
 * a terceros por un clic—; una serie se borra por alcance (esta / siguientes /
 * todas) y su vuelta atrás no es un único .ics. Ahí no se ofrece.
 */
export function puedeDeshacerBorrado(event: EventoDeshacible): boolean {
  if (event.readOnly || !event.href || !event.calendarUrl || !event.ical) return false;
  if (tieneRepeticion(event.ical)) return false;
  return !tieneAgenda(event.ical);
}

/** Tamaño máximo de un .ics restaurado: un evento con descripción larga cabe de sobra. */
export const MAX_ICS_RESTAURADO = 256 * 1024;

/**
 * El .ics que el BFF acepta volver a escribir al deshacer un borrado, o null si no
 * vale. Es lo único que el BFF escribe sin generarlo él, así que se acota a lo que
 * deshacer necesita: un VCALENDAR con un VEVENT, sin ORGANIZER/ATTENDEE (un PUT
 * con invitados haría que el servicio mandase invitaciones en nombre de quien
 * figure como organizador) y sin METHOD (un recurso CalDAV no lo lleva, RFC 4791
 * §4.1). Si llega solo el VEVENT —hay servidores que lo entregan así—, se envuelve.
 */
export function icsRestaurable(ical: unknown): string | null {
  if (typeof ical !== 'string') return null;
  const texto = ical.trim();
  if (!texto || texto.length > MAX_ICS_RESTAURADO) return null;
  const completo = icsCompleto(texto);
  if (!/^BEGIN:VCALENDAR\r?\n[\s\S]*\r?\nEND:VCALENDAR$/i.test(completo)) return null;
  if (!/^BEGIN:VEVENT\r?$/im.test(completo) || !/^END:VEVENT\r?$/im.test(completo)) return null;
  if (tieneAgenda(completo) || /^METHOD[;:]/im.test(completo)) return null;
  return completo.endsWith('\r\n') ? completo : `${completo}\r\n`;
}

/** ORGANIZER o ATTENDEE en alguna línea (RFC 5545: la propiedad abre la línea). */
function tieneAgenda(ical: string): boolean {
  return /^(?:ORGANIZER|ATTENDEE)[;:]/im.test(ical);
}
