/**
 * Crear/editar/eliminar un evento CalDAV (servicio Go Sytue Calendar).
 *
 * POST   /api/event  -> crea o actualiza: { calendarUrl, eventInput, href?, etag? }
 *                       Si no hay href, se genera <calendarUrl>/<uid>.ics.
 * PUT    /api/event  -> restaura un evento recién borrado («Deshacer»): { href, ical }
 * DELETE /api/event  -> elimina: { href }
 *
 * El cuerpo iCalendar (VEVENT) lo genera el servidor con buildVevent (no se
 * confia en el cliente para el .ics). Al editar (con href) se lee antes el
 * recurso guardado y se funde (fundirVevent): el formulario solo sustituye lo
 * que gestiona, y lo demás —una RRULE con BYDAY, las excepciones de la serie,
 * CLASS, CATEGORIES, los CN de los invitados…— se queda. Proxy BFF autorizado
 * por cookie.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { HttpError } from '@sytue/api-clients';
import { calDavClient, fallo, json, rutaDav } from '../../lib/caldav.js';
import {
  buildVevent,
  esRRuleSegura,
  fundirVevent,
  newUid,
  type EventInput,
} from '../../lib/ical.js';
import { icsRestaurable } from '../../lib/deshacer.js';

export const prerender = false;

interface PostPayload {
  calendarUrl?: string;
  eventInput?: Partial<EventInput>;
  href?: string;
  etag?: string;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as PostPayload;
  const calendarUrl = rutaDav(body.calendarUrl);
  if (!calendarUrl || !body.eventInput?.summary || !body.eventInput.start) {
    return json({ error: 'invalid_payload' }, 400);
  }
  // Un `href` propio también tiene que ser del servicio (o no venir): si no, el
  // PUT llevaría el Bearer a donde diga quien envía el cuerpo.
  if (body.href !== undefined && rutaDav(body.href) === null) {
    return json({ error: 'invalid_payload' }, 400);
  }

  const uid = body.eventInput.uid || newUid();
  const attendees = (body.eventInput.attendees ?? []).map((e) => e.trim()).filter(Boolean);
  // Passthrough VALARM (recordatorios de clientes CalDAV externos que la UI no
  // edita). Solo se aceptan bloques con la forma BEGIN:VALARM..END:VALARM; el
  // usuario ya puede escribir iCal arbitrario en su calendario via PUT CalDAV.
  const alarms = (body.eventInput.alarms ?? []).filter(
    (a) => typeof a === 'string' && /^BEGIN:VALARM\r?\n/.test(a) && /END:VALARM\s*$/.test(a),
  );
  // EXDATE arrastrados al desgajar una serie: solo tokens iCal de fecha/fecha-hora.
  const exdates = (body.eventInput.exdates ?? []).filter(
    (e) => typeof e === 'string' && /^\d{8}(T\d{6}Z?)?$/.test(e),
  );
  // Respuestas ya dadas de los invitados: solo las tres que alguien puede dar, y solo
  // de los invitados de ESTE guardado. No abre nada nuevo —el dueño del calendario ya
  // puede escribir el .ics que quiera por CalDAV—; solo evita borrarlas al guardar.
  const partstats: Record<string, string> = {};
  const lista = new Set(attendees.map((e) => e.toLowerCase()));
  for (const [email, ps] of Object.entries(body.eventInput.partstats ?? {})) {
    const valor = typeof ps === 'string' ? ps.toUpperCase() : '';
    if (lista.has(email.toLowerCase()) && /^(ACCEPTED|TENTATIVE|DECLINED)$/.test(valor)) {
      partstats[email.toLowerCase()] = valor;
    }
  }
  // TZID: solo un nombre IANA seguro (letras/dígitos/_ + - /). Se inyecta en
  // DTSTART;TZID=<tzid>:, así que se descarta cualquier valor que pudiera romper
  // el iCal (CRLF, «:», «;»). Ausente/invalido => el evento se emite en UTC.
  const tzid =
    typeof body.eventInput.tzid === 'string' && /^[A-Za-z0-9_+/-]{1,64}$/.test(body.eventInput.tzid)
      ? body.eventInput.tzid
      : undefined;
  // Color: solo un hex #RRGGBB de la paleta; cualquier otra cosa se descarta
  // (no se inyecta en el iCal). buildVevent lo emite como X-APPLE-CALENDAR-COLOR.
  const color =
    typeof body.eventInput.color === 'string' && /^#[0-9A-Fa-f]{6}$/.test(body.eventInput.color)
      ? body.eventInput.color
      : undefined;
  // RRULE que el formulario no sabe editar: solo una regla con forma de RRULE (se
  // inyecta como línea del .ics, igual que el TZID).
  const rruleRaw =
    typeof body.eventInput.rruleRaw === 'string' && esRRuleSegura(body.eventInput.rruleRaw)
      ? body.eventInput.rruleRaw
      : undefined;
  const event: EventInput = {
    uid,
    summary: body.eventInput.summary,
    start: body.eventInput.start,
    end: body.eventInput.end,
    allDay: body.eventInput.allDay,
    ...(tzid ? { tzid } : {}),
    location: body.eventInput.location,
    description: body.eventInput.description,
    recurrence: body.eventInput.recurrence,
    ...(rruleRaw ? { rruleRaw } : {}),
    ...(body.eventInput.recurrenceUnchanged === true ? { recurrenceUnchanged: true } : {}),
    ...(color ? { color } : {}),
    // «Mostrar como»: solo los dos valores del RFC; otra cosa no viaja y el TRANSP
    // guardado se queda como estaba.
    ...(body.eventInput.transp === 'OPAQUE' || body.eventInput.transp === 'TRANSPARENT'
      ? { transp: body.eventInput.transp }
      : {}),
    // ORGANIZER = usuario autenticado (el navegador no dicta el organizador).
    // Solo emitimos ORGANIZER/ATTENDEE si hay invitados, para no marcar como
    // "reunion" los eventos personales.
    ...(attendees.length ? { attendees, organizer: session.user.email, partstats } : {}),
    ...(alarms.length ? { alarms } : {}),
    ...(exdates.length ? { exdates } : {}),
  };
  const href = body.href ?? `${calendarUrl.replace(/\/$/, '')}/${encodeURIComponent(uid)}.ics`;

  try {
    const cliente = calDavClient(session.accessToken, session.user.id);
    // Editar: fundir sobre lo guardado. Una petición más por guardado, a cambio de no
    // borrar lo que el formulario no ve. Si el recurso ya no está (404), se escribe
    // como hasta ahora y el If-Match del navegador decide.
    const guardado = body.href ? await leer(cliente, body.href) : null;
    const ical = guardado ? fundirVevent(guardado.ical, event) : buildVevent(event);
    // El ETag que vio el navegador manda: si otro cambió el evento después, el PUT
    // da 412 en vez de pisarlo. Sin él, el de lo que se acaba de leer.
    const etag = body.etag ?? guardado?.etag;
    await cliente.putEvent(href, ical, etag);
    return json({ ok: true, href, uid });
  } catch (err) {
    return fallo(err, 'caldav_error');
  }
};

/** El recurso guardado, o null si ya no existe. */
async function leer(
  cliente: ReturnType<typeof calDavClient>,
  href: string,
): Promise<{ ical: string; etag?: string | undefined } | null> {
  try {
    return await cliente.getEvent(href);
  } catch (err) {
    if (err instanceof HttpError && err.status === 404) return null;
    throw err;
  }
}

