/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA INVITACIÓN DE UN CORREO, LEÍDA PARA PINTARLA (D-MAIL-04).
 *
 * Quien organiza en Google, Outlook o Apple manda su REQUEST iMIP (RFC 6047) como
 * parte `text/calendar` del correo. El lector lo resume en una tarjeta —qué, cuándo,
 * dónde y quién convoca— con Sí/Quizá/No. Solo se pinta: lo que se guarda en
 * Calendar lo vuelve a leer el BFF del propio mensaje (`/api/invitacion`), nunca de
 * lo que mande el navegador.
 *
 * Los campos de texto salen de `parseVevent` (el SDK de Calendar); aquí solo se aísla
 * el VEVENT —sin él, el DTSTART del VTIMEZONE pasaba por el del evento—, se desdoblan
 * las líneas y se interpretan las fechas.
 */
import { parseVevent } from '@sytue/api-clients';

export interface ResumenInvitacion {
  uid: string;
  summary: string;
  start: Date | null;
  end: Date | null;
  /** VALUE=DATE: evento de día entero (sin hora que mostrar). */
  allDay: boolean;
  location?: string;
  /** Dirección de ORGANIZER sin «mailto:». */
  organizer: string;
  /** Su CN, si lo trae. */
  organizerName?: string;
}

/** Respuestas que ofrece la tarjeta, en el orden en que se pintan. */
export const RESPUESTAS = ['ACCEPTED', 'TENTATIVE', 'DECLINED'] as const;
export type Respuesta = (typeof RESPUESTAS)[number];

export function esRespuesta(v: unknown): v is Respuesta {
  return typeof v === 'string' && (RESPUESTAS as readonly string[]).includes(v);
}

/** Desdoblado RFC 5545 §3.1: una línea que empieza por espacio o tab continúa la previa. */
function desdoblar(ics: string): string {
  return ics.replace(/\r?\n[ \t]/g, '').replace(/\r\n/g, '\n');
}

/** Texto RFC 5545 §3.3.11: `\,` `\;` `\\` `\n`. */
function texto(v: string | undefined): string | undefined {
  return v?.replace(/\\([,;\\nN])/g, (_, c: string) => (c === 'n' || c === 'N' ? '\n' : c));
}

/**
 * Fecha iCalendar → Date. `…Z` es UTC; sin Z (con o sin TZID) se toma como hora local,
 * y `AAAAMMDD` es un día entero.
 *
 * ponytail: un TZID distinto de la zona de quien lee se pinta en la hora local sin
 * convertir. Lo exacto lo guarda Calendar con el ICS entero; si importara aquí, se
 * resolvería el VTIMEZONE con `Intl`.
 */
function fecha(v: string | undefined): { d: Date | null; dia: boolean } {
  const m = /^(\d{4})(\d{2})(\d{2})(?:T(\d{2})(\d{2})(\d{2})(Z)?)?$/.exec(v?.trim() ?? '');
  if (!m) return { d: null, dia: false };
  const [, a, mes, dia, h, min, s, z] = m;
  const n = (x: string | undefined): number => Number(x ?? 0);
  if (h === undefined) return { d: new Date(n(a), n(mes) - 1, n(dia)), dia: true };
  const d = z
    ? new Date(Date.UTC(n(a), n(mes) - 1, n(dia), n(h), n(min), n(s)))
    : new Date(n(a), n(mes) - 1, n(dia), n(h), n(min), n(s));
  return { d, dia: false };
}

/**
 * Resume el iCalendar de un correo si es una invitación que se puede contestar: un
 * REQUEST con evento y organizador. Cualquier otra cosa (CANCEL, REPLY, PUBLISH, un
 * .ics suelto) → null, y el lector no pinta tarjeta.
 */
export function leerInvitacion(ics: string | undefined): ResumenInvitacion | null {
  if (!ics) return null;
  const plano = desdoblar(ics);
  if (!/^METHOD:\s*REQUEST\s*$/im.test(plano)) return null;
  const vevent = /^BEGIN:VEVENT$([\s\S]*?)^END:VEVENT$/m.exec(plano)?.[1];
  if (!vevent) return null;
  // Las alarmas van dentro del VEVENT y traen su propio DESCRIPTION/SUMMARY.
  const sinAlarmas = vevent.replace(/^BEGIN:VALARM$[\s\S]*?^END:VALARM$/gm, '');
  const ev = parseVevent(sinAlarmas);
  const org = /^ORGANIZER([^:]*):(.*)$/m.exec(sinAlarmas);
  const organizer = (org?.[2] ?? '').trim().replace(/^mailto:/i, '');
  if (!ev.uid || !organizer) return null;
  const cn = /;CN=(?:"([^"]*)"|([^;:]*))/i.exec(org?.[1] ?? '');
  const organizerName = (cn?.[1] ?? cn?.[2] ?? '').trim();
  const ini = fecha(ev.start);
  const fin = fecha(ev.end);
  const location = texto(ev.location)?.trim();
  return {
    uid: ev.uid,
    // Sin SUMMARY el SDK pone su propio relleno, sin traducir: aquí se deja vacío y
    // la tarjeta pone el suyo.
    summary: /^SUMMARY[;:]/m.test(sinAlarmas) ? (texto(ev.summary) ?? '') : '',
    start: ini.d,
    end: fin.d,
    allDay: ini.dia,
    ...(location ? { location } : {}),
    organizer,
    ...(organizerName ? { organizerName } : {}),
  };
}
