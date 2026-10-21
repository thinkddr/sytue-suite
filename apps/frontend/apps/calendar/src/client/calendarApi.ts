/**
 * Cliente de navegador de Sytue Calendar: habla con los endpoints BFF same-origin
 * (cookie). El servidor reenvia al servicio Go Sytue Calendar (CalDAV) con el
 * access token Bearer. Asi el access token no toca el navegador para los datos de
 * calendario. Reexporta los tipos del SDK @sytue/api-clients para coherencia.
 */
import { HttpError, type Calendar as CalDavCalendar, type CalendarEvent } from '@sytue/api-clients';
import type { Recurrence } from '../lib/ical.js';
/*
 * `withBase` en la vuelta al login: la app se sirve bajo su prefijo en el dominio
 * de organización (`<org>.sytue.eu/mail`, `/calendar`…), y ahí `/auth/login` a pelo
 * es la raíz del host, que devuelve el 404 de Traefik. O sea: al caducar la sesión
 * se acababa en una página de error en vez de en el formulario. El shim de `fetch`
 * (packages/ui/src/basePath.ts) no llega hasta aquí porque esto no es un fetch.
 */
import { withBase } from '@sytue/ui';

export type { CalendarEvent };
export type { Recurrence };

/** Estado de una suscripción ICS externa (calendario de solo lectura). */
export interface CalendarSubscription {
  url: string;
  lastStatus?: string | undefined;
  lastError?: string | undefined;
  lastFetchedAt?: string | undefined;
}

/** Nivel de acceso de una compartición de calendario. `manage` es la DELEGACIÓN de
 * agenda: el asistente gestiona (ve + crea/edita/borra) la agenda del dueño. */
export type SharePermission = 'reader' | 'writer' | 'manage';

/** Calendario del BFF: la coleccion CalDAV + su identidad REST (gestion). */
export interface Calendar extends CalDavCalendar {
  /** ID REST del servicio Go (renombrar/color/borrar). */
  id?: string | undefined;
  /** El calendario por defecto no se puede borrar. */
  isDefault?: boolean | undefined;
  /** Solo lectura: una suscripción ICS externa, o un compartido de solo lectura
   * (reader). En ambos casos la rejilla bloquea arrastrar/editar. */
  readOnly?: boolean | undefined;
  /** Estado de la suscripción ICS (presente si readOnly por suscripción). */
  subscription?: CalendarSubscription | undefined;
  /** Verdadero si es un calendario compartido CON el usuario («Compartido conmigo»). */
  shared?: boolean | undefined;
  /** Permiso concedido si `shared` (reader → readOnly; writer → editable). */
  permission?: SharePermission | undefined;
  /** Sub del dueño que compartió, si `shared`. */
  sharedBy?: string | undefined;
  /** Siendo lector, ya ha pedido editarlo (`FranjaPeticion`). */
  editRequested?: boolean | undefined;
}

/** Una compartición de un calendario propio (para «con quién está compartido»). */
export interface CalendarShare {
  id: string;
  grantee_email: string;
  permission: SharePermission;
  created_at: string;
}

export interface EventInput {
  uid?: string;
  summary: string;
  start: string;
  end?: string;
  allDay?: boolean;
  /** Zona horaria IANA del evento (hora-de-pared del start/end). */
  tzid?: string;
  location?: string;
  description?: string;
  recurrence?: Recurrence;
  /** Color del evento (hex #RRGGBB de la paleta). Vacio => color del calendario. */
  color?: string;
  /** URL de la videollamada (RFC 7986 CONFERENCE). Vacia => el evento no la tiene. */
  conferenceUrl?: string;
  /** Emails de los invitados (ATTENDEE). */
  attendees?: string[];
  /** Bloques VALARM crudos: recordatorios editados + passthrough del externo. */
  alarms?: string[];
  /** EXDATE (tokens iCal) a arrastrar a una serie nueva al desgajar. */
  exdates?: string[];
}

