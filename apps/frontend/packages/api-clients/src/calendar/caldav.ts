/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Calendar — SDK CalDAV (RFC 4791) contra el servicio Go propio
 * (calendar.sytue.eu, dev :8087). Implementacion clean-room sobre el estandar
 * CalDAV (NO Nextcloud). Descubre calendarios (PROPFIND) y consulta/crea/borra
 * eventos (REPORT/PUT/DELETE con cuerpos iCalendar VEVENT). Estado en
 * PostgreSQL. Autorizacion Bearer (JWT de Sytue ID).
 *
 * Base CalDAV del servicio Go: /dav/calendars/<user>/
 */
import { HttpClient, sameOriginUrl, type AuthProvider } from '../core/http.js';
import { firstText, parseXml } from '../core/xml.js';

export interface CalDavOptions {
  /** URL base, p.ej. https://calendar.sytue.eu */
  baseUrl: string;
  userId: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

export interface Calendar {
  /** URL relativa de la coleccion del calendario. */
  url: string;
  displayName: string;
  color?: string | undefined;
  ctag?: string | undefined;
}

export interface CalendarEvent {
  /** href del recurso .ics. */
  href: string;
  etag?: string | undefined;
  uid: string;
  summary: string;
  start: string;
  end?: string | undefined;
  location?: string | undefined;
  description?: string | undefined;
  /**
   * URL de la videollamada del evento (RFC 7986 CONFERENCE). Es lo que permite pintar
   * «Unirse» sin releer el ICS; ausente si el evento no tiene videollamada.
   */
  conferenceUrl?: string | undefined;
  /** iCalendar crudo (VCALENDAR/VEVENT). */
  ical: string;
}

const PROPFIND_CALENDARS = `<?xml version="1.0"?>
<d:propfind xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/" xmlns:ic="http://apple.com/ns/ical/">
  <d:prop>
    <d:resourcetype/>
    <d:displayname/>
    <cs:getctag/>
    <ic:calendar-color/>
  </d:prop>
</d:propfind>`;

function calendarQuery(start?: string, end?: string): string {
  const range = start ? `<c:time-range start="${start}"${end ? ` end="${end}"` : ''}/>` : '';
  return `<?xml version="1.0"?>
<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop><d:getetag/><c:calendar-data/></d:prop>
  <c:filter>
    <c:comp-filter name="VCALENDAR">
      <c:comp-filter name="VEVENT">${range}</c:comp-filter>
    </c:comp-filter>
  </c:filter>
</c:calendar-query>`;
}

export class CalDavClient {
  private readonly http: HttpClient;
  private readonly calRoot: string;

  constructor(options: CalDavOptions) {
    this.http = new HttpClient({
      baseUrl: options.baseUrl,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
    this.calRoot = `/dav/calendars/${encodeURIComponent(options.userId)}`;
  }

  /** Lista los calendarios del usuario. */
  async listCalendars(): Promise<Calendar[]> {
    const res = await this.http.request({
      method: 'PROPFIND',
      path: `${this.calRoot}/`,
      headers: { depth: '1', 'content-type': 'application/xml' },
      body: PROPFIND_CALENDARS,
    });
    const doc = parseXml(await res.text());
    return Array.from(doc.getElementsByTagNameNS('DAV:', 'response'))
      .map((node): Calendar | null => {
        const url = firstText(node, 'href');
        const isCalendar =
          node.getElementsByTagNameNS('urn:ietf:params:xml:ns:caldav', 'calendar').length > 0;
        if (!url || !isCalendar) return null;
        return {
          url: decodeURIComponent(url),
          displayName: firstText(node, 'displayname') ?? 'Calendario',
          color: firstText(node, 'calendar-color'),
          ctag: firstText(node, 'getctag'),
        } satisfies Calendar;
      })
      .filter((c): c is Calendar => c !== null);
  }

  /** Consulta eventos de un calendario en un rango (calendar-query REPORT). */
  async listEvents(
    calendarUrl: string,
    range?: { start: string; end?: string },
  ): Promise<CalendarEvent[]> {
    const res = await this.http.request({
      method: 'REPORT',
      url: this.absolute(calendarUrl),
      headers: { depth: '1', 'content-type': 'application/xml' },
      body: calendarQuery(range?.start, range?.end),
    });
    const doc = parseXml(await res.text());
    return Array.from(doc.getElementsByTagNameNS('DAV:', 'response'))
      .map((node): CalendarEvent | null => {
        const href = firstText(node, 'href');
        const ical = firstText(node, 'calendar-data');
        if (!href || !ical) return null;
        return {
          href: decodeURIComponent(href),
          etag: firstText(node, 'getetag'),
          ...parseVevent(ical),
          ical,
        } satisfies CalendarEvent;
      })
      .filter((e): e is CalendarEvent => e !== null);
  }

  /** Crea o actualiza un evento (PUT del .ics). */
  async putEvent(eventUrl: string, ical: string, etag?: string): Promise<void> {
    await this.http.request({
      method: 'PUT',
      url: this.absolute(eventUrl),
      headers: {
        'content-type': 'text/calendar; charset=utf-8',
        ...(etag ? { 'if-match': etag } : { 'if-none-match': '*' }),
      },
      body: ical,
    });
  }

  /** Elimina un evento. */
  async deleteEvent(eventUrl: string): Promise<void> {
    await this.http.request({ method: 'DELETE', url: this.absolute(eventUrl) });
  }

  // El recurso lo elige quien llama (y, vía el BFF, la consulta del navegador):
  // se exige que quede en el servicio configurado para no mandar el Bearer fuera.
  private absolute(url: string): string {
    return sameOriginUrl(this.http.baseUrl, url);
  }
}

/** Extrae los campos basicos de un VEVENT (parser ligero, no exhaustivo). */
export function parseVevent(
  ical: string,
): Pick<
  CalendarEvent,
  'uid' | 'summary' | 'start' | 'end' | 'location' | 'description' | 'conferenceUrl'
> {
  const get = (key: string): string | undefined => {
    const match = ical.match(new RegExp(`^${key}[^:]*:(.*)$`, 'm'));
    return match?.[1]?.trim();
  };
  return {
    uid: get('UID') ?? '',
    summary: get('SUMMARY') ?? '(Sin titulo)',
    start: get('DTSTART') ?? '',
    end: get('DTEND'),
    location: get('LOCATION'),
    description: get('DESCRIPTION'),
    // CONFERENCE viaja con parámetros (VALUE=URI;FEATURE=VIDEO;LABEL=…), y `get` ya
    // ignora todo lo anterior al ':' — que es justo lo que hace falta aquí.
    conferenceUrl: enlaceDeReunion(get('CONFERENCE')),
  };
}

/**
 * El CONFERENCE de un VEVENT, solo si es un enlace web.
 *
 * Acaba como `href` del botón «Unirse», y el ICS lo escribe quien sea: el dueño de
 * un calendario compartido, un feed suscrito, cualquier cliente CalDAV. El servicio
 * sanea su columna derivada, pero la web lee el ICS crudo, así que la verja va aquí,
 * donde el valor entra. Es la misma regla con la que se ESCRIBE (`buildIcs`, en la
 * app de Calendar): http o https, sin espacios.
 */
function enlaceDeReunion(valor: string | undefined): string | undefined {
  return valor !== undefined && /^https?:\/\/\S+$/i.test(valor) ? valor : undefined;
}
