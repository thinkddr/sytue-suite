/**
 * Crear/editar/eliminar un evento CalDAV (servicio Go Sytue Calendar).
 *
 * POST   /api/event  -> crea o actualiza: { calendarUrl, eventInput, href?, etag? }
 *                       Si no hay href, se genera <calendarUrl>/<uid>.ics.
 * DELETE /api/event  -> elimina: { href }
 *
 * El cuerpo iCalendar (VEVENT) lo genera el servidor con buildVevent (no se
 * confia en el cliente para el .ics). Proxy BFF autorizado por cookie.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { calDavClient, fallo, json, rutaDav } from '../../lib/caldav.js';
import { buildVevent, newUid, type EventInput } from '../../lib/ical.js';

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
    ...(color ? { color } : {}),
    // ORGANIZER = usuario autenticado (el navegador no dicta el organizador).
    // Solo emitimos ORGANIZER/ATTENDEE si hay invitados, para no marcar como
    // "reunion" los eventos personales.
    ...(attendees.length ? { attendees, organizer: session.user.email } : {}),
    ...(alarms.length ? { alarms } : {}),
    ...(exdates.length ? { exdates } : {}),
  };
  const ical = buildVevent(event);
  const href = body.href ?? `${calendarUrl.replace(/\/$/, '')}/${encodeURIComponent(uid)}.ics`;

  try {
    await calDavClient(session.accessToken, session.user.id).putEvent(href, ical, body.etag);
    return json({ ok: true, href, uid });
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