/** Invitacion de la bandeja de planificacion (iTIP), enriquecida por el BFF. */
export interface Invitation {
  id: string;
  uid: string;
  method: string;
  organizer?: string;
  recipient: string;
  status: 'pending' | 'accepted' | 'declined' | 'tentative' | 'delivered' | 'cancelled';
  created_at: string;
  responded_at?: string;
  summary?: string;
  start?: string;
  /** La organización que convoca, solo si no es la de la sesión (COM-15). */
  organization?: string;
  /** Su nombre visible, si Sytue ID lo da; si no, se enseña `organization`. */
  organization_name?: string;
}

export type InvitationResponse = 'accepted' | 'declined' | 'tentative';

/** Recordatorio vencido (VALARM) para la entrega in-app (centro de avisos). */
export interface Reminder {
  id: string;
  event_id: string;
  uid: string;
  summary: string;
  /** RFC3339: inicio del evento (deep-link a su día). */
  dtstart: string;
  trigger_at: string;
  action: string;
}

/** Alcance de una edición/borrado de una instancia de serie recurrente. */
export type RecurrenceScope = 'this' | 'following' | 'all';

/**
 * Resultado de la búsqueda server-side de eventos (proyección REST de
 * domain.Event del servicio Go). Es el MAESTRO de cada serie: las recurrentes
 * llevan `rrule`/`rdate` (aparecen una vez, no una por ocurrencia). Fechas en
 * RFC3339 (instante UTC); la UI las muestra en la zona de visualización.
 */
export interface EventSearchResult {
  id: string;
  calendar_id: string;
  uid: string;
  summary: string;
  description?: string;
  location?: string;
  dtstart: string;
  dtend: string;
  all_day: boolean;
  rrule?: string;
  rdate?: string[];
}

/** Parámetros de una búsqueda de eventos (todos salvo `q` opcionales). */
export interface EventSearchParams {
  q: string;
  from?: string | undefined;
  to?: string | undefined;
  calendar?: string | undefined;
  cursor?: string | undefined;
  limit?: number | undefined;
}

/**
 * Busca eventos del usuario por texto (título/descripción/lugar/asistentes) con
 * rango y calendario opcionales, paginado por keyset (`next_cursor`). GET de solo
 * lectura (sin CSRF); el aislamiento por dueño lo aplica el servicio Go.
 */
export function searchEvents(
  params: EventSearchParams,
): Promise<{ events: EventSearchResult[]; next_cursor: string }> {
  const q = new URLSearchParams({ q: params.q });
  if (params.from) q.set('from', params.from);
  if (params.to) q.set('to', params.to);
  if (params.calendar) q.set('calendar', params.calendar);
  if (params.cursor) q.set('cursor', params.cursor);
  if (params.limit) q.set('limit', String(params.limit));
  return getJson(`/api/search?${q.toString()}`);
}

