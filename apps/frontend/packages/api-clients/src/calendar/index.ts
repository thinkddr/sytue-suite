/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Calendar — entrada del SDK al servicio Go propio (calendar.sytue.eu,
 * dev :8087). Expone la API REST JSON (comoda para la UI) y, via re-export, el
 * cliente CalDAV (RFC 4791) para sincronizacion estandar. Estado en PostgreSQL.
 * Autorizacion Bearer (JWT de Sytue ID).
 *
 * REST base:    /api/v1
 * CalDAV base:  /dav/calendars/<user>/  (ver CalDavClient)
 */
import { HttpClient, type AuthProvider } from '../core/http.js';

export interface CalendarOptions {
  /** URL base, p.ej. https://calendar.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

export interface CalendarInfo {
  id: string;
  displayName: string;
  color?: string;
  /** Calendario de solo lectura (p.ej. suscripciones). */
  readOnly?: boolean;
  /** El calendario por defecto de su dueño: donde se crea lo que no dice dónde. */
  isDefault?: boolean;
}

export interface EventInput {
  calendarId: string;
  summary: string;
  /**
   * RFC 3339 CON zona (`2026-09-09T10:00:00+02:00`).
   *
   * El servicio decodifica esto a `time.Time`, que rechaza una fecha sin zona:
   * mandarla devuelve un 400 que no dice cuál de los campos estaba mal. Se
   * comprueba aquí, donde sí se puede señalar el campo.
   */
  start: string;
  end?: string;
  allDay?: boolean;
  location?: string;
  description?: string;
  /** Regla de recurrencia RFC 5545 (RRULE) sin el prefijo. */
  rrule?: string;
  attendees?: string[];
}

export interface Event extends EventInput {
  id: string;
  uid: string;
  etag?: string;
  createdAt: string;
  updatedAt: string;
}

/** Lo que devuelve el servicio, tal cual: iCalendar en `snake_case`. */
interface CalendarioCrudo {
  id: string;
  name?: string;
  display_name?: string;
  color?: string;
  read_only?: boolean;
  is_default?: boolean;
}

interface EventoCrudo {
  id: string;
  uid: string;
  calendar_id: string;
  summary: string;
  description?: string;
  location?: string;
  dtstart: string;
  dtend?: string;
  all_day?: boolean;
  rrule?: string;
  attendees?: { email?: string }[];
  etag?: string;
  created_at?: string;
  updated_at?: string;
}

interface InstanciaCruda {
  event: EventoCrudo;
  start: string;
  end?: string;
}

/**
 * El evento tal y como lo devuelve el servicio → el del SDK.
 *
 * Un evento del servicio habla iCalendar en `snake_case` (`dtstart`,
 * `all_day`, `calendar_id`). Sin esta traducción, un consumidor que lea
 * `evento.start` recibe `undefined` y pinta una fecha vacía sin que falle
 * nada: el fallo aparece en la pantalla, no en la consola.
 */
function eventoAEvento(e: EventoCrudo): Event {
  return {
    id: e.id,
    uid: e.uid,
    calendarId: e.calendar_id,
    summary: e.summary,
    start: e.dtstart,
    ...(e.dtend ? { end: e.dtend } : {}),
    ...(e.all_day ? { allDay: true } : {}),
    ...(e.location ? { location: e.location } : {}),
    ...(e.description ? { description: e.description } : {}),
    ...(e.rrule ? { rrule: e.rrule } : {}),
    ...(e.attendees?.length
      ? { attendees: e.attendees.map((a) => a.email ?? '').filter(Boolean) }
      : {}),
    ...(e.etag ? { etag: e.etag } : {}),
    createdAt: e.created_at ?? '',
    updatedAt: e.updated_at ?? '',
  };
}

const CON_ZONA = /(?:Z|[+-]\d{2}:?\d{2})$/;

/**
 * El evento del SDK → el cuerpo que el servicio espera.
 *
 * `calendarId` NO va aquí: el calendario lo fija la ruta. Y los asistentes son
 * objetos con `email`, no cadenas sueltas, que es lo que la lista de este lado
 * parece decir.
 */
function aCuerpo(input: EventInput): Record<string, unknown> {
  for (const [campo, valor] of [
    ['start', input.start],
    ['end', input.end],
  ] as const) {
    if (valor && !CON_ZONA.test(valor)) {
      throw new TypeError(`El campo «${campo}» necesita zona horaria (RFC 3339): ${valor}`);
    }
  }
  return {
    summary: input.summary,
    start: input.start,
    ...(input.end ? { end: input.end } : {}),
    ...(input.allDay ? { all_day: true } : {}),
    ...(input.location ? { location: input.location } : {}),
    ...(input.description ? { description: input.description } : {}),
    ...(input.rrule ? { rrule: input.rrule } : {}),
    ...(input.attendees?.length ? { attendees: input.attendees.map((email) => ({ email })) } : {}),
  };
}

function instanciaAEvento(i: InstanciaCruda): Event {
  // La instancia manda sobre el maestro en las fechas, y solo en ellas: una
  // repetición cae en SU día, no en el del primer evento de la serie.
  return {
    ...eventoAEvento(i.event),
    start: i.start || i.event.dtstart,
    ...((i.end ?? i.event.dtend) ? { end: i.end ?? i.event.dtend } : {}),
  };
}

export class CalendarClient {
  private readonly http: HttpClient;