/**
 * «Deshacer» un borrado: vuelve a escribir el .ics que tenía el evento, con su UID,
 * su href y sus VALARM, en vez de regenerarlo desde un formulario que no los ve.
 * Es el único .ics que esta puerta no genera ella, así que `icsRestaurable` lo
 * acota (sin ORGANIZER/ATTENDEE: nada que mande correos a terceros) y el PUT va con
 * `If-None-Match: *`: solo RE-crea; si en ese href ya hay algo —otro cliente lo
 * volvió a crear—, el servicio contesta 412 y no se pisa.
 */
export const PUT: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as { href?: unknown; ical?: unknown };
  const href = rutaDav(body.href);
  const ical = icsRestaurable(body.ical);
  if (!href || !ical) return json({ error: 'invalid_payload' }, 400);

  try {
    await calDavClient(session.accessToken, session.user.id).putEvent(href, ical);
    return json({ ok: true, href });
  } catch (err) {
    return fallo(err, 'caldav_error');
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as { href?: string };
  const href = rutaDav(body.href);
  if (!href) return json({ error: 'missing_href' }, 400);

  try {
    await calDavClient(session.accessToken, session.user.id).deleteEvent(href);
    return json({ ok: true });
  } catch (err) {
    return fallo(err, 'caldav_error');
  }
};