async function getJson<T>(url: string): Promise<T> {
  const res = await fetch(url, { credentials: 'include', headers: { accept: 'application/json' } });
  if (res.status === 401) {
    window.location.assign(
      withBase(
        import.meta.env.BASE_URL,
        `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
      ),
    );
    throw new Error('No autenticado');
  }
  // Con su estado: un 403/404 («ya no tienes acceso») no se trata como una caída.
  if (!res.ok) throw new HttpError(res.status, res.statusText, url);
  return (await res.json()) as T;
}

async function sendJson<T>(method: string, url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`Error ${res.status}`);
  return (await res.json()) as T;
}

export function listCalendars(): Promise<{ calendars: Calendar[] }> {
  return getJson('/api/calendars');
}

/** Crea un calendario (color de la paleta --sytue-*, hex). */
export function createCalendar(input: {
  displayName: string;
  color: string;
}): Promise<{ calendar: Calendar }> {
  return sendJson('POST', '/api/calendars', input);
}

/** Renombra y/o recolorea un calendario. */
export function updateCalendar(
  id: string,
  patch: { displayName?: string; color?: string },
): Promise<{ calendar: Calendar }> {
  return sendJson('POST', '/api/calendars', { id, ...patch });
}

/** Borra un calendario (el por defecto es inmutable: 403 del servicio Go). */
export function deleteCalendar(id: string): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/calendars', { id, delete: true });
}

/**
 * Suscribe un calendario ICS externo por URL (festivos, equipo, …). Se crea como
 * un calendario de SOLO LECTURA; el servicio Go valida la URL (SSRF-safe), la
 * obtiene y cachea sus eventos.
 */
export function subscribeIcs(input: {
  url: string;
  displayName: string;
  color: string;
}): Promise<{ calendar: Calendar }> {
  return sendJson('POST', '/api/subscriptions', input);
}

/** Fuerza un refresco inmediato de una suscripción ICS («actualizar ahora»). */
export function refreshSubscription(id: string): Promise<{ calendar: Calendar }> {
  return sendJson('POST', '/api/subscriptions', { id, refresh: true });
}

/**
 * Compartir calendarios con ACL. Solo el DUEÑO gestiona los shares de sus
 * calendarios (el servicio Go verifica la propiedad); el destinatario se casa por
 * su correo/sub verificado del JWT al acceder. «Compartido conmigo» lista los
 * calendarios que otros han compartido con el usuario.
 */

/** Lista con quién está compartido un calendario propio (solo el dueño). */
export function listCalendarShares(calendarId: string): Promise<{ shares: CalendarShare[] }> {
  return getJson(`/api/shares?calendar=${encodeURIComponent(calendarId)}`);
}

/** Comparte un calendario propio con un destinatario por correo + permiso. */
export function shareCalendar(
  calendarId: string,
  email: string,
  permission: SharePermission,
): Promise<{ share: CalendarShare }> {
  return sendJson('POST', '/api/shares', { calendar: calendarId, email, permission });
}

/** Revoca un share (corta el acceso del destinatario). Mutación por POST (CSRF-safe). */
export function revokeCalendarShare(calendarId: string, shareId: string): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/shares', { calendar: calendarId, shareId, delete: true });
}

/** Lista los calendarios compartidos CON el usuario (dueño + permiso). */
export function listSharedWithMe(): Promise<{ calendars: Calendar[] }> {
  return getJson('/api/shared');
}

export function listEvents(
  calendarUrl: string,
  range?: { start: string; end?: string },
): Promise<{ events: CalendarEvent[] }> {
  const params = new URLSearchParams({ calendarUrl });
  if (range?.start) params.set('start', range.start);
  if (range?.end) params.set('end', range.end);
  return getJson(`/api/events?${params.toString()}`);
}

/**
 * Los eventos de varios calendarios a la vez. Un 403/404 de UNO —le han retirado
 * la compartición, o lo borraron en otro dispositivo— no tumba los demás: vuelve
 * en `sinAcceso` para que la vista lo quite y avise. Cualquier otro fallo lanza.
 */
export async function listEventsOf(
  cals: readonly Calendar[],
  range?: { start: string; end?: string },
  list: typeof listEvents = listEvents,
): Promise<{ byCal: Record<string, CalendarEvent[]>; sinAcceso: Calendar[] }> {
  const sinAcceso: Calendar[] = [];
  const pairs = await Promise.all(
    cals.map((c) =>
      list(c.url, range).then(
        ({ events }) => [c.url, events] as const,
        (e: unknown) => {
          if (!(e instanceof HttpError) || (e.status !== 403 && e.status !== 404)) throw e;
          sinAcceso.push(c);
          return [c.url, [] as CalendarEvent[]] as const;
        },
      ),
    ),
  );
  return { byCal: Object.fromEntries(pairs), sinAcceso };
}

export function saveEvent(input: {
  calendarUrl: string;
  eventInput: EventInput;
  href?: string;
  etag?: string;
}): Promise<{ ok: boolean; href: string; uid: string }> {
  return sendJson('POST', '/api/event', input);
}

export function deleteEvent(href: string): Promise<{ ok: boolean }> {
  return sendJson('DELETE', '/api/event', { href });
}

/** Hueco libre comun (instantes UTC RFC3339) devuelto por «buscar hueco». */
export interface FreeSlot {
  start: string;
  end: string;
}

/**
 * «Buscar hueco»: huecos libres comunes de los participantes dentro de la ventana
 * y la franja laboral. El organizador (usuario autenticado) se incluye SIEMPRE en
 * el servidor; `users` (subs) son participantes adicionales. GET (lectura, sin
 * CSRF). Duracion en minutos; workStart/workEnd en «HH:MM» interpretados en `tz`.
 */
export function suggestSlots(params: {
  start: string;
  end: string;
  durationMinutes: number;
  tz?: string;
  workStart?: string;
  workEnd?: string;
  weekdays?: boolean;
  users?: string[];
}): Promise<{ slots: FreeSlot[]; duration_minutes: number; tz: string }> {
  const q = new URLSearchParams({
    start: params.start,
    end: params.end,
    duration: String(params.durationMinutes),
  });
  if (params.tz) q.set('tz', params.tz);
  if (params.workStart) q.set('work_start', params.workStart);
  if (params.workEnd) q.set('work_end', params.workEnd);
  if (params.weekdays) q.set('weekdays', 'true');
  for (const u of params.users ?? []) q.append('user', u);
  return getJson(`/api/freebusy?${q.toString()}`);
}

/** Lista las invitaciones de la bandeja (por defecto, solo las pendientes). */
export function listInvitations(pendingOnly = true): Promise<{ invitations: Invitation[] }> {
  return getJson(`/api/invitations${pendingOnly ? '?pending=true' : ''}`);
}

/** Responde una invitacion (Aceptar/Rechazar/Quizas). */
export function respondInvitation(id: string, status: InvitationResponse): Promise<Invitation> {
  return sendJson('POST', '/api/invitations', { id, status });
}

/** Lista los recordatorios vencidos del usuario (entrega in-app), keyset. */
export function listDueReminders(
  cursor?: string,
): Promise<{ reminders: Reminder[]; next_cursor: string }> {
  return getJson(`/api/reminders${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ''}`);
}

/** Descarta un recordatorio in-app (mutacion por POST, CSRF-safe). */
export function dismissReminder(id: string): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/reminders', { id });
}

/**
 * Recorta el maestro de una serie recurrente para editar/borrar por alcance:
 *   - mode 'exclude' («solo este»): EXDATE de esa instancia.
 *   - mode 'until'   («este y siguientes»): UNTIL del RRULE antes de la instancia.
 * La instancia/serie nueva la crea el cliente aparte por el PUT CalDAV habitual.
 */
export function amendRecurrence(input: {
  calendarId: string;
  uid: string;
  recurrenceId: string; // RFC3339: inicio original de la instancia objetivo
  mode: 'exclude' | 'until';
  etag?: string;
}): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/amend-recurrence', input);
}

/**
 * Recursos reservables (salas, equipos) — patrón resource-calendar. Reservar/liberar
 * y ver la disponibilidad los puede hacer cualquier usuario del tenant; dar de alta un
 * recurso exige rol admin/owner y editarlo/borrarlo ser su gestor (lo hace cumplir el
 * servicio Go). La disponibilidad y el anti doble-reserva atómico viven en Go.
 */

export type ResourceKind = 'room' | 'equipment';

/** Un recurso reservable del servicio Go (domain.Resource). */
export interface Resource {
  id: string;
  calendar_id: string;
  kind: ResourceKind;
  name: string;
  capacity: number;
  location?: string;
  email?: string;
  auto_accept: boolean;
  managers: string[];
  active: boolean;
}

/** Un periodo OCUPADO (free/busy) devuelto con la disponibilidad de un recurso. */
export interface BusyPeriod {
  start: string;
  end: string;
  type: string;
}

/** Recurso + sus periodos ocupados en la ventana consultada (libre = busy vacío). */
export interface ResourceWithBusy extends Resource {
  busy: BusyPeriod[];
}

/** Resultado de reservar un recurso: aceptada, rechazada (ocupado) o pendiente. */
export interface BookingResult {
  status: 'accepted' | 'declined' | 'pending';
  resource: Resource;
  conflict?: BusyPeriod;
}

/**
 * Lista los recursos activos del tenant. Con un rango [start, end) (RFC3339) adjunta
 * la disponibilidad (periodos ocupados) de cada uno para pintar «libre/ocupado» en la
 * franja elegida. GET de solo lectura (sin CSRF).
 */
export function listResources(range?: {
  start: string;
  end: string;
}): Promise<{ resources: ResourceWithBusy[] }> {
  const q = new URLSearchParams();
  if (range) {
    q.set('start', range.start);
    q.set('end', range.end);
  }
  const qs = q.toString();
  return getJson(`/api/resources${qs ? `?${qs}` : ''}`);
}

/** Da de alta un recurso (solo admin/owner del tenant: el servicio Go lo verifica). */
export function createResource(input: {
  kind: ResourceKind;
  name: string;
  capacity?: number;
  location?: string;
  email?: string;
  auto_accept?: boolean;
  managers?: string[];
}): Promise<Resource> {
  return sendJson('POST', '/api/resources', { action: 'create', ...input });
}

/** Edita un recurso (solo su gestor o un admin). */
export function updateResource(
  id: string,
  patch: Partial<{
    name: string;
    capacity: number;
    location: string;
    email: string;
    auto_accept: boolean;
    active: boolean;
    managers: string[];
  }>,
): Promise<Resource> {
  return sendJson('POST', '/api/resources', { action: 'update', id, ...patch });
}

/** Da de baja (desactiva) un recurso (solo su gestor o un admin). */
export function deleteResource(id: string): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/resources', { action: 'delete', id });
}

/**
 * Reserva un recurso para un evento (por su UID). Devuelve accept/decline (o pending
 * si el recurso requiere aprobación manual). La comprobación de disponibilidad es
 * ATÓMICA en el servidor (dos reservas simultáneas de la misma franja no ganan ambas).
 */
export function bookResource(
  resourceId: string,
  booking: {
    uid: string;
    summary?: string;
    organizer?: string;
    start: string;
    end: string;
    tzid?: string;
    all_day?: boolean;
    rrule?: string;
  },
): Promise<BookingResult> {
  return sendJson('POST', '/api/resources', { action: 'book', id: resourceId, ...booking });
}

/** Libera la reserva del (evento, recurso). Permitido a quien reservó o a un gestor. */
export function releaseResource(resourceId: string, sourceUid: string): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/resources', {
    action: 'release',
    id: resourceId,
    source_uid: sourceUid,
  });
}

/**
 * Crea (o reutiliza) la videollamada de Sytue Rooms de un evento y devuelve su enlace.
 * Idempotente por UID: reabrir el evento y volver a pulsar da LA MISMA sala, así que el
 * enlace que ya tienen los invitados sigue sirviendo.
 */
export function createConference(uid: string, title: string): Promise<{ url: string }> {
  return sendJson('POST', '/api/conference', { uid, title });
}

/**
 * URL de descarga del calendario ENTERO como .ics («Descargar .ics» del menú de
 * un calendario). CON `withBase`, por lo mismo que `rawDownloadUrl` de Sytue
 * Mail: esto acaba en un `href`, no en un `fetch`, así que el shim de
 * `basePathScript` no lo alcanza y sin el prefijo la petición sale a la raíz
 * del host de la organización, donde no hay ingress para `/api`.
 */
export function calendarIcsUrl(calendarId: string): string {
  return withBase(
    import.meta.env.BASE_URL,
    `/api/calendar-export?id=${encodeURIComponent(calendarId)}`,
  );
}