  constructor(options: CalendarOptions) {
    this.http = new HttpClient({
      baseUrl: `${options.baseUrl.replace(/\/$/, '')}/api/v1`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  /**
   * Calendarios del usuario, el de por defecto primero (el servicio lo crea en el
   * primer acceso). Leía la respuesta como una lista cuando el servicio contesta
   * `{ calendars }` en `snake_case`: devolvía un objeto y nadie la usaba.
   */
  async listCalendars(): Promise<CalendarInfo[]> {
    const { calendars } = await this.http.json<{ calendars: CalendarioCrudo[] }>({
      path: '/calendars',
    });
    return (calendars ?? []).map((c) => ({
      id: c.id,
      displayName: c.display_name || c.name || '',
      ...(c.color ? { color: c.color } : {}),
      ...(c.read_only ? { readOnly: true } : {}),
      ...(c.is_default ? { isDefault: true } : {}),
    }));
  }

  /** Eventos de uno o varios calendarios en un rango (ISO 8601). */
  /**
   * Eventos en un rango, ya expandidas las recurrencias.
   *
   * Pedía `GET /events?from&to`, que NO EXISTE en el servicio: sus rutas son
   * `/calendars` y `/calendars/{id}/instances?start&end`, y los parámetros se
   * llaman `start` y `end`. Así que este método devolvía 404 siempre y quien lo
   * usaba se quedaba sin agenda sin un solo error a la vista (la tarjeta «Hoy»
   * del Hub decía «no está disponible», medido en producción el 2026-09-09).
   *
   * Son N+1 peticiones —una de calendarios y una por calendario—, que es lo que
   * el servicio ofrece; van en paralelo y un calendario que falle no tumba al
   * resto: se devuelve lo que sí contestó.
   *
   * El servicio habla en `snake_case` y con los nombres de iCalendar
   * (`dtstart`, `all_day`); aquí se traducen a los del SDK una sola vez, que es
   * justo lo que un cliente tipado viene a hacer.
   */
  async listEvents(params: { calendarIds?: string[]; from: string; to: string }): Promise<Event[]> {
    const { calendars } = await this.http.json<{ calendars: CalendarioCrudo[] }>({
      path: '/calendars',
    });
    const elegidos = params.calendarIds?.length
      ? calendars.filter((c) => params.calendarIds?.includes(c.id))
      : calendars;
    const query = { start: params.from, end: params.to };
    const tandas = await Promise.allSettled(
      elegidos.map((c) =>
        this.http.json<{ instances: InstanciaCruda[] }>({
          path: `/calendars/${encodeURIComponent(c.id)}/instances`,
          query,
        }),
      ),
    );
    return (
      tandas
        .flatMap((r) => (r.status === 'fulfilled' ? r.value.instances : []))
        .map((i) => instanciaAEvento(i))
        // La instancia manda sobre el maestro: una repetición cae en SU día y no
        // en el del primer evento de la serie.
        .sort((a, b) => a.start.localeCompare(b.start))
    );
  }

  /**
   * Busca eventos por texto (el servicio resuelve el índice; nunca se trae el
   * calendario entero al cliente). Lo usa el selector «vincular un evento» de
   * Sytue Notepads, que es el otro extremo de la referencia bidireccional.
   */
  async searchEvents(q: string, limit = 20): Promise<Event[]> {
    const r = await this.http.json<{ events?: EventoCrudo[] }>({
      path: '/events/search',
      query: { q, limit },
    });
    return (r.events ?? []).map(eventoAEvento);
  }

  /**
   * Crea un evento en el calendario que dice `input.calendarId`.
   *
   * Pedía `POST /events`, que NO EXISTE: el servicio crea SIEMPRE dentro de un
   * calendario (`POST /calendars/{id}/events`), porque un evento sin
   * calendario no tiene dueño ni permisos que aplicar. Devolvía 404 desde el
   * primer día; hoy no lo llama nadie, y por eso no se había notado.
   */
  async createEvent(input: EventInput): Promise<Event> {
    const crudo = await this.http.postJson<EventoCrudo>(
      `/calendars/${encodeURIComponent(input.calendarId)}/events`,
      aCuerpo(input),
    );
    return eventoAEvento(crudo);
  }

  /**
   * Cambia lo que se le diga de un evento y deja el resto como estaba.
   *
   * Dos cosas que este método hacía mal y se notan solo en producción:
   * mandaba PATCH a una ruta que solo acepta PUT (405), y mandaba un parche
   * suelto a un endpoint que REEMPLAZA el evento entero con lo que reciba, así
   * que cambiar el título habría borrado hora, lugar y asistentes.
   *
   * Por eso se lee antes de escribir: el parche se funde con el evento actual
   * y se envía completo, que es lo que el servicio espera.
   */
  async updateEvent(id: string, patch: Partial<EventInput>): Promise<Event> {
    const ruta = `/events/${encodeURIComponent(id)}`;
    const actual = eventoAEvento(await this.http.json<EventoCrudo>({ path: ruta }));
    const crudo = await this.http.json<EventoCrudo>({
      method: 'PUT',
      path: ruta,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(aCuerpo({ ...actual, ...patch })),
    });
    return eventoAEvento(crudo);
  }

  /** Elimina un evento. */
  async deleteEvent(id: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/events/${encodeURIComponent(id)}` });
  }
}

export { CalDavClient, parseVevent } from './caldav.js';
export type { CalDavOptions, Calendar, CalendarEvent } from './caldav.js';
